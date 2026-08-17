package model

// AgentPolicy controls which MCP tools an agent may use against this app.
// Declared in qorm.json under "agent"."policy". When empty the runtime keeps
// backward-compatible full access (no manifest policy).
type AgentPolicy struct {
	// Level is a preset name: read-only, preview-only, operate, design, full.
	Level string
	// Tools overrides the preset per tool name. Values: true, false, or
	// "requiresPreview" (meaningful for qorm_apply_patch only).
	Tools map[string]bool
	// RequiresPreview marks tools that need a prior preview when set to true
	// in the manifest (Tools map stores bool; this holds tool names using the
	// string form "requiresPreview").
	RequiresPreview map[string]bool
	HostCall        HostCallPolicy
}

// HostCallPolicy gates dangerous host operations (e.g. qorm_window op=eval).
type HostCallPolicy struct {
	Allowed bool
	Ops     []string // empty with Allowed=true means all host ops; empty with Allowed=false means none
}

// HasPolicy reports whether the manifest declared an agent policy block.
func (p AgentPolicy) HasPolicy() bool {
	return p.Level != "" || len(p.Tools) > 0 || len(p.RequiresPreview) > 0 ||
		p.HostCall.Allowed || len(p.HostCall.Ops) > 0
}
