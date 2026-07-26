package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestPlacesExampleDemonstratesScrollSurfaces is the end-to-end gate for the
// three scroll/overlay capabilities the `places` example exists to demonstrate:
// a large title that collapses on scroll, a frosted panel, and a draggable
// bottom sheet. It loads the app the way `qorm run` does, so a regression in
// any of the three shows up as a failing example rather than as a silent visual
// change nobody notices.
func TestPlacesExampleDemonstratesScrollSurfaces(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "places")
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.Diagnostics) != 0 {
		t.Errorf("the example must load with zero diagnostics: %v", app.Diagnostics)
	}
	rt := qrt.New(app)

	closed := render.RenderScene(rt, "main")
	for _, want := range []string{
		// collapsing large title
		`class="qorm-lt"`, `data-qorm-largetitle`, `class="qorm-lt-mini"`,
		`class="qorm-lt-big"`, "position:sticky;top:0;z-index:20;",
		// frosted glass
		"--qorm-bdb:18px;", "--qorm-bdt:color-mix(",
	} {
		if !strings.Contains(closed.HTML, want) {
			t.Errorf("places/main lacks %q", want)
		}
	}
	if strings.Contains(closed.HTML, "qorm-dsheet") {
		t.Error("the sheet must stay closed until state.detail is set")
	}
	if len(closed.Unknown) != 0 {
		t.Errorf("unrecognised widget types: %v", closed.Unknown)
	}

	// Opening the sheet is an ordinary action dispatch, not a client-only trick.
	rt.Dispatch("openDetail", nil)
	open := render.RenderScene(rt, "main")
	for _, want := range []string{
		`class="qorm-dsheet"`, `data-qorm-sheet="detail"`,
		`data-snaps="30,60,92"`, `data-snap="1"`, "height:60%;",
		`class="qorm-dsheet-grab"`, `class="qorm-dsheet-body"`,
	} {
		if !strings.Contains(open.HTML, want) {
			t.Errorf("the open sheet lacks %q", want)
		}
	}

	// onSnap registers one handler per stop; dispatching the middle one must
	// move state exactly as a real drag-and-settle would.
	var snapH []render.Handler
	for _, h := range open.Handlers {
		if h.Name == "setSnap" {
			snapH = append(snapH, h)
		}
	}
	if len(snapH) != 3 {
		t.Fatalf("expected one setSnap handler per stop, got %d", len(snapH))
	}
	rt.Dispatch(snapH[2].Name, map[string]any{"snap": snapH[2].Args["snap"]})
	if got := rt.State["snap"]; got != "2" && got != float64(2) {
		t.Errorf("settling on the top stop should write state.snap=2, got %#v", got)
	}
}
