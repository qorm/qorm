// Package audio provides minimal WAV playback for the canvas engine:
// background-music loops and one-shot sound effects. The decoder is the
// stdlib wave package; output is through a no-op sink by default so headless
// tests stay silent, and the platform layer can install a real player via
// RegisterSink.
//
// Source files are resolved relative to an app's BaseDir (the same jail
// images use) — an agent-authored scene cannot point audio at /etc/passwd.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sound is a decoded WAV: PCM samples + format headers, ready to hand to a
// sink. Zero-value fields mean "uninitialized" — callers must check Loaded.
type Sound struct {
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
	Samples       []byte // interleaved little-endian PCM, length = total samples * bytes-per-sample
}

// LoadSound reads a WAV file from disk and returns a decoded Sound. The
// caller owns the returned Samples slice; sinks copy what they need.
//
//	loaded, err := audio.LoadSound(rt.App.BaseDir, "assets/coin.wav")
//	if err != nil { ... }
//	engine.Play(loaded, false) // one-shot
func LoadSound(baseDir, src string) (*Sound, error) {
	path, err := resolveSrc(baseDir, src)
	if err != nil {
		return nil, err
	}
	soundCacheMu.Lock()
	if cached, ok := soundCache[path]; ok {
		soundCacheMu.Unlock()
		return cached, nil
	}
	soundCacheMu.Unlock()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("audio: open %q: %w", src, err)
	}
	defer f.Close()

	dec := &wavDecoder{r: f}
	if err := dec.readHeader(); err != nil {
		return nil, fmt.Errorf("audio: parse %q: %w", src, err)
	}
	if dec.bitsPerSample != 8 && dec.bitsPerSample != 16 {
		return nil, fmt.Errorf("audio: %q: unsupported bits per sample %d (8 or 16 PCM only)", src, dec.bitsPerSample)
	}
	if dec.audioFormat != 1 {
		return nil, fmt.Errorf("audio: %q: unsupported format %d (PCM=1 only)", src, dec.audioFormat)
	}
	samples, err := io.ReadAll(dec.dataReader)
	if err != nil {
		return nil, fmt.Errorf("audio: read samples from %q: %w", src, err)
	}
	snd := &Sound{
		SampleRate:    dec.sampleRate,
		Channels:      dec.numChannels,
		BitsPerSample: dec.bitsPerSample,
		Samples:       samples,
	}
	soundCacheMu.Lock()
	soundCache[path] = snd
	soundCacheMu.Unlock()
	return snd, nil
}

// Player writes samples to a platform-specific output. The default sink is a
// silent no-op (tests, CI); the native app installs a real sink at init.
type Player interface {
	Play(s *Sound, loop bool) error
	Stop() error
}

var (
	sinkMu sync.RWMutex
	sink   Player = nopSink{}

	soundCacheMu sync.Mutex
	soundCache   = map[string]*Sound{}
)

// RegisterSink installs the platform audio sink. Pass nil to restore the
// silent default (useful in tests that need to swap sinks mid-run).
func RegisterSink(p Player) {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	if p == nil {
		sink = nopSink{}
		return
	}
	sink = p
}

func activeSink() Player {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return sink
}

// ActiveSink returns the currently-registered platform sink (the silent
// default when none is installed). Runtime helpers that need to push audio
// through the active sink use this directly instead of going through
// expr.CallBuiltin.
func ActiveSink() Player { return activeSink() }

// SrcPlayer is an optional Player extension for URL-based playback (WASM /
// HTML hosts). When the app runs with App.Web, the runtime prefers PlaySrc
// over LoadSound+Play so the browser fetches the WAV from BaseDir+src.
type SrcPlayer interface {
	Player
	PlaySrc(url string, loop bool) error
}

// nopSink discards samples. Default; headless tests stay quiet.
type nopSink struct{}

func (nopSink) Play(*Sound, bool) error { return nil }
func (nopSink) Stop() error             { return nil }

// ---- WAV decoder (stdlib wave is too heavy for the use case) ----

type wavDecoder struct {
	r             io.Reader
	audioFormat   uint16
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
	dataSize      uint32
	dataReader    io.Reader
}

func (d *wavDecoder) readHeader() error {
	var hdr [12]byte
	if _, err := io.ReadFull(d.r, hdr[:]); err != nil {
		return fmt.Errorf("read RIFF header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return fmt.Errorf("not a RIFF/WAVE file")
	}
	// Walk chunks until we hit "fmt " and "data".
	for {
		var ch [8]byte
		if _, err := io.ReadFull(d.r, ch[:]); err != nil {
			return fmt.Errorf("read chunk header: %w", err)
		}
		id := string(ch[0:4])
		size := uint32(ch[4]) | uint32(ch[5])<<8 | uint32(ch[6])<<16 | uint32(ch[7])<<24
		switch id {
		case "fmt ":
			if size < 16 {
				return fmt.Errorf("fmt chunk too small: %d", size)
			}
			var fmtHdr [16]byte
			if _, err := io.ReadFull(d.r, fmtHdr[:]); err != nil {
				return fmt.Errorf("read fmt: %w", err)
			}
			d.audioFormat = uint16(fmtHdr[0]) | uint16(fmtHdr[1])<<8
			d.numChannels = uint16(fmtHdr[2]) | uint16(fmtHdr[3])<<8
			d.sampleRate = uint32(fmtHdr[4]) | uint32(fmtHdr[5])<<8 | uint32(fmtHdr[6])<<16 | uint32(fmtHdr[7])<<24
			d.bitsPerSample = uint16(fmtHdr[14]) | uint16(fmtHdr[15])<<8
			// Skip any extra fmt bytes.
			if extra := size - 16; extra > 0 {
				if _, err := io.CopyN(io.Discard, d.r, int64(extra)); err != nil {
					return fmt.Errorf("skip fmt extra: %w", err)
				}
			}
		case "data":
			d.dataSize = size
			d.dataReader = io.LimitReader(d.r, int64(size))
			return nil
		default:
			// Skip unknown chunks (LIST, JUNK, bext, …).
			if _, err := io.CopyN(io.Discard, d.r, int64(size)); err != nil {
				return fmt.Errorf("skip %s: %w", id, err)
			}
		}
	}
}

// resolveSrc maps an author-provided src to an absolute path under the app
// BaseDir, refusing path-traversal and absolute paths (the same jail as
// the image loader).
func resolveSrc(baseDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("empty src")
	}
	if strings.Contains(src, "://") || strings.HasPrefix(src, "data:") {
		return "", fmt.Errorf("remote src not supported")
	}
	if filepath.IsAbs(src) {
		return "", fmt.Errorf("absolute src not allowed")
	}
	if baseDir == "" {
		return "", fmt.Errorf("no app BaseDir")
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.Join(baseAbs, src))
	if clean != baseAbs && !strings.HasPrefix(clean, baseAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("src escapes app dir")
	}
	return clean, nil
}

// ---- StdoutSink: shell out to afplay / aplay / paplay ----

// StdoutSink plays WAV files via the host's audio tool. macOS's `afplay`
// takes a file path, not stdin — so we write a temp .wav and feed that path
// to the tool. Loop=true is the music slot; one-shots use a separate SFX
// slot so jump/coin cannot kill the BGM (the previous single-process sink
// made every playSound restart music and hitch the frame).
type StdoutSink struct {
	Cmd string // override; defaults to per-OS best guess
	mu  sync.Mutex

	music     *exec.Cmd
	musicTmp  string
	musicDone chan struct{}

	sfx    []*exec.Cmd
	sfxTmp []string
}

const maxSFX = 4

// Play implements Player.
func (s *StdoutSink) Play(snd *Sound, loop bool) error {
	path, err := cachedTempWAV(snd)
	if err != nil {
		return err
	}
	if loop {
		return s.playMusic(snd, path)
	}
	return s.playSFX(path)
}

func (s *StdoutSink) playMusic(snd *Sound, path string) error {
	s.mu.Lock()
	s.killLocked(s.music)
	s.music = nil
	s.musicTmp = ""
	cmd := s.cmd()
	cmd.Args = append(cmd.Args, path)
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.music = cmd
	s.musicTmp = path
	done := make(chan struct{})
	s.musicDone = done
	s.mu.Unlock()

	start := time.Now()
	go func() { _ = cmd.Wait(); close(done) }()
	go func() {
		<-done
		s.mu.Lock()
		still := s.music == cmd
		// A music process that dies almost immediately means the sink is
		// broken (no audio device — headless CI, muted containers). The loop
		// is implemented as respawn-on-exit, so re-playing here would churn
		// out failing processes forever. Real tracks play for seconds, so a
		// sub-500ms lifetime is the signal to stop, not loop.
		if still && time.Since(start) < 500*time.Millisecond {
			still = false
		}
		s.mu.Unlock()
		if still {
			_ = s.Play(snd, true)
		}
	}()
	return nil
}

func (s *StdoutSink) playSFX(path string) error {
	s.mu.Lock()
	s.reapSFXLocked()
	if len(s.sfx) >= maxSFX {
		s.killLocked(s.sfx[0])
		s.sfx = s.sfx[1:]
		s.sfxTmp = s.sfxTmp[1:]
	}
	cmd := s.cmd()
	cmd.Args = append(cmd.Args, path)
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.sfx = append(s.sfx, cmd)
	s.sfxTmp = append(s.sfxTmp, path)
	s.mu.Unlock()
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *StdoutSink) killLocked(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func (s *StdoutSink) reapSFXLocked() {
	live := s.sfx[:0]
	liveT := s.sfxTmp[:0]
	for i, c := range s.sfx {
		if c == nil || c.ProcessState != nil {
			continue
		}
		live = append(live, c)
		if i < len(s.sfxTmp) {
			liveT = append(liveT, s.sfxTmp[i])
		}
	}
	s.sfx = live
	s.sfxTmp = liveT
}

// Stop implements Player — stops music and every in-flight one-shot.
func (s *StdoutSink) Stop() error {
	s.mu.Lock()
	s.killLocked(s.music)
	s.music = nil
	s.musicTmp = ""
	for _, c := range s.sfx {
		s.killLocked(c)
	}
	s.sfx = nil
	s.sfxTmp = nil
	s.mu.Unlock()
	return nil
}

func (s *StdoutSink) cmd() *exec.Cmd {
	if s.Cmd != "" {
		return exec.Command(s.Cmd)
	}
	// Per-OS default. afplay (macOS), aplay (ALSA Linux), paplay (PulseAudio).
	for _, tool := range []string{"afplay", "aplay", "paplay"} {
		if _, err := exec.LookPath(tool); err == nil {
			return exec.Command(tool)
		}
	}
	return exec.Command("true") // sink that does nothing
}

var (
	wavTmpMu sync.Mutex
	wavTmp   = map[string]string{} // content key → temp path (process lifetime)
)

func cachedTempWAV(snd *Sound) (string, error) {
	if snd == nil {
		return "", fmt.Errorf("audio: nil sound")
	}
	key := fmt.Sprintf("%d-%d-%d-%d-%x", snd.SampleRate, snd.Channels, snd.BitsPerSample, len(snd.Samples), samplesPrefix(snd.Samples))
	wavTmpMu.Lock()
	if p, ok := wavTmp[key]; ok {
		wavTmpMu.Unlock()
		return p, nil
	}
	wavTmpMu.Unlock()
	path, err := writeTempWAV(snd)
	if err != nil {
		return "", err
	}
	wavTmpMu.Lock()
	wavTmp[key] = path
	wavTmpMu.Unlock()
	return path, nil
}

func samplesPrefix(b []byte) uint32 {
	var h uint32 = 2166136261
	n := len(b)
	if n > 64 {
		n = 64
	}
	for i := 0; i < n; i++ {
		h ^= uint32(b[i])
		h *= 16777619
	}
	h ^= uint32(len(b))
	return h
}

// writeTempWAV writes a full RIFF/WAVE file (header + PCM samples) to a
// temp file and returns its path. The caller owns removal.
func writeTempWAV(snd *Sound) (string, error) {
	f, err := os.CreateTemp("", "qorm-audio-*.wav")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := encodeWAV(f, snd); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// encodeWAV writes a RIFF/WAVE header followed by the raw PCM samples.
// Field types must match what the decoder reads (mix of uint16 + uint32).
func encodeWAV(w io.Writer, snd *Sound) error {
	dataSize := uint32(len(snd.Samples))
	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(36)+dataSize); err != nil {
		return err
	}
	if _, err := w.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	// fmt chunk (16 bytes): uint32 size, uint16 format, uint16 channels,
	// uint32 sample rate, uint32 byte rate, uint16 block align, uint16 bps.
	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(1)); err != nil {
		return err // PCM
	}
	if err := binary.Write(w, binary.LittleEndian, snd.Channels); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, snd.SampleRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian,
		snd.SampleRate*uint32(snd.Channels)*uint32(snd.BitsPerSample/8)); err != nil {
		return err // byte rate
	}
	if err := binary.Write(w, binary.LittleEndian,
		snd.Channels*(snd.BitsPerSample/8)); err != nil {
		return err // block align
	}
	if err := binary.Write(w, binary.LittleEndian, snd.BitsPerSample); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	_, err := w.Write(snd.Samples)
	return err
}

// ---- Built-in player handle ----

// playOnce / playLoop / stopAll live in the runtime (it owns the BaseDir
// and the active sink). The audio package only exposes the Player /
// ActiveSink / RegisterSink surface; the runtime's audioAdapter wires it.

// playOnce / playLoop / stopAll live in the runtime (it owns the BaseDir
// and the active sink). The audio package only exposes the Player /
// ActiveSink / RegisterSink surface; the runtime's audioAdapter wires it.
