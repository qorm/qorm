package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Round 8 closes the last known injection hole in the renderer: the STYLE
// attribute breakout. boxCSS/textCSS interpolated author- or bound style
// values (background, gradient, shadow, cursor, transition, fontFamily, ...)
// RAW into the quoted style="..." attribute, and a handful of emitters append
// an author prop the same way (colour/curve/fit/background passthroughs): a
// double quote terminated the attribute and injected arbitrary attributes —
// the round-6 id= breakout class, since %q's \" is a literal backslash to an
// HTML parser. CSS url(javascript:) is inert, so the attribute breakout is
// the only live vector. The fix entity-encodes the value at emission
// (styleAttr / html.EscapeString): transparent for legitimate CSS values —
// & < > " ' never appear in them, and the browser HTML-unescapes the
// attribute value before CSS parsing — so safe values render byte-identical
// (the pre-existing exact-HTML style assertions stay untouched) and an
// adversarial quote or ampersand round-trips as an entity.

// TestEscapingStyleAttributeBreakout sweeps the quote-breakout payload through
// every author/bound string key boxCSS and textCSS interpolate and asserts the
// quote is entity-encoded inside the style attribute so the value cannot
// escape it.
func TestEscapingStyleAttributeBreakout(t *testing.T) {
	keys := []string{
		// boxCSS
		"background", "gradient", "shadow", "cursor", "transition", "position",
		// textCSS
		"color", "fontFamily", "fontStyle", "textDecoration", "textTransform", "textAlign",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
				Style: map[string]any{key: xssAttr}})
			assertAttrSafe(t, res.HTML, "style-"+key)
		})
	}

	t.Run("borderColor", func(t *testing.T) {
		// borderColor is only emitted alongside borderWidth.
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"borderWidth": float64(1), "borderColor": xssAttr}})
		assertAttrSafe(t, res.HTML, "style-borderColor")
	})

	t.Run("bound-style-value", func(t *testing.T) {
		// The bound path: a {{ state.* }} style value resolves through
		// resolveStyle into the same interpolation, so it must encode too.
		res := renderWidgetState(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": "{{ state.v }}"}},
			map[string]any{"v": xssAttr})
		assertAttrSafe(t, res.HTML, "bound-style-background")
	})

	t.Run("quote-entity-inside-style-attr", func(t *testing.T) {
		// The attribute-breakout itself: a double quote in the value must
		// render as &#34; INSIDE the style attribute — no raw quote may
		// survive to terminate the attribute and open a new one.
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": `x" onmouseover="alert(1)`}})
		if !strings.Contains(res.HTML, "background:x&#34; onmouseover=&#34;alert(1);") {
			t.Errorf("double quote must render as &#34; inside the style attribute:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, `onmouseover="`) {
			t.Errorf("quote breakout injected a raw attribute:\n%s", res.HTML)
		}
	})
}

// TestEscapingStyleAttributeTransparency pins the fix's transparency contract:
// an ampersand value renders entity-encoded exactly once (containerCSS appends
// the already-encoded boxCSS output and must not double-encode), while
// ordinary values render byte-identical with no entities at all.
func TestEscapingStyleAttributeTransparency(t *testing.T) {
	t.Run("ampersand-encoded", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": "a&b"}})
		if !strings.Contains(res.HTML, "background:a&amp;b;") {
			t.Errorf("& must be entity-encoded inside the style attribute:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "background:a&b;") {
			t.Errorf("raw & must not survive in the style attribute:\n%s", res.HTML)
		}
	})

	t.Run("container-encodes-once", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "column", ID: "c",
			Style: map[string]any{"background": "a&b"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "background:a&amp;b;") || strings.Contains(res.HTML, "&amp;amp;") {
			t.Errorf("containerCSS must encode exactly once:\n%s", res.HTML)
		}
	})

	t.Run("normal-values-unchanged", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": "red", "fontFamily": "Arial", "transition": "all .2s"}})
		for _, w := range []string{"background:red;", "font-family:Arial;", "transition:all .2s;"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("safe style value must render verbatim, lacks %q:\n%s", w, res.HTML)
			}
		}
		if strings.Contains(res.HTML, "&#") {
			t.Errorf("safe style values must not be entity-encoded:\n%s", res.HTML)
		}
	})
}

// TestEscapingStylePropBreakout sweeps the sibling emitters that interpolate
// an author prop into a quoted style attribute WITHOUT going through
// boxCSS/textCSS (the colour/curve/fit/background/menuStyle passthroughs):
// same breakout class, same entity-encoding fix.
func TestEscapingStylePropBreakout(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
	}{
		{"appbar-background", &model.Node{Type: "appbar", ID: "a", Label: "T", Props: map[string]any{"background": xssAttr}}},
		{"largetitle-background", &model.Node{Type: "largetitle", ID: "lt", Label: "T", Props: map[string]any{"background": xssAttr}}},
		{"badge-color", &model.Node{Type: "badge", ID: "b", Label: "1", Props: map[string]any{"color": xssAttr}, Children: textKids("c")}},
		{"spinner-color", &model.Node{Type: "spinner", ID: "sp", Props: map[string]any{"color": xssAttr}}},
		{"progress-color", &model.Node{Type: "progress", ID: "p", Value: "0.4", Props: map[string]any{"color": xssAttr}}},
		{"image-fit", &model.Node{Type: "image", ID: "i", Props: map[string]any{"src": "p.png", "fit": xssAttr}}},
		{"richtext-span-color", &model.Node{Type: "richtext", ID: "rt", Props: map[string]any{"spans": []any{map[string]any{"text": "x", "color": xssAttr}}}}},
		{"contextmenu-menustyle", &model.Node{Type: "contextmenu", ID: "cm",
			Props:    map[string]any{"items": []any{map[string]any{"id": "i", "title": "T"}}, "menuStyle": xssAttr},
			Children: textKids("c")}},
		{"swipeactions-action-color", &model.Node{Type: "swipeactions", ID: "sw",
			Props:    map[string]any{"actions": []any{map[string]any{"label": "Del", "color": xssAttr}}},
			Children: textKids("c")}},
		{"motion-curve-repeat", &model.Node{Type: "motion", ID: "mo",
			Props: map[string]any{"curve": xssAttr, "repeat": xssAttr}, Children: textKids("c")}},
		{"animatedcontainer-curve", &model.Node{Type: "animatedcontainer", ID: "ac",
			Props: map[string]any{"curve": xssAttr}, Children: textKids("c")}},
		{"wrapanimation-curve-repeat", &model.Node{Type: "text", ID: "wt", Text: "x",
			Props: map[string]any{"animation": "fadeup", "curve": xssAttr, "repeat": xssAttr}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertAttrSafe(t, renderWidget(t, tc.node).HTML, tc.name)
		})
	}

	t.Run("normal-props-unchanged", func(t *testing.T) {
		// Legitimate prop values still pass through verbatim (no entities).
		res := renderWidget(t, &model.Node{Type: "spinner", ID: "sp", Props: map[string]any{"color": "#ff0000"}})
		if !strings.Contains(res.HTML, "border-top-color:#ff0000;") {
			t.Errorf("safe colour prop must render verbatim:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "&#") {
			t.Errorf("safe colour prop must not be entity-encoded:\n%s", res.HTML)
		}
	})
}

// TestEscapingFillStrokeAttributeBreakout covers round 8's deferred sibling:
// SVG fill= / stroke= ATTRIBUTES. chartBars and chartLine (the chart widget)
// and circularProgress interpolate an author `color` prop RAW into a
// double-quoted SVG attribute; a double quote terminates the attribute and
// injects arbitrary attributes — the round-6 id= breakout class. The fix
// entity-encodes the colour value at each emission, leaving the surrounding
// constant SVG markup untouched.
func TestEscapingFillStrokeAttributeBreakout(t *testing.T) {
	const payload = `x" onmouseover="alert(1)`
	const encoded = `x&#34; onmouseover=&#34;alert(1)`
	cases := []struct {
		name string
		node *model.Node
		// attrs are the exact entity-encoded fill=/stroke= fragments that
		// must appear (one per emission site the payload reaches).
		attrs []string
	}{
		{"chart-bar-fill",
			&model.Node{Type: "chart", ID: "c1", Props: map[string]any{"data": []any{float64(1), float64(2)}, "color": payload}},
			[]string{`fill="` + encoded + `"`}},
		{"chart-line-stroke",
			&model.Node{Type: "chart", ID: "c2", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "line", "color": payload}},
			[]string{`stroke="` + encoded + `"`}},
		{"chart-sparkline-stroke",
			&model.Node{Type: "chart", ID: "c3", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "sparkline", "color": payload}},
			[]string{`stroke="` + encoded + `"`}},
		{"chart-area-fill-and-stroke",
			&model.Node{Type: "chart", ID: "c4", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "area", "color": payload}},
			[]string{`fill="` + encoded + `"`, `stroke="` + encoded + `"`}},
		{"circularprogress-determinate-stroke",
			&model.Node{Type: "circularprogress", ID: "cp1", Props: map[string]any{"value": "0.5", "color": payload}},
			[]string{`stroke="` + encoded + `"`}},
		{"circularprogress-indeterminate-stroke",
			&model.Node{Type: "circularprogress", ID: "cp2", Props: map[string]any{"color": payload}},
			[]string{`stroke="` + encoded + `"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			for _, attr := range tc.attrs {
				if !strings.Contains(res.HTML, attr) {
					t.Errorf("%s: colour must be entity-encoded inside the SVG attribute, lacks %s:\n%s", tc.name, attr, res.HTML)
				}
			}
			// No raw double quote from the payload may survive to terminate
			// the attribute and open an injected handler.
			if strings.Contains(res.HTML, `onmouseover="`) {
				t.Errorf("%s: raw quote closed the attribute and injected a handler:\n%s", tc.name, res.HTML)
			}
		})
	}
}

// TestEscapingFillStrokeTransparency pins the fix's transparency contract:
// legitimate colours (hex, rgb(), named, var()) contain none of &<>"' and must
// render byte-identical — defaults included — so every pre-existing exact-HTML
// chart/progress assertion (all safe colours) is unaffected.
func TestEscapingFillStrokeTransparency(t *testing.T) {
	for _, color := range []string{"#3b82f6", "rgb(59,130,246)", "red", "var(--success)"} {
		t.Run("chart-"+color, func(t *testing.T) {
			bar := renderWidget(t, &model.Node{Type: "chart", ID: "cb", Props: map[string]any{"data": []any{float64(1), float64(2)}, "color": color}})
			if !strings.Contains(bar.HTML, `fill="`+color+`"`) {
				t.Errorf("safe chart colour must render verbatim in fill=, lacks fill=%q:\n%s", color, bar.HTML)
			}
			line := renderWidget(t, &model.Node{Type: "chart", ID: "cl", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "line", "color": color}})
			if !strings.Contains(line.HTML, `stroke="`+color+`"`) {
				t.Errorf("safe chart colour must render verbatim in stroke=, lacks stroke=%q:\n%s", color, line.HTML)
			}
			if strings.Contains(bar.HTML, "&#") || strings.Contains(line.HTML, "&#") {
				t.Errorf("safe chart colour must not be entity-encoded:\n%s\n%s", bar.HTML, line.HTML)
			}
		})

		t.Run("circularprogress-"+color, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "circularprogress", ID: "cp", Props: map[string]any{"value": "0.5", "color": color}})
			if !strings.Contains(res.HTML, `stroke="`+color+`"`) {
				t.Errorf("safe progress colour must render verbatim in stroke=, lacks stroke=%q:\n%s", color, res.HTML)
			}
			if strings.Contains(res.HTML, "&#") {
				t.Errorf("safe progress colour must not be entity-encoded:\n%s", res.HTML)
			}
		})
	}

	t.Run("defaults-unchanged", func(t *testing.T) {
		// With no colour prop the default var(--accent) must render verbatim.
		bar := renderWidget(t, &model.Node{Type: "chart", ID: "db", Props: map[string]any{"data": []any{float64(1), float64(2)}}})
		if !strings.Contains(bar.HTML, `fill="var(--accent)"`) {
			t.Errorf("chart default colour must render verbatim:\n%s", bar.HTML)
		}
		line := renderWidget(t, &model.Node{Type: "chart", ID: "dl", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "line"}})
		if !strings.Contains(line.HTML, `stroke="var(--accent)"`) {
			t.Errorf("line chart default colour must render verbatim:\n%s", line.HTML)
		}
		cp := renderWidget(t, &model.Node{Type: "circularprogress", ID: "dcp", Props: map[string]any{"value": "0.5"}})
		if !strings.Contains(cp.HTML, `stroke="var(--accent)"`) {
			t.Errorf("circularprogress default colour must render verbatim:\n%s", cp.HTML)
		}
	})
}
