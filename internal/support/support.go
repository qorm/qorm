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
	{"Runtime", "Live state + actions + bindings", [7]Status{y, y, y, y, y, y, n}, "mini-program is static export only — no on-device runtime"},
	{"Runtime", "Expression bindings ({{ ... }})", [7]Status{y, y, y, y, y, y, n}, "arithmetic, comparisons, ternary, string ops, functions; mini-program is static export only (evaluated once at export)"},
	{"Runtime", "Conditional render + data-bound lists", [7]Status{y, y, y, y, y, y, n}, "if:, list repeat with {{item.*}} scope; mini-program is static export only (evaluated once at export)"},
	{"Runtime", "Go middle-layer (custom native ops)", [7]Status{y, y, y, y, y, y, n}, "one native/desktop.go into desktop AND mobile/web WASM"},
	{"Runtime", "Hardware / OS capabilities", [7]Status{y, y, y, y, p, p, p}, "per-capability support is in capabilities.md"},

	// Agent / AI
	{"Agent", "MCP server (read / edit / verify a live app)", [7]Status{y, y, y, y, y, y, n}, "stdio or /mcp against a running app; mini-program is static export only — no live tools apply"},
	{"Agent", "Live human-AI shared session (SSE)", [7]Status{y, y, y, y, y, y, n}, "AI edits appear in the human's browser instantly; the human's clicks show in qorm_activity"},
	{"Agent", "Review-bound edits (preview → apply)", [7]Status{y, y, y, y, y, y, n}, "apply_patch must carry the preview token; mini-program is static export only"},
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

var targetsZH = map[string]string{
	"Web":          "Web",
	"iOS":          "iOS",
	"Android":      "Android",
	"macOS":        "macOS",
	"Linux":        "Linux",
	"Windows":      "Windows",
	"Mini-program": "小程序",
}

var groupsZH = map[string]string{
	"Distribution": "分发 (Distribution)",
	"Rendering":    "渲染 (Rendering)",
	"Runtime":      "运行时 (Runtime)",
	"Agent":        "智能体 (Agent)",
}

var featuresZH = map[string]string{
	"Installable package":                          "可安装包 (Installable package)",
	"Offline / self-contained":                     "离线 / 自包含 (Offline / self-contained)",
	"PWA install (Add to Home Screen)":             "PWA 安装 (PWA install)",
	"Signed bundle (ed25519)":                      "签名包 (Signed bundle)",
	"Over-the-air update + rollback":               "热更新与回滚 (OTA)",
	"Declarative HTML/CSS render":                  "声明式 HTML/CSS 渲染",
	"Full widget set":                              "全量组件集 (Full widget set)",
	"Themes (Apple / Material / dark)":             "主题（Apple / Material / 深色）",
	"Custom components (JSON-defined)":             "自定义组件（JSON 定义）",
	"i18n messages + RTL":                          "多语言与 RTL 支持 (i18n / RTL)",
	"Native window (chromeless / transparent)":     "原生窗口（无边框/透明）",
	"System menu bar / tray / right-click menu":    "系统菜单栏/托盘/右键菜单",
	"Live state + actions + bindings":              "实时状态/动作/绑定",
	"Expression bindings ({{ ... }})":              "表达式绑定 (Expression bindings)",
	"Conditional render + data-bound lists":        "条件渲染与数据绑定列表",
	"Go middle-layer (custom native ops)":          "Go 中间层（自定义原生操作）",
	"Hardware / OS capabilities":                   "硬件与 OS 能力",
	"MCP server (read / edit / verify a live app)": "MCP 服务端（读取/编辑/验证）",
	"Live human-AI shared session (SSE)":           "人机共享实时会话 (SSE)",
	"Review-bound edits (preview → apply)":         "审查限制编辑 (preview → apply)",
	"Self-verify render (qorm measure / check)":    "自我验证渲染 (qorm measure / check)",
}

var notesZH = map[string]string{
	"Installable package":                          "桌面端为针对不同平台的 cgo 构建；小程序为微信小程序项目",
	"Offline / self-contained":                     "Web/移动端通过 Go→WASM 离线运行；小程序渲染静态 UI",
	"PWA install (Add to Home Screen)":             "Web 清单 + Service Worker；iOS/Android 支持添加到主屏幕",
	"Signed bundle (ed25519)":                      "纯 Go 自校验签名包；小程序由平台签名",
	"Over-the-air update + rollback":               "小程序更新受微信平台控制",
	"Declarative HTML/CSS render":                  "小程序渲染映射后的 WXML/WXSS",
	"Full widget set":                              "布局、输入、媒体、结构 —— 参见 widgets.md",
	"Themes (Apple / Material / dark)":             "设计 Token；小程序携带 Token WXSS",
	"Custom components (JSON-defined)":             "在 qorm.json 中声明，采用 {{prop.x}} 模板",
	"i18n messages + RTL":                          "ICU 消息、复数、货币、自右向左文本支持",
	"Native window (chromeless / transparent)":     "需使用 -tags desktop 编译；macOS 为参考实现",
	"Live state + actions + bindings":              "小程序为仅静态导出（static export only），设备端无运行时",
	"Expression bindings ({{ ... }})":              "算术、比较、三元、字符串操作、内置函数；小程序为仅静态导出（导出时求值一次）",
	"Conditional render + data-bound lists":        "if: 条件渲染，列表重复以及 {{item.*}} 作用域；小程序为仅静态导出（导出时求值一次）",
	"Go middle-layer (custom native ops)":          "将单个 native/desktop.go 编译入桌面端以及移动端/Web 的 WASM 中",
	"Hardware / OS capabilities":                   "各能力的详细支持情况见 capabilities.md",
	"MCP server (read / edit / verify a live app)": "对运行中的应用通过 stdio 或 /mcp 进行交互；小程序为仅静态导出（static export only），实时工具不适用",
	"Live human-AI shared session (SSE)":           "AI 的编辑立即显示在人类浏览器中；人类的点击和输入焦点反馈在 qorm_activity 中",
	"Review-bound edits (preview → apply)":         "应用补丁的 apply_patch 必须携带 preview token；小程序为仅静态导出（static export only）",
	"Self-verify render (qorm measure / check)":    "渲染应用并报告真实的几何空间布局",
}

// SummaryZH returns a compact, transposed Chinese summary table for README.zh.md.
func SummaryZH() string {
	cols := []struct{ label, feature string }{
		{"可安装包", "Installable package"},
		{"UI 渲染", "Declarative HTML/CSS render"},
		{"状态/动作", "Live state + actions + bindings"},
		{"智能体/MCP", "MCP server (read / edit / verify a live app)"},
	}
	byName := map[string]Feature{}
	for _, f := range Matrix {
		byName[f.Name] = f
	}
	var b strings.Builder
	b.WriteString("| 运行目标 |")
	for _, c := range cols {
		b.WriteString(" " + c.label + " |")
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(cols)) + "\n")
	for i, t := range Targets {
		zhT := targetsZH[t]
		if zhT == "" {
			zhT = t
		}
		b.WriteString("| " + zhT + " |")
		for _, c := range cols {
			b.WriteString(" " + byName[c.feature].Cells[i].mark() + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// MarkdownZH renders the support matrix as docs/zh/platforms/support-matrix.md.
func MarkdownZH() string {
	var b strings.Builder
	b.WriteString("# 平台支持矩阵\n\n")
	b.WriteString("> 由支持矩阵注册表自动生成 —— 请勿手动修改。\n\n")
	b.WriteString("QORM 在各运行目标上的支持情况一览。 **`ok`** = 已支持并测试； **`beta`** = 基础支持/部分或受限支持； **`—`** = 不适用。硬件/原生能力的详细支持情况请参见 [能力清单](capabilities.md)。\n\n")

	header := "| 特征/能力 |"
	sep := "|---|"
	for _, t := range Targets {
		zhT := targetsZH[t]
		if zhT == "" {
			zhT = t
		}
		header += " " + zhT + " |"
		sep += "---|"
	}

	var group string
	for _, f := range Matrix {
		if f.Group != group {
			group = f.Group
			zhG := groupsZH[group]
			if zhG == "" {
				zhG = group
			}
			b.WriteString("\n## " + zhG + "\n\n")
			b.WriteString(header + "\n" + sep + "\n")
		}
		zhName := featuresZH[f.Name]
		if zhName == "" {
			zhName = f.Name
		}
		row := "| " + zhName + " |"
		for _, c := range f.Cells {
			row += " " + c.mark() + " |"
		}
		b.WriteString(row + "\n")
	}

	// Notes
	b.WriteString("\n## 备注说明\n\n")
	for _, f := range Matrix {
		zhName := featuresZH[f.Name]
		if zhName == "" {
			zhName = f.Name
		}
		zhNote := notesZH[f.Name]
		if zhNote == "" && f.Note != "" {
			zhNote = f.Note
		}
		if zhNote != "" {
			b.WriteString("- **" + zhName + "** —— " + zhNote + "\n")
		}
	}
	return b.String()
}
