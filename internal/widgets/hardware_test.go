package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/capability"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
)

func hwRuntime() *runtime.Runtime {
	return runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}})
}

func findCap(stem string) capability.Cap {
	for _, c := range capability.All {
		if c.Stem == stem {
			return c
		}
	}
	return capability.Cap{Stem: stem}
}

func TestHardwarePrimaryOpPrefersReads(t *testing.T) {
	if op := (Hardware{findCap("clipboard")}).primaryOp(); op != "clipboardGet" {
		t.Errorf("clipboard primary = %q, want clipboardGet (not the *Set write)", op)
	}
	if op := (Hardware{findCap("volume")}).primaryOp(); op != "volumeGet" {
		t.Errorf("volume primary = %q, want volumeGet", op)
	}
	if op := (Hardware{findCap("battery")}).primaryOp(); op != "battery" {
		t.Errorf("battery primary = %q, want battery", op)
	}
	if op := (Hardware{findCap("bluetooth")}).primaryOp(); op != "" {
		t.Errorf("bluetooth (slow stem) primary = %q, want none", op)
	}
}

func TestHardwarePressInvokesAndSyncs(t *testing.T) {
	rt := hwRuntime()
	var gotOp string
	canvas.SetNativeInvoker(func(op string, data map[string]any, cb func(string, any)) {
		gotOp = op
		cb("qormOnNetwork", `{"online":true,"type":"desktop"}`)
	})
	defer canvas.SetNativeInvoker(nil)

	n := &model.Node{Type: "network", ID: "net", Value: "{{state.net}}"}
	hw := Hardware{findCap("network")}
	if !hw.HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerPress, X: 10, Y: 40}, &canvas.Interaction{}, image.Rect(0, 0, 240, 62)) {
		t.Fatal("press on a fast capability was not consumed")
	}
	if gotOp != "networkStatus" {
		t.Errorf("invoked op = %q, want networkStatus", gotOp)
	}
	if v := rt.State["net"]; v != "Online (desktop)" {
		t.Errorf("bound state = %v, want formatted result", v)
	}
	if v := lastResult.Load(n); v != "Online (desktop)" {
		t.Errorf("lastResult = %v", v)
	}
}

func TestHardwareSlowStemAndNoBridgeDegrade(t *testing.T) {
	rt := hwRuntime()
	canvas.SetNativeInvoker(nil)
	n := &model.Node{Type: "bluetooth", ID: "bt"}
	if (Hardware{findCap("bluetooth")}).HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerPress}, &canvas.Interaction{}, image.Rectangle{}) {
		t.Error("slow stem must not invoke")
	}
	nn := &model.Node{Type: "network", ID: "net"}
	if (Hardware{findCap("network")}).HandlePointer(nn, rt, canvas.PointerInput{Type: canvas.PointerPress}, &canvas.Interaction{}, image.Rectangle{}) {
		t.Error("no bridge installed: must not invoke")
	}
	if out := (Hardware{findCap("network")}).outputText(nn, rt); out != "native bridge unavailable on this host" {
		t.Errorf("output without bridge = %q", out)
	}
}

func TestHardwareSpeakArgsFromProps(t *testing.T) {
	n := &model.Node{Type: "tts", ID: "sp", Props: map[string]any{"text": "hello world"}}
	hw := Hardware{findCap("tts")}
	var args map[string]any
	for _, b := range hw.buttons() {
		if b.op == "speak" {
			args = b.args(n, hwRuntime())
		}
	}
	if args["text"] != "hello world" {
		t.Errorf("speak args = %v", args)
	}
}

// Stretched hardware cards keep their action row hittable: the row band is
// derived from the (stretch-invariant) measured height, not the width.
func TestHardwareStretchedCardButtonsHit(t *testing.T) {
	rt := hwRuntime()
	var ops []string
	canvas.SetNativeInvoker(func(op string, data map[string]any, cb func(string, any)) {
		ops = append(ops, op)
		cb("qormOnVolume", "55")
	})
	defer canvas.SetNativeInvoker(nil)

	hw := Hardware{findCap("volume")}
	n := &model.Node{Type: "volume", ID: "vol"}
	// A card stretched from 240 to 436 (flex full-width): the action row sits
	// at y 28..62 (scale 1) — the whole row must hit, including its top edge.
	frame := image.Rect(0, 0, 436, 62)
	for _, y := range []float64{29, 45, 60} {
		if !hw.HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerPress, X: 100, Y: y}, &canvas.Interaction{}, frame) {
			t.Errorf("stretched card: press at y=%v missed the action row", y)
		}
	}
	if len(ops) == 0 {
		t.Fatal("no op invoked on a stretched card")
	}
	// Above the row: not the widget's business (lets the card host it).
	if hw.HandlePointer(n, rt, canvas.PointerInput{Type: canvas.PointerPress, X: 100, Y: 10}, &canvas.Interaction{}, frame) {
		t.Error("press above the action row must not be consumed")
	}
}
