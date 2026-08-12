package canvas

import "testing"

func TestMeasureTextTracking(t *testing.T) {
	base := MeasureText("AB", 20)
	spaced := MeasureTextTracking("AB", 20, 4)
	if spaced != base+4 {
		t.Fatalf("letterSpacing 4: got %v want %v", spaced, base+4)
	}
	if MeasureTextTracking("A", 20, 10) != MeasureText("A", 20) {
		t.Fatal("single rune must ignore letterSpacing gaps")
	}
}

func TestLineHeightMult(t *testing.T) {
	if got := lineHeightMult(0, 14); got != 1.2 {
		t.Fatalf("default mult = %v", got)
	}
	if got := textLineHM(20, 2); got != 40 {
		t.Fatalf("2×20 = %d want 40", got)
	}
}
