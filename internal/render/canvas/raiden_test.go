package canvas

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// nowMinus returns now - d; tiny helper so the test stays a single import.
func nowMinus(d time.Duration) time.Time { return time.Now().Add(-d) }

// raidenFixture loads examples/raiden and runs scene onEnter (restart).
func raidenFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "raiden"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("loader diagnostic: %s", d)
	}
	if len(app.Diagnostics) != 0 {
		t.FailNow()
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()
	if rt.LastScriptError != "" {
		t.Fatalf("onEnter: %s", rt.LastScriptError)
	}
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(image.Pt(320, 560)), rt
}

// TestRaidenFirstFrame renders the freshly-loaded raiden scene.
func TestRaidenFirstFrame(t *testing.T) {
	e, surf, rt := raidenFixture(t)
	e.DrawFrame(surf)
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v at start, want playing", s)
	}
	if x, _ := rt.State["player"].(map[string]any)["x"].(float64); x != 132 {
		t.Errorf("player.x = %v at start, want 132", x)
	}
}

// TestRaidenTickAdvances holds FIRE and drives the timer, checking that
// bullets and enemies get populated by the fire / spawn logic.
func TestRaidenTickAdvances(t *testing.T) {
	e, surf, rt := raidenFixture(t)
	// Hold the fire key so fireBullet runs every tick.
	keys, _ := rt.State["keys"].(map[string]any)
	keys["fire"] = true
	for i := 0; i < 35; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = nowMinus(time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	bullets, _ := rt.State["bullets"].([]any)
	if len(bullets) == 0 {
		t.Error("no bullets fired after 35 ticks with FIRE held")
	}
	enemies, _ := rt.State["enemies"].([]any)
	if len(enemies) == 0 {
		t.Error("no enemies spawned after 35 ticks")
	}
}

// TestRaidenTurretFires places a ground turret on screen and advances enough
// ticks for it to shoot an aimed enemy bullet (type 9) at the player.
func TestRaidenTurretFires(t *testing.T) {
	e, surf, rt := raidenFixture(t)
	e.DrawFrame(surf)
	// Make the player invulnerable so the turret's "snap to ground" y=444
	// doesn't kill it via checkPlayerHits (the test only cares that the
	// turret FIRES, not the player/enemy interaction). Without this, a
	// bigger-sprite player or a closer turret silently kills the player
	// and the bullet we want to see is filtered out as a player-hit.
	rt.State["player"].(map[string]any)["invuln"] = 9999.0
	// Plant a live turret mid-field.
	enemies := rt.State["enemies"].([]any)
	enemies = append(enemies,
		map[string]any{"x": 160.0, "y": 300.0, "type": 4.0, "alive": true, "form": 0.0, "phase": 0.0, "dir": 1.0, "drops": 0.0},
	)
	rt.State["enemies"] = enemies
	// Advance enough ticks for mod(tick,20)==0 to hit at least once.
	for i := 0; i < 40; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = nowMinus(time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	if rt.LastScriptError != "" {
		t.Errorf("script error: %s", rt.LastScriptError)
	}
	found := false
	for _, b := range rt.State["bullets"].([]any) {
		if b.(map[string]any)["type"] == 9.0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("turret did not fire an enemy bullet within 40 ticks")
	}
}

// TestRaidenBombClearsScreen fills the field with enemies, fires a bomb, and
// checks every enemy dies and one bomb is consumed.
func TestRaidenBombClearsScreen(t *testing.T) {
	e, surf, rt := raidenFixture(t)
	e.DrawFrame(surf)
	// Seed a few live enemies.
	enemies := rt.State["enemies"].([]any)
	enemies = append(enemies,
		map[string]any{"x": 100.0, "y": 100.0, "type": 1.0, "alive": true, "form": 0.0, "phase": 0.0, "dir": 1.0, "drops": 0.0},
		map[string]any{"x": 200.0, "y": 150.0, "type": 2.0, "alive": true, "form": 0.0, "phase": 0.0, "dir": 1.0, "drops": 0.0},
	)
	rt.State["enemies"] = enemies
	before := rt.State["player"].(map[string]any)["bombs"].(float64)
	if err := rt.DispatchErr("bomb", nil); err != nil {
		t.Fatalf("bomb dispatch: %v", err)
	}
	after := rt.State["player"].(map[string]any)["bombs"].(float64)
	if after != before-1 {
		t.Errorf("bombs = %v after bomb, want %v", after, before-1)
	}
	for _, en := range rt.State["enemies"].([]any) {
		if en.(map[string]any)["alive"] == true {
			t.Error("an enemy survived the bomb")
		}
	}
}
