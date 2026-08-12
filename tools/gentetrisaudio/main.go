// gentetrisaudio bakes chiptune-style WAV files for examples/tetris:
// a looping original BGM plus one-shot SFX. Output is CC0 (no copyright).
// The melody is an original A-minor stacker theme — not Korobeiniki and
// not any console Tetris arrangement.
//
// Usage: go run ./tools/gentetrisaudio [outdir]
//
//	outdir defaults to ./examples/tetris/audio.
package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

const sampleRate = 22050

func freq(midi int) float64    { return 440 * math.Pow(2, float64(midi-69)/12) }
func nSamples(dur float64) int { return int(dur * float64(sampleRate)) }

func writeWAV(path string, samples []float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataSize := uint32(len(samples) * 2)
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36)+dataSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
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

// pulseWave is a NES-style pulse (duty is 0–1; 0.125 / 0.25 are the bright ones).
func pulseWave(dur, f, duty float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		phase := math.Mod(float64(i)*f/float64(sampleRate), 1)
		if phase < duty {
			out[i] = 1
		} else {
			out[i] = -1
		}
	}
	return out
}

func triangleWave(dur, f float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		phase := float64(i) * f / float64(sampleRate)
		out[i] = 4*math.Abs(phase-math.Floor(phase+0.5)) - 1
	}
	return out
}

// sweepPulse glides f0→f1 with a phase accumulator so the chirp stays clean.
func sweepPulse(dur, f0, f1, duty, amp float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	phase := 0.0
	for i := 0; i < n; i++ {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		f := f0 * math.Pow(f1/f0, t)
		phase += f / float64(sampleRate)
		phase -= math.Floor(phase)
		if phase < duty {
			out[i] = amp
		} else {
			out[i] = -amp
		}
	}
	return out
}

func sweepTri(dur, f0, f1, amp float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	phase := 0.0
	for i := 0; i < n; i++ {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		f := f0 * math.Pow(f1/f0, t)
		phase += f / float64(sampleRate)
		phase -= math.Floor(phase)
		out[i] = amp * (4*math.Abs(phase-0.5) - 1)
	}
	return out
}

// noiseWave is a 15-bit LFSR clocked every clockDiv samples (NES-noise crunch).
func noiseWave(dur float64, clockDiv int) []float64 {
	if clockDiv < 1 {
		clockDiv = 1
	}
	n := nSamples(dur)
	out := make([]float64, n)
	var lfsr uint16 = 1
	bit := 1.0
	for i := 0; i < n; i++ {
		if i%clockDiv == 0 {
			b := (lfsr ^ (lfsr >> 1)) & 1
			lfsr = (lfsr >> 1) | (b << 14)
			if lfsr&1 == 1 {
				bit = 1
			} else {
				bit = -1
			}
		}
		out[i] = bit
	}
	return out
}

func adsr(samples []float64, a, r float64) []float64 {
	n := len(samples)
	aN := int(a * float64(sampleRate))
	rN := int(r * float64(sampleRate))
	if aN < 0 {
		aN = 0
	}
	if rN < 0 {
		rN = 0
	}
	if rN > n {
		rN = n
	}
	for i := 0; i < n; i++ {
		var env float64
		switch {
		case aN > 0 && i < aN:
			env = float64(i) / float64(aN)
		case rN > 0 && i > n-rN:
			env = float64(n-i) / float64(rN)
		default:
			env = 1
		}
		samples[i] *= env
	}
	return samples
}

func gain(samples []float64, g float64) []float64 {
	out := make([]float64, len(samples))
	for i, v := range samples {
		out[i] = v * g
	}
	return out
}

func silence(dur float64) []float64 { return make([]float64, nSamples(dur)) }

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
	for i := range out {
		out[i] = math.Tanh(out[i] * 0.75)
	}
	return out
}

func overlay(dst, src []float64, offset int, g float64) {
	for i, v := range src {
		j := i + offset
		if j >= 0 && j < len(dst) {
			dst[j] += v * g
		}
	}
}

func pulseNote(midi int, dur, duty, g, a, r float64) []float64 {
	return gain(adsr(pulseWave(dur, freq(midi), duty), a, r), g)
}

func triNote(midi int, dur, g, a, r float64) []float64 {
	return gain(adsr(triangleWave(dur, freq(midi)), a, r), g)
}

// --- SFX -----------------------------------------------------------------

// moveSFX: a short mid click. Soft-drop / gravity never fire this.
func moveSFX() []float64 {
	return adsr(sweepPulse(0.038, 980, 860, 0.25, 0.28), 0.002, 0.016)
}

// rotateSFX: a two-step twist (low then high).
func rotateSFX() []float64 {
	a := adsr(sweepPulse(0.038, 740, 820, 0.125, 0.32), 0.002, 0.014)
	b := adsr(sweepPulse(0.048, 980, 1240, 0.125, 0.34), 0.002, 0.020)
	return mix(concat(a, b))
}

// lockSFX: a low thud — piece settled.
func lockSFX() []float64 {
	body := adsr(sweepPulse(0.095, 210, 78, 0.25, 0.48), 0.002, 0.045)
	thump := adsr(sweepTri(0.11, 140, 55, 0.40), 0.001, 0.055)
	grit := gain(adsr(noiseWave(0.07, 8), 0.001, 0.045), 0.22)
	return mix(body, thump, grit)
}

// dropSFX: a falling whoosh for hard drop (lock SFX still plays after).
func dropSFX() []float64 {
	fall := adsr(sweepPulse(0.16, 920, 130, 0.125, 0.42), 0.003, 0.05)
	air := gain(adsr(noiseWave(0.14, 2), 0.002, 0.09), 0.18)
	return mix(fall, air)
}

// clearSFX: rising four-note arpeggio — a single / double / triple.
func clearSFX() []float64 {
	notes := []int{76, 79, 83, 88} // E5 G5 B5 E6
	var parts [][]float64
	for _, n := range notes {
		lead := pulseNote(n, 0.072, 0.25, 0.42, 0.004, 0.035)
		fifth := pulseNote(n-5, 0.072, 0.25, 0.16, 0.004, 0.035)
		parts = append(parts, mix(lead, fifth))
	}
	return concat(parts...)
}

// tetrisSFX: longer fanfare for a 4-line clear. Original — not the GB jingle.
func tetrisSFX() []float64 {
	type step struct {
		midi int
		dur  float64
	}
	lead := []step{
		{76, 0.08}, {79, 0.08}, {83, 0.08}, {88, 0.14},
		{86, 0.08}, {83, 0.08}, {79, 0.08}, {88, 0.32},
	}
	var parts [][]float64
	for _, s := range lead {
		hi := pulseNote(s.midi, s.dur, 0.125, 0.44, 0.004, 0.04)
		lo := pulseNote(s.midi-4, s.dur, 0.25, 0.20, 0.004, 0.04)
		bass := triNote(s.midi-24, s.dur, 0.22, 0.006, 0.05)
		parts = append(parts, mix(hi, lo, bass))
	}
	spark := gain(adsr(noiseWave(0.06, 3), 0.001, 0.04), 0.12)
	out := concat(parts...)
	overlay(out, spark, 0, 1)
	overlay(out, spark, nSamples(0.38), 0.7)
	return mix(out)
}

// levelupSFX: bright rising hexachord when lines cross a 10-line boundary.
func levelupSFX() []float64 {
	notes := []int{72, 74, 76, 79, 84, 88} // C5 D5 E5 G5 C6 E6
	var parts [][]float64
	for i, n := range notes {
		dur := 0.055
		if i == len(notes)-1 {
			dur = 0.16
		}
		parts = append(parts, pulseNote(n, dur, 0.125, 0.38, 0.003, 0.03))
	}
	return mix(concat(parts...))
}

// gameoverSFX: descending phrase + low rumble. Original, not the NES death.
func gameoverSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{69, 0.16}, {67, 0.16}, {64, 0.16}, {62, 0.20}, {60, 0.22}, {57, 0.48},
	}
	var lead [][]float64
	var bass [][]float64
	for _, n := range notes {
		lead = append(lead, pulseNote(n.midi, n.dur, 0.25, 0.38, 0.006, 0.08))
		bass = append(bass, triNote(n.midi-12, n.dur, 0.28, 0.01, 0.10))
	}
	rumble := gain(adsr(noiseWave(1.30, 12), 0.02, 0.55), 0.14)
	return mix(concat(lead...), concat(bass...), rumble)
}

// pauseSFX: two-tone confirm (pause and the first tap of resume share this).
func pauseSFX() []float64 {
	a := pulseNote(76, 0.09, 0.25, 0.32, 0.004, 0.04)
	gap := silence(0.03)
	b := pulseNote(71, 0.14, 0.25, 0.30, 0.004, 0.06)
	return concat(a, gap, b)
}

// --- BGM -----------------------------------------------------------------

// musicLoop is an original 8-bar A-minor stacker theme. Pulse lead, triangle
// bass, a quiet fifth harmony, and noise hats. Loops on the downbeat.
func musicLoop() []float64 {
	bpm := 148.0
	beat := 60.0 / bpm
	eighth := 0.5 * beat

	// 32 eighths × 2 = 8 bars. 0 = rest.
	lead := []int{
		69, 72, 76, 74, 72, 69, 67, 69, // A4 C5 E5 D5  C5 A4 G4 A4
		76, 79, 81, 79, 76, 74, 72, 69, // E5 G5 A5 G5  E5 D5 C5 A4
		77, 76, 74, 72, 74, 76, 69, 72, // F5 E5 D5 C5  D5 E5 A4 C5
		76, 74, 72, 67, 69, 64, 69, 0, // E5 D5 C5 G4  A4 E4 A4 —
		69, 72, 76, 79, 81, 79, 76, 74, // A4 C5 E5 G5  A5 G5 E5 D5
		72, 74, 76, 72, 69, 67, 69, 72, // C5 D5 E5 C5  A4 G4 A4 C5
		74, 77, 76, 74, 72, 69, 67, 64, // D5 F5 E5 D5  C5 A4 G4 E4
		69, 72, 76, 74, 72, 69, 67, 69, // A4 C5 E5 D5  C5 A4 G4 A4
	}
	// Harmony sits a third (or fourth) below the lead — same rhythm.
	harm := []int{
		64, 69, 72, 71, 69, 64, 62, 64,
		72, 74, 76, 74, 72, 71, 69, 64,
		72, 72, 71, 69, 71, 72, 64, 69,
		72, 71, 69, 62, 64, 60, 64, 0,
		64, 69, 72, 74, 76, 74, 72, 71,
		69, 71, 72, 69, 64, 62, 64, 69,
		71, 72, 72, 71, 69, 64, 62, 60,
		64, 69, 72, 71, 69, 64, 62, 64,
	}
	// Bass: one quarter per two eighths (32 quarters across 8 bars).
	bass := []int{
		45, 45, 52, 52, 53, 53, 52, 52, // A2 A2 E3 E3 F3 F3 E3 E3
		50, 50, 45, 45, 52, 52, 45, 45, // D3 D3 A2 A2 E3 E3 A2 A2
		45, 45, 48, 48, 43, 43, 45, 45, // A2 A2 C3 C3 G2 G2 A2 A2
		50, 53, 52, 50, 45, 40, 45, 45, // D3 F3 E3 D3 A2 E2 A2 A2
	}

	var leadBuf, harmBuf []float64
	for i, n := range lead {
		if n == 0 {
			leadBuf = append(leadBuf, silence(eighth)...)
			harmBuf = append(harmBuf, silence(eighth)...)
			continue
		}
		leadBuf = append(leadBuf, pulseNote(n, eighth, 0.125, 0.34, 0.008, 0.045)...)
		if harm[i] == 0 {
			harmBuf = append(harmBuf, silence(eighth)...)
		} else {
			harmBuf = append(harmBuf, pulseNote(harm[i], eighth, 0.25, 0.12, 0.010, 0.050)...)
		}
	}

	var bassBuf []float64
	q := beat
	for _, n := range bass {
		if n == 0 {
			bassBuf = append(bassBuf, silence(q)...)
			continue
		}
		bassBuf = append(bassBuf, triNote(n, q, 0.30, 0.012, 0.07)...)
	}

	// Hats on every eighth; a slightly heavier click on the beat.
	var hats []float64
	for i := 0; i < len(lead); i++ {
		div := 3
		g := 0.055
		if i%2 == 0 {
			div = 5
			g = 0.08
		}
		click := gain(adsr(noiseWave(eighth*0.35, div), 0.001, eighth*0.28), g)
		slot := silence(eighth)
		overlay(slot, click, 0, 1)
		hats = append(hats, slot...)
	}

	// Soft kick on beats 1 and 3 of each bar (every 4 eighths).
	var kicks []float64
	for i := 0; i < len(lead); i++ {
		slot := silence(eighth)
		if i%4 == 0 {
			k := adsr(sweepTri(eighth*0.55, 110, 48, 0.28), 0.002, eighth*0.35)
			overlay(slot, k, 0, 1)
		}
		kicks = append(kicks, slot...)
	}

	// A hair of delayed lead for width — 3 sixteenths later, quieter.
	echo := make([]float64, len(leadBuf)+nSamples(eighth*0.5))
	copy(echo, leadBuf)
	overlay(echo, leadBuf, nSamples(eighth*0.5), 0.18)

	return mix(echo, harmBuf, bassBuf, hats, kicks)
}

func main() {
	outdir := "./examples/tetris/audio"
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
		{"move.wav", moveSFX},
		{"rotate.wav", rotateSFX},
		{"lock.wav", lockSFX},
		{"drop.wav", dropSFX},
		{"clear.wav", clearSFX},
		{"tetris.wav", tetrisSFX},
		{"levelup.wav", levelupSFX},
		{"gameover.wav", gameoverSFX},
		{"pause.wav", pauseSFX},
		{"music.wav", musicLoop},
	}
	for _, f := range files {
		if err := writeWAV(filepath.Join(outdir, f.name), f.gen()); err != nil {
			panic(err)
		}
	}
}
