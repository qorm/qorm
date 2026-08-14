package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

// Media sources are data-driven in a polymorphic feed: one `image` template
// inside a list has to resolve {{item.src}} per row. src/alt were read raw,
// so such a row emitted the literal binding as its URL and loaded nothing.
func TestMediaSourcesInterpolate(t *testing.T) {
	app := &model.App{
		Entry: "main",
		GlobalState: model.GlobalState{
			Initial: map[string]any{
				"photo": "https://example.com/a.png",
				"clip":  "https://example.com/a.mp4",
				"who":   "Ada Lovelace",
			},
		},
		Scenes: map[string]*model.Node{"main": {Type: "column", Children: []*model.Node{
			{Type: "image", ID: "img", Props: map[string]any{
				"src": "{{state.photo}}", "alt": "{{state.who}}",
			}},
			{Type: "avatar", ID: "av", Props: map[string]any{"src": "{{state.photo}}"}},
			{Type: "video", ID: "vid", Props: map[string]any{"src": "{{state.clip}}"}},
		}}},
	}
	html := Render(runtime.New(app)).HTML

	for _, want := range []string{
		`id="img" src="https://example.com/a.png"`,
		`alt="Ada Lovelace"`,
		`id="av" src="https://example.com/a.png"`,
		`id="vid" src="https://example.com/a.mp4"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q\ngot: %s", want, html)
		}
	}
	if strings.Contains(html, "{{") {
		t.Errorf("an unresolved binding leaked into the markup:\n%s", html)
	}
}

// A binding that resolves to nothing must not leave the raw expression as a
// URL: the avatar falls back to its initials path, the image emits an empty
// src rather than requesting "{{item.missing}}".
func TestMediaSourceEmptyBindingDoesNotLeak(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", Children: []*model.Node{
			{Type: "image", ID: "img", Props: map[string]any{"src": "{{state.nope}}"}},
			{Type: "avatar", ID: "av", Props: map[string]any{
				"src": "{{state.nope}}", "initials": "AL",
			}},
		}}},
	}
	html := Render(runtime.New(app)).HTML

	if strings.Contains(html, "{{") {
		t.Errorf("an unresolved binding leaked into the markup:\n%s", html)
	}
	if !strings.Contains(html, `id="img" src=""`) {
		t.Errorf("image should emit an empty src, got: %s", html)
	}
	if !strings.Contains(html, "AL") {
		t.Errorf("avatar should fall back to initials, got: %s", html)
	}
}
