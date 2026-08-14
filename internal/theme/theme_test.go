package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qorm/platform/internal/anim"
)

func writeTheme(t *testing.T, body string) *Theme {
	t.Helper()
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	th, err := LoadTheme(path)
	if err != nil {
		t.Fatalf("LoadTheme: %v", err)
	}
	return th
}

func TestMotionTokensParsed(t *testing.T) {
	th := writeTheme(t, `{
		"name": "t",
		"colors": {"focus": "#123456"},
		"motion": {
			"durationFast": 80, "durationNormal": 167, "durationSlow": 300,
			"easingStandard": "easeInOutCubic", "easingEmphasized": "linear"
		}
	}`)
	if got := th.DurationMs("fast"); got != 80 {
		t.Errorf("DurationMs(fast) = %d, want 80", got)
	}
	if got := th.DurationMs("normal"); got != 167 {
		t.Errorf("DurationMs(normal) = %d, want 167", got)
	}
	if got := th.DurationMs("slow"); got != 300 {
		t.Errorf("DurationMs(slow) = %d, want 300", got)
	}
	if c := th.Easing("standard"); c(0.25) != anim.EaseInOutCubic(0.25) {
		t.Error("Easing(standard) should resolve easingStandard=easeInOutCubic")
	}
	if c := th.Easing("emphasized"); c(0.3) != anim.Linear(0.3) {
		t.Error("Easing(emphasized) should resolve easingEmphasized=linear")
	}
	if _, ok := th.GetColor("focus"); !ok {
		t.Error("focus color should resolve")
	}
}

func TestMotionDefaults(t *testing.T) {
	// Theme with no motion section at all.
	th := writeTheme(t, `{"name": "t", "colors": {}}`)
	if got := th.DurationMs("fast"); got != DefaultDurationFast {
		t.Errorf("DurationMs(fast) default = %d, want %d", got, DefaultDurationFast)
	}
	if got := th.DurationMs("normal"); got != DefaultDurationNormal {
		t.Errorf("DurationMs(normal) default = %d, want %d", got, DefaultDurationNormal)
	}
	if got := th.DurationMs("slow"); got != DefaultDurationSlow {
		t.Errorf("DurationMs(slow) default = %d, want %d", got, DefaultDurationSlow)
	}
	if got := th.DurationMs("bogus"); got != DefaultDurationNormal {
		t.Errorf("unknown duration name should fall back to normal, got %d", got)
	}

	// Nil receiver must be safe (canvas path with no theme loaded).
	var nilTh *Theme
	if got := nilTh.DurationMs("normal"); got != DefaultDurationNormal {
		t.Errorf("nil Theme DurationMs = %d, want default", got)
	}
	if c := nilTh.Easing("standard"); c(0.4) != anim.EaseOutCubic(0.4) {
		t.Error("nil Theme Easing(standard) should fall back to EaseOutCubic")
	}
	if c := nilTh.Easing("emphasized"); c(0.4) != anim.EaseInOutCubic(0.4) {
		t.Error("nil Theme Easing(emphasized) should fall back to EaseInOutCubic")
	}
}

func TestMotionUnknownEasingFallsBack(t *testing.T) {
	th := writeTheme(t, `{
		"name": "t",
		"motion": {"easingStandard": "springyMcspringface"}
	}`)
	if c := th.Easing("standard"); c(0.4) != anim.EaseOutCubic(0.4) {
		t.Error("unknown easing name must never panic and must fall back to EaseOutCubic")
	}
}

func TestGetDefaultHasInteractionAndMotionTokens(t *testing.T) {
	th := GetDefault()
	btn := th.Components["button"]
	if btn.PressedBackgroundColor == "" || btn.HoveredBackgroundColor == "" || btn.PressedOpacity == nil {
		t.Error("default theme button must define pressed/hovered feedback so the canvas backend works without themes/*.json")
	}
	if _, ok := th.GetColor("focus"); !ok {
		t.Error("default theme must define a focus color")
	}
	if th.Motion == nil {
		t.Fatal("default theme must define motion tokens")
	}
	if got := th.DurationMs("normal"); got != DefaultDurationNormal {
		t.Errorf("default DurationMs(normal) = %d", got)
	}
}

func TestInteractiveStateFieldsLoad(t *testing.T) {
	th := writeTheme(t, `{
		"name": "t",
		"colors": {},
		"components": {
			"button": {
				"pressedBackgroundColor": "#010203",
				"hoveredBackgroundColor": "#040506",
				"pressedOpacity": 0.75
			}
		}
	}`)
	btn := th.Components["button"]
	if btn.PressedBackgroundColor != "#010203" || btn.HoveredBackgroundColor != "#040506" {
		t.Error("interactive background keys must load from JSON")
	}
	if btn.PressedOpacity == nil || *btn.PressedOpacity != 0.75 {
		t.Error("pressedOpacity must load from JSON")
	}
}
