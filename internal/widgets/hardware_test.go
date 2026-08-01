package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
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
