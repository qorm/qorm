//go:build !desktop

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func writeFlowFile(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFlowReport(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode report: %v\n%s", err, b)
	}
	return got
}

func flowActions(t *testing.T, report map[string]any) []string {
	t.Helper()
	steps, _ := report["steps"].([]any)
	got := make([]string, 0, len(steps))
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		action, _ := step["action"].(string)
		got = append(got, action)
	}
	return got
}

func TestRunCheckPureCanvasCounterInteractiveFlow(t *testing.T) {
	flow := map[string]any{"steps": []any{
		map[string]any{"name": "real press", "do": map[string]any{"press": "btn_increment"}, "checks": []any{map[string]any{"id": "number", "text": "1"}}},
		map[string]any{"name": "state alias", "do": map[string]any{"state": map[string]any{"path": "count", "value": 5}}, "checks": []any{map[string]any{"id": "number", "text": "5"}}},
		map[string]any{"name": "existing dispatch", "do": map[string]any{"dispatch": "decrement"}, "checks": []any{map[string]any{"id": "number", "text": "4"}}},
	}}
	out := filepath.Join(t.TempDir(), "report.json")
	checks := writeFlowFile(t, flow)
	if code := cmdCheck([]string{counterDir(), "--checks", checks, "--width", "400", "-o", out}); code != 0 {
		t.Fatalf("cmdCheck exited %d", code)
	}
	report := readFlowReport(t, out)
	if ok, _ := report["ok"].(bool); !ok {
		t.Fatalf("counter flow failed: %#v", report)
	}
	want := []string{"press #btn_increment", "set_state count=5", "dispatch decrement"}
	if got := flowActions(t, report); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %q, want %q", got, want)
	}
}

func TestRunCheckPureCanvasCanvasFXOffscreenPressFlow(t *testing.T) {
	flow := map[string]any{"steps": []any{
		map[string]any{"name": "press offscreen", "do": map[string]any{"press": "btn_pseudo_opacity"}, "checks": []any{map[string]any{"id": "pseudo_status", "text": "active: 1"}}},
		map[string]any{"name": "enable", "do": map[string]any{"press": "btn_toggle_disabled"}, "checks": []any{map[string]any{"id": "btn_disabled_probe", "text": "Enabled"}}},
		map[string]any{"name": "press enabled", "do": map[string]any{"press": "btn_disabled_probe"}, "checks": []any{map[string]any{"id": "pseudo_status", "text": "disabled attempts: 1"}}},
		map[string]any{"name": "deterministic wait", "do": map[string]any{"wait": "250ms"}, "checks": []any{map[string]any{"id": "pseudo_state_panel", "visible": true}}},
	}}
	out := filepath.Join(t.TempDir(), "report.json")
	canvasFX := filepath.Join("..", "..", "examples", "canvas-fx")
	if err := runCheck(canvasFX, writeFlowFile(t, flow), out, false, 380, false); err != nil {
		t.Fatal(err)
	}
	report := readFlowReport(t, out)
	if ok, _ := report["ok"].(bool); !ok {
		t.Fatalf("canvas-fx flow failed: %#v", report)
	}
}

func TestEvalCanvasFlowTypeKeyScrollAndWait(t *testing.T) {
	input := &model.Node{Type: "input", ID: "name", Value: "{{state.name}}", Placeholder: "Name"}
	mirror := &model.Node{Type: "text", ID: "mirror", Text: "{{state.name}}"}
	button := &model.Node{Type: "button", ID: "submit", Label: "Submit", OnPress: &model.Invoke{Name: "submit"}}
	children := make([]*model.Node, 12)
	for i := range children {
		children[i] = &model.Node{Type: "text", ID: "line_" + string(rune('a'+i)), Text: "scroll line", Style: map[string]any{"height": 28.0}}
	}
	scroller := &model.Node{Type: "scroll", ID: "scroller", Style: map[string]any{"width": 220.0, "height": 70.0}, Children: []*model.Node{{Type: "column", Children: children}}}
	timer := &model.Node{Type: "timer", ID: "later", Props: map[string]any{"after": 100.0, "onTick": "finish"}}
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{"width": 280.0}, Children: []*model.Node{input, mirror, button, scroller, timer, {Type: "text", ID: "status", Text: "{{state.status}}"}}}
	app := &model.App{Name: "flow fixture", Entry: "main", Scenes: map[string]*model.Node{"main": root}, Actions: map[string]*model.Action{
		"submit": {ID: "submit", Steps: []model.Step{{Type: "state.set", Path: "status", Value: "submitted"}}},
		"finish": {ID: "finish", Steps: []model.Step{{Type: "state.set", Path: "status", Value: "finished"}}},
	}}
	rt := qrt.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["name"] = "old"
	rt.State["status"] = "ready"
	flow := map[string]any{"steps": []any{
		map[string]any{"name": "type", "do": map[string]any{"type": map[string]any{"id": "name", "text": "Ada", "clear": true}}, "checks": []any{map[string]any{"id": "mirror", "text": "Ada"}}},
		map[string]any{"name": "keyboard activate", "do": map[string]any{"key": map[string]any{"id": "submit", "key": "Enter"}}, "checks": []any{map[string]any{"id": "status", "text": "submitted"}}},
		map[string]any{"name": "scroll", "do": map[string]any{"scroll": map[string]any{"id": "scroller", "dy": 50}}, "checks": []any{map[string]any{"id": "scroller", "visible": true}}},
		map[string]any{"name": "wait", "do": map[string]any{"wait": 100}, "checks": []any{map[string]any{"id": "status", "text": "finished"}}},
	}}
	b, _ := json.Marshal(flow)
	reportBytes, err := evalCanvasFlow(rt, 320, 280, true, b)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	if ok, _ := report["ok"].(bool); !ok {
		t.Fatalf("input/key/scroll/wait flow failed:\n%s", reportBytes)
	}
	want := []string{`type #name "Ada"`, "key return", "scroll #scroller dx=0 dy=50", "wait 100ms"}
	if got := flowActions(t, report); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %q, want %q", got, want)
	}
}

func TestEvalCanvasFlowRejectsAmbiguousAndMissingTargets(t *testing.T) {
	app := &model.App{Name: "bad flow", Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}}
	for _, tc := range []struct {
		name string
		do   map[string]any
		want string
	}{
		{"ambiguous", map[string]any{"press": "root", "wait": 1}, "exactly one operation"},
		{"missing", map[string]any{"press": "absent"}, "target #absent not found"},
		{"zero scroll", map[string]any{"scroll": map[string]any{"id": "root"}}, "non-zero dx or dy"},
		{"bad wait", map[string]any{"wait": "tomorrow"}, "milliseconds or a duration string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := qrt.New(app)
			b, _ := json.Marshal(map[string]any{"steps": []any{map[string]any{"do": tc.do}}})
			_, err := evalCanvasFlow(rt, 200, 100, true, b)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}
