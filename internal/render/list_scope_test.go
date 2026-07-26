package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// These tests cover the list/gridview item-template scope: the built-in
// index/first/last bindings, the `as` alias (with its namespaced
// rowIndex/rowFirst/rowLast forms), and the nesting rules — an aliased inner
// list keeps the whole outer scope visible, a default-named inner list
// shadows the outer item/index/first/last exactly as it always shadowed
// `item`.

func textNode(id, text string) *model.Node {
	return &model.Node{Type: "text", ID: id, Text: text}
}

func TestListIndexFirstLast(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.names }}",
		Template: &model.Node{Type: "column", ID: "row", Children: []*model.Node{
			textNode("n", "{{ index }}:{{ item }}"),
			// classic "no separator after the last row"
			{Type: "text", ID: "sep", Text: "---", Props: map[string]any{"if": "{{ !last }}"}},
			// and a header marker only on the first row
			{Type: "text", ID: "hdr", Text: "FIRST", Props: map[string]any{"if": "{{ first }}"}},
		}},
	}, map[string]any{"names": []any{"a", "b", "c"}})

	for _, want := range []string{">0:a<", ">1:b<", ">2:c<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("list should render 0-based index with the item, missing %q:\n%s", want, res.HTML)
		}
	}
	if got := strings.Count(res.HTML, ">---<"); got != 2 {
		t.Errorf("if:{{!last}} separator should render for all but the last item, got %d of 2:\n%s", got, res.HTML)
	}
	if got := strings.Count(res.HTML, ">FIRST<"); got != 1 {
		t.Errorf("if:{{first}} should render exactly once, got %d:\n%s", got, res.HTML)
	}
	// FIRST must precede the second row's text (i.e. it is on row 0).
	if strings.Index(res.HTML, ">FIRST<") > strings.Index(res.HTML, ">1:b<") {
		t.Errorf("first flag must be true on row 0 only:\n%s", res.HTML)
	}
}

func TestListZebraStriping(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.names }}",
		Template: textNode("n", "{{ index % 2 == 0 ? 'even' : 'odd' }}:{{ item }}"),
	}, map[string]any{"names": []any{"a", "b", "c"}})
	for _, want := range []string{">even:a<", ">odd:b<", ">even:c<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("zebra ternary over index should alternate, missing %q:\n%s", want, res.HTML)
		}
	}
}

func TestListAsAlias(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.rows }}", Props: map[string]any{"as": "row"},
		Template: textNode("n", "{{ rowIndex }}={{ row.name }} first={{ rowFirst }} last={{ rowLast }} item=[{{ item.name }}]"),
	}, map[string]any{"rows": []any{
		map[string]any{"name": "alice"},
		map[string]any{"name": "bob"},
	}})
	for _, want := range []string{
		">0=alice first=true last=false item=[]<",
		">1=bob first=false last=true item=[]<",
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("as:\"row\" should bind row/rowIndex/rowFirst/rowLast (and NOT item), missing %q:\n%s", want, res.HTML)
		}
	}
}

// TestListAliasReservedFallsBack: an alias that would shadow a base context
// root (state/t/viewport/route/prop) is ignored — the template still sees the
// default item binding and {{state.x}} keeps working inside it.
func TestListAliasReservedFallsBack(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.names }}", Props: map[string]any{"as": "state"},
		Template: textNode("n", "{{ item }}/{{ state.tag }}"),
	}, map[string]any{"names": []any{"a"}, "tag": "ok"})
	if !strings.Contains(res.HTML, ">a/ok<") {
		t.Errorf("reserved alias must fall back to item and keep state visible:\n%s", res.HTML)
	}
}

// TestNestedListAliasKeepsOuterScope is the grouped-list shape: an outer
// section list (as: "section") whose template renders an inner row list — the
// inner template reads the outer alias, the outer index, and its own
// item/index at the same time.
func TestNestedListAliasKeepsOuterScope(t *testing.T) {
	inner := &model.Node{
		Type: "list", ID: "rows", Data: "{{ section.rows }}",
		Template: textNode("r", "{{ sectionIndex }}.{{ section.title }}/{{ item }}({{ index }})"),
	}
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "sections", Data: "{{ state.sections }}", Props: map[string]any{"as": "section"},
		Template: &model.Node{Type: "column", ID: "sec", Children: []*model.Node{
			textNode("h", "# {{ section.title }}"),
			inner,
		}},
	}, map[string]any{"sections": []any{
		map[string]any{"title": "A", "rows": []any{"a1", "a2"}},
		map[string]any{"title": "B", "rows": []any{"b1"}},
	}})
	for _, want := range []string{
		"># A<", "># B<",
		">0.A/a1(0)<", ">0.A/a2(1)<", ">1.B/b1(0)<",
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("aliased outer scope must stay visible in the inner template, missing %q:\n%s", want, res.HTML)
		}
	}
}

// TestNestedListDefaultShadows locks in the status-quo rule: without `as`, an
// inner list rebinds item/index/first/last and the outer values are hidden —
// no implicit outer-item magic.
func TestNestedListDefaultShadows(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "outer", Data: "{{ state.groups }}",
		Template: &model.Node{
			Type: "list", ID: "inner", Data: "{{ item.rows }}",
			Template: textNode("r", "{{ item }}@{{ index }}"),
		},
	}, map[string]any{"groups": []any{
		map[string]any{"rows": []any{"x", "y"}},
	}})
	for _, want := range []string{">x@0<", ">y@1<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("default nested list must rebind item/index to the inner loop, missing %q:\n%s", want, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "map[") {
		t.Errorf("outer item (a map) leaked into the inner template output:\n%s", res.HTML)
	}
}

func TestGridViewScopeParity(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "gridview", ID: "g", Data: "{{ state.cells }}",
		Props:    map[string]any{"as": "cell", "crossAxisCount": float64(2)},
		Template: textNode("c", "{{ cellIndex }}:{{ cell }} {{ cellFirst }}/{{ cellLast }}"),
	}, map[string]any{"cells": []any{"p", "q"}})
	for _, want := range []string{">0:p true/false<", ">1:q false/true<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("gridview must inject the same as/index/first/last scope as list, missing %q:\n%s", want, res.HTML)
		}
	}
}

// TestListHandlerScopeCapturesIndex: handlers registered inside a template
// capture the item scope, so an onPress arg can reference {{ index }} (and
// alias forms) and the dispatch-time re-evaluation resolves it per row.
func TestListHandlerScopeCapturesIndex(t *testing.T) {
	res := renderWidgetState(t, &model.Node{
		Type: "list", ID: "l", Data: "{{ state.names }}",
		Template: &model.Node{Type: "button", ID: "b", Text: "{{ item }}",
			OnPress: &model.Invoke{Name: "pick", Args: map[string]string{"i": "{{ index }}"}}},
	}, map[string]any{"names": []any{"a", "b"}})
	var got []any
	for _, h := range res.Handlers {
		if h.Name == "pick" {
			got = append(got, h.Scope["index"])
		}
	}
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("handler scope should capture per-row index, got %v", got)
	}
}
