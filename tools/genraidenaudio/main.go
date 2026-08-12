// genraidenaudio bakes chiptune-style WAV files for examples/raiden:
// shoot / hit / explode / death / music. CC0 (original compositions).
//
// Usage: go run ./tools/genraidenaudio [outdir]
//
//	outdir defaults to ./examples/raiden/audio.
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
		_ = binary.Write(f, binary.LittleEndian, v)
	}
	return nil
}

func squareWave(dur, f float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		if math.Sin(2*math.Pi*f*t) >= 0 {
			out[i] = 1
		} else {
			out[i] = -1
		}
	}
	return out
}

func noiseWave(dur float64) []float64 {
	n := nSamples(dur)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = math.Sin(float64(i)*12.9898)*43758.5453 - math.Floor(math.Sin(float64(i)*12.9898)*43758.5453)
		out[i] = out[i]*2 - 1
	}
	return out
}

func adsr(samples []float64, a, r float64) []float64 {
	n := len(samples)
	aN := int(a * float64(sampleRate))
	rN := int(r * float64(sampleRate))
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

func concat(parts ...[]float64) []float64 {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	out := make([]float64, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func silence(dur float64) []float64 { return make([]float64, nSamples(dur)) }

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
		out[i] = math.Tanh(out[i] * 0.7)
	}
	return out
}

// shootSFX: short downward blip — classic "pew" of a laser.
func shootSFX() []float64 {
	n := nSamples(0.06)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		f := 1200 * math.Pow(400/1200, t/0.06)
		if math.Sin(2*math.Pi*f*t) >= 0 {
			out[i] = 0.6
		} else {
			out[i] = -0.6
		}
	}
	return adsr(out, 0.002, 0.02)
}

// hitSFX: short high-pitched "ding" — small fighter explosion.
func hitSFX() []float64 {
	return adsr(noiseWave(0.10), 0.002, 0.06)
}

// explodeSFX: longer noise burst with low rumble — big explosion.
func explodeSFX() []float64 {
	return adsr(noiseWave(0.35), 0.005, 0.25)
}

// deathSFX: descending dramatic crash — game over.
func deathSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{60, 0.15}, {58, 0.15}, {55, 0.15}, {52, 0.20}, {48, 0.50},
	}
	var parts [][]float64
	for _, n := range notes {
		w := squareWave(n.dur, freq(n.midi))
		parts = append(parts, adsr(w, 0.005, 0.10))
	}
	return mix(concat(parts...))
}

// powerupSFX: ascending arpeggio — classic "you got a power-up" chime.
func powerupSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{72, 0.06}, {76, 0.06}, {79, 0.06}, {84, 0.10}, {88, 0.15},
	}
	var parts [][]float64
	for _, n := range notes {
		parts = append(parts, adsr(squareWave(n.dur, freq(n.midi)), 0.005, 0.04))
	}
	return mix(concat(parts...))
}

// winSFX: triumphant major fanfare — boss defeated / stage clear.
func winSFX() []float64 {
	notes := []struct {
		midi int
		dur  float64
	}{
		{60, 0.10}, {64, 0.10}, {67, 0.10}, {72, 0.20},
		{76, 0.15}, {79, 0.15}, {84, 0.40},
	}
	var parts [][]float64
	for _, n := range notes {
		parts = append(parts, adsr(squareWave(n.dur, freq(n.midi)), 0.005, 0.06))
	}
	return mix(concat(parts...))
}

// musicLoop: 8-bar driving action loop in E minor — classic shmup vibe.
func musicLoop() []float64 {
	bpm := 144.0
	beat := 60.0 / bpm
	lead := []int{76, 79, 81, 79, 76, 74, 72, 74, 76, 79, 76, 74, 72, 69, 67, 64}
	bass := []int{40, 40, 43, 43, 40, 40, 38, 38, 40, 40, 43, 43, 40, 40, 38, 38}
	durs := make([]float64, 16)
	for i := range durs {
		durs[i] = 0.5
	}

	var leadBuf []float64
	for i, n := range lead {
		if n == 0 {
			leadBuf = append(leadBuf, silence(durs[i]*beat)...)
			continue
		}
		leadBuf = append(leadBuf, adsr(squareWave(durs[i]*beat, freq(n)), 0.01, 0.06)...)
	}
	var bassBuf []float64
	for i, n := range bass {
		if n == 0 {
			bassBuf = append(bassBuf, silence(durs[i]*beat)...)
			continue
		}
		bassBuf = append(bassBuf, adsr(squareWave(durs[i]*beat, freq(n)), 0.02, 0.08)...)
	}
	return mix(leadBuf, bassBuf)
}

func main() {
	outdir := "./examples/raiden/audio"
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
		{"shoot.wav", shootSFX},
		{"hit.wav", hitSFX},
		{"explode.wav", explodeSFX},
		{"powerup.wav", powerupSFX},
		{"win.wav", winSFX},
		{"death.wav", deathSFX},
		{"music.wav", musicLoop},
	}
	for _, f := range files {
		if err := writeWAV(filepath.Join(outdir, f.name), f.gen()); err != nil {
			panic(err)
		}
	}
}
