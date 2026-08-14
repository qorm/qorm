package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
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

// ---- native constraint validation: submit buttons ---------------------------

// validityGate is the inline native-validation gate submitAttrs prefixes onto a
// gated submit button's onclick. Spelled out here so a change to it has to be
// deliberate.
const validityGate = `if(this.form){if(!this.form.noValidate){if(!this.form.reportValidity())return;}}`

// TestButtonSubmitType covers the type-attribute decision for every branch of
// the `submit` prop: absent (no attribute at all — the pre-validation output),
// true (type=submit), false (type=button, the Cancel escape hatch), and the
// novalidate opt-out (formnovalidate + no gate).
func TestButtonSubmitType(t *testing.T) {
	press := &model.Invoke{Name: "save"}
	cases := []struct {
		name    string
		props   map[string]any
		onPress *model.Invoke
		want    []string
		wantNot []string
	}{
		{"absent-no-type", nil, press,
			[]string{`class="qorm-tap" style="`, ` onclick="qorm(0)"`},
			[]string{"type=", "reportValidity", "formnovalidate"}},
		{"absent-no-type-no-press", nil, nil,
			[]string{`class="qorm-tap" style="`},
			[]string{"type=", "onclick", "reportValidity"}},
		{"submit-true", map[string]any{"submit": true}, press,
			[]string{` type="submit"`, ` onclick="` + validityGate + `qorm(0)"`},
			[]string{"formnovalidate", `onclick="qorm(`}},
		{"submit-true-no-press", map[string]any{"submit": true}, nil,
			[]string{` type="submit"`},
			[]string{"onclick", "reportValidity", "formnovalidate"}},
		{"submit-false-is-plain-button", map[string]any{"submit": false}, press,
			[]string{` type="button"`, ` onclick="qorm(0)"`},
			[]string{"reportValidity", "formnovalidate", `type="submit"`}},
		{"novalidate-opts-out", map[string]any{"submit": true, "novalidate": true}, press,
			[]string{` type="submit" formnovalidate`, ` onclick="qorm(0)"`},
			[]string{"reportValidity"}},
		{"novalidate-false-keeps-the-gate", map[string]any{"submit": true, "novalidate": false}, press,
			[]string{` type="submit"`, "reportValidity"},
			[]string{"formnovalidate"}},
		{"novalidate-without-submit-is-inert", map[string]any{"novalidate": true}, press,
			[]string{` onclick="qorm(0)"`},
			[]string{"type=", "formnovalidate", "reportValidity"}},
		{"submit-string-true", map[string]any{"submit": "true"}, press,
			[]string{` type="submit"`, "reportValidity"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "button", ID: "b", Label: "Go", Props: tc.props, OnPress: tc.onPress})
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
}

// TestButtonSubmitBound: both props resolve a {{binding}} over state, so an
// agent can drive the submit semantics the same way it drives everything else.
func TestButtonSubmitBound(t *testing.T) {
	node := func() *model.Node {
		return &model.Node{Type: "button", ID: "b", Label: "Go", OnPress: &model.Invoke{Name: "save"},
			Props: map[string]any{"submit": "{{state.isSubmit}}", "novalidate": "{{state.skip}}"}}
	}
	on := renderWidgetState(t, node(), map[string]any{"isSubmit": true, "skip": false}).HTML
	if !strings.Contains(on, ` type="submit"`) || !strings.Contains(on, "reportValidity") {
		t.Errorf("bound submit=true should render a gated submit button:\n%s", on)
	}
	off := renderWidgetState(t, node(), map[string]any{"isSubmit": false, "skip": false}).HTML
	if !strings.Contains(off, ` type="button"`) || strings.Contains(off, "reportValidity") {
		t.Errorf("bound submit=false should render a plain button:\n%s", off)
	}
	skip := renderWidgetState(t, node(), map[string]any{"isSubmit": true, "skip": true}).HTML
	if !strings.Contains(skip, ` formnovalidate`) || strings.Contains(skip, "reportValidity") {
		t.Errorf("bound novalidate=true should opt out of the gate:\n%s", skip)
	}
}

// TestFormSubmitGating is the end-to-end shape: a form with a required field, a
// gated submit button and a Cancel button that must NOT submit. Only the marked
// button becomes a submit button, and only it carries the gate — the generic
// button renderer must not turn every button into a submit.
func TestFormSubmitGating(t *testing.T) {
	form := &model.Node{Type: "form", ID: "f", Children: []*model.Node{
		{Type: "input", ID: "email", Props: map[string]any{"required": true, "pattern": `[^@]+@[^@]+`}},
		{Type: "button", ID: "cancel", Label: "Cancel", OnPress: &model.Invoke{Name: "close"},
			Props: map[string]any{"submit": false}},
		{Type: "button", ID: "save", Label: "Save", OnPress: &model.Invoke{Name: "save"},
			Props: map[string]any{"submit": true}},
	}}
	h := renderWidget(t, form).HTML
	for _, w := range []string{`<form id="f"`, ` required`, ` pattern="[^@]+@[^@]+"`} {
		if !strings.Contains(h, w) {
			t.Errorf("form html lacks %q:\n%s", w, h)
		}
	}
	if n := strings.Count(h, "reportValidity"); n != 1 {
		t.Errorf("exactly one button should carry the validity gate, got %d:\n%s", n, h)
	}
	cancel := h[strings.Index(h, `id="cancel"`):strings.Index(h, `id="save"`)]
	save := h[strings.Index(h, `id="save"`):]
	if !strings.Contains(save, ` type="submit"`) || !strings.Contains(save, validityGate) {
		t.Errorf("the submit button should be type=submit and gated:\n%s", save)
	}
	if !strings.Contains(cancel, ` type="button"`) || strings.Contains(cancel, "reportValidity") {
		t.Errorf("a submit:false button must be type=button and never gated:\n%s", cancel)
	}
}

// TestButtonSubmitInjectionClosure: the new attributes must stay a single
// well-formed attribute each, whatever the author puts in the id/label/props —
// the gate is a constant, and the id keeps riding through attrID.
func TestButtonSubmitInjectionClosure(t *testing.T) {
	for _, payload := range []string{
		`b" onclick="alert(1)`,
		`b"><script>alert(1)</script>`,
		`b" type="button`,
	} {
		res := renderWidget(t, &model.Node{Type: "button", ID: payload, Label: payload,
			OnPress: &model.Invoke{Name: "save"}, Props: map[string]any{"submit": true}})
		h := res.HTML
		for _, bad := range []string{`" onclick="alert`, "<script", `" type="button`} {
			if strings.Contains(h, bad) {
				t.Errorf("payload %q leaked %q:\n%s", payload, bad, h)
			}
		}
		if strings.Count(h, ` type="submit"`) != 1 || strings.Count(h, ` onclick="`) != 1 {
			t.Errorf("payload %q: expected exactly one type= and one onclick= attribute:\n%s", payload, h)
		}
	}
}

// TestButtonValidationBackCompat is the shape guard for existing apps: a button
// that uses none of the new props emits exactly the attribute sequence it
// emitted before native validation existed — id, class, style and (only with a
// press handler) a bare qorm() onclick, and nothing else. Matching the whole
// element against an anchored pattern catches an injected attribute anywhere in
// the tag, without pinning the default style string (which other work moves).
func TestButtonValidationBackCompat(t *testing.T) {
	tagRe := regexp.MustCompile(`<button id="b" class="qorm-tap" style="[^"]*"( onclick="qorm\(0\)")?>Go</button>`)
	for _, tc := range []struct {
		name string
		n    *model.Node
		want bool // want an onclick
	}{
		{"bare", &model.Node{Type: "button", ID: "b", Label: "Go"}, false},
		{"with-press", &model.Node{Type: "button", ID: "b", Label: "Go", OnPress: &model.Invoke{Name: "save"}}, true},
		{"variant-only", &model.Node{Type: "button", ID: "b", Label: "Go", Props: map[string]any{"variant": "outlined"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := renderWidget(t, tc.n).HTML
			m := tagRe.FindString(h)
			if m == "" {
				t.Fatalf("button output changed for a node with no new props:\n%s", h)
			}
			if got := strings.Contains(m, "onclick"); got != tc.want {
				t.Errorf("onclick presence = %v, want %v:\n%s", got, tc.want, m)
			}
		})
	}
}

// ---- native constraint validation: textformfield error co-operation ---------

// TestTextFormFieldErrorEcho: once a field opts into native validation
// (required/pattern), its author-supplied `error` is echoed onto the native
// channels — aria-invalid for assistive tech, title for the browser's own
// pattern-mismatch bubble — without disturbing the existing inline message.
func TestTextFormFieldErrorEcho(t *testing.T) {
	cases := []struct {
		name    string
		props   map[string]any
		want    []string
		wantNot []string
	}{
		{"error-with-pattern", map[string]any{"pattern": `\d+`, "error": "Digits only"},
			[]string{` aria-invalid="true"`, ` title="Digits only"`, `>Digits only</span>`}, nil},
		{"error-with-required", map[string]any{"required": true, "error": "Required"},
			[]string{` aria-invalid="true"`, ` title="Required"`}, nil},
		// No native constraint: pre-existing behaviour, byte-for-byte.
		{"error-without-constraint", map[string]any{"error": "Bad"},
			[]string{`>Bad</span>`}, []string{"aria-invalid", "title="}},
		// Constraint but no error: nothing to echo.
		{"constraint-without-error", map[string]any{"required": true},
			[]string{` required`}, []string{"aria-invalid", "title="}},
		// An author title wins: a second title attribute on the same element is a
		// parse error, so the echo yields.
		{"author-title-not-duplicated", map[string]any{"required": true, "error": "Bad", "title": "Hint"},
			[]string{` aria-invalid="true"`, ` title="Hint"`}, []string{`title="Bad"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tff", Value: "x", Props: tc.props})
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
			if strings.Count(res.HTML, " title=") > 1 {
				t.Errorf("props %v: duplicate title attribute:\n%s", tc.props, res.HTML)
			}
		})
	}

	// The echoed title is an author string in a quoted attribute: it must be
	// entity-encoded like every other attribute value.
	t.Run("title-escaping", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tff", Props: map[string]any{
			"required": true, "error": `a" onmouseover="alert(1)`}})
		if strings.Contains(res.HTML, `" onmouseover=`) {
			t.Errorf("error text broke out of the title attribute:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, `title="a&#34; onmouseover=&#34;alert(1)"`) {
			t.Errorf("error title should be entity-encoded:\n%s", res.HTML)
		}
	})

	// A bound error keeps the echo reactive: it appears only while the binding
	// evaluates non-empty.
	t.Run("bound-error", func(t *testing.T) {
		node := func() *model.Node {
			return &model.Node{Type: "textformfield", ID: "tff", Value: "{{state.v}}",
				Props: map[string]any{"required": true, "error": "{{state.err}}"}}
		}
		bad := renderWidgetState(t, node(), map[string]any{"v": "x", "err": "Too short"}).HTML
		if !strings.Contains(bad, ` aria-invalid="true"`) || !strings.Contains(bad, ` title="Too short"`) {
			t.Errorf("bound error should echo natively:\n%s", bad)
		}
		ok := renderWidgetState(t, node(), map[string]any{"v": "xyz", "err": ""}).HTML
		if strings.Contains(ok, "aria-invalid") || strings.Contains(ok, " title=") {
			t.Errorf("empty bound error must echo nothing:\n%s", ok)
		}
	})
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
	for _, k := range []string{"zIndex", "alignSelf", "flexShrink", "x", "y"} {
		if !KnownStyleKeys[k] {
			t.Errorf("KnownStyleKeys must include %q (loader whitelist)", k)
		}
	}
}
