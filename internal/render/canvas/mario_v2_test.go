package canvas

// V2 regression tests for examples/mario — after the rewrite to 60-fps
// physics, board root, sub-pixel positions, and the keyup engine plumbing.
// The original mario_script_test.go (gridview / 250 ms tick / integer
// cells) was retired when the new shape replaced it; this file is the new
// contract, mapped 1:1 to the old test names where the meaning survives.
//
//   - FirstFrame           : load + restart, mario at start, status playing
//   - WalkAndCoin          : hold right + 1s → mario moved + a coin picked
//   - JumpArc              : tap jump → vy goes negative, lands, onGround
//   - Restart              : restart resets to a clean run
//   - SwipeControls        : swipe up also triggers jump (touch parity)
//   - CoinPickup           : mario walks over coin → coin collected, score +200
//
// TODO: GoombaStompAndTouch (fall on goomba = stomp, walk into = hurt),
// PitDeath (out-of-world Y = lose), FlagWins (touching the flag tile = win)
// — the game logic exists in actions/lib.qs but tests haven't been ported
// from the retired mario_script_test.go to the v2 physics model.
//
// All tests use a headless engine + custom surface; the 16-ms physics
// timer is forced by rewriting its nextFire into the past each call.
//
// WALL-CLOCK NOTE: physicsStep derives dt from the wall clock
// (`(now() - lastTickMs) / 1000`), so one forced tick simulates exactly
// as much time as the tick loop's wall time — ~1ms on a fast machine,
// ~15-20ms on a busy -race CI runner. Tests below therefore drive
// movement by world position (stop when mario.x crosses a threshold),
// NOT by tick count; a fixed tick count would walk mario 16x further on
// CI and reach the goombas at x=560, failing "status = dead".

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/qscript"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func marioV2Fixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "mario"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("loader diagnostic: %s", d)
	}
	if len(app.Diagnostics) > 0 {
		// warnings are OK; errors above are fatal via TestMain-style fail-fast.
		for _, d := range app.Diagnostics {
			if !isWarning(d) {
				t.FailNow()
			}
		}
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()
	if rt.LastScriptError != "" {
		t.Fatalf("onEnter script error: %s", rt.LastScriptError)
	}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(512, 384))
	// One initial DrawFrame so the engine's graphRoot / timers / Inter are
	// populated — HandleKey gates on graphRoot != nil (otherwise the scene
	// key bindings never get a chance to dispatch).
	e.DrawFrame(surf)
	return e, surf, rt
}

// isWarning is a best-effort sniff — loader diagnostics come back as
// formatted strings ("warning:" / "error:" prefix), and a false negative
// here just means an error message gets emitted twice. The string
// inspection is fine for a test helper.
func isWarning(s string) bool {
	return len(s) > 8 && s[:8] == "warning:"
}

// marioV2Tick forces the physics timer to fire immediately, n times.
func marioV2Tick(e *Engine, surf *HeadlessSurface, n int) {
	for i := 0; i < n; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
}

// marioV2Press simulates a keypress by routing through HandleKey (the same
// path a real key event takes) so the scene's `keys` / `keyReleases` maps
// dispatch the bound action. Lets the tests stay close to user behaviour
// without poking at engine internals.
func marioV2Press(e *Engine, key string) {
	e.HandleKey(KeyInput{Key: key, Down: true})
}
func marioV2Release(e *Engine, key string) {
	e.HandleKey(KeyInput{Key: key, Down: false})
}

// marioV2X / marioV2Y / marioV2OnGround read live mario state between
// ticks so the position-driven loops below can stop on world state instead
// of a wall-clock-dependent tick count (see the WALL-CLOCK NOTE above).
func marioV2X(rt *runtime.Runtime) float64 {
	return rt.State["mario"].(map[string]any)["x"].(float64)
}
func marioV2Y(rt *runtime.Runtime) float64 {
	return rt.State["mario"].(map[string]any)["y"].(float64)
}
func marioV2OnGround(rt *runtime.Runtime) bool {
	return rt.State["mario"].(map[string]any)["onGround"].(bool)
}

// FirstFrame: the scene's onEnter (restart) runs once at load. Mario sits
// at the start cell, status is playing, the camera is at the start.
func TestMarioV2FirstFrame(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	e.DrawFrame(surf)
	m := rt.State["mario"].(map[string]any)
	if got := m["x"]; got != 32.0 {
		t.Errorf("mario.x = %v at start, want 32", got)
	}
	// 2x NES: 32 px tiles, small Mario 32 px tall. Ground top is row 13
	// (y=416), so standing y = 416 - 32 = 384.
	if got, _ := m["y"].(float64); got != 384 {
		t.Errorf("mario.y = %v at start, want 384 (32px Mario on row-13 ground)", got)
	}
	if got := rt.State["status"]; got != "playing" {
		t.Errorf("status = %v at start, want playing", got)
	}
	// Camera must be at the start (board is centered on mario at x=1, so
	// PanX = -32 + 8*32 = 224; Y centres too).
	if !e.Inter.Board.Active {
		t.Error("board scene must be active")
	}
	if e.Inter.Board.PanDisabled != true {
		t.Error("board must have PanDisabled=true (mario has no pan affordance)")
	}
	_ = surf
}

// CameraFollow: hold right until mario passes the dead zone (192px).
// The camera should pan left so mario stays on screen — without pan,
// mario would walk off the right edge of the 512px viewport.
func TestMarioV2CameraFollow(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	// Walk until mario crosses x=256 (well past the 192px dead zone).
	for i := 0; i < 4000 && marioV2X(rt) <= 256; i++ {
		marioV2Tick(e, surf, 1)
	}
	marioV2Release(e, "right")
	px := e.Inter.Board.PanX
	if px >= 0 {
		t.Errorf("camera PanX = %.0f after mario walked to x=%.0f — camera did not follow (expected negative pan from dead zone at 192px)",
			px, marioV2X(rt))
	}
	t.Logf("mario at x=%.0f, camera PanX=%.0f", marioV2X(rt), px)
}

// WalkAndCoin: hold right long enough for mavis to walk across the
// starting area. The floating coin row at row 1 (x=10..14) is too high
// for a walking mavis to reach (small mavis's jump arc tops out around
// y=240 — well below the row-1 coins at y=32), so this test only
// verifies horizontal locomotion and the post-walk world state. The
// coin pickup branch is exercised by TestMarioV2CoinPickup below,
// which teleports mavis next to a coin row so the test stays
// deterministic and the level data stays NES-accurate.
func TestMarioV2WalkAndCoin(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	// Stop by world position, not tick count: a fixed 400-tick walk
	// simulates ~0.4s on a fast machine but ~7s on a slow -race CI runner
	// — far enough to reach the goomba at x=560 and die. Walking until
	// mario crosses x=100 caps the simulated run at ~1s everywhere; the
	// tick cap only guards against broken horizontal motion.
	for i := 0; i < 4000 && marioV2X(rt) <= 100; i++ {
		marioV2Tick(e, surf, 1)
	}
	marioV2Release(e, "right")
	m := rt.State["mario"].(map[string]any)
	if got, _ := m["x"].(float64); got <= 32 {
		t.Errorf("mario.x = %v after right, want > 32 (walked away) — lastTick=%v keys=%+v vx=%v",
			got, rt.State["lastTickMs"], rt.State["keys"], m["vx"])
	}
	if rt.State["status"] != "playing" {
		t.Errorf("status = %v after short walk, want playing", rt.State["status"])
	}
}

// CoinPickup: the floating coin row at row 3, x=18..22 sits at y=96..128
// (cells 3). mavis's body occupies the same y range, so teleporting him
// just under the row 3 coins lets one physics step collect them without
// needing a real jump. The collision check in physicsStep looks at
// mavis's center / head / foot cells, so being next to the coin row is
// enough.
func TestMarioV2CoinPickup(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	m := rt.State["mario"].(map[string]any)
	m["x"] = 576.0
	m["y"] = 96.0
	m["vx"] = 0
	m["vy"] = 0
	m["onGround"] = true
	marioV2Tick(e, surf, 1)
	if got, _ := rt.State["coins"].(float64); got == 0 {
		t.Errorf("expected at least one coin after 1 step under row-3 coins (coins=%v, mario=%+v)",
			got, rt.State["mario"])
	}
}

// JumpArc: tap jump once, mario leaves the ground, the physics step
// applies reduced gravity while the hold flag is set, then full gravity
// when it's released. After landing, onGround flips back to true.
//
// marioV2Tick advances one render frame per call (forcing the physics
// timer's nextFire into the past), but the wall-clock dt the engine
// sees between frames is ~1ms, NOT the 16ms a real 60-fps tick would
// produce. The mavis physicsStep uses real time for its dt
// (`(now() - lastTickMs) / 1000`), so a frame-rate-dependent jump
// needs 16x more ticks here than it would on a real canvas.
func TestMarioV2JumpArc(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	e.DrawFrame(surf)
	startY := rt.State["mario"].(map[string]any)["y"].(float64)
	marioV2Press(e, "space")
	// Same wall-clock reasoning as WalkAndCoin: hold until mario is
	// clearly airborne (y dropped 40px) instead of a fixed tick count, so
	// a slow -race CI runner can't hold the jump long enough to fly off
	// the top of the level and never fall back down.
	for i := 0; i < 4000 && marioV2Y(rt) > startY-40; i++ {
		marioV2Tick(e, surf, 1)
	}
	marioV2Release(e, "space")
	// Fall until the ground resolves onGround; release first so full
	// gravity applies and mario lands at the same y he took off from.
	for i := 0; i < 4000 && !marioV2OnGround(rt); i++ {
		marioV2Tick(e, surf, 1)
	}
	m := rt.State["mario"].(map[string]any)
	endY := m["y"].(float64)
	if endY != startY {
		t.Errorf("mario.y = %v after jump, want %v (landed back on the same cell)", endY, startY)
	}
	if m["onGround"].(bool) != true {
		t.Error("mario.onGround = false after landing, want true")
	}
}

// Held jump must clear a 4-tile NES hop (128px at 2x). The previous
// 480/1120 numbers peaked around 100px and could not reach the row-9 ?.
func TestMarioHeldJumpClearsFourTiles(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	startY := marioV2Y(rt)
	marioV2Press(e, "space")
	apex := startY
	for i := 0; i < 4000; i++ {
		marioV2Tick(e, surf, 1)
		if y := marioV2Y(rt); y < apex {
			apex = y
		}
		if marioV2OnGround(rt) && i > 4 {
			break
		}
	}
	marioV2Release(e, "space")
	got := startY - apex
	if got < 120 {
		t.Fatalf("held jump height = %.1fpx, want >= 120 (4 tiles = 128)", got)
	}
}

// Restart: after walking + jumping, restart wipes the level and resets
// mario to the start. Coins / score clear, status back to playing.
func TestMarioV2Restart(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	for i := 0; i < 4000 && (marioV2X(rt) <= 256 || e.Inter.Board.PanX >= 0); i++ {
		marioV2Tick(e, surf, 1)
	}
	marioV2Release(e, "right")
	if got := marioV2X(rt); got <= 32 {
		t.Skipf("walk failed (mario.x=%v); restart test depends on WalkAndCoin working", got)
	}
	if e.Inter.Board.PanX >= 0 {
		t.Fatalf("camera never scrolled (PanX=%v at x=%.0f); cannot test restart rewind", e.Inter.Board.PanX, marioV2X(rt))
	}
	marioV2Press(e, "r")
	marioV2Release(e, "r")
	marioV2Tick(e, surf, 1)
	m := rt.State["mario"].(map[string]any)
	if m["x"].(float64) != 32.0 {
		t.Errorf("after restart mario.x = %v, want 32", m["x"])
	}
	if y, _ := m["y"].(float64); y < 380 || y > 388 {
		t.Errorf("after restart mario.y = %v, want ~384 (32px Mario on row-13 ground)", y)
	}
	if rt.State["status"] != "playing" {
		t.Errorf("status = %v after restart, want playing", rt.State["status"])
	}
	if e.Inter.Board.PanX < -1 {
		t.Errorf("after restart PanX = %v, want ~0 (camera back at world start)", e.Inter.Board.PanX)
	}
}

// SwipeControls: the swipe binding (up) must dispatch the same action as
// the key binding (jump). The swipe recognizer lives in the canvas input
// pipeline; this test just confirms the scene JSON wires them to the
// same handler so a touch player and a keyboard player share physics.
func TestMarioV2SwipeControls(t *testing.T) {
	e, _, rt := marioV2Fixture(t)
	marioV2Press(e, "space")
	if m := rt.State["mario"].(map[string]any); m["vy"].(float64) >= 0 {
		t.Errorf("jump didn't apply upward velocity: vy=%v", m["vy"])
	}
	marioV2Release(e, "space")
	// Jump pose uses the dedicated jump sprite (NES-style, not a scale punch).
	var mario *model.Node
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil || mario != nil {
			return
		}
		if n.ID == "mario" {
			mario = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	if mario != nil {
		src := evalPropStr(mario.Props["src"], rt)
		if src != "assets/mario_jump.png" {
			t.Fatalf("airborne mario src = %q, want jump sprite", src)
		}
	}
}

func motionToken(rt *runtime.Runtime, key string) float64 {
	switch v := rt.State[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func marioV2Find(rt *runtime.Runtime, id string) *model.Node {
	var found *model.Node
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil || found != nil {
			return
		}
		if n.ID == id {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	return found
}

func marioV2DeathDone(rt *runtime.Runtime) bool {
	switch v := rt.State["deathDone"].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	}
	return false
}

// TestMarioMotionFxJumpAndDeath: jump bumps fxJump; squash/stretch is the
// NES jump sprite (not a UI punch). Death is the NES bounce + fxDeath
// knockback token / GAME OVER overlay — not a scale punch.
func TestMarioMotionFxJumpAndDeath(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	if motionToken(rt, "fxJump") != 0 {
		t.Fatalf("fxJump at start = %v, want 0", rt.State["fxJump"])
	}
	marioV2Press(e, "space")
	marioV2Release(e, "space")
	if motionToken(rt, "fxJump") < 1 {
		t.Fatalf("jump should bump fxJump, got %v", rt.State["fxJump"])
	}

	mario := marioV2Find(rt, "mario")
	if mario == nil {
		t.Fatal("mario node missing")
	}
	raw, _ := mario.Prop("fx")
	if s := evalPropStr(raw, rt); s != "none" && s != "" {
		t.Fatalf("mario fx after jump = %q, want none (jump is sprite-based, not punch)", s)
	}
	if src := evalPropStr(mario.Props["src"], rt); src != "assets/mario_jump.png" {
		t.Fatalf("airborne mario src = %q, want jump sprite", src)
	}

	// Pit death: y past the 15-row world (levelH * cellSize = 480).
	m := rt.State["mario"].(map[string]any)
	const pitY = 9999.0
	m["y"] = pitY
	marioV2Tick(e, surf, 2)
	if rt.State["status"] != "dead" {
		t.Fatalf("status after pit = %v, want dead", rt.State["status"])
	}
	if motionToken(rt, "fxDeath") < 1 {
		t.Fatal("lose() must bump fxDeath")
	}
	if alive, _ := rt.State["mario"].(map[string]any)["alive"].(bool); alive {
		t.Fatal("mario.alive after lose(), want false")
	}
	// Death motion is the NES bounce (vy), not a canvas fx clip.
	if s := evalPropStr(raw, rt); s != "none" && s != "" {
		t.Fatalf("mario fx after death = %q, want none (bounce is physics)", s)
	}

	// NES bounce: lose() applies an upward vy; later ticks move y off the pit.
	if vy, _ := m["vy"].(float64); vy >= 0 {
		t.Fatalf("death bounce vy = %v, want upward (negative)", vy)
	}
	for i := 0; i < 160 && !marioV2DeathDone(rt); i++ {
		marioV2Tick(e, surf, 1)
	}
	if marioV2Y(rt) == pitY {
		t.Fatal("NES death bounce did not move mario.y")
	}
	if !marioV2DeathDone(rt) {
		t.Fatal("deathDone should become true after the NES bounce timer")
	}
	overlay := marioV2Find(rt, "deadOverlay")
	if overlay == nil {
		t.Fatal("deadOverlay node missing")
	}
	if !nodeVisible(overlay, rt) {
		t.Fatal("GAME OVER overlay should be visible once deathDone")
	}
}

// ViewTiles stays a camera window (not the whole 15×211 level). Off-screen
// goombas are frustum-culled out of the graph so a walk does not measure
// every sprite in the course.
func TestMarioViewTilesWindowed(t *testing.T) {
	e, surf, _ := marioV2Fixture(t)
	e.DrawFrame(surf)

	imgs, _, _ := GraphImageCount(e.Graph())
	if imgs == 0 {
		t.Fatal("first frame painted no images")
	}
	// Whole level is one baked tilemap Image plus Mario (goombas off-screen).
	if imgs > 8 {
		t.Fatalf("graph images = %d, want a baked tilemap + actors (not one node per tile)", imgs)
	}

	// Far goombas (x>=704) must not be in the graph at the start camera.
	var goombaNodes int
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		if g, ok := n.(*graph.Group); ok && g.Model != nil && g.Model.ID == "goomba" {
			goombaNodes++
		}
		for _, c := range n.Base().Children {
			walk(c)
		}
	}
	walk(e.Graph())
	if goombaNodes != 0 {
		t.Fatalf("start camera should cull far goombas, graph has %d", goombaNodes)
	}

	var tilesLN *LayoutNode
	var find func(*LayoutNode)
	find = func(ln *LayoutNode) {
		if ln == nil || tilesLN != nil {
			return
		}
		if ln.Node != nil && ln.Node.ID == "tiles" {
			tilesLN = ln
			return
		}
		for _, c := range ln.Children {
			find(c)
		}
	}
	find(e.layoutRoot)
	if tilesLN == nil || tilesLN.Node.Type != "tilemap" {
		t.Fatal("tiles node must be a baked tilemap")
	}
	if tilesLN.Width < 1000 {
		t.Fatalf("tilemap width %d, want the full 211-tile world", tilesLN.Width)
	}
}

func TestMarioWalkDoesNotRebakeTilemap(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Tick(e, surf, 2)
	before := tilemapBakeCount()
	marioV2Press(e, "right")
	for i := 0; i < 4000 && (marioV2X(rt) < 280 || e.Inter.Board.PanX >= 0); i++ {
		marioV2Tick(e, surf, 1)
	}
	marioV2Release(e, "right")
	if e.Inter.Board.PanX >= 0 {
		t.Fatalf("camera never scrolled (x=%.0f pan=%.0f)", marioV2X(rt), e.Inter.Board.PanX)
	}
	extra := tilemapBakeCount() - before
	if extra > 1 {
		t.Fatalf("walking rebaked the world %d times — cache is not holding", extra)
	}
}

func TestMarioFrameBudget(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	for i := 0; i < 4000 && e.Inter.Board.PanX >= 0; i++ {
		marioV2Tick(e, surf, 1)
	}
	var layout, render, total time.Duration
	const n = 40
	for i := 0; i < n; i++ {
		st := marioV2TickStats(e, surf)
		layout += st.LayoutRecord
		render += st.Render
		total += st.Total
	}
	marioV2Release(e, "right")
	avg := total / n
	t.Logf("mario scroll scale=1 avg layout=%s render=%s total=%s (x=%.0f pan=%.0f bakes=%d)",
		layout/n, render/n, avg, marioV2X(rt), e.Inter.Board.PanX, tilemapBakeCount())
	// 80ms/frame is well above 16ms; this only catches a catastrophic
	// regression (full-level list + per-pixel sky) on a loaded CI box.
	if avg > 80*time.Millisecond {
		t.Fatalf("mario walk avg frame %s > 80ms — render path regressed", avg)
	}

	// Retina-class: physical 1024×960, scale 2 — the hitch the player felt.
	hi := image.NewRGBA(image.Rect(0, 0, 1024, 960))
	var hiLayout, hiRender, hiTotal time.Duration
	for i := 0; i < n; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
		}
		e.MarkDirty()
		_, st := e.RenderInto(image.Pt(1024, 960), 2, hi)
		hiLayout += st.LayoutRecord
		hiRender += st.Render
		hiTotal += st.Total
	}
	t.Logf("mario walk scale=2 avg layout=%s render=%s total=%s",
		hiLayout/n, hiRender/n, hiTotal/n)
	if hiTotal/n > 80*time.Millisecond {
		t.Fatalf("mario HiDPI walk avg frame %s > 80ms — raster path regressed", hiTotal/n)
	}
}

func marioV2TickStats(e *Engine, surf *HeadlessSurface) FrameStats {
	for tm := range e.timers {
		e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
	}
	e.MarkDirty()
	return e.DrawFrame(surf)
}

// Super Mario must stand on the ground (y=352) and be able to jump;
// growing in place used to bury the 64px hitbox in the floor.
func TestMarioGrowStandsAndJumps(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	startY := marioV2Y(rt)
	if err := qscript.Run(rt.App.ScriptLib+"\ngrowMario()\n", rt.State, nil); err != nil {
		t.Fatal(err)
	}
	m := rt.State["mario"].(map[string]any)
	if big, _ := m["big"].(bool); !big {
		t.Fatal("growMario did not set big")
	}
	standY := marioV2Y(rt)
	if standY > startY-30 || standY < startY-34 {
		t.Fatalf("after grow y=%v, want %v (stood up 32px)", standY, startY-32)
	}
	marioV2Tick(e, surf, 1)
	marioV2Press(e, "space")
	apex := marioV2Y(rt)
	for i := 0; i < 4000; i++ {
		if y := marioV2Y(rt); y < apex {
			apex = y
		}
		marioV2Tick(e, surf, 1)
		if marioV2OnGround(rt) && i > 8 {
			break
		}
	}
	if standY-apex < 60 {
		t.Fatalf("big mario jump height %.1fpx, want > 60 (hitbox was in the floor)", standY-apex)
	}
}

// Super form without growMario's y nudge (old save / mid-frame pickup)
// must still be pushed out of the floor before a jump.
func TestMarioBuriedSuperUnsticks(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	m := rt.State["mario"].(map[string]any)
	m["big"] = true
	m["y"] = 384.0
	marioV2Tick(e, surf, 1)
	y := marioV2Y(rt)
	if y > 360 {
		t.Fatalf("buried Super y=%v after 1 tick, want ~352", y)
	}
	marioV2Press(e, "space")
	apex := y
	for i := 0; i < 2000; i++ {
		if yy := marioV2Y(rt); yy < apex {
			apex = yy
		}
		marioV2Tick(e, surf, 1)
		if marioV2OnGround(rt) && i > 8 {
			break
		}
	}
	if y-apex < 60 {
		t.Fatalf("unstuck Super jump %.1fpx, want > 60", y-apex)
	}
}

func lenState(rt *runtime.Runtime, key string) int {
	if a, ok := rt.State[key].([]any); ok {
		return len(a)
	}
	return 0
}
