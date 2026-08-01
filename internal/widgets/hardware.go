package widgets

// Generic hardware-capability widgets (desktop-hardware example's interactive
// cards): one widget per capability stem, driven by the capability.All
// registry. v1 contract: the whole card is the button — a press invokes the
// stem's primary op through the host's native bridge and shows the result
// (plus syncing it into a bound state path). Async/long ops (bluetooth scan,
// recorder, sensors) are explicitly out: the bridge runs on the render
// thread, so only fast ops are invoked; the rest render a quiet note.

import (
	"fmt"
	"encoding/json"
	"image/color"
	"sync"
	"strings"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
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

// primaryOp picks the op a card-press invokes: the first READ-style op
// (Get/Info/Status/scan or the stem itself); *Set writes are skipped for v1
// (they need typed args the generic card cannot supply yet).
func (h Hardware) primaryOp() string {
	if slowStems[h.cap.Stem] || len(h.cap.Ops) == 0 {
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

// opArgs builds an op's argument map from node props and the bound value.
func (h Hardware) opArgs(n *model.Node, rt *runtime.Runtime, op string) map[string]any {
	propStr := func(k string) string {
		if v, ok := n.Prop(k); ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	switch op {
	case "speak":
		t := propStr("text")
		if t == "" {
			t = boundValue(n, rt)
		}
		return map[string]any{"text": t}
	case "openURL":
		u := propStr("url")
		if u == "" {
			u = boundValue(n, rt)
		}
		return map[string]any{"url": u}
	case "notify":
		return map[string]any{"title": propStr("title"), "body": propStr("text")}
	case "keepAwake":
		return map[string]any{"on": true}
	case "clipboardSet":
		return map[string]any{"text": boundValue(n, rt)}
	}
	return map[string]any{}
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

// label picks the button text: an explicit `action` prop, else a verb+stem
// derived from the primary op (HTML per-kind defaults like "Battery Level"
// read the same way). The capability Desc stays on the output line.
func (h Hardware) label(n *model.Node) string {
	if s, ok := n.Prop("action"); ok {
		if v := strings.TrimSpace(fmt.Sprint(s)); v != "" {
			return v
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
	case strings.Contains(op, "Speak"), op == "speak":
		verb = "Speak"
	case op == "openURL":
		verb = "Open"
	case op == "notify":
		verb = "Send"
	case strings.Contains(op, "Up"):
		verb = "Raise"
	case strings.Contains(op, "Down"):
		verb = "Lower"
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

// Measure: one output line + one action row.
func (h Hardware) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, hgt int) {
	if scale < 1 {
		scale = 1
	}
	w = 240 * scale
	hgt = (20 + 8 + 34) * scale // output + gap + button
	return
}

// Record: the output line (bound value, last result, or "—"), then the
// accent action button (or a muted note when there is nothing to invoke).
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

	label := h.label(ln.Node)
	op := h.primaryOp()
	btn := draw.NewRect()
	btnY := float64(20+8) * float64(scale)
	btnH := float64(34 * scale)
	btnW := float64(int(canvas.MeasureText(label, float64(fs))) + 32*scale)
	btn.X = 0
	btn.Y = btnY
	btn.Width = btnW
	btn.Height = btnH
	btn.BorderRadius = 10 * float64(scale)
	if op == "" {
		btn.Fill = themeColor(rt, "inputBg", color.RGBA{238, 238, 240, 255})
	} else {
		btn.Fill = themeColor(rt, "primary", color.RGBA{0, 122, 255, 255})
	}
	g.AddChild(btn)

	bt := draw.NewText()
	bt.Content = label
	bt.FontSize = float64(fs)
	if op == "" {
		bt.Fill = themeColor(rt, "textSecondary", color.RGBA{110, 110, 115, 255})
	} else {
		bt.Fill = color.RGBA{255, 255, 255, 255}
	}
	bt.X = float64(16 * scale)
	bt.Y = btnY + (btnH-float64(int(float64(fs)*1.2)))/2
	g.AddChild(bt)

	return g
}

// outputText: the bound state value when present, else the last result, else
// an em-dash (mirroring hwList's initial "—").
func (h Hardware) outputText(n *model.Node, rt *runtime.Runtime) string {
	if !canvas.NativeAvailable() {
		return "native bridge unavailable on this host"
	}
	if op := h.primaryOp(); op == "" {
		if slowStems[h.cap.Stem] {
			return h.cap.Desc + " (needs the async path — not invoked on canvas yet)"
		}
		if v := boundValue(n, rt); v != "" {
			return v
		}
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

// HandlePointer makes the whole card the button: a press invokes the primary
// op (fast, render-thread) and shows/syncs the result.
func (h Hardware) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction) bool {
	if p.Type != canvas.PointerPress {
		return false
	}
	op := h.primaryOp()
	if op == "" || !canvas.NativeAvailable() {
		return false
	}
	got := false
	canvas.InvokeNative(op, h.opArgs(n, rt, op), func(name string, arg any) {
		got = true
		text := formatResult(h.cap.Stem, arg)
		if path := boundStatePath(n); path != "" {
			rt.State[path] = text
		}
		lastResult.Store(n, text)
	})
	return got
}
