package server

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// swipeDirection classifies press→release travel into "left"/"right"/"up"/
// "down", or "" when it is not a swipe. Mirrors the canvas engine
// (internal/render/canvas swipeMinDist=24, swipeAxisDominance=1.3) and the
// app.js recognizer (SWIPE_MIN / SWIPE_AXIS) — keep the three in lockstep.
func swipeDirection(dx, dy float64) string {
	const (
		swipeMinDist       = 24.0
		swipeAxisDominance = 1.3
	)
	ax, ay := math.Abs(dx), math.Abs(dy)
	if math.Max(ax, ay) < swipeMinDist {
		return ""
	}
	if ax > ay*swipeAxisDominance {
		if dx > 0 {
			return "right"
		}
		return "left"
	}
	if ay > ax*swipeAxisDominance {
		if dy > 0 {
			return "down"
		}
		return "up"
	}
	return ""
}

func TestSwipeDirection(t *testing.T) {
	cases := []struct {
		dx, dy float64
		want   string
	}{
		{-100, 5, "left"}, {100, -5, "right"}, {3, -100, "up"}, {-3, 100, "down"},
		{-10, 0, ""},  // below the distance floor
		{100, 90, ""}, // diagonal: no dominant axis
		{0, 0, ""},    // no travel at all
		{24, 0, "right"},
		{23.9, 0, ""},
	}
	for _, c := range cases {
		if got := swipeDirection(c.dx, c.dy); got != c.want {
			t.Errorf("swipeDirection(%v, %v) = %q, want %q", c.dx, c.dy, got, c.want)
		}
	}
}

func sceneBindingsApp(keys, swipes map[string]string) *runtime.Runtime {
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	}
	if len(keys) > 0 {
		app.SceneKeys = map[string]map[string]string{"main": keys}
	}
	if len(swipes) > 0 {
		app.SceneSwipes = map[string]map[string]string{"main": swipes}
	}
	return runtime.New(app)
}

func TestQormSwipeBindingsEmit(t *testing.T) {
	rt := sceneBindingsApp(
		map[string]string{"left": "slideLeft"},
		map[string]string{"left": "slideLeft", "up": "slideUp"},
	)
	snip := qormKeyBindings(rt)
	if !strings.Contains(snip, "window.__qormSwipes=") {
		t.Fatalf("expected __qormSwipes in snippet, got %q", snip)
	}
	if !strings.Contains(snip, "window.__qormKeys=") {
		t.Fatalf("keys must still emit when swipes are present: %q", snip)
	}
	// The map is JSON-marshaled; check both directions are present.
	for _, want := range []string{`"left":"slideLeft"`, `"up":"slideUp"`} {
		if !strings.Contains(snip, want) {
			t.Errorf("snippet missing %s: %s", want, snip)
		}
	}
	// Page embeds the snippet in the script body.
	page := Page(rt, "x", 0)
	if !strings.Contains(page, "window.__qormSwipes=") {
		t.Error("Page must embed __qormSwipes for the HTML client")
	}
}

func TestQormSwipeBindingsOnly(t *testing.T) {
	// A scene with swipes but no keys must still emit bindings (gamespad-only).
	rt := sceneBindingsApp(nil, map[string]string{"down": "drop"})
	snip := qormKeyBindings(rt)
	if snip == "" {
		t.Fatal("swipes-only scene must not return an empty bindings snippet")
	}
	if !strings.Contains(snip, "window.__qormSwipes=") {
		t.Fatalf("expected __qormSwipes, got %q", snip)
	}
	var m map[string]string
	// Extract the JSON object after __qormSwipes=
	i := strings.Index(snip, "window.__qormSwipes=")
	rest := snip[i+len("window.__qormSwipes="):]
	end := strings.Index(rest, ";")
	if end < 0 {
		t.Fatalf("unterminated __qormSwipes: %q", snip)
	}
	if err := json.Unmarshal([]byte(rest[:end]), &m); err != nil {
		t.Fatalf("parse swipes JSON: %v (%q)", err, rest[:end])
	}
	if m["down"] != "drop" {
		t.Errorf("swipes map = %v, want down=drop", m)
	}
}

func TestQormKeyBindingsUnchangedWithoutSwipes(t *testing.T) {
	rt := sceneBindingsApp(map[string]string{"a": "act"}, nil)
	snip := qormKeyBindings(rt)
	if !strings.Contains(snip, "window.__qormKeys=") {
		t.Fatal("keys must still emit")
	}
	// Swipes field is present (null or {}) so the client can always read it.
	if !strings.Contains(snip, "window.__qormSwipes=") {
		t.Fatal("snippet should still declare __qormSwipes (null/empty) for a stable client shape")
	}
}

func TestClientScriptSceneSwipeWiring(t *testing.T) {
	js := qormAppJS(1, "tok")
	for _, want := range []string{
		"window.__qormSceneSwipeReady",
		"window.__qormSwipeDirection",
		"var SWIPE_MIN = 24",
		"var SWIPE_AXIS = 1.3",
		"function swipeDirection(dx, dy)",
		"window.__qormSwipes",
		"typeof qormSwipe",
		"typeof qormAction",
		"document.addEventListener('pointerdown'",
		"document.addEventListener('pointerup'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks scene-swipe wiring %q", want)
		}
	}
	// Keys path must still be intact.
	for _, want := range []string{
		"window.__qormKeys",
		"document.addEventListener('keydown'",
		"document.addEventListener('keyup'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js must keep key bindings after swipe wiring: missing %q", want)
		}
	}
}
