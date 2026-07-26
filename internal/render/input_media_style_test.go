package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// ---- input / textarea / textformfield native-HTML attributes ----------------

// TestInputNativeAttrs covers the zero-JS attribute channel of the input
// widget: maxLength, autofocus, readonly, required, autocomplete, inputMode
// and pattern all render as their native HTML attributes, and none of them is
// emitted when the prop is absent (backward-compatible output).
func TestInputNativeAttrs(t *testing.T) {
	cases := []struct {
		name    string
		props   map[string]any
		want    []string
		wantNot []string
	}{
		{"maxLength", map[string]any{"maxLength": float64(20)}, []string{` maxlength="20"`}, nil},
		{"maxLength-zero-ignored", map[string]any{"maxLength": float64(0)}, nil, []string{"maxlength"}},
		{"autofocus", map[string]any{"autofocus": true}, []string{` autofocus`}, nil},
		{"autofocus-false", map[string]any{"autofocus": false}, nil, []string{"autofocus"}},
		{"readonly", map[string]any{"readonly": true}, []string{` readonly`}, nil},
		{"required", map[string]any{"required": true}, []string{` required`}, nil},
		{"autocomplete", map[string]any{"autocomplete": "email"}, []string{` autocomplete="email"`}, nil},
		{"inputMode-passthrough", map[string]any{"inputMode": "decimal"}, []string{` inputmode="decimal"`}, nil},
		{"inputMode-number-alias", map[string]any{"inputMode": "number"}, []string{` inputmode="numeric"`}, nil},
		{"inputMode-phone-alias", map[string]any{"inputMode": "phone"}, []string{` inputmode="tel"`}, nil},
		{"pattern", map[string]any{"pattern": "[0-9]{4}"}, []string{` pattern="[0-9]{4}"`}, nil},
		{"pattern-required-together", map[string]any{"pattern": "[a-z]+", "required": true},
			[]string{` pattern="[a-z]+"`, ` required`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "input", ID: "in", Props: tc.props})
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("props %v: html lacks %q:\n%s", tc.props, w, res.HTML)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(res.HTML, w) {
					t.Errorf("props %v: html should not contain %q:\n%s", tc.props, w, res.HTML)
				}
			}
		})
	}

	// Backward compatibility: a bare input emits none of the new attributes.
	t.Run("absent-props-emit-nothing", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "input", ID: "bare", Placeholder: "Name"})
		for _, w := range []string{"maxlength", "autofocus", "readonly", "required", "autocomplete", "inputmode", "pattern"} {
			if strings.Contains(res.HTML, w) {
				t.Errorf("bare input must not emit %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestInputAttrValueEscaping guards the quoted-attribute injection class for
// the new string-valued attributes: a double quote in an author value must be
// entity-encoded, never terminate the attribute.
func TestInputAttrValueEscaping(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "input", ID: "esc", Props: map[string]any{
		"pattern":      `a"b`,
		"autocomplete": `x" onmouseover="alert(1)`,
		"inputMode":    `t" onfocus="alert(2)`,
	}})
	for _, bad := range []string{`pattern="a"b"`, `" onmouseover=`, `" onfocus=`} {
		if strings.Contains(res.HTML, bad) {
			t.Errorf("attribute breakout leaked %q:\n%s", bad, res.HTML)
		}
	}
	if !strings.Contains(res.HTML, `pattern="a&#34;b"`) {
		t.Errorf("pattern quote should be entity-encoded:\n%s", res.HTML)
	}
}

// TestTextareaNativeAttrs: textarea shares the attribute set except pattern,
// which is an <input>-only attribute in HTML and must be skipped.
func TestTextareaNativeAttrs(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "textarea", ID: "ta", Props: map[string]any{
		"maxLength":    float64(120),
		"autofocus":    true,
		"readonly":     true,
		"required":     true,
		"autocomplete": "off",
		"inputMode":    "number",
		"pattern":      "[0-9]+", // must NOT render on a textarea
	}})
	for _, w := range []string{` maxlength="120"`, ` autofocus`, ` readonly`, ` required`, ` autocomplete="off"`, ` inputmode="numeric"`} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("textarea lacks %q:\n%s", w, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "pattern") {
		t.Errorf("textarea must not emit the input-only pattern attribute:\n%s", res.HTML)
	}

	t.Run("absent-props-emit-nothing", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textarea", ID: "tb"})
		for _, w := range []string{"maxlength", "autofocus", "readonly", "required", "autocomplete", "inputmode", "pattern"} {
			if strings.Contains(res.HTML, w) {
				t.Errorf("bare textarea must not emit %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestTextFormFieldNativeAttrs: the inner input carries the shared attribute
// set, and maxLength drives BOTH the native maxlength attribute and the
// existing footer counter at the same limit.
func TestTextFormFieldNativeAttrs(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tff", Value: "abc", Props: map[string]any{
		"maxLength": float64(10),
		"required":  true,
		"pattern":   `\d+`,
	}})
	for _, w := range []string{` maxlength="10"`, `>3/10</span>`, ` required`, ` pattern="\d+"`} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("textformfield lacks %q:\n%s", w, res.HTML)
		}
	}
}

func TestNormalizeInputMode(t *testing.T) {
	cases := map[string]string{
		"number":  "numeric",
		"phone":   "tel",
		"numeric": "numeric",
		"tel":     "tel",
		"email":   "email",
		"url":     "url",
		"decimal": "decimal",
		"search":  "search",
		"none":    "none",
		"custom":  "custom", // unknown values pass through (browsers ignore them)
	}
	for in, want := range cases {
		if got := normalizeInputMode(in); got != want {
			t.Errorf("normalizeInputMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- image robustness --------------------------------------------------------

func TestImageLazyLoading(t *testing.T) {
	t.Run("default-lazy", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "p.png"}})
		if !strings.Contains(res.HTML, ` loading="lazy"`) {
			t.Errorf("image should lazy-load by default:\n%s", res.HTML)
		}
	})
	t.Run("lazy-false-opts-out", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "p.png", "lazy": false}})
		if strings.Contains(res.HTML, "loading=") {
			t.Errorf("lazy:false must drop the loading attribute:\n%s", res.HTML)
		}
	})
	t.Run("lazy-true-explicit", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "p.png", "lazy": true}})
		if !strings.Contains(res.HTML, ` loading="lazy"`) {
			t.Errorf("lazy:true should keep lazy loading:\n%s", res.HTML)
		}
	})
}

func TestImagePlaceholder(t *testing.T) {
	cases := []struct {
		name        string
		placeholder string
		want        string
	}{
		{"hex-color", "#e5e7eb", "background:#e5e7eb;"},
		{"theme-var", "var(--fill)", "background:var(--fill);"},
		{"rgba-color", "rgba(0,0,0,.5)", "background:rgba(0,0,0,.5);"},
		{"relative-url", "thumbs/lo.jpg", "background:url(thumbs/lo.jpg) center/cover no-repeat;"},
		{"bare-file-url", "lo.png", "background:url(lo.png) center/cover no-repeat;"},
		{"absolute-url", "https://cdn.example/lo.webp", "background:url(https://cdn.example/lo.webp) center/cover no-repeat;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "image", ID: "im",
				Props: map[string]any{"src": "hi.png", "placeholder": tc.placeholder}})
			if !strings.Contains(res.HTML, tc.want) {
				t.Errorf("placeholder %q: html lacks %q:\n%s", tc.placeholder, tc.want, res.HTML)
			}
		})
	}
	t.Run("absent-emits-nothing", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "p.png"}})
		if strings.Contains(res.HTML, "background:url(") {
			t.Errorf("image without placeholder must not paint a background image:\n%s", res.HTML)
		}
	})
	// A quote in the placeholder is entity-encoded by styleAttr and cannot
	// terminate the style attribute.
	t.Run("placeholder-breakout-escaped", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im",
			Props: map[string]any{"src": "p.png", "placeholder": `x.png") no-repeat;" onload="alert(1)`}})
		if strings.Contains(res.HTML, `" onload=`) {
			t.Errorf("placeholder broke out of the style attribute:\n%s", res.HTML)
		}
	})
}

func TestImageFallback(t *testing.T) {
	t.Run("renders-onerror-swap", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im",
			Props: map[string]any{"src": "hi.png", "fallback": "alt.png"}})
		// jsStringID makes the URL a JS string literal, html.EscapeString
		// entity-encodes the whole handler for the quoted attribute.
		want := ` onerror="this.onerror=null;this.src=&#34;alt.png&#34;"`
		if !strings.Contains(res.HTML, want) {
			t.Errorf("image fallback lacks %q:\n%s", want, res.HTML)
		}
	})
	t.Run("absent-emits-nothing", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "p.png"}})
		if strings.Contains(res.HTML, "onerror") {
			t.Errorf("image without fallback must not emit onerror:\n%s", res.HTML)
		}
	})
}

// TestImageFallbackInjectionClosure is the injection-closure guard for the one
// inline event handler the image widget emits: an adversarial fallback value
// must neither terminate the onerror attribute (quote breakout) nor smuggle a
// script/close-tag sequence past the HTML parser (jsStringID neutralises "<").
func TestImageFallbackInjectionClosure(t *testing.T) {
	payloads := []string{
		`x.png" onload="alert(1)`,
		`x.png"><script>alert(1)</script>`,
		`</script><script>alert(1)</script>`,
		`x.png';alert(1);'`,
		"x.png\\\";alert(1);//",
	}
	for _, p := range payloads {
		res := renderWidget(t, &model.Node{Type: "image", ID: "im",
			Props: map[string]any{"src": "hi.png", "fallback": p}})
		h := res.HTML
		for _, bad := range []string{`" onload=`, "<script", "</script", `alert(1)"`} {
			if strings.Contains(h, bad) {
				t.Errorf("fallback %q leaked %q into the markup:\n%s", p, bad, h)
			}
		}
		// The handler must still be a single well-formed attribute: exactly one
		// ` onerror="` attribute opening (the handler body's own
		// "this.onerror=null" is not an attribute) and the payload's raw double
		// quotes all entity-encoded.
		if strings.Count(h, ` onerror="`) != 1 {
			t.Errorf("fallback %q: expected exactly one onerror attribute:\n%s", p, h)
		}
	}
}

// ---- new style keys ----------------------------------------------------------

// TestBoxCSSNewKeys covers zIndex / alignSelf / flexShrink end-to-end through
// boxCSS (same shape as TestBoxCSSDeclarations in style_test.go).
func TestBoxCSSNewKeys(t *testing.T) {
	cases := []struct {
		name  string
		style map[string]any
		want  []string
	}{
		{"z-index", map[string]any{"zIndex": float64(5)}, []string{"z-index:5;"}},
		{"flex-shrink-zero", map[string]any{"flexShrink": float64(0)}, []string{"flex-shrink:0;"}},
		{"flex-shrink", map[string]any{"flexShrink": float64(2)}, []string{"flex-shrink:2;"}},
		{"align-self-center", map[string]any{"alignSelf": "center"}, []string{"align-self:center;"}},
		{"align-self-start", map[string]any{"alignSelf": "start"}, []string{"align-self:flex-start;"}},
		{"align-self-end", map[string]any{"alignSelf": "end"}, []string{"align-self:flex-end;"}},
		{"align-self-stretch", map[string]any{"alignSelf": "stretch"}, []string{"align-self:stretch;"}},
		{"align-self-auto", map[string]any{"alignSelf": "auto"}, []string{"align-self:auto;"}},
		{"align-self-baseline", map[string]any{"alignSelf": "baseline"}, []string{"align-self:baseline;"}},
		{"width-percent", map[string]any{"width": "50%"}, []string{"width:50%;"}},
		{"width-vw", map[string]any{"width": "30vw"}, []string{"width:30vw;"}},
		{"height-vh", map[string]any{"height": "40vh"}, []string{"height:40vh;"}},
		{"height-px-string", map[string]any{"height": "120px"}, []string{"height:120px;"}},
		{"width-fill-still-works", map[string]any{"width": "fill"}, []string{"width:100%;"}},
		{"width-number-still-works", map[string]any{"width": float64(80)}, []string{"width:80px;"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := styleHTML(t, tc.style, nil)
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("style %v: html lacks %q:\n%s", tc.style, w, html)
				}
			}
		})
	}

	// Unknown / malformed size strings stay ignored, as before.
	for _, bad := range []string{"wrap", "abc%", "12em", "50 %", "%"} {
		t.Run("size-ignored-"+bad, func(t *testing.T) {
			html := styleHTML(t, map[string]any{"width": bad}, nil)
			if strings.Contains(html, "width:") {
				t.Errorf("width %q should be ignored, got:\n%s", bad, html)
			}
		})
	}

	// An unknown alignSelf keyword falls back to flex-start (flexAlign's
	// documented fallback) rather than leaking the raw value into the CSS.
	t.Run("align-self-unknown-fallback", func(t *testing.T) {
		html := styleHTML(t, map[string]any{"alignSelf": "bogus"}, nil)
		if !strings.Contains(html, "align-self:flex-start;") || strings.Contains(html, "bogus") {
			t.Errorf("unknown alignSelf should map to flex-start:\n%s", html)
		}
	})
}

// TestSizeUnit is the direct unit test of the string-size parser: only
// <number><unit> for the whitelisted units parses, and the output is
// re-rendered from the parsed number (normalize-don't-trust).
func TestSizeUnit(t *testing.T) {
	ok := map[string]string{
		"50%":    "50%",
		"30vw":   "30vw",
		"40.5vh": "40.5vh",
		"120px":  "120px",
		"-10%":   "-10%",
		"050%":   "50%", // normalized through the float round-trip
	}
	for in, want := range ok {
		got, k := sizeUnit(in)
		if !k || got != want {
			t.Errorf("sizeUnit(%q) = %q,%t, want %q,true", in, got, k, want)
		}
	}
	for _, in := range []string{"fill", "wrap", "abc%", "12em", "50 %", "%", "vw", "", "50", "1e2junkpx"} {
		if got, k := sizeUnit(in); k {
			t.Errorf("sizeUnit(%q) = %q, should not parse", in, got)
		}
	}
}

// TestKnownStyleKeysNewEntries pins the loader-whitelist contract: the loader
// accepts exactly the keys in KnownStyleKeys, so each newly consumed key must
// be present or authors get a spurious unknown-key warning.
func TestKnownStyleKeysNewEntries(t *testing.T) {
	for _, k := range []string{"zIndex", "alignSelf", "flexShrink"} {
		if !KnownStyleKeys[k] {
			t.Errorf("KnownStyleKeys must include %q (loader whitelist)", k)
		}
	}
}
