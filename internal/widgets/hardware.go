package widgets

// Generic hardware-capability widgets (desktop-hardware example's interactive
// cards): one widget per capability stem, driven by the capability.All
// registry. The action row mirrors the HTML path's per-kind UX (render.go):
// hwAdjust steppers for volume/brightness, a primary button for the rest.
// v1 contract: ops run synchronously on the render thread, so only fast ops
// are invoked; scan/record/cgo-only stems render an honest note instead of a
// dead button.

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"

	"github.com/qorm/platform/internal/capability"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/draw"
	"github.com/qorm/platform/internal/runtime"
)

func init() {
	for _, c := range capability.All {
		canvas.RegisterWidget(c.Widget, Hardware{cap: c})
	}
}

// Hardware is the generic capability widget (one instance per stem).
type Hardware struct {
	cap capability.Cap
}

// slowStems are the capabilities whose ops block (scans, streams, recording)
// or need OS permission flows unsuitable for a synchronous render-thread
// call — v1 renders a note instead of invoking.
var slowStems = map[string]bool{
	"camera": true, "recorder": true, "sensors": true, "bluetooth": true,
	"screenrecord": true, "nfc": true, "biometric": true, "location": true,
}

// unsupportedStems need the cgo bridge (AppKit): no pure-Go path exists, so
// the card shows an honest note instead of a dead button.
var unsupportedStems = map[string]bool{
	"wifi": true, "share": true, "screenshot": true, "screens": true,
	"badge": true, "loginitem": true, "filepicker": true, "storage": true,
	"systemmodes": true,
}

// primaryOp picks the op a card-press invokes: the first READ-style op
// (Get/Info/Status/scan or the stem itself); *Set writes are skipped for v1
// (they need typed args the generic card cannot supply yet).
func (h Hardware) primaryOp() string {
	if slowStems[h.cap.Stem] || unsupportedStems[h.cap.Stem] || len(h.cap.Ops) == 0 {
		return ""
	}
	for _, op := range h.cap.Ops {
		if strings.HasSuffix(op, "Set") {
			continue
		}
		return op
	}
	return ""
}

// label picks the button text for the generic single-button case: an
// explicit `action` prop, else verb+stem derived from the primary op (HTML
// per-kind defaults like "Battery Level" read the same way).
func (h Hardware) label(n *model.Node) string {
	if n != nil {
		if s, ok := n.Prop("action"); ok {
			if v := strings.TrimSpace(fmt.Sprint(s)); v != "" {
				return v
			}
		}
	}
	op := h.primaryOp()
	verb := "Run"
	switch {
	case op == "":
		verb = "Info"
	case strings.Contains(op, "Get"), strings.Contains(op, "Info"), strings.Contains(op, "Status"), op == h.cap.Stem:
		verb = "Get"
	case strings.Contains(op, "Scan"):
		verb = "Scan"
	case strings.Contains(op, "Toggle"):
		verb = "Toggle"
	case op == "speak":
		verb = "Speak"
	case op == "openURL":
		verb = "Open"
	case op == "notify":
		verb = "Send"
	}
	stem := h.cap.Stem
	if stem == "tts" {
		stem = "speech"
	}
	return verb + " " + strings.ToUpper(stem[:1]) + stem[1:]
}

// lastResult caches the most recent op result per node, so the output line
// survives re-renders when the widget has no bound state path. Entries live
// as long as their model nodes do (scene lifetime; R7's map-hygiene note
// accepted for v1).
var lastResult = &resultCache{m: map[*model.Node]string{}}

type resultCache struct {
	mu sync.RWMutex
	m  map[*model.Node]string
}

func (c *resultCache) Load(n *model.Node) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[n]
}

func (c *resultCache) Store(n *model.Node, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[n] = v
}

// hwBtn visual styles (HTML per-kind parity: hwAdjust steppers, primary
// action buttons, the recorder's destructive red).
const (
	hwBtnNeutral = iota // gray fill, label ink (the [-] stepper)
	hwBtnPrimary        // accent, white ink
	hwBtnDanger         // destructive red, white ink
)

// hwButton is one action a capability card offers.
type hwButton struct {
	label string
	op    string
	style int
	args  func(n *model.Node, rt *runtime.Runtime) map[string]any
}

// buttons is the per-stem UX spec, mirroring render.go's per-kind widgets
// (hwAdjust steppers, hwList single action).
func (h Hardware) buttons() []hwButton {
	none := func(n *model.Node, rt *runtime.Runtime) map[string]any { return map[string]any{} }
	switch h.cap.Stem {
	case "volume":
		return []hwButton{{"−", "volumeDown", hwBtnNeutral, none}, {"+", "volumeUp", hwBtnPrimary, none}}
	case "brightness":
		return []hwButton{{"−", "brightnessDown", hwBtnNeutral, none}, {"+", "brightnessUp", hwBtnPrimary, none}}
	case "keepawake":
		return []hwButton{{"Keep Screen Awake", "keepAwake", hwBtnPrimary,
			func(n *model.Node, rt *runtime.Runtime) map[string]any { return map[string]any{"on": true} }}}
	case "notify":
		return []hwButton{{"Send", "notify", hwBtnPrimary,
			func(n *model.Node, rt *runtime.Runtime) map[string]any {
				title, body := "QORM", ""
				if v, ok := n.Prop("title"); ok {
					title = strings.TrimSpace(fmt.Sprint(v))
				}
				if v, ok := n.Prop("text"); ok {
					body = strings.TrimSpace(fmt.Sprint(v))
				}
				return map[string]any{"title": title, "body": body}
			}}}
	case "clipboard":
		return []hwButton{
			{"Copy", "clipboardSet", hwBtnNeutral,
				func(n *model.Node, rt *runtime.Runtime) map[string]any {
					return map[string]any{"text": boundValue(n, rt)}
				}},
			{"Paste", "clipboardGet", hwBtnPrimary, none}}
	case "tts":
		return []hwButton{{"Speak", "speak", hwBtnPrimary,
			func(n *model.Node, rt *runtime.Runtime) map[string]any {
				t := ""
				if v, ok := n.Prop("text"); ok {
					t = strings.TrimSpace(fmt.Sprint(v))
				}
				if t == "" {
					t = boundValue(n, rt)
				}
				return map[string]any{"text": t}
			}}}
	case "openurl":
		return []hwButton{{"Open", "openURL", hwBtnPrimary,
			func(n *model.Node, rt *runtime.Runtime) map[string]any {
				u := ""
				if v, ok := n.Prop("url"); ok {
					u = strings.TrimSpace(fmt.Sprint(v))
				}
				if u == "" {
					u = boundValue(n, rt)
				}
				return map[string]any{"url": u}
			}}}
	case "securestorage":
		return []hwButton{{"Read", "secureGet", hwBtnPrimary,
			func(n *model.Node, rt *runtime.Runtime) map[string]any {
				k := ""
				if v, ok := n.Prop("key"); ok {
					k = strings.TrimSpace(fmt.Sprint(v))
				}
				return map[string]any{"key": k}
			}}}
	}
	// Every other fast stem: one primary button on its read op, labelled by
	// the verb+stem rule (or an explicit `action` prop).
	if op := h.primaryOp(); op != "" {
		return []hwButton{{h.label(nil), op, hwBtnPrimary, none}}
	}
	return nil
}

// Measure: one output line + one action row.
func (h Hardware) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, hgt int) {
	if scale < 1 {
		scale = 1
	}
	w = 240 * scale
	hgt = (20 + 8 + 34) * scale // output + gap + button row
	return
}

// rowGeom returns the action row's vertical band (top, bottom) inside the
// widget, in logical units scaled by sc.
func rowGeom(sc float64) (top, bot float64) {
	return 28 * sc, 62 * sc
}

// Record: the output line (bound value, last result, or "—"), then the
// action row of spec'd buttons (equal width, like hwAdjust's flex:1 pair).
func (h Hardware) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	fs := 14 * scale
	g := draw.NewGroup()

	out := draw.NewText()
	out.Content = h.outputText(ln.Node, rt)
	out.FontSize = float64(fs)
	out.Fill = themeColor(rt, "text", color.RGBA{30, 30, 32, 255})
	out.X = 0
	out.Y = 0
	g.AddChild(out)

	rowY, _ := rowGeom(float64(scale))
	btns := h.buttons()
	if len(btns) == 0 {
		note := draw.NewText()
		if slowStems[h.cap.Stem] {
			note.Content = h.cap.Desc + " (needs the async path — not invoked on canvas yet)"
		} else {
			note.Content = "not available on the pure-Go canvas build"
		}
		note.FontSize = float64(12 * scale)
		note.Fill = themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})
		note.X = 0
		note.Y = rowY + 8*float64(scale)
		g.AddChild(note)
		return g
	}

	gap := float64(8 * scale)
	btnH := float64(34 * scale)
	btnW := (float64(ln.Width) - float64(len(btns)-1)*gap) / float64(len(btns))
	for i, b := range btns {
		x := float64(i) * (btnW + gap)
		r := draw.NewRect()
		r.X = x
		r.Y = rowY
		r.Width = btnW
		r.Height = btnH
		r.BorderRadius = 10 * float64(scale)
		ink := color.RGBA{255, 255, 255, 255}
		switch b.style {
		case hwBtnPrimary:
			r.Fill = themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
		case hwBtnDanger:
			r.Fill = color.RGBA{255, 59, 48, 255}
		default:
			r.Fill = themeColor(rt, "inputBg", color.RGBA{238, 238, 240, 255})
			ink = themeColor(rt, "text", color.RGBA{30, 30, 32, 255})
		}
		g.AddChild(r)

		t := draw.NewText()
		t.Content = b.label
		t.FontSize = float64(fs)
		t.Fill = ink
		tw := int(canvas.MeasureText(b.label, float64(fs)))
		th := int(float64(fs) * 1.2)
		t.X = x + (btnW-float64(tw))/2
		t.Y = rowY + (btnH-float64(th))/2
		g.AddChild(t)
	}
	return g
}

// HandlePointer: a press inside the action row invokes that button's op with
// its spec'd args (fast, render-thread); the result syncs into bound state
// and the output cache. The engine supplies the widget's frame (seam v2), so
// the row hit-test uses the same geometry Record drew.
func (h Hardware) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, frame image.Rectangle) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	btns := h.buttons()
	if len(btns) == 0 || !canvas.NativeAvailable() {
		return false
	}
	// Recover the geometry scale: Measure pins height at 62*scale and flex
	// stretch only widens the box, so the height — not the width — carries
	// the true scale. (The old width/240 estimate mis-located the action row
	// on any stretched card: buttons went partly or fully dead.)
	sc := float64(frame.Dy()) / 62
	if sc < 1 {
		sc = 1
	}
	rowTop, rowBot := rowGeom(sc)
	relX := p.X - float64(frame.Min.X)
	relY := p.Y - float64(frame.Min.Y)
	if relY < rowTop || relY >= rowBot {
		return false // outside the action row: not ours (lets the card host it)
	}
	gap := 8 * sc
	btnW := (float64(frame.Dx()) - float64(len(btns)-1)*gap) / float64(len(btns))
	idx := int(relX / (btnW + gap))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(btns) {
		idx = len(btns) - 1
	}
	b := btns[idx]
	args := map[string]any{}
	if b.args != nil {
		args = b.args(n, rt)
	}
	got := false
	canvas.InvokeNative(b.op, args, func(name string, arg any) {
		got = true
		text := formatResult(h.cap.Stem, arg)
		if path := boundStatePath(n); path != "" {
			rt.State[path] = text
		}
		lastResult.Store(n, text)
	})
	return got
}

// outputText: the bound state value when present, else the last result, else
// an em-dash (mirroring hwList's initial "—").
func (h Hardware) outputText(n *model.Node, rt *runtime.Runtime) string {
	if !canvas.NativeAvailable() {
		return "native bridge unavailable on this host"
	}
	if slowStems[h.cap.Stem] || unsupportedStems[h.cap.Stem] {
		return h.cap.Desc
	}
	if v := boundValue(n, rt); v != "" {
		return v
	}
	if v := lastResult.Load(n); v != "" {
		return v
	}
	return "—"
}

// boundValue reads the current bound state value for the output line.
func boundValue(n *model.Node, rt *runtime.Runtime) string {
	path := boundStatePath(n)
	if path == "" || rt == nil {
		return ""
	}
	if v, ok := rt.State[path]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// boundStatePath extracts `x` from a value="{{state.x}}" prop (HTML
// boundPath semantics).
func boundStatePath(n *model.Node) string {
	v := strings.TrimSpace(n.Value)
	if strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}") {
		expr := strings.TrimSpace(v[2 : len(v)-2])
		if strings.HasPrefix(expr, "state.") {
			return strings.TrimPrefix(expr, "state.")
		}
	}
	return ""
}

// formatResult renders the callback payload for the output line: common JSON
// shapes get their human spelling (like the JS qormOn<Stem> formatters);
// anything else passes through raw.
func formatResult(stem string, arg any) string {
	s := fmt.Sprint(arg)
	switch stem {
	case "battery":
		var st struct {
			Level string `json:"level"`
			State string `json:"state"`
		}
		if json.Unmarshal([]byte(s), &st) == nil {
			if st.Level != "" {
				return st.Level + "% (" + st.State + ")"
			}
			if st.State == "ac" {
				return "AC power"
			}
		}
	case "network":
		var st struct {
			Online bool   `json:"online"`
			Type   string `json:"type"`
		}
		if json.Unmarshal([]byte(s), &st) == nil {
			if st.Online {
				return "Online (" + st.Type + ")"
			}
			return "Offline"
		}
	}
	return s
}
