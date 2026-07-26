package playcore

import (
	"strings"
	"testing"
)

// counterDocs is a minimal counter app as a raw doc array — the shape the
// playground's qormCompile receives after JSON-decoding the docs array.
func counterDocs() []map[string]any {
	return []map[string]any{
		{
			"type": "app", "id": "counter", "entry": "main",
			"globalState": map[string]any{
				"schema":  map[string]any{"count": "number"},
				"initial": map[string]any{"count": 0},
			},
		},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{
				"type": "column", "id": "root",
				"children": []any{
					map[string]any{"type": "text", "id": "label", "text": "{{state.count}}"},
					map[string]any{"type": "button", "id": "inc", "text": "+", "onPress": "increment"},
				},
			},
		},
		{
			"type": "action", "id": "increment",
			"steps": []any{
				map[string]any{"type": "set", "path": "count", "value": "{{count + 1}}"},
			},
		},
	}
}

// TestCompileDocsCounter guards the clean path: a well-formed counter compiles
// to non-empty HTML with no diagnostics and no unknown widgets, and yields a
// live runtime + handler table the WASM wrapper can install.
func TestCompileDocsCounter(t *testing.T) {
	res := CompileDocs(counterDocs())
	if res.HTML == "" {
		t.Fatal("expected non-empty HTML for a counter app")
	}
	if !strings.Contains(res.HTML, "0") {
		t.Errorf("counter render should show the initial count 0:\n%s", res.HTML)
	}
	if len(res.Diagnostics) != 0 {
		t.Errorf("clean counter should have no diagnostics, got %v", res.Diagnostics)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("clean counter should have no unknown widgets, got %v", res.Unknown)
	}
	if res.RT == nil {
		t.Error("CompileDocs should return a live runtime for the wrapper to install")
	}
	if len(res.Handlers) == 0 {
		t.Error("counter's button onPress should register a handler")
	}
}

// TestCompileDocsRTLLocale guards the direction bit that crosses into JS: an
// app whose active locale is right-to-left compiles with Dir "rtl", and a
// locale-less app stays "ltr".
func TestCompileDocsRTLLocale(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "rtl", "entry": "main", "defaultLocale": "ar"},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{
				"type": "column", "id": "root",
				"children": []any{map[string]any{"type": "text", "id": "t", "text": "hi"}},
			},
		},
	}
	res := CompileDocs(docs)
	if res.Dir != "rtl" {
		t.Errorf("Arabic default locale should compile with dir=rtl, got %q", res.Dir)
	}
	if res.HTML == "" {
		t.Error("RTL app should still render HTML")
	}

	// The default (no locale anywhere) stays LTR.
	if res := CompileDocs(counterDocs()); res.Dir != "ltr" {
		t.Errorf("locale-less app should compile with dir=ltr, got %q", res.Dir)
	}
}

// TestCompileDocsUnknownStyleKey guards that a typo'd style key surfaces as a
// (non-fatal) diagnostic instead of being silently dropped.
func TestCompileDocsUnknownStyleKey(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "s", "entry": "main"},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{
				"type": "column", "id": "root",
				"style":    map[string]any{"bogusKey": "red"},
				"children": []any{map[string]any{"type": "text", "text": "hi"}},
			},
		},
	}
	res := CompileDocs(docs)
	found := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d, "bogusKey") {
			found = true
		}
	}
	if !found {
		t.Errorf("unknown style key should produce a diagnostic, got %v", res.Diagnostics)
	}
}

// TestCompileDocsTypelessDocDiagnosed guards that a doc with no recognised
// "type" surfaces in the diagnostics strip instead of vanishing: before the
// loader diagnosed unknown/missing types, setDocs([{id:"x"}]) compiled to
// "no scene to render" with zero diagnostics and zero unknown widgets, giving
// the playground nothing to show the author.
func TestCompileDocsTypelessDocDiagnosed(t *testing.T) {
	res := CompileDocs([]map[string]any{{"id": "x"}})
	if len(res.Diagnostics) == 0 {
		t.Fatal("a single typeless doc must produce a non-empty diagnostics slice")
	}
	found := false
	for _, d := range res.Diagnostics {
		if strings.HasPrefix(d, "error:") && strings.Contains(d, "unknown or missing") {
			found = true
		}
	}
	if !found {
		t.Errorf("diagnostics should contain the unknown/missing-type error, got %v", res.Diagnostics)
	}
}

// TestCompileDocsUnknownWidget guards that a typo'd widget type is reported via
// the unknown list (the self-verify surface) while still rendering.
func TestCompileDocsUnknownWidget(t *testing.T) {
	docs := []map[string]any{
		{"type": "app", "id": "u", "entry": "main"},
		{
			"type": "scene", "id": "main",
			"root": map[string]any{
				"type": "colunm", "id": "oops", // typo of "column"
				"children": []any{map[string]any{"type": "text", "text": "KEPT"}},
			},
		},
	}
	res := CompileDocs(docs)
	found := false
	for _, u := range res.Unknown {
		if u == "colunm" {
			found = true
		}
	}
	if !found {
		t.Errorf("unknown widget should be reported, got %v", res.Unknown)
	}
	if !strings.Contains(res.HTML, "KEPT") {
		t.Errorf("unknown widget children should still render:\n%s", res.HTML)
	}
}
