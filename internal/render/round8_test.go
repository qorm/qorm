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

// assertStyleValueRejected is the CSS-injection contract for the string style
// keys: a value that is not a well-formed CSS value (cssStyleValue) never
// reaches the style attribute at all — the declaration is simply not emitted.
// That subsumes the round-8 quote-breakout contract (nothing to break out with)
// and additionally closes the declaration-injection hole entity-encoding left
// open, since html.EscapeString does not touch ";".
func assertStyleValueRejected(t *testing.T, html, ctx string, prop string, fragments ...string) {
	t.Helper()
	if strings.Contains(html, "<script>QORMXSS") {
		t.Errorf("%s: attribute breakout leaked a raw script tag:\n%s", ctx, html)
	}
	if strings.Contains(html, prop+":") {
		t.Errorf("%s: a rejected value must not be emitted as %s: at all:\n%s", ctx, prop, html)
	}
	for _, f := range fragments {
		if strings.Contains(html, f) {
			t.Errorf("%s: rejected value leaked %q:\n%s", ctx, f, html)
		}
	}
}

// TestEscapingStyleAttributeBreakout sweeps the quote-breakout payload through
// every author/bound string key boxCSS and textCSS interpolate. Round 8 asserted
// the quote was entity-encoded; the CSS-declaration-injection fix goes further
// and rejects the whole value at colorStr, so nothing is emitted.
func TestEscapingStyleAttributeBreakout(t *testing.T) {
	// key -> the CSS property it feeds, so the assertion can prove the whole
	// declaration is gone rather than merely that no quote survived.
	keys := map[string]string{
		// boxCSS
		"background": "background", "gradient": "background", "shadow": "box-shadow",
		"cursor": "cursor", "transition": "transition", "position": "position",
		// textCSS
		"color": "color", "fontFamily": "font-family", "fontStyle": "font-style",
		"textDecoration": "text-decoration", "textTransform": "text-transform",
		"textAlign": "text-align",
	}
	for key, prop := range keys {
		t.Run(key, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
				Style: map[string]any{key: xssAttr}})
			assertStyleValueRejected(t, res.HTML, "style-"+key, prop)
		})
	}

	t.Run("borderColor", func(t *testing.T) {
		// borderColor is only emitted alongside borderWidth; a rejected colour
		// falls back to the constant var(--sep) rather than vanishing.
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"borderWidth": float64(1), "borderColor": xssAttr}})
		if !strings.Contains(res.HTML, "border:1px solid var(--sep);") {
			t.Errorf("a rejected borderColor must fall back to var(--sep):\n%s", res.HTML)
		}
		assertStyleValueRejected(t, res.HTML, "style-borderColor", "no-such-prop", "QORMXSS", "&#34;")
	})

	t.Run("bound-style-value", func(t *testing.T) {
		// The bound path: a {{ state.* }} style value resolves through
		// resolveStyle into the same interpolation, so it is validated too.
		res := renderWidgetState(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": "{{ state.v }}"}},
			map[string]any{"v": xssAttr})
		assertStyleValueRejected(t, res.HTML, "bound-style-background", "background")
	})

	// The declaration-injection payload the red team used: no quote at all, so
	// entity encoding alone let it through verbatim and turned any styled node
	// into a full-screen click-jacking overlay plus an outbound image beacon.
	// It arrives the same way a legitimate colour does — an http response
	// written to state, MCP qorm_set_state, or an input bound to a style key.
	t.Run("declaration-injection-clickjack-overlay", func(t *testing.T) {
		const payload = `#fff;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999;background-image:url(//attacker/beacon.png)`
		for _, tc := range []struct {
			name string
			res  Result
		}{
			{"literal", renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
				Style: map[string]any{"background": payload}})},
			{"bound", renderWidgetState(t, &model.Node{Type: "text", ID: "s", Text: "x",
				Style: map[string]any{"background": "{{ state.v }}"}}, map[string]any{"v": payload})},
		} {
			for _, frag := range []string{"position:fixed", "z-index:99999", "background-image", "attacker", "100vw"} {
				if strings.Contains(tc.res.HTML, frag) {
					t.Errorf("%s: CSS declaration injection leaked %q:\n%s", tc.name, frag, tc.res.HTML)
				}
			}
		}
	})

	// url() on its own — the outbound-beacon half of the payload, without any
	// declaration injection. The browser really fetches it, leaking the visit
	// and the Referer, so it is rejected even though it breaks nothing.
	t.Run("url-beacon-rejected", func(t *testing.T) {
		for _, v := range []string{
			"url(//attacker/beacon.png)",
			"URL(https://attacker/b.png)",
			"var(--bg) url(//attacker/b.png) no-repeat",
			"image-set(url(//attacker/b.png) 1x)",
		} {
			res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
				Style: map[string]any{"background": v}})
			if strings.Contains(res.HTML, "attacker") {
				t.Errorf("a url() style value must be rejected, got %q:\n%s", v, res.HTML)
			}
		}
	})
}

// TestEscapingStyleAttributeTransparency pins the fix's transparency contract:
// legitimate CSS values render byte-identical with no entities at all, and a
// value carrying a character no CSS value ever contains is dropped rather than
// smuggled through as an entity.
func TestEscapingStyleAttributeTransparency(t *testing.T) {
	t.Run("ampersand-rejected", func(t *testing.T) {
		// "&" is not a CSS value character. It used to render as &amp;, which
		// was safe but meaningless; now the declaration is dropped.
		res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x",
			Style: map[string]any{"background": "a&b"}})
		if strings.Contains(res.HTML, "background:") {
			t.Errorf("a value carrying & is not a CSS value and must be dropped:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "background:a&b;") {
			t.Errorf("raw & must not survive in the style attribute:\n%s", res.HTML)
		}
	})

	t.Run("container-value-rejected-once", func(t *testing.T) {
		// containerCSS appends boxCSS's already-encoded output; a rejected
		// value must leave no trace and no double-encoding on either half.
		res := renderWidget(t, &model.Node{Type: "column", ID: "c",
			Style: map[string]any{"background": "a&b"}, Children: textKids("x")})
		if strings.Contains(res.HTML, "background:") || strings.Contains(res.HTML, "&amp;") {
			t.Errorf("containerCSS must drop a non-CSS value cleanly:\n%s", res.HTML)
		}
	})

	// The whole legitimate CSS-value vocabulary the repo's examples use must
	// survive the validator byte-identically — this is the compatibility guard
	// for the allowlist.
	t.Run("legitimate-values-pass", func(t *testing.T) {
		for _, v := range []string{
			"#0af", "#ff0000", "red", "transparent", "var(--accent)",
			"var(--qorm-token-color-bg)", "rgba(28,28,30,0.92)", "rgb(0 0 0 / 50%)",
			"color-mix(in srgb,var(--success) 15%,transparent)",
			"linear-gradient(135deg,#007aff,#5e5ce6)",
			"0 1px 3px rgba(0,0,0,.08)", "all .2s ease", "-apple-system, sans-serif",
		} {
			if got := cssStyleValue(v); got != v {
				t.Errorf("legitimate CSS value %q must pass through, got %q", v, got)
			}
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

// cssInjectionPayloads are the values every CSS entry point is swept with, one
// per attack class the red team demonstrated against a style value:
//
//   - attr-breakout — round 8's quote breakout: the value ends the style="…"
//     attribute and injects its own attributes/markup.
//   - declaration-injection — the value ends its OWN declaration and appends a
//     full-screen overlay, i.e. clickjacking (an invisible layer over the real
//     controls) plus, with an image, an outbound beacon. html.EscapeString does
//     not touch ";", so entity encoding leaves this completely open: only the
//     value allowlist closes it.
//   - url-beacon — a background image the browser really fetches, leaking the
//     visit and the page's Referer to a third party with no script involved.
//   - comment-truncation — an unterminated /* swallows every declaration that
//     follows it, stripping the layout the widget itself relies on.
var cssInjectionPayloads = map[string]string{
	"attr-breakout":         xssAttr,
	"declaration-injection": "#fff;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999",
	"url-beacon":            "url(//attacker/beacon.png)",
	"comment-truncation":    "red/*",
}

// assertNoCSSInjection asserts that no signature fragment of any
// cssInjectionPayloads value reached the rendered HTML — neither raw nor
// entity-encoded, since the contract is rejection rather than escaping.
func assertNoCSSInjection(t *testing.T, html, ctx string) {
	t.Helper()
	for _, frag := range []string{"QORMXSS", "&#34;", "100vw", "z-index:99999", "//attacker", "/*"} {
		if strings.Contains(html, frag) {
			t.Errorf("%s: rejected CSS payload leaked %q:\n%s", ctx, frag, html)
		}
	}
}

// TestEscapingStylePropBreakout sweeps the sibling emitters that interpolate an
// author PROP into a quoted style attribute without going through
// boxCSS/textCSS (the colour/curve/fit/background passthroughs). Round 8
// entity-encoded those values, which stops the attribute breakout but not the
// declaration injection sitting behind it — ";" is not one of the five
// characters html.EscapeString touches. They now go through the same CSS-value
// allowlist as the style map (cssValueOr / cssStyleValue), so a hostile value
// is rejected outright and the widget's built-in default takes over: the
// declaration around it (`animation:… %s …`, `border-top-color:%s`) stays
// well-formed, which dropping the value would not.
func TestEscapingStylePropBreakout(t *testing.T) {
	cases := []struct {
		name string
		node func(payload string) *model.Node
		// want is the declaration the widget falls back to once the hostile
		// prop is rejected — proof it degrades to its built-in look rather
		// than to a broken or a half-applied style.
		want string
	}{
		{"appbar-background", func(p string) *model.Node {
			return &model.Node{Type: "appbar", ID: "a", Label: "T", Props: map[string]any{"background": p}}
		}, "background:var(--surface);"},
		{"largetitle-background", func(p string) *model.Node {
			return &model.Node{Type: "largetitle", ID: "lt", Label: "T", Props: map[string]any{"background": p}}
		}, "background:var(--bg);"},
		{"badge-color", func(p string) *model.Node {
			return &model.Node{Type: "badge", ID: "b", Label: "1", Props: map[string]any{"color": p}, Children: textKids("c")}
		}, "background:#ef4444;"},
		{"spinner-color", func(p string) *model.Node {
			return &model.Node{Type: "spinner", ID: "sp", Props: map[string]any{"color": p}}
		}, "border-top-color:var(--accent);"},
		{"progress-color", func(p string) *model.Node {
			return &model.Node{Type: "progress", ID: "p", Value: "0.4", Props: map[string]any{"color": p}}
		}, "background:var(--accent);"},
		{"image-fit", func(p string) *model.Node {
			return &model.Node{Type: "image", ID: "i", Props: map[string]any{"src": "p.png", "fit": p}}
		}, "object-fit:cover;"},
		{"richtext-span-color", func(p string) *model.Node {
			return &model.Node{Type: "richtext", ID: "rt", Props: map[string]any{"spans": []any{map[string]any{"text": "x", "color": p}}}}
		}, `<span style="">x</span>`}, // no default colour: the declaration is simply not emitted
		{"motion-curve-repeat", func(p string) *model.Node {
			return &model.Node{Type: "motion", ID: "mo", Props: map[string]any{"curve": p, "repeat": p}, Children: textKids("c")}
		}, "animation:qa-fade 450ms cubic-bezier(.34,1.2,.64,1) 0ms 1 both;"},
		{"animatedcontainer-curve", func(p string) *model.Node {
			return &model.Node{Type: "animatedcontainer", ID: "ac", Props: map[string]any{"curve": p}, Children: textKids("c")}
		}, "transition:all var(--qorm-motion-normal,250ms) cubic-bezier(.4,0,.2,1);"},
		{"wrapanimation-curve-repeat", func(p string) *model.Node {
			return &model.Node{Type: "text", ID: "wt", Text: "x", Props: map[string]any{"animation": "fadeup", "curve": p, "repeat": p}}
		}, "animation:qa-fadeup 450ms cubic-bezier(.34,1.2,.64,1) 0ms 1 both;"},
	}
	for _, tc := range cases {
		for pname, payload := range cssInjectionPayloads {
			t.Run(tc.name+"/"+pname, func(t *testing.T) {
				res := renderWidget(t, tc.node(payload))
				assertNoCSSInjection(t, res.HTML, tc.name+"/"+pname)
				if !strings.Contains(res.HTML, tc.want) {
					t.Errorf("%s/%s: a rejected prop must fall back to %q:\n%s", tc.name, pname, tc.want, res.HTML)
				}
			})
		}
	}

	// The image `placeholder` is the one prop that legitimately builds a url(),
	// so it is validated per branch (cssURLToken / cssStyleValue) instead of by
	// the single value allowlist — see cssURLToken for why ")" is the character
	// that matters there.
	t.Run("image-placeholder", func(t *testing.T) {
		for _, p := range []string{
			// colour branch: ends its own declaration and covers the screen
			"#eee;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999",
			// URL branch: looksLikeImageURL says "URL", the ")" closes the
			// url() token early and the rest is parsed as fresh declarations
			"a.png);position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999;background:url(b.png",
			// quote breakout, and a comment that would eat object-fit
			xssAttr, "#eee/*",
		} {
			res := renderWidget(t, &model.Node{Type: "image", ID: "ip",
				Props: map[string]any{"src": "p.png", "placeholder": p}})
			assertNoCSSInjection(t, res.HTML, "image-placeholder")
			if strings.Contains(res.HTML, "background:") {
				t.Errorf("image-placeholder %q: a rejected placeholder must not be painted at all:\n%s", p, res.HTML)
			}
		}
	})

	t.Run("image-placeholder-legitimate", func(t *testing.T) {
		// Both branches keep working byte-for-byte, the data: URI included —
		// its ";" is inside the url() token, where the CSS tokenizer keeps it.
		for ph, want := range map[string]string{
			"#e5e7eb":                     "background:#e5e7eb;",
			"var(--fill)":                 "background:var(--fill);",
			"blur.png":                    "background:url(blur.png) center/cover no-repeat;",
			"/static/thumbs/a_1.jpg?v=2":  "background:url(/static/thumbs/a_1.jpg?v=2) center/cover no-repeat;",
			"data:image/png;base64,iVBOR": "background:url(data:image/png;base64,iVBOR) center/cover no-repeat;",
		} {
			res := renderWidget(t, &model.Node{Type: "image", ID: "ip",
				Props: map[string]any{"src": "p.png", "placeholder": ph}})
			if !strings.Contains(res.HTML, want) {
				t.Errorf("legitimate placeholder %q must render as %q:\n%s", ph, want, res.HTML)
			}
		}
	})

	// menuStyle stays a RAW declaration list (cssRawDecls): its contract IS
	// "append these declarations", and it cannot carry untrusted data because
	// propStr never evaluates a binding. The one rule that still applies is the
	// one that reaches off the page.
	t.Run("contextmenu-menustyle", func(t *testing.T) {
		ctxMenu := func(v string) *model.Node {
			return &model.Node{Type: "contextmenu", ID: "cm",
				Props:    map[string]any{"items": []any{map[string]any{"id": "i", "title": "T"}}, "menuStyle": v},
				Children: textKids("c")}
		}
		res := renderWidget(t, ctxMenu(xssAttr))
		assertAttrSafe(t, res.HTML, "contextmenu-menustyle") // quote stays entity-encoded
		for _, bad := range []string{"url(//attacker/beacon.png)", "background:red/*"} {
			res := renderWidget(t, ctxMenu(bad))
			if strings.Contains(res.HTML, "//attacker") || strings.Contains(res.HTML, "/*") {
				t.Errorf("menuStyle %q must be dropped wholesale:\n%s", bad, res.HTML)
			}
		}
		res = renderWidget(t, ctxMenu("background:hotpink;padding:12px;"))
		if !strings.Contains(res.HTML, "background:hotpink;padding:12px;") {
			t.Errorf("a legitimate menuStyle declaration list must pass through verbatim:\n%s", res.HTML)
		}
	})

	// swipeactions reads its per-action colour through colorStr, so it now gets
	// the stronger CSS-value validation instead of entity encoding: an
	// adversarial colour is dropped and the constant default takes over.
	t.Run("swipeactions-action-color", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "swipeactions", ID: "sw",
			Props:    map[string]any{"actions": []any{map[string]any{"label": "Del", "color": xssAttr}}},
			Children: textKids("c")})
		if strings.Contains(res.HTML, "QORMXSS") || strings.Contains(res.HTML, "&#34;") {
			t.Errorf("swipeactions colour must be rejected outright:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "background:var(--danger);") {
			t.Errorf("a rejected swipeactions colour must fall back to var(--danger):\n%s", res.HTML)
		}
	})

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

	// The vocabulary these props actually carry must survive untouched — the
	// compatibility guard for routing them through the allowlist. cubic-bezier
	// matters most: it is the DEFAULT curve, so a filter that rejected it would
	// break every animated widget in the repo.
	t.Run("legitimate-prop-values-pass", func(t *testing.T) {
		for _, v := range []string{
			"cubic-bezier(.34,1.2,.64,1)", "ease-in-out", "linear", "steps(4, end)",
			"infinite", "3", "cover", "contain", "scale-down",
			"#0af", "rgba(28,28,30,0.92)", "rgb(0 0 0 / 50%)", "var(--accent)",
			"color-mix(in srgb,var(--success) 15%,transparent)",
			"linear-gradient(135deg,#007aff,#5e5ce6)",
		} {
			if got := cssValueOr(v, "DEFAULT"); got != v {
				t.Errorf("legitimate prop value %q must pass through, got %q", v, got)
			}
		}
		if got := cssValueOr("", "var(--accent)"); got != "var(--accent)" {
			t.Errorf("an absent prop must yield the default, got %q", got)
		}
	})
}

// TestEscapingFillStrokeAttributeBreakout covers round 8's deferred sibling:
// SVG fill= / stroke= ATTRIBUTES. chartBars/chartLine (the chart widget) and
// circularProgress interpolate an author `color` prop into a double-quoted SVG
// attribute; round 8 entity-encoded it there, closing the quote breakout. A
// presentation attribute holds a single value, so ";" cannot open a second
// declaration the way it can inside style="…" — but the value is CSS paint all
// the same, so it now goes through the same allowlist as every other colour and
// a hostile value falls back to var(--accent) instead of being escaped into the
// attribute.
func TestEscapingFillStrokeAttributeBreakout(t *testing.T) {
	cases := []struct {
		name string
		node func(payload string) *model.Node
		// attrs are the exact fill=/stroke= fragments the fallback must
		// produce (one per emission site the payload reaches).
		attrs []string
	}{
		{"chart-bar-fill", func(p string) *model.Node {
			return &model.Node{Type: "chart", ID: "c1", Props: map[string]any{"data": []any{float64(1), float64(2)}, "color": p}}
		}, []string{`fill="var(--accent)"`}},
		{"chart-line-stroke", func(p string) *model.Node {
			return &model.Node{Type: "chart", ID: "c2", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "line", "color": p}}
		}, []string{`stroke="var(--accent)"`}},
		{"chart-sparkline-stroke", func(p string) *model.Node {
			return &model.Node{Type: "chart", ID: "c3", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "sparkline", "color": p}}
		}, []string{`stroke="var(--accent)"`}},
		{"chart-area-fill-and-stroke", func(p string) *model.Node {
			return &model.Node{Type: "chart", ID: "c4", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "area", "color": p}}
		}, []string{`fill="var(--accent)"`, `stroke="var(--accent)"`}},
		{"circularprogress-determinate-stroke", func(p string) *model.Node {
			return &model.Node{Type: "circularprogress", ID: "cp1", Props: map[string]any{"value": "0.5", "color": p}}
		}, []string{`stroke="var(--accent)"`}},
		{"circularprogress-indeterminate-stroke", func(p string) *model.Node {
			return &model.Node{Type: "circularprogress", ID: "cp2", Props: map[string]any{"color": p}}
		}, []string{`stroke="var(--accent)"`}},
	}
	// The round-8 payload (a quote breakout) plus the paint-specific one: an
	// external paint server is the fill/stroke shape of the url() beacon.
	payloads := map[string]string{
		"attr-breakout": `x" onmouseover="alert(1)`,
		"url-beacon":    "url(//attacker/x.svg#p)",
	}
	for _, tc := range cases {
		for pname, payload := range payloads {
			t.Run(tc.name+"/"+pname, func(t *testing.T) {
				res := renderWidget(t, tc.node(payload))
				for _, attr := range tc.attrs {
					if !strings.Contains(res.HTML, attr) {
						t.Errorf("%s/%s: a rejected colour must fall back to %s:\n%s", tc.name, pname, attr, res.HTML)
					}
				}
				for _, frag := range []string{"onmouseover", "&#34;", "//attacker"} {
					if strings.Contains(res.HTML, frag) {
						t.Errorf("%s/%s: rejected colour leaked %q:\n%s", tc.name, pname, frag, res.HTML)
					}
				}
			})
		}
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
