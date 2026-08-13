//go:build !race

package canvas

// raceEnabled reports whether this binary was built with the race detector
// (see race_enabled.go).
const raceEnabled = false
