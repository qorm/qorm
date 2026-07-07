package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func TestScaffoldedAppRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	if code := cmdNew([]string{dir, "--name", "My App"}); code != 0 {
		t.Fatalf("qorm new exited %d", code)
	}

	// The scaffold loads and renders a runnable app.
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load scaffold: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	if !strings.Contains(html, "My App") {
		t.Error("scaffold should render the app name")
	}
	if !strings.Contains(html, "<button") {
		t.Error("scaffold should render a button")
	}

	// Its action works: pressing the button increments count.
	btn := findChild(app.EntryRoot(), "tap")
	if btn == nil || btn.OnPress == nil {
		t.Fatal("scaffold missing tap button with onPress")
	}
	rt.Dispatch(btn.OnPress.Name, rt.EvalArgs(btn.OnPress.Args))
	if rt.State["count"] != float64(1) {
		t.Errorf("tap should increment count to 1, got %v", rt.State["count"])
	}

	// Refuses to scaffold into a non-empty directory.
	if code := cmdNew([]string{dir}); code == 0 {
		t.Error("qorm new into a non-empty dir should fail")
	}
}

func findChild(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if g := findChild(c, id); g != nil {
			return g
		}
	}
	return nil
}
