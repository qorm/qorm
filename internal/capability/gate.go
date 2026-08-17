package capability

import (
	"encoding/json"
	"sort"

	"github.com/qorm/qorm/internal/model"
)

// frameworkOps are runtime housekeeping calls not tied to a single capability
// widget but required for normal app boot when a gate is active.
var frameworkOps = []string{
	"platform", "pendingShortcut", "getInsets", "getModes", "updateWidget",
	"winDragStart", "winDragMove",
}

// CapPolicyJSON is the shape injected as window.__qormCapPolicy in the page.
type CapPolicyJSON struct {
	Enforce   bool     `json:"enforce"`
	Ops       []string `json:"ops,omitempty"`
	Deny      []string `json:"deny,omitempty"`
	CustomOps []string `json:"customOps,omitempty"`
}

// EnforceRuntimeGate reports whether call-time qormToNative adjudication applies.
func EnforceRuntimeGate(app *model.App) bool {
	if app == nil {
		return false
	}
	if app.Capabilities.Mode == "open" {
		return false
	}
	if app.Capabilities.HasPolicy() {
		return true
	}
	return len(app.RequiredCapabilities) > 0
}

// CapPolicyFor builds the client-side policy for an app.
func CapPolicyFor(app *model.App) CapPolicyJSON {
	if !EnforceRuntimeGate(app) {
		return CapPolicyJSON{Enforce: false}
	}
	allowed := allowedOps(app)
	deny := expandDeny(app)
	return CapPolicyJSON{
		Enforce:   true,
		Ops:       allowed,
		Deny:      deny,
		CustomOps: append([]string(nil), app.Capabilities.CustomOps...),
	}
}

// CapPolicyScript returns JS initializing window.__qormCapPolicy, or "null".
func CapPolicyScript(app *model.App) string {
	p := CapPolicyFor(app)
	if !p.Enforce {
		return "null"
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// AuthorizeOp rejects a qormToNative op when the runtime gate is active.
func AuthorizeOp(app *model.App, op string) error {
	if app == nil || op == "" || !EnforceRuntimeGate(app) {
		return nil
	}
	for _, d := range expandDeny(app) {
		if d == op {
			return &GateError{Op: op, Reason: "denied by capabilities policy"}
		}
	}
	for _, fo := range frameworkOps {
		if fo == op {
			return nil
		}
	}
	for _, custom := range app.Capabilities.CustomOps {
		if custom == op {
			return nil
		}
	}
	for _, allowed := range allowedOps(app) {
		if allowed == op {
			return nil
		}
	}
	return &GateError{Op: op, Reason: "not in capability allowlist"}
}

// GateError describes a blocked native op.
type GateError struct {
	Op     string
	Reason string
}

func (e *GateError) Error() string {
	return "capability gate: " + e.Reason + ": " + e.Op
}

func allowedOps(app *model.App) []string {
	stems := allowedStems(app)
	ops := map[string]bool{}
	for _, s := range stems {
		for _, op := range opsForStem(s) {
			ops[op] = true
		}
	}
	for _, fo := range frameworkOps {
		ops[fo] = true
	}
	out := make([]string, 0, len(ops))
	for op := range ops {
		out = append(out, op)
	}
	sort.Strings(out)
	return out
}

func allowedStems(app *model.App) []string {
	mode := app.Capabilities.Mode
	if mode == "" || mode == "used-only" {
		used := StemsFromApp(app)
		merged := MergeStems(used, app.RequiredCapabilities)
		merged = MergeStems(merged, app.Capabilities.Allow)
		return subtractStems(merged, app.Capabilities.Deny)
	}
	if mode == "manifest" {
		allow := app.Capabilities.Allow
		if len(allow) == 0 {
			allow = append([]string(nil), app.RequiredCapabilities...)
		}
		return subtractStems(allow, app.Capabilities.Deny)
	}
	return nil
}

func subtractStems(stems, deny []string) []string {
	if len(deny) == 0 {
		return stems
	}
	denySet := map[string]bool{}
	for _, d := range deny {
		denySet[d] = true
		if c := byStem[d]; c != nil {
			denySet[c.Widget] = true
		}
	}
	out := make([]string, 0, len(stems))
	for _, s := range stems {
		if !denySet[s] {
			out = append(out, s)
		}
	}
	return out
}

func expandDeny(app *model.App) []string {
	if len(app.Capabilities.Deny) == 0 {
		return nil
	}
	ops := map[string]bool{}
	for _, d := range app.Capabilities.Deny {
		ops[d] = true
		if c := byStem[d]; c != nil {
			for _, op := range c.Ops {
				ops[op] = true
			}
		} else if c := byWidget[d]; c != nil {
			for _, op := range c.Ops {
				ops[op] = true
			}
		}
	}
	out := make([]string, 0, len(ops))
	for op := range ops {
		out = append(out, op)
	}
	sort.Strings(out)
	return out
}

func opsForStem(stem string) []string {
	c := byStem[stem]
	if c == nil {
		return nil
	}
	if len(c.Ops) == 0 {
		return []string{stem}
	}
	return append([]string(nil), c.Ops...)
}
