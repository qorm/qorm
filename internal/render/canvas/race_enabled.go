//go:build race

package canvas

// raceEnabled reports whether this binary was built with the race detector.
// The Go toolchain sets the implicit `race` build tag under `go test -race`.
// Perf-budget tests use it to skip wall-clock thresholds: race instrumentation
// slows the software rasterizer several-fold, so a frame-time budget measured
// under it says nothing about production speed.
const raceEnabled = true
