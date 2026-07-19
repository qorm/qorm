// Package measure joins a running app's self-reported layout (rects + computed
// styles) with the user's expressed node intent, and evaluates AI-authored
// checks against it — so an agent can completely and precisely interpret and
// verify what a user expressed. Shared by the CLI and the MCP agent surface.
package measure

import (
	"encoding/json"
	"fmt"
	"github.com/qorm/qorm/internal/render"
	"strings"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// NodeIndex maps id -> the user's expressed intent for that node.
func NodeIndex(n *model.Node, m map[string]map[string]any) {
	if n == nil {
		return
	}
	if n.ID != "" {
		e := map[string]any{"type": n.Type}
		if n.Text != "" {
			e["text"] = n.Text
		}
		if n.Label != "" {
			e["label"] = n.Label
		}
		if n.Value != "" {
			e["binding"] = n.Value
		}
		m[n.ID] = e
	}
	for _, c := range n.Children {
		NodeIndex(c, m)
	}
	NodeIndex(n.Template, m)
	// index both branches of a `when` node — whichever is live can be measured
	NodeIndex(n.Then, m)
	NodeIndex(n.Else, m)
}

func index(rt *qrt.Runtime) map[string]map[string]any {
	idx := map[string]map[string]any{}
	if root := rt.App.EntryRoot(); root != nil {
		NodeIndex(root, idx)
	}
	return idx
}

// intentText returns the text the user expressed for an intent entry.
func intentText(intent map[string]any) string {
	for _, k := range []string{"text", "label", "binding"} {
		if v, ok := intent[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Join returns the measured rows enriched with each node's type + expressed
// text, and returns them keyed by id.
func joinRows(rt *qrt.Runtime, measured []byte) ([]map[string]any, map[string]map[string]any) {
	idx := index(rt)
	var rows []map[string]any
	if len(measured) > 0 {
		json.Unmarshal(measured, &rows)
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		if intent, ok := idx[id]; ok {
			r["type"] = intent["type"]
			r["intent"] = intent
			r["intentText"] = intentText(intent)
		}
		byID[id] = r
	}
	return rows, byID
}

// Report renders the complete intent+result report as JSON.
func Report(rt *qrt.Runtime, measured []byte) ([]byte, error) {
	rows, _ := joinRows(rt, measured)
	return json.MarshalIndent(map[string]any{
		"app": rt.App.Name, "components": len(rows), "measured": rows,
	}, "", "  ")
}

// Eval verifies AI-expressed checks against the measured render and returns a
// pass/fail report with actual values.
func Eval(rt *qrt.Runtime, measured, checksJSON []byte) ([]byte, error) {
	_, byID := joinRows(rt, measured)
	var checks []map[string]any
	if err := json.Unmarshal(checksJSON, &checks); err != nil {
		return nil, fmt.Errorf("bad checks JSON: %w", err)
	}
	num := func(v any) (float64, bool) { f, ok := v.(float64); return f, ok }
	rectOf := func(r map[string]any) (x, y, w, h float64) {
		x, _ = num(r["x"])
		y, _ = num(r["y"])
		w, _ = num(r["w"])
		h, _ = num(r["h"])
		return
	}
	var results []map[string]any
	pass, fail := 0, 0
	for _, c := range checks {
		id, _ := c["id"].(string)
		r := byID[id]
		res := map[string]any{"id": id}
		var fails []string
		if r == nil {
			fails = append(fails, "not rendered (no element with this id)")
		} else {
			for k, want := range c {
				if k == "id" {
					continue
				}
				switch k {
				case "visible":
					if b, _ := r["visible"].(bool); b != (want == true) {
						fails = append(fails, fmt.Sprintf("visible=%v want %v", b, want))
					}
				case "type":
					if fmt.Sprint(r["type"]) != fmt.Sprint(want) {
						fails = append(fails, fmt.Sprintf("type=%v want %v", r["type"], want))
					}
				case "text":
					w := fmt.Sprint(want)
					if !strings.Contains(fmt.Sprint(r["intentText"]), w) && !strings.Contains(fmt.Sprint(r["text"]), w) {
						fails = append(fails, fmt.Sprintf("text %q/%q lacks %q", r["intentText"], r["text"], w))
					}
				case "noOverflow":
					if ov, _ := r["overflowX"].(bool); ov {
						fails = append(fails, "has x-overflow")
					}
				case "minW", "maxW", "minH", "maxH", "x", "y":
					wv, _ := num(want)
					var got float64
					switch k {
					case "minW", "maxW":
						got, _ = num(r["w"])
					case "minH", "maxH":
						got, _ = num(r["h"])
					case "x":
						got, _ = num(r["x"])
					case "y":
						got, _ = num(r["y"])
					}
					bad := (k == "minW" || k == "minH") && got < wv ||
						(k == "maxW" || k == "maxH") && got > wv ||
						(k == "x" || k == "y") && (got < wv-3 || got > wv+3)
					if bad {
						fails = append(fails, fmt.Sprintf("%s=%g want %g", k, got, wv))
					}
				case "within":
					p := byID[fmt.Sprint(want)]
					if p == nil {
						fails = append(fails, fmt.Sprintf("within: %v not found", want))
					} else {
						cx, cy, cw, ch := rectOf(r)
						px, py, pw, ph := rectOf(p)
						if cx < px-2 || cy < py-2 || cx+cw > px+pw+2 || cy+ch > py+ph+2 {
							fails = append(fails, fmt.Sprintf("not within %v", want))
						}
					}
				case "below":
					if p := byID[fmt.Sprint(want)]; p != nil {
						_, cy, _, _ := rectOf(r)
						_, py, _, ph := rectOf(p)
						if cy < py+ph-3 {
							fails = append(fails, fmt.Sprintf("not below %v", want))
						}
					}
				case "backgroundNot":
					if strings.Contains(fmt.Sprint(r["background"]), fmt.Sprint(want)) {
						fails = append(fails, fmt.Sprintf("background %v contains %v", r["background"], want))
					}
				case "colorNot":
					if strings.Contains(fmt.Sprint(r["color"]), fmt.Sprint(want)) {
						fails = append(fails, fmt.Sprintf("color %v contains %v", r["color"], want))
					}
				case "role":
					// The rendered DOM role (includes roles the renderer injects
					// implicitly, e.g. root=main, modal=dialog), not just what the
					// author wrote — so the assertion reflects what assistive tech sees.
					if fmt.Sprint(r["role"]) != fmt.Sprint(want) {
						fails = append(fails, fmt.Sprintf("role=%q want %q", r["role"], want))
					}
				case "hasAriaLabel":
					has := fmt.Sprint(r["ariaLabel"]) != ""
					if has != (want == true) {
						fails = append(fails, fmt.Sprintf("hasAriaLabel=%v want %v", has, want))
					}
				case "contrastRatio":
					// Minimum WCAG contrast (client computes it against the effective
					// background). WCAG AA: 4.5 for normal text, 3.0 for large text.
					wv, _ := num(want)
					got, ok := num(r["contrast"])
					if !ok || got == 0 {
						fails = append(fails, "contrastRatio unavailable (no client measurement)")
					} else if got < wv {
						fails = append(fails, fmt.Sprintf("contrastRatio=%.2f want >=%.2f", got, wv))
					}
				case "focusTrap":
					// Focus containment is a DYNAMIC behaviour (Tab-order simulation),
					// not a static snapshot. Reject rather than silently pass — a
					// verification tool must never report a check it cannot make.
					fails = append(fails, "focusTrap assertion not supported yet (dynamic; tracked in planning/real-env-acceptance.md)")
				}
			}
			res["actual"] = map[string]any{"x": r["x"], "y": r["y"], "w": r["w"], "h": r["h"],
				"visible": r["visible"], "type": r["type"], "color": r["color"], "background": r["background"],
				"role": r["role"], "ariaLabel": r["ariaLabel"], "contrast": r["contrast"]}
		}
		if len(fails) == 0 {
			res["pass"] = true
			pass++
		} else {
			res["pass"] = false
			res["fails"] = fails
			fail++
		}
		results = append(results, res)
	}
	return json.MarshalIndent(map[string]any{
		"app": rt.App.Name, "checks": len(checks), "passed": pass, "failed": fail,
		"ok": fail == 0, "results": results,
	}, "", "  ")
}

// hscrollDescendants collects ids of every node inside a horizontally scrolling
// or paged container (pageview/carousel/horizontal scroll) — their children are
// legitimately positioned off-screen, so they are exempt from bounds checks.
func hscrollDescendants(n *model.Node, inside bool, out map[string]bool) {
	if n == nil {
		return
	}
	paged := n.Type == "pageview" || n.Type == "carousel"
	if n.Type == "scroll" {
		if o, ok := n.Prop("orientation"); ok && fmt.Sprint(o) == "horizontal" {
			paged = true
		}
	}
	cur := inside || paged
	if cur && n.ID != "" {
		out[n.ID] = true
	}
	for _, c := range n.Children {
		hscrollDescendants(c, cur, out)
	}
	hscrollDescendants(n.Template, cur, out)
	hscrollDescendants(n.Then, cur, out)
	hscrollDescendants(n.Else, cur, out)
}

// Audit runs generic layout invariants over the measured render: every VISIBLE
// component must have a non-zero size and stay within the window (horizontal
// scroll/off-screen widgets excepted). Returns a pass/fail report — a one-shot
// regression check needing no hand-authored expectations.
func Audit(rt *qrt.Runtime, measured []byte) ([]byte, error) {
	rows, _ := joinRows(rt, measured)
	num := func(v any) float64 { f, _ := v.(float64); return f }
	// The stage may be centered in a wider window (desktop), so bounds are the
	// app container's actual box, not just its width at x=0. Prefer qorm-root
	// (the DOM container — always measured, whatever the scene's root node is
	// called); a scene node literally id'd "root" is the legacy fallback.
	var rootL float64 = 0
	var rootR float64 = 400
	for _, r := range rows {
		if id, _ := r["id"].(string); id == "root" {
			rootL = num(r["x"])
			rootR = num(r["x"]) + num(r["w"])
		}
	}
	for _, r := range rows {
		if id, _ := r["id"].(string); id == "qorm-root" {
			rootL = num(r["x"])
			rootR = num(r["x"]) + num(r["w"])
		}
	}
	scrollTypes := map[string]bool{"pageview": true, "carousel": true, "scroll": true, "table": true, "badge": true}
	exempt := map[string]bool{}
	if root := rt.App.EntryRoot(); root != nil {
		hscrollDescendants(root, false, exempt)
	}
	var issues []map[string]any
	visible := 0
	for _, r := range rows {
		id, _ := r["id"].(string)
		if id == "qorm-root" {
			continue // the bounds source itself, not a component
		}
		vis, _ := r["visible"].(bool)
		if !vis {
			continue
		}
		visible++
		typ := fmt.Sprint(r["type"])
		x, y, w, h := num(r["x"]), num(r["y"]), num(r["w"]), num(r["h"])
		add := func(kind, detail string) {
			issues = append(issues, map[string]any{"id": id, "type": typ, "kind": kind, "detail": detail})
		}
		if w <= 0 || h <= 0 {
			add("zero-size", fmt.Sprintf("%gx%g", w, h))
		}
		fixed := fmt.Sprint(r["position"]) == "fixed"
		if !scrollTypes[typ] && !exempt[id] && !fixed {
			if ov, _ := r["overflowX"].(bool); ov {
				add("x-overflow", "content wider than box")
			}
			if x+w > rootR+3 {
				add("out-of-bounds", fmt.Sprintf("right=%g > %g", x+w, rootR))
			}
			if x < rootL-3 {
				add("out-of-bounds", fmt.Sprintf("left=%g < %g", x, rootL))
			}
		}
		_ = y
	}
	// Unrecognised widget types (likely typos) — surfaced so a typo is caught by
	// self-verify, not shipped as a broken/empty container.
	for _, u := range render.Render(rt).Unknown {
		issues = append(issues, map[string]any{"id": "", "type": u, "kind": "unknown-widget", "detail": "unrecognised widget type — a typo? no renderer handles it"})
	}
	return json.MarshalIndent(map[string]any{
		"app": rt.App.Name, "visibleComponents": visible, "issues": len(issues),
		"ok": len(issues) == 0, "details": issues,
	}, "", "  ")
}
