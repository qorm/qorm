package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// The renderer half of three input-side interactions whose behaviour lives in
// internal/server/app.js: debounced change dispatch, a custom "this field is
// required" message, and naming the button Enter should press. Each is a mount
// point — a data attribute the client re-reads at event time — and each is
// opt-in, so what these tests pin hardest is that a widget which declares none
// of them emits exactly the bytes it did before.

func inputNode(props map[string]any, onChange *model.Invoke) *model.Node {
	return &model.Node{Type: "input", ID: "q", Value: "{{state.q}}", Props: props, OnChange: onChange}
}

// TestInputDebounceWiring: `debounce` replaces the blur-time `onchange` with the
// delegated per-keystroke wiring, carrying the interval and the handler index
// the client re-reads when the timer fires.
func TestInputDebounceWiring(t *testing.T) {
	// Default: the plain onchange, unchanged.
	plain := renderWidget(t, inputNode(nil, &model.Invoke{Name: "search"}))
	if !strings.Contains(plain.HTML, ` onchange="qorm(0)"`) {
		t.Errorf("without `debounce` an onChange input must keep its onchange:\n%s", plain.HTML)
	}
	if strings.Contains(plain.HTML, "data-qorm-debounce") {
		t.Errorf("a plain input must gain no debounce markup:\n%s", plain.HTML)
	}

	deb := renderWidget(t, inputNode(map[string]any{"debounce": 300.0}, &model.Invoke{Name: "search"}))
	if !strings.Contains(deb.HTML, ` data-qorm-debounce="300" data-qorm-debounce-h="0"`) {
		t.Errorf("`debounce` must emit the interval and the handler index:\n%s", deb.HTML)
	}
	if strings.Contains(deb.HTML, "onchange=") {
		t.Errorf("a debounced control must not ALSO dispatch on blur (double dispatch):\n%s", deb.HTML)
	}
	// The handler is still registered, so the debounced dispatch reaches the
	// same action table entry a click would.
	if len(deb.Handlers) != 1 || deb.Handlers[0].Name != "search" {
		t.Errorf("the debounced onChange must register its action: %#v", deb.Handlers)
	}

	// Bound but no onChange: the debounce still applies, to the -1 state-sync.
	sync := renderWidget(t, inputNode(map[string]any{"debounce": 250.0}, nil))
	if !strings.Contains(sync.HTML, ` data-qorm-debounce="250" data-qorm-debounce-h="-1"`) {
		t.Errorf("a bound control with no onChange debounces the state sync:\n%s", sync.HTML)
	}
	// Neither bound nor handled: there is nothing to dispatch, so no wiring.
	none := renderWidget(t, &model.Node{Type: "input", ID: "q", Props: map[string]any{"debounce": 300.0}})
	if strings.Contains(none.HTML, "data-qorm-debounce") {
		t.Errorf("an unbound input with no onChange must emit no wiring:\n%s", none.HTML)
	}
	// A non-positive / non-numeric debounce is "no debounce", not "dispatch
	// instantly with a broken timer".
	for _, v := range []any{0.0, -5.0, "nope"} {
		res := renderWidget(t, inputNode(map[string]any{"debounce": v}, &model.Invoke{Name: "search"}))
		if strings.Contains(res.HTML, "data-qorm-debounce") || !strings.Contains(res.HTML, "onchange=") {
			t.Errorf("debounce %#v must fall back to the plain onchange:\n%s", v, res.HTML)
		}
	}
	// A bound debounce, so an agent can turn it on/off from state.
	bound := renderWidgetState(t, inputNode(map[string]any{"debounce": "{{state.ms}}"}, nil), map[string]any{"ms": 400.0})
	if !strings.Contains(bound.HTML, `data-qorm-debounce="400"`) {
		t.Errorf("a bound debounce must resolve against state:\n%s", bound.HTML)
	}
}

// TestDebounceAcrossControls: the wiring lives in the shared changeWiring, so
// every state-bound control in this file gets it — a textarea and a searchbar
// are the two that matter for typing.
func TestDebounceAcrossControls(t *testing.T) {
	for _, w := range []string{"textarea", "searchbar", "select", "slider"} {
		res := renderWidget(t, &model.Node{Type: w, ID: "c", Value: "{{state.v}}",
			Props: map[string]any{"debounce": 200.0, "options": []any{"a", "b"}}})
		if !strings.Contains(res.HTML, `data-qorm-debounce="200"`) {
			t.Errorf("%s must honour `debounce`:\n%s", w, res.HTML)
		}
	}
	// The searchbar's own client-side filtering is untouched by the debounce —
	// typing still filters the panel instantly, only the DISPATCH waits.
	sb := renderWidget(t, &model.Node{Type: "searchbar", ID: "s", Value: "{{state.q}}",
		Props: map[string]any{"debounce": 300.0}})
	if !strings.Contains(sb.HTML, `oninput="qormSearch(this)"`) {
		t.Errorf("debounce must not displace the searchbar's local filtering:\n%s", sb.HTML)
	}
}

// TestRequiredMessage: the custom valueMissing wording travels as
// data-qorm-error, only on a field that is actually `required`, and only when
// the app supplies it.
func TestRequiredMessage(t *testing.T) {
	req := renderWidget(t, &model.Node{Type: "input", ID: "e", Value: "{{state.e}}",
		Props: map[string]any{"required": true, "requiredMessage": "We need your email"}})
	if !strings.Contains(req.HTML, ` required data-qorm-error="We need your email"`) {
		t.Errorf("`requiredMessage` must ride next to the native required:\n%s", req.HTML)
	}
	// Not required: nothing to say "you left this empty" about.
	opt := renderWidget(t, &model.Node{Type: "input", ID: "e",
		Props: map[string]any{"requiredMessage": "We need your email"}})
	if strings.Contains(opt.HTML, "data-qorm-error") {
		t.Errorf("a field that is not `required` must not carry a required message:\n%s", opt.HTML)
	}
	// Required alone is byte-identical to before the message existed.
	bare := renderWidget(t, &model.Node{Type: "input", ID: "e", Props: map[string]any{"required": true}})
	if strings.Contains(bare.HTML, "data-qorm-error") {
		t.Errorf("`required` alone must gain nothing:\n%s", bare.HTML)
	}
	// It is an expression like every other prop, and it is escaped into the
	// attribute rather than concatenated.
	dyn := renderWidgetState(t, &model.Node{Type: "textarea", ID: "b", Value: "{{state.b}}",
		Props: map[string]any{"required": true, "requiredMessage": "{{state.msg}}"}},
		map[string]any{"msg": `say "hi" & <b>`})
	if !strings.Contains(dyn.HTML, `data-qorm-error="say &#34;hi&#34; &amp; &lt;b&gt;"`) {
		t.Errorf("the message must interpolate and escape:\n%s", dyn.HTML)
	}
	// textformfield shares inputAttrs, and its reactive `error` echo (title +
	// aria-invalid) composes with the required message rather than replacing it.
	tff := renderWidget(t, &model.Node{Type: "textformfield", ID: "t", Value: "{{state.t}}",
		Props: map[string]any{"required": true, "requiredMessage": "Required", "error": "Bad"}})
	for _, want := range []string{`data-qorm-error="Required"`, `aria-invalid="true"`, `title="Bad"`} {
		if !strings.Contains(tff.HTML, want) {
			t.Errorf("textformfield lacks %q:\n%s", want, tff.HTML)
		}
	}
}

// TestSubmitOnEnter: naming the Enter target is opt-in and does not touch the
// button's own wiring — the client clicks the button, so a `submit: true`
// button keeps its native validity gate.
func TestSubmitOnEnter(t *testing.T) {
	plain := renderWidget(t, &model.Node{Type: "button", ID: "b", Label: "Save",
		OnPress: &model.Invoke{Name: "save"}})
	if strings.Contains(plain.HTML, "data-qorm-enter") {
		t.Errorf("a plain button must not claim the Enter key:\n%s", plain.HTML)
	}
	on := renderWidget(t, &model.Node{Type: "button", ID: "b", Label: "Save",
		Props: map[string]any{"submitOnEnter": true}, OnPress: &model.Invoke{Name: "save"}})
	if !strings.Contains(on.HTML, ` data-qorm-enter onclick="qorm(0)"`) {
		t.Errorf("`submitOnEnter` must mark the button and keep its own press wiring:\n%s", on.HTML)
	}
	gated := renderWidget(t, &model.Node{Type: "button", ID: "b", Label: "Save",
		Props: map[string]any{"submitOnEnter": true, "submit": true}, OnPress: &model.Invoke{Name: "save"}})
	if !strings.Contains(gated.HTML, `type="submit"`) || !strings.Contains(gated.HTML, "reportValidity()") {
		t.Errorf("the Enter target must keep the native validity gate:\n%s", gated.HTML)
	}
}
