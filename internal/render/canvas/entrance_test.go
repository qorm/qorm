package canvas

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func entranceFixture(effect string) (*model.Node, *runtime.Runtime, *Interaction, time.Time) {
	n := &model.Node{Type: "card", ID: "c", Props: map[string]any{"animation": effect, "duration": 400.0}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}})
	start := time.Now()
	return n, rt, &Interaction{}, start
}

func TestEntranceFadeupLifecycle(t *testing.T) {
	n, rt, inter, start := entranceFixture("fadeup")

	ep := entranceFor(n, 0, rt, inter, start)
	if ep.opacity != 0 || !ep.running {
		t.Errorf("t=0: opacity=%v running=%v, want 0/true", ep.opacity, ep.running)
	}
	ep = entranceFor(n, 0, rt, inter, start.Add(200*time.Millisecond))
	if !ep.running || ep.opacity <= 0 || ep.opacity >= 1 || ep.dy <= 0 {
		t.Errorf("t=mid: %+v, want 0<opacity<1, dy>0, running", ep)
	}
	ep = entranceFor(n, 0, rt, inter, start.Add(500*time.Millisecond))
	if ep.opacity != 1 || ep.running || ep.dy != 0 {
		t.Errorf("t=end: %+v, want settled (opacity 1, dy 0, not running)", ep)
	}
}

func TestEntranceMountAndSceneSwitchReplay(t *testing.T) {
	n, rt, inter, start := entranceFixture("fade")

	ep := entranceFor(n, 0, rt, inter, start)
	first := ep
	ep = entranceFor(n, 0, rt, inter, start.Add(500*time.Millisecond))
	if ep.running {
		t.Fatal("same mount must not replay after settling")
	}
	// A scene switch resets Interaction — the same node mounting again
	// replays from zero.
	inter2 := &Interaction{}
	ep2 := entranceFor(n, 0, rt, inter2, start.Add(600*time.Millisecond))
	if !ep2.running || ep2.opacity != first.opacity {
		t.Errorf("remount after scene switch did not replay: %+v", ep2)
	}
}

func TestEntranceDelayAndRepeat(t *testing.T) {
	n := &model.Node{Type: "card", ID: "c", Props: map[string]any{"animation": "fade", "duration": 100.0, "delay": 1000.0}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}})
	inter := &Interaction{}
	start := time.Now()

	if ep := entranceFor(n, 0, rt, inter, start.Add(500*time.Millisecond)); ep.opacity != 0 || !ep.running {
		t.Errorf("in delay window: %+v, want initial opacity, running", ep)
	}

	n2 := &model.Node{Type: "card", ID: "c", Props: map[string]any{"animation": "pulse", "duration": 100.0, "repeat": "infinite"}}
	if ep := entranceFor(n2, 0, rt, inter, start.Add(1050*time.Millisecond)); !ep.running {
		t.Error("infinite repeat must keep running past one cycle")
	}
}

func TestEntranceListInstancesAreIndependent(t *testing.T) {
	n, rt, inter, start := entranceFixture("fade")
	a := entranceFor(n, 0, rt, inter, start)
	b := entranceFor(n, 1, rt, inter, start.Add(500*time.Millisecond))
	// Instance 0 settled at t+500; instance 1 starts fresh at t+500.
	_ = a
	if !b.running || b.opacity != 0 {
		t.Errorf("instance 1 did not get its own clock: %+v", b)
	}
}

func TestEntranceDrivesFrameLoopAndSettles(t *testing.T) {
	// Scene level: a mounted card with a short entrance keeps the engine
	// animating, then settles to full opacity.
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "card", ID: "c", Props: map[string]any{"animation": "fadeup", "duration": 50.0},
			Style: map[string]any{"width": 100.0, "height": 60.0, "background": "#FF0000"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(200, 200))

	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("a mounting entrance must keep the frame loop alive")
	}
	time.Sleep(120 * time.Millisecond)
	e.DrawFrame(surf)
	if e.Animating() {
		t.Error("entrance did not settle after its duration")
	}
	// Settled card paints fully opaque red at its center.
	c := surf.Frame().RGBAAt(50, 30)
	if c.R != 255 || c.A != 255 {
		t.Errorf("settled center pixel = %v, want opaque red", c)
	}
}
