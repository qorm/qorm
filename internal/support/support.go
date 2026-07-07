// Package support is the single source of truth for QORM's platform × feature
// support matrix — which capabilities are available and tested on which target,
// rendered to docs/platforms/support-matrix.md and kept in sync by a test so the
// human-facing "what works where" table never drifts from reality.
package support

import "strings"

// Status is a cell's support level.
type Status int

const (
	No      Status = iota // not applicable / not supported
	Yes                   // supported and tested
	Partial               // implemented as a foundation / partial or unverified
)

func (s Status) mark() string {
	switch s {
	case Yes:
		return "ok"
	case Partial:
		return "beta"
	default:
		return "—"
	}
}

// Targets are the columns, in display order.
var Targets = []string{"Web", "iOS", "Android", "macOS", "Linux", "Windows", "Mini-program"}

// Feature is one row: its group, name, per-target status (aligned with Targets),
// and an optional note.
type Feature struct {
	Group string
	Name  string
	Cells [7]Status
	Note  string
}

// cell shorthands
const (
	y = Yes
	p = Partial
	n = No
)

// Matrix is the declared support matrix. `Yes` means implemented and covered by
// tests / verified working; `Partial` means a foundation or platform-limited
// path; `No` means not applicable.
var Matrix = []Feature{
	// Distribution
	{"Distribution", "Installable package", [7]Status{y, y, y, y, p, p, y}, "desktop is a per-platform cgo build; mini-program is a WeChat project"},
	{"Distribution", "Offline / self-contained", [7]Status{y, y, y, y, y, y, p}, "web/mobile run offline via Go→WASM; mini-program renders static UI"},
	{"Distribution", "PWA install (Add to Home Screen)", [7]Status{y, y, p, n, n, n, n}, "web manifest + service worker; iOS/Android add-to-home"},
	{"Distribution", "Signed bundle (ed25519)", [7]Status{y, y, y, y, y, y, n}, "pure-Go verify-the-bundle; mini-programs are vendor-signed"},
	{"Distribution", "Over-the-air update + rollback", [7]Status{y, y, y, y, y, y, n}, "mini-program updates are vendor-controlled"},

	// Rendering
	{"Rendering", "Declarative HTML/CSS render", [7]Status{y, y, y, y, y, y, p}, "mini-program renders remapped WXML/WXSS"},
	{"Rendering", "Full widget set", [7]Status{y, y, y, y, y, y, p}, "layout, input, media, structure — see widgets.md"},
	{"Rendering", "Themes (Apple / Material / dark)", [7]Status{y, y, y, y, y, y, p}, "design tokens; mini-program carries the token WXSS"},
	{"Rendering", "Custom components (JSON-defined)", [7]Status{y, y, y, y, y, y, p}, "declared in qorm.json, {{prop.x}} templates"},
	{"Rendering", "i18n messages + RTL", [7]Status{y, y, y, y, y, y, p}, "ICU messages, plurals, currency, right-to-left"},
	{"Rendering", "Native window (chromeless / transparent)", [7]Status{n, n, n, y, p, p, n}, "-tags desktop; macOS is the reference"},
	{"Rendering", "System menu bar / tray / right-click menu", [7]Status{n, n, n, y, p, p, n}, ""},

	// Runtime
	{"Runtime", "Live state + actions + bindings", [7]Status{y, y, y, y, y, y, p}, "mini-program is static in the foundation slice"},
	{"Runtime", "Expression bindings ({{ ... }})", [7]Status{y, y, y, y, y, y, p}, "arithmetic, comparisons, ternary, string ops, functions"},
	{"Runtime", "Conditional render + data-bound lists", [7]Status{y, y, y, y, y, y, p}, "if:, list repeat with {{item.*}} scope"},
	{"Runtime", "Go middle-layer (custom native ops)", [7]Status{y, y, y, y, y, y, n}, "one native/desktop.go into desktop AND mobile/web WASM"},
	{"Runtime", "Hardware / OS capabilities", [7]Status{y, y, y, y, p, p, p}, "per-capability support is in capabilities.md"},

	// Agent / AI
	{"Agent", "MCP server (read / edit / verify a live app)", [7]Status{y, y, y, y, y, y, p}, "stdio or /mcp against a running app"},
	{"Agent", "Live human-AI shared session (SSE)", [7]Status{y, y, y, y, y, y, n}, "AI edits appear in the human's browser instantly; the human's clicks show in qorm_activity"},
	{"Agent", "Review-bound edits (preview → apply)", [7]Status{y, y, y, y, y, y, y}, "apply_patch must carry the preview token"},
	{"Agent", "Self-verify render (qorm measure / check)", [7]Status{y, y, y, y, y, y, n}, "renders the app and reports real geometry"},
}

// Summary is a compact, transposed glance table (targets as rows, four headline
// capabilities as columns) for the top-level README. The full detail is Markdown().
func Summary() string {
	cols := []struct{ label, feature string }{
		{"Package", "Installable package"},
		{"Render", "Declarative HTML/CSS render"},
		{"Live app", "Live state + actions + bindings"},
		{"Agent / MCP", "MCP server (read / edit / verify a live app)"},
	}
	byName := map[string]Feature{}
	for _, f := range Matrix {
		byName[f.Name] = f
	}
	var b strings.Builder
	b.WriteString("| Target |")
	for _, c := range cols {
		b.WriteString(" " + c.label + " |")
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(cols)) + "\n")
	for i, t := range Targets {
		b.WriteString("| " + t + " |")
		for _, c := range cols {
			b.WriteString(" " + byName[c.feature].Cells[i].mark() + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Markdown renders the matrix as docs/platforms/support-matrix.md.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# Platform support matrix\n\n")
	b.WriteString("> Auto-generated from the support registry — do not edit by hand.\n\n")
	b.WriteString("What QORM supports on each target, at a glance. **`ok`** = supported and tested; **`beta`** = foundation / partial or platform-limited; **`—`** = not applicable. Per-capability hardware detail is in [capabilities.md](capabilities.md).\n\n")

	header := "| Feature |"
	sep := "|---|"
	for _, t := range Targets {
		header += " " + t + " |"
		sep += "---|"
	}

	var group string
	for _, f := range Matrix {
		if f.Group != group {
			group = f.Group
			b.WriteString("\n## " + group + "\n\n")
			b.WriteString(header + "\n" + sep + "\n")
		}
		row := "| " + f.Name + " |"
		for _, c := range f.Cells {
			row += " " + c.mark() + " |"
		}
		b.WriteString(row + "\n")
	}

	// Notes
	b.WriteString("\n## Notes\n\n")
	for _, f := range Matrix {
		if f.Note != "" {
			b.WriteString("- **" + f.Name + "** — " + f.Note + "\n")
		}
	}
	return b.String()
}
