// genmarioaudio bakes chiptune-style WAV files for examples/mario:
// background music loop and one-shot SFX. Output is CC0 (no copyright).
//
// Usage: go run ./tools/genmarioaudio [outdir]
//   outdir defaults to ./examples/mario/audio.
package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

const sampleRate = 22050 // small enough to keep files cheap, plenty for chiptune

// note freq in Hz — A4 = 440, equal-temperament.
func freq(midi int) float64 { return 440 * math.Pow(2, float64(midi-69)/12) }

// writeWAV encodes mono 16-bit PCM into a RIFF/WAVE file at sampleRate.
func writeWAV(path string, samples []float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataSize := uint32(len(samples) * 2)
	// RIFF header
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36)+dataSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	// fmt chunk (16 bytes) — write each field so binary.Write doesn't reject
	// the heterogeneous []any.
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	} // PCM
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	} // mono
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	} // byte rate
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil {
		return err
	} // block align
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil {
		return err
	} // bits per sample
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	for _, s := range samples {
		v := int16(math.MaxInt16 * math.Max(-1, math.Min(1, s)))
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return nil
}

// squareWave returns a +-1 square wave of freq Hz, dur seconds.
// Sounds retro (NES-style) — dominant odd harmonics, harsh and bright.
func squareWave(dur, f float64) []float64 {
	n := int(dur * sampleRate)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		// Soft square via sign of sine (already +-1).
		if math.Sin(2*math.Pi*f*t) >= 0 {
			out[i] = 1
		} else {
			out[i] = -1
		}
	}
	return out
}

// triangleWave returns a +-1 triangle wave (softer than square).
func triangleWave(dur, f float64) []float64 {
	n := int(dur * sampleRate)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		// Triangular: 4*|t*freq - floor(t*freq + 0.5)| pattern.
		phase := t * f
		out[i] = 4*math.Abs(phase-math.Floor(phase+0.5)) - 1
	}
	return out
}

// adsr applies an attack-sustain-release envelope so notes don't click.
// a,r in seconds; sustain held at 1.0 between attack and release.
func adsr(samples []float64, a, r float64) []float64 {
	n := len(samples)
	aN := int(a * sampleRate)
	rN := int(r * sampleRate)
	for i := 0; i < n; i++ {
		var env float64
		switch {
		case i < aN:
			env = float64(i) / float64(aN)
		case i > n-rN:
			env = float64(n-i) / float64(rN)
		default:
			env = 1
		}
		samples[i] *= env
	}
	return samples
}

// mix sums waves element-wise, clamping at +-1.
func mix(parts ...[]float64) []float64 {
	max := 0
	for _, p := range parts {
		if len(p) > max {
			max = len(p)
		}
	}
	out := make([]float64, max)
	for _, p := range parts {
		for i, v := range p {
			out[i] += v
		}
	}
	// Soft-clip via tanh to avoid harsh clipping artifacts.
	for i := range out {
		out[i] = math.Tanh(out[i] * 0.7)
	}
	return out
}

// concat joins sequential clips into one buffer.
func concat(parts ...[]float64) []float64 {
	var total int
	for _, p := range parts {
		total += len(p)
	}
	out := make([]float64, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// silence returns dur seconds of silence (used as a gap between notes).
func silence(dur float64) []float64 { return make([]float64, int(dur*sampleRate)) }

// coinSFX: a quick upward arpeggio — C5 E5 G5 C6 — classic coin pickup.
func coinSFX() []float64 {
	notes := []struct {
		midi  int
		dur   float64
		isTri bool
	}{
		{72, 0.06, true}, {76, 0.06, true}, {79, 0.06, true}, {84, 0.10, true},
	}
	var parts [][]float64
	for _, n := range notes {
		var w []float64
		if n.isTri {
			w = triangleWave(n.dur, freq(n.midi))
		} else {
			w = squareWave(n.dur, freq(n.midi))
		}
		parts = append(parts, adsr(w, 0.005, 0.04))
	}
	return mix(concat(parts...))
}

// nSamples converts a duration in seconds to a sample count.
func nSamples(dur float64) int { return int(dur * float64(sampleRate)) }

// jumpSFX: a quick upward sweep — 220 Hz -> 660 Hz in 0.15s.
func jumpSFX() []float64 {
	n := nSamples(0.15)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		// Exponential glide.
		f := 220 * math.Pow(660/220, t/0.15)
		out[i] = math.Sin(2*math.Pi*f*t) * 0.5
	}
	return adsr(out, 0.005, 0.03)
}

// stompSFX: a quick downward thud — square wave 200Hz -> 80Hz in 0.12s.
func stompSFX() []float64 {
	n := nSamples(0.12)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		f := 200 * math.Pow(80/200, t/0.12)
		if math.Sin(2*math.Pi*f*t) >= 0 {
			out[i] = 0.7
		} else {
			out[i] = -0.7
		}
	}
	return adsr(out, 0.003, 0.05)
}

// deathSFX: descending minor arpeggio — the classic "you died" sound.
func deathSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{72, 0.18}, {71, 0.18}, {67, 0.18}, {65, 0.40},
	}
	var parts [][]float64
	for _, n := range notes {
		w := squareWave(n.dur, freq(n.midi))
		parts = append(parts, adsr(w, 0.005, 0.10))
	}
	return mix(concat(parts...))
}

// winSFX: ascending major triad fanfare.
func winSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{60, 0.10}, {64, 0.10}, {67, 0.10}, {72, 0.30},
		{76, 0.20}, {79, 0.20}, {84, 0.40},
	}
	var parts [][]float64
	for _, n := range notes {
		w := triangleWave(n.dur, freq(n.midi))
		parts = append(parts, adsr(w, 0.005, 0.06))
	}
	return mix(concat(parts...))
}

// musicLoop: a simple 4-bar overworld-style melody. CC0 — original composition.
// Bass: root-fifth pattern under a 16-note lead. Loops seamlessly.
func musicLoop() []float64 {
	bpm := 132.0
	beat := 60.0 / bpm // seconds per quarter note
	// Lead melody (MIDI note numbers, 0 = rest). Two bars of C major.
	lead := []int{72, 0, 76, 79, 76, 74, 72, 74, 76, 79, 84, 79, 76, 72, 0, 72}
	// Bass: alternating root and fifth, two beats per note (same length as lead).
	bass := []int{36, 36, 43, 43, 41, 41, 43, 43, 36, 36, 43, 43, 41, 41, 43, 43}
	durs := []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	var leadBuf []float64
	for i, n := range lead {
		if n == 0 {
			leadBuf = append(leadBuf, silence(durs[i]*beat)...)
			continue
		}
		w := squareWave(durs[i]*beat, freq(n))
		leadBuf = append(leadBuf, adsr(w, 0.01, 0.06)...)
	}
	var bassBuf []float64
	for i, n := range bass {
		if n == 0 {
			bassBuf = append(bassBuf, silence(durs[i]*beat)...)
			continue
		}
		w := triangleWave(durs[i]*beat, freq(n))
		bassBuf = append(bassBuf, adsr(w, 0.02, 0.08)...)
	}
	return mix(leadBuf, bassBuf)
}

func main() {
	outdir := "./examples/mario/audio"
	if len(os.Args) > 1 {
		outdir = os.Args[1]
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		panic(err)
	}
	files := []struct {
		name string
		gen  func() []float64
	}{
		{"coin.wav", coinSFX},
		{"jump.wav", jumpSFX},
		{"stomp.wav", stompSFX},
		{"death.wav", deathSFX},
		{"win.wav", winSFX},
		{"music.wav", musicLoop},
	}
	for _, f := range files {
		path := filepath.Join(outdir, f.name)
		if err := writeWAV(path, f.gen()); err != nil {
			panic(err)
		}
	}
}