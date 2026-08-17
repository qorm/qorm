package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func TestBreakpointWhenBranch(t *testing.T) {
	root := &model.Node{
		Type: "column", ID: "root",
		Children: []*model.Node{
			{
				Type: "when", ID: "sw",
				Condition: "{{ breakpoint.md }}",
				Then:      &model.Node{Type: "text", ID: "wide", Text: "WIDE"},
				Else:      &model.Node{Type: "text", ID: "narrow", Text: "NARROW"},
			},
		},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Viewport = runtime.Viewport{W: 900, H: 600}
	if html := Render(rt).HTML; !strings.Contains(html, "WIDE") {
		t.Fatalf("900px should match md:\n%s", html)
	}
	rt.Viewport = runtime.Viewport{W: 500, H: 800}
	if html := Render(rt).HTML; !strings.Contains(html, "NARROW") {
		t.Fatalf("500px should be below md:\n%s", html)
	}
}

func TestBreakpointCustomManifest(t *testing.T) {
	app := &model.App{
		Entry:            "main",
		BreakpointWidths: map[string]int{"md": 900},
		Scenes: map[string]*model.Node{"main": {
			Type: "text", ID: "t", Text: "{{ breakpoint.md ? 'BIG' : 'SMALL' }}",
		}},
	}
	rt := runtime.New(app)
	rt.Viewport = runtime.Viewport{W: 900, H: 600}
	if html := Render(rt).HTML; !strings.Contains(html, "BIG") {
		t.Fatalf("custom md=900 at 900px:\n%s", html)
	}
}
