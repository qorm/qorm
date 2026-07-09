package integration

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestResponsiveExampleSwapsLayout is the P1.1 acceptance gate: the responsive
// demo renders a column at a phone viewport (375px) and a row at a desktop
// viewport (1440px), driven only by the scene JSON's `when` node.
func TestResponsiveExampleSwapsLayout(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "responsive"))
	if err != nil {
		t.Fatalf("load responsive example: %v", err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("responsive example diagnostic: %s", d)
	}
	rt := qrt.New(app)

	rt.Viewport = qrt.Viewport{W: 375, H: 667}
	narrow := render.Render(rt).HTML
	if !strings.Contains(narrow, `id="cards-narrow"`) || strings.Contains(narrow, `id="cards-wide"`) {
		t.Errorf("375px viewport must render the column (else) branch only:\n%s", narrow)
	}
	if !strings.Contains(narrow, "viewport: 375 x 667 (portrait)") {
		t.Errorf("viewport readout not interpolated at 375x667:\n%s", narrow)
	}

	rt.Viewport = qrt.Viewport{W: 1440, H: 900}
	wide := render.Render(rt).HTML
	if !strings.Contains(wide, `id="cards-wide"`) || strings.Contains(wide, `id="cards-narrow"`) {
		t.Errorf("1440px viewport must render the row (then) branch only:\n%s", wide)
	}
	if !strings.Contains(wide, "viewport: 1440 x 900 (landscape)") {
		t.Errorf("viewport readout not interpolated at 1440x900:\n%s", wide)
	}

	// Server first frame: unknown viewport falls back to the else branch.
	rt.Viewport = qrt.Viewport{}
	first := render.Render(rt).HTML
	if !strings.Contains(first, `id="cards-narrow"`) {
		t.Errorf("unknown viewport must render the else branch:\n%s", first)
	}
}
