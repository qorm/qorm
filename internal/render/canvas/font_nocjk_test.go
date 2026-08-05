//go:build qorm_nocjk

package canvas

import "testing"

// With the qorm_nocjk tag the phase-2 engine must not exist: no provider,
// no DefaultTextMeasurer swap, pure phase-1 bitmap behaviour.
func TestNoTTFEngineWithoutCJKTag(t *testing.T) {
	if ttfProvider != nil {
		t.Error("ttfProvider must be nil with the qorm_nocjk build tag")
	}
	if activeTTFEngine() != nil {
		t.Error("activeTTFEngine must be nil with the qorm_nocjk build tag")
	}
	if _, isBitmap := DefaultTextMeasurer.(bitmapMeasurer); !isBitmap {
		t.Error("DefaultTextMeasurer must stay the bitmap measurer with qorm_nocjk")
	}
	if w := MeasureText("中文", 10); w != 20 {
		t.Errorf("MeasureText(中文, 10) = %v, want 20 (bitmap metrics)", w)
	}
}
