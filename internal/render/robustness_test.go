package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// renderCompState renders an app with the given components, a root holding the
// component instances, and initial state (so bound list/grid data resolves).
func renderCompState(t *testing.T, components map[string]*model.Node, root *model.Node, state map[string]any) Result {
	t.Helper()
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		Components:  components,
		GlobalState: model.GlobalState{Initial: state},
	}
	return Render(runtime.New(app))
}

// TestMalformedDestinationItems feeds the destination widgets (bottomnav /
// navigationrail / navigationdrawer) an items array polluted with non-map
// elements (a string, a number, a nil) alongside one valid item. The renderer
// must skip the odd shapes, render the valid one, and never report unknown.
func TestMalformedDestinationItems(t *testing.T) {
	items := []any{
		"just-a-string",
		float64(42),
		nil,
		map[string]any{"value": "ok", "label": "Valid", "icon": "home"},
	}
	for _, typ := range []string{"bottomnav", "navigationrail", "navigationdrawer"} {
		t.Run(typ, func(t *testing.T) {
			res := renderWidgetState(t,
				&model.Node{Type: typ, ID: "d", Value: "{{ state.tab }}", Props: map[string]any{"items": items}},
				map[string]any{"tab": "ok"})
			if !strings.Contains(res.HTML, "Valid") {
				t.Errorf("%s should render the valid item:\n%s", typ, res.HTML)
			}
			if len(res.Unknown) != 0 {
				t.Errorf("%s with odd items should not be unknown: %v", typ, res.Unknown)
			}
		})
	}
}

// TestHandlerArgMerge verifies that widgets which synthesize a per-item arg
// (value / column) PRESERVE any extra authored onChange args alongside it, across
// every widget that does this merge.
func TestHandlerArgMerge(t *testing.T) {
	t.Run("navigationrail", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "navigationrail", ID: "nr", Value: "{{ state.t }}",
				Props:    map[string]any{"items": []any{map[string]any{"value": "h", "label": "H"}}},
				OnChange: &model.Invoke{Name: "go", Args: map[string]string{"src": "rail"}}},
			map[string]any{"t": "h"})
		if len(res.Handlers) != 1 || res.Handlers[0].Args["src"] != "rail" || res.Handlers[0].Args["value"] != "h" {
			t.Errorf("navigationrail should merge authored args + value: %+v", res.Handlers)
		}
	})
	t.Run("datepicker-wheels", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "datepicker", ID: "dp", Value: "2026-07-15",
			OnChange: &model.Invoke{Name: "set", Args: map[string]string{"extra": "e"}}})
		if len(res.Handlers) == 0 {
			t.Fatal("datepicker should register wheel handlers")
		}
		for _, h := range res.Handlers {
			if h.Args["extra"] != "e" || h.Args["value"] == "" {
				t.Errorf("each wheel handler should carry extra+value: %+v", h)
			}
		}
	})
	t.Run("picker", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "picker", ID: "pk", Value: "S",
			Props:    map[string]any{"options": []any{map[string]any{"value": "S", "label": "Small"}}},
			OnChange: &model.Invoke{Name: "set", Args: map[string]string{"extra": "e"}}})
		if len(res.Handlers) != 1 || res.Handlers[0].Args["extra"] != "e" || res.Handlers[0].Args["value"] != "S" {
			t.Errorf("picker should merge extra+value: %+v", res.Handlers)
		}
	})
	t.Run("table-header", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "table", ID: "tb", Props: map[string]any{
				"columns": []any{map[string]any{"value": "n", "label": "N"}},
				"data":    []any{map[string]any{"n": "x"}}},
				OnChange: &model.Invoke{Name: "sort", Args: map[string]string{"view": "grid"}}},
			nil)
		if len(res.Handlers) != 1 || res.Handlers[0].Args["view"] != "grid" || res.Handlers[0].Args["column"] != "n" {
			t.Errorf("table header should merge authored args + column: %+v", res.Handlers)
		}
	})
	t.Run("dropdown-option", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "dropdownbutton", ID: "dd", Value: "{{ state.c }}",
				Props:    map[string]any{"options": []any{map[string]any{"value": "a", "label": "A"}}},
				OnChange: &model.Invoke{Name: "set", Args: map[string]string{"src": "dd"}}},
			map[string]any{"c": "a"})
		if len(res.Handlers) != 1 || res.Handlers[0].Args["src"] != "dd" || res.Handlers[0].Args["value"] != "a" {
			t.Errorf("dropdown option should merge authored args + value: %+v", res.Handlers)
		}
	})
}

// TestEdgeCasesNoCrash probes numeric/shape edge cases that could produce NaN, a
// division by zero, or a panic, asserting graceful output.
func TestEdgeCasesNoCrash(t *testing.T) {
	t.Run("rangeSlider-zero-span", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "rangeslider", ID: "rs", Props: map[string]any{"min": float64(5), "max": float64(5),
				"low": "{{ state.lo }}", "high": "{{ state.hi }}"}},
			map[string]any{"lo": float64(5), "hi": float64(5)})
		if strings.Contains(res.HTML, "NaN") {
			t.Errorf("rangeSlider with min==max must not emit NaN:\n%s", res.HTML)
		}
	})

	t.Run("chart-bars-all-zero", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch", Props: map[string]any{"data": []any{float64(0), float64(0)}}})
		if strings.Contains(res.HTML, "NaN") || !strings.Contains(res.HTML, "<svg") {
			t.Errorf("all-zero bar chart must not emit NaN:\n%s", res.HTML)
		}
	})
	t.Run("chart-line-all-equal", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch2", Props: map[string]any{"data": []any{float64(7), float64(7)}, "chartType": "line"}})
		if strings.Contains(res.HTML, "NaN") || !strings.Contains(res.HTML, "<polyline") {
			t.Errorf("flat line chart must not emit NaN:\n%s", res.HTML)
		}
	})
	t.Run("chart-line-single-point", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch3", Props: map[string]any{"data": []any{float64(7)}, "chartType": "line"}})
		if strings.Contains(res.HTML, "<polyline") {
			t.Errorf("a single point cannot form a line:\n%s", res.HTML)
		}
	})

	t.Run("numprop-empty-and-wrongtype", func(t *testing.T) {
		// An empty-string or wrong-typed transform prop evaluates to no transform.
		res := renderWidget(t, &model.Node{Type: "transform", ID: "tf", Props: map[string]any{"rotate": ""}, Children: textKids("x")})
		if strings.Contains(res.HTML, "transform:") {
			t.Errorf("empty rotate should yield no transform:\n%s", res.HTML)
		}
		res = renderWidget(t, &model.Node{Type: "transform", ID: "tf2", Props: map[string]any{"rotate": true}, Children: textKids("x")})
		if strings.Contains(res.HTML, "transform:") {
			t.Errorf("wrong-typed rotate should yield no transform:\n%s", res.HTML)
		}
	})

	t.Run("pagination-disabled-ends", func(t *testing.T) {
		// page 1 of 3: prev is disabled; page 3: next is disabled.
		res := renderWidgetState(t,
			&model.Node{Type: "pagination", ID: "pg", Props: map[string]any{"page": "{{ state.p }}", "total": float64(3)}, OnPress: &model.Invoke{Name: "g"}},
			map[string]any{"p": float64(1)})
		if !strings.Contains(res.HTML, "opacity:.5;cursor:default;") {
			t.Errorf("page 1 should disable the prev button:\n%s", res.HTML)
		}
		res = renderWidgetState(t,
			&model.Node{Type: "pagination", ID: "pg2", Props: map[string]any{"page": "{{ state.p }}", "total": float64(3)}, OnPress: &model.Invoke{Name: "g"}},
			map[string]any{"p": float64(3)})
		if !strings.Contains(res.HTML, "opacity:.5;cursor:default;") {
			t.Errorf("last page should disable the next button:\n%s", res.HTML)
		}
	})
}

// TestClosedOverlaysAndOddShapes covers overlay closed-state early returns and
// the skip-odd-element branches of the action/item parsers.
func TestClosedOverlaysAndOddShapes(t *testing.T) {
	t.Run("alertdialog-closed", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alertdialog", ID: "ad", Props: map[string]any{"open": "false", "title": "T"}})
		if strings.Contains(res.HTML, "T") {
			t.Errorf("closed alertdialog should render nothing:\n%s", res.HTML)
		}
	})
	t.Run("actionsheet-closed", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "actionsheet", ID: "as", Props: map[string]any{"open": "0", "title": "T"}})
		if strings.Contains(res.HTML, "qorm-sheet") {
			t.Errorf("closed actionsheet should render nothing:\n%s", res.HTML)
		}
	})
	t.Run("alertdialog-actions-skip-odd", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alertdialog", ID: "ad2", Props: map[string]any{"open": "true",
			"actions": []any{"not-a-map", map[string]any{"label": "Real"}}}})
		if !strings.Contains(res.HTML, "Real") {
			t.Errorf("alertdialog should render the valid action and skip the odd one:\n%s", res.HTML)
		}
	})
	t.Run("actionsheet-action-and-cancel-onpress", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "actionsheet", ID: "as2", Props: map[string]any{"open": "true",
			"actions": []any{map[string]any{"label": "A", "onPress": map[string]any{"name": "doA"}}},
			"cancel":  []any{map[string]any{"label": "C", "onPress": map[string]any{"name": "doC"}}}}})
		names := map[string]bool{}
		for _, h := range res.Handlers {
			names[h.Name] = true
		}
		if !names["doA"] || !names["doC"] {
			t.Errorf("actionsheet should wire both action and cancel onPress: %+v", res.Handlers)
		}
	})
	t.Run("swipeactions-skip-odd-action", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "swipeactions", ID: "swa", Props: map[string]any{"actions": []any{
			"not-a-map",
			map[string]any{"label": "Del", "name": "del"},
		}}, Children: textKids("row")})
		if !strings.Contains(res.HTML, "Del") {
			t.Errorf("swipeactions should render the valid action and skip the odd one:\n%s", res.HTML)
		}
	})
	t.Run("menu-children-below-items", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "menu", ID: "mn", Label: "M",
			Props:    map[string]any{"items": []any{map[string]any{"label": "I"}}},
			Children: []*model.Node{{Type: "text", ID: "extra", Text: "CHILD-ROW"}}})
		if !strings.Contains(res.HTML, "CHILD-ROW") {
			t.Errorf("menu should still render children below its items:\n%s", res.HTML)
		}
	})
	t.Run("contextmenu-actions-separator", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "contextmenu", ID: "cx",
			Props:    map[string]any{"actions": []any{map[string]any{"label": "One"}, map[string]any{"label": "Two", "style": "destructive"}}},
			Children: textKids("t")})
		if !strings.Contains(res.HTML, "One") || !strings.Contains(res.HTML, "Two") || !strings.Contains(res.HTML, "var(--danger)") {
			t.Errorf("contextmenu actions should render with a separator + destructive color:\n%s", res.HTML)
		}
	})
	t.Run("listtile-rich-children", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "listtile", ID: "lt", Label: "T",
			Children: []*model.Node{{Type: "button", ID: "act", Label: "Act"}}})
		if !strings.Contains(res.HTML, "Act") {
			t.Errorf("listtile should render rich trailing children:\n%s", res.HTML)
		}
	})
	t.Run("listsection-separator-between-children", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "listsection", ID: "ls", Children: textKids("r1", "r2")})
		if !strings.Contains(res.HTML, "height:.5px;background:var(--sep);margin-left:16px;") {
			t.Errorf("listsection should separate multiple children with an inset hairline:\n%s", res.HTML)
		}
	})
	t.Run("checkboxlisttile-bound-checked", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "checkboxlisttile", ID: "clt", Label: "C", Value: "{{ state.on }}"},
			map[string]any{"on": true})
		if !strings.Contains(res.HTML, " checked") {
			t.Errorf("checkboxlisttile bound to a truthy value should be checked:\n%s", res.HTML)
		}
	})
	t.Run("textformfield-empty-footer", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tf", Value: "abc"})
		// no error and no helper -> an empty placeholder span keeps the counter row balanced
		if !strings.Contains(res.HTML, "<span></span>") {
			t.Errorf("textformfield without error/helper should emit an empty footer span:\n%s", res.HTML)
		}
	})
	t.Run("a11y-tooltip", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "t", Text: "x", Props: map[string]any{"tooltip": "hint"}})
		if !strings.Contains(res.HTML, `data-tooltip="hint"`) {
			t.Errorf("tooltip prop should render a data-tooltip attribute:\n%s", res.HTML)
		}
	})
}

// TestComponentScopeSurvivesListMerge guards that a component's prop scope is
// carried into a list/gridview renderItem (the scope-merge loop preserves
// non-`item` keys), so {{prop.x}} still resolves inside a repeated template.
func TestComponentScopeSurvivesListMerge(t *testing.T) {
	state := map[string]any{"items": []any{map[string]any{"t": "A"}, map[string]any{"t": "B"}}}

	t.Run("list", func(t *testing.T) {
		comps := map[string]*model.Node{
			"Repeat": {Type: "list", ID: "L", Data: "{{ state.items }}",
				Template: &model.Node{Type: "text", ID: "r", Text: "{{ item.t }}:{{ prop.suffix }}"}},
		}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
			{Type: "Repeat", ID: "rp", Props: map[string]any{"suffix": "S"}},
		}}
		res := renderCompState(t, comps, root, state)
		if !strings.Contains(res.HTML, "A:S") || !strings.Contains(res.HTML, "B:S") {
			t.Errorf("prop scope should survive into each list item:\n%s", res.HTML)
		}
	})
	t.Run("gridview", func(t *testing.T) {
		comps := map[string]*model.Node{
			"Grid": {Type: "gridview", ID: "G", Data: "{{ state.items }}",
				Props:    map[string]any{"crossAxisCount": float64(2)},
				Template: &model.Node{Type: "text", ID: "c", Text: "{{ item.t }}{{ prop.tag }}"}},
		}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
			{Type: "Grid", ID: "gd", Props: map[string]any{"tag": "!"}},
		}}
		res := renderCompState(t, comps, root, state)
		if !strings.Contains(res.HTML, "A!") || !strings.Contains(res.HTML, "B!") {
			t.Errorf("prop scope should survive into each grid cell:\n%s", res.HTML)
		}
	})
}
