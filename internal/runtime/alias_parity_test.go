package runtime_test

// The `forEach` step's item alias and a list renderItem's item alias MUST
// resolve identically: the JSON shows one `as` key, so it must mean one thing.
//
// The two implementations are separate on purpose — the runtime cannot import
// the renderer, because the renderer imports the runtime — which is exactly the
// kind of duplication that silently drifts. This test is the pin. It lives in
// the EXTERNAL test package (runtime_test) because that is the only way a test
// beside the runtime may import the renderer at all.

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

func TestForEachAliasNamesMatchRenderer(t *testing.T) {
	cases := []string{
		"", "item", "row", "section", "msg", "_x", "x9",
		// Reserved evaluation roots: binding the element over one of them would
		// break every {{state.x}} inside the body/template.
		"state", "t", "viewport", "route", "prop",
		// Shapes the expression language could never reference.
		"my alias", "2rows", "a-b", "a.b", "😀",
	}
	for _, as := range cases {
		wa, wi, wf, wl := render.ListAliasNames(as)
		// Drive the runtime side through a real dispatch: the loop body records
		// the four names it can actually see, so this compares BOUND scope keys
		// against the renderer's answer rather than one helper against another.
		got := forEachBoundNames(t, as)
		want := []string{wa, wi, wf, wl}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("as=%q: runtime bound %v, renderer resolves %v", as, got, want)
				break
			}
		}
	}
}

// forEachBoundNames dispatches a one-element forEach with the given alias and
// reports which of the candidate scope names actually carried the element, the
// index, and the first/last flags.
func forEachBoundNames(t *testing.T, as string) [4]string {
	t.Helper()
	alias, idx, first, last := render.ListAliasNames(as)
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"one": []any{"element"}}},
		Actions: map[string]*model.Action{"run": {ID: "run", Steps: []model.Step{{
			Type: "forEach", In: "{{ state.one }}", As: as,
			Steps: []model.Step{
				{Type: "state.set", Path: "gotItem", Value: "{{ " + alias + " }}"},
				{Type: "state.set", Path: "gotIndex", Value: "{{ " + idx + " }}"},
				{Type: "state.set", Path: "gotFirst", Value: "{{ " + first + " }}"},
				{Type: "state.set", Path: "gotLast", Value: "{{ " + last + " }}"},
			},
		}}}},
	}
	rt := runtime.New(app)
	rt.Dispatch("run", nil)
	out := [4]string{}
	if rt.State["gotItem"] == "element" {
		out[0] = alias
	}
	if rt.State["gotIndex"] == 0.0 {
		out[1] = idx
	}
	if rt.State["gotFirst"] == true {
		out[2] = first
	}
	if rt.State["gotLast"] == true {
		out[3] = last
	}
	return out
}
