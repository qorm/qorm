package integration

import (
	"bytes"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fenceGoDoc turns go-doc's tab-indented example blocks into fenced ```go code,
// because the docs-site markdown renderer only recognises fenced code, not
// indented code.
func fenceGoDoc(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var b strings.Builder
	inCode := false
	for _, ln := range lines {
		indented := strings.HasPrefix(ln, "\t")
		switch {
		case indented && !inCode:
			b.WriteString("```go\n")
			inCode = true
			b.WriteString(strings.TrimPrefix(ln, "\t") + "\n")
		case indented && inCode:
			b.WriteString(strings.TrimPrefix(ln, "\t") + "\n")
		case !indented && inCode && strings.TrimSpace(ln) == "":
			b.WriteString("\n") // blank line inside a code block
		case !indented && inCode:
			b.WriteString("```\n" + ln + "\n")
			inCode = false
		default:
			b.WriteString(ln + "\n")
		}
	}
	if inCode {
		b.WriteString("```\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- 1. Declarative UI contract: props.md -----------------------------------

// nodeSchema is the fixed set of top-level keys the loader reads off every node
// (internal/loader/loader.go buildNode); everything else falls into `props`.
var nodeSchema = [][4]string{
	{"type", "string", "widget name — see the [widget catalog](widgets.md)", "组件名——见[组件目录](widgets.md)"},
	{"id", "string", "stable node id (for state binding, patching, `data-state`)", "稳定的节点 id(用于状态绑定、补丁、`data-state`)"},
	{"text", "string", "text content (text/heading/paragraph nodes)", "文本内容(text/heading/paragraph 节点)"},
	{"label", "string", "button / control label", "按钮 / 控件标签"},
	{"placeholder", "string", "input placeholder", "输入框占位符"},
	{"value", "string", "input or bound value; may contain `{{ binding }}`", "输入值或绑定值;可含 `{{ binding }}`"},
	{"style", "object", "visual style — see [common style props](#common-style-props)", "视觉样式——见[通用样式属性](#通用样式属性)"},
	{"layout", "object", "layout hints: `width` `height` `align` `justify`", "布局提示:`width` `height` `align` `justify`"},
	{"onPress", "action / string", "press handler — an action id, or inline steps", "按下处理器——动作 id 或内联 steps"},
	{"onChange", "action / string", "change handler (inputs, toggles, sliders, selects)", "变化处理器(输入、开关、滑块、下拉)"},
	{"renderItem", "node", "item template for a bound `list`", "绑定 `list` 的条目模板"},
	{"data", "string", "list data-binding expression (e.g. `state.todos`)", "列表数据绑定表达式(如 `state.todos`)"},
	{"children", "node[]", "child nodes", "子节点"},
	{"…", "any", "any other key is a widget-specific **prop** (table below)", "其余任何键都是组件专有**属性**(见下表)"},
}

// commonStyle is the shared style vocabulary rendered by boxCSS/textCSS/
// containerCSS/a11y in internal/render/render_style.go — available on the nodes
// that render a styled box, regardless of type.
var commonStyle = []struct{ group, keys string }{
	{"Box (`style`)", "`width` `height` `minWidth` `maxWidth` `minHeight` `maxHeight` `padding` `margin` `gap` `background` `gradient` `borderRadius` `borderWidth` `borderColor` `shadow` `opacity` `aspectRatio` `flexGrow` `position` `top` `right` `bottom` `left` `cursor` `transition`"},
	{"Text (`style`)", "`color` `fontSize` `fontWeight` `fontFamily` `lineHeight` `letterSpacing` `fontStyle` `textDecoration` `textTransform` `textAlign` `lineClamp`"},
	{"Layout (`layout`)", "`width` `height` `align` `justify` (`wrap` on containers, `columns` on `grid`, `orientation` on `scroll`)"},
	{"Accessibility (top-level)", "`role` `ariaLabel` `title` `tooltip`"},
}

func TestAPIRefProps(t *testing.T) {
	rows := widgetPropTable(t)

	var en, zh strings.Builder
	writeGenHeader(&en, "en", "Node & Widget Props",
		"The declarative contract every QORM app is written against. A UI is a tree of **nodes**; each node is one JSON object.",
		"props table")
	writeGenHeader(&zh, "zh", "节点与组件属性",
		"每个 QORM 应用据以编写的声明式契约。界面是一棵**节点**树,每个节点是一个 JSON 对象。",
		"属性表")

	en.WriteString("## Node schema\n\nEvery node object may carry these top-level keys:\n\n| Key | Type | Meaning |\n|---|---|---|\n")
	zh.WriteString("## 节点结构\n\n每个节点对象可携带这些顶层键:\n\n| 键 | 类型 | 含义 |\n|---|---|---|\n")
	for _, r := range nodeSchema {
		en.WriteString("| `" + r[0] + "` | " + r[1] + " | " + r[2] + " |\n")
		zh.WriteString("| `" + r[0] + "` | " + r[1] + " | " + r[3] + " |\n")
	}

	en.WriteString("\n## Common style props\n\nRead by the shared renderer, so they work on any node that draws a box:\n\n")
	zh.WriteString("\n## 通用样式属性\n\n由共享渲染器读取,任何绘制盒子的节点都可用:\n\n")
	for _, g := range commonStyle {
		en.WriteString("- **" + g.group + "** — " + g.keys + "\n")
		zh.WriteString("- **" + g.group + "** — " + g.keys + "\n")
	}

	en.WriteString("\n## Per-widget props\n\nThe widget-specific keys each renderer reads, on top of the common style props above. Auto-extracted from the `node()` switch in `internal/render` — a `—` means the widget takes only common props.\n\n| Widget | Props |\n|---|---|\n")
	zh.WriteString("\n## 各组件专有属性\n\n在上述通用样式属性之外,每个渲染器额外读取的专有键。由 `internal/render` 的 `node()` 分发自动抽取——`—` 表示该组件只接受通用属性。\n\n| 组件 | 属性 |\n|---|---|\n")
	for _, r := range rows {
		line := "| `" + r[0] + "` | " + r[1] + " |\n"
		en.WriteString(line)
		zh.WriteString(line)
	}

	syncDoc(t, "../../api/props.md", en.String())
	syncDoc(t, "../../api/zh/props.md", zh.String())
}

// ---- 2. Actions & state: actions.md -----------------------------------------

// stepFields documents model.Step (internal/model). Which fields a step uses
// depends on its type — see the notes.
var stepFields = [][4]string{
	{"type", "string", "the step kind (table above) — required", "步骤类型(见上表)——必填"},
	{"path", "string", "target state path, e.g. `todos` or `user.name`", "目标状态路径,如 `todos` 或 `user.name`"},
	{"value", "string", "value expression; may contain `{{ bindings }}`", "值表达式;可含 `{{ bindings }}`"},
	{"match", "string", "expression selecting an array element (with `matchKey`)", "选中某个数组元素的表达式(配合 `matchKey`)"},
	{"matchKey", "string", "object key compared against `match` (default `id`)", "与 `match` 比较的对象键(默认 `id`)"},
	{"field", "string", "field to toggle/update within the matched object", "在匹配对象内切换 / 更新的字段"},
	{"item", "object", "field → value expressions for `state.appendObject`", "`state.appendObject` 的字段→值表达式"},
	{"to", "string", "`navigate`: target scene id · `state.move`: target index", "`navigate`:目标场景 id · `state.move`:目标索引"},
	{"back", "bool", "`navigate`: pop the back stack instead of pushing", "`navigate`:弹出返回栈而非压入"},
	{"from", "string", "`state.move`: source index", "`state.move`:源索引"},
	{"url", "string", "`http.*`: request URL (may contain `{{ bindings }}`)", "`http.*`:请求 URL(可含 `{{ bindings }}`)"},
	{"method", "string", "`http.request`: HTTP method override", "`http.request`:覆盖 HTTP 方法"},
	{"body", "string", "`http.*`: request body", "`http.*`:请求体"},
	{"headers", "object", "`http.*`: request headers", "`http.*`:请求头"},
	{"result", "string", "`http.*`: state path to store the parsed response", "`http.*`:存放解析后响应的状态路径"},
	{"error", "string", "`http.*`: state path to store an error message", "`http.*`:存放错误信息的状态路径"},
}

func stepDesc(typ string) (en, zh string) {
	m := map[string][2]string{
		"navigate":           {"go to another scene (or `back`)", "跳转到另一个场景(或 `back`)"},
		"state.set":          {"set a state path to a value", "把状态路径设为某值"},
		"state.append":       {"append a value to an array", "向数组追加一个值"},
		"state.appendObject": {"append an object (built from `item` field expressions)", "追加一个对象(由 `item` 字段表达式构建)"},
		"state.toggle":       {"flip a boolean, or a `field` on a matched array element", "翻转布尔值,或匹配数组元素上的某个 `field`"},
		"state.increment":    {"add to a number (`value` is the delta, default +1)", "对数字累加(`value` 为增量,默认 +1)"},
		"state.remove":       {"remove the array element selected by `match`", "移除 `match` 选中的数组元素"},
		"state.updateWhere":  {"update `field` on every element matching `match`", "更新所有匹配 `match` 的元素的 `field`"},
		"state.merge":        {"shallow-merge an object into a state path", "把一个对象浅合并进状态路径"},
		"state.sort":         {"sort an array by `field`", "按 `field` 对数组排序"},
		"state.move":         {"move an array element `from` index `to` index", "把数组元素从 `from` 移到 `to`"},
		"state.clear":        {"empty an array or clear a string/number", "清空数组,或清除字符串 / 数字"},
		"http.get":           {"GET a URL, store the parsed JSON at `result`", "GET 一个 URL,把解析后的 JSON 存到 `result`"},
		"http.post":          {"POST `body`, store the response at `result`", "POST `body`,把响应存到 `result`"},
		"http.put":           {"PUT `body`, store the response at `result`", "PUT `body`,把响应存到 `result`"},
		"http.delete":        {"DELETE a URL", "DELETE 一个 URL"},
		"http.request":       {"generic request with an explicit `method`", "带显式 `method` 的通用请求"},
	}
	v := m[typ]
	return v[0], v[1]
}

func TestAPIRefActions(t *testing.T) {
	types := stepTypes(t)

	var en, zh strings.Builder
	writeGenHeader(&en, "en", "Actions & State",
		"An action is `{ \"type\": \"action\", \"id\": …, \"steps\": [ … ] }`. Each step mutates state, calls a backend, or navigates. `onPress`/`onChange` run an action by id (or inline steps).",
		"step vocabulary")
	writeGenHeader(&zh, "zh", "动作与状态",
		"一个动作是 `{ \"type\": \"action\", \"id\": …, \"steps\": [ … ] }`。每个步骤修改状态、调用后端或导航。`onPress`/`onChange` 按 id 触发动作(或内联 steps)。",
		"步骤词汇表")

	en.WriteString("## Step types\n\nExtracted from the runtime dispatch (`internal/runtime`):\n\n| `type` | What it does |\n|---|---|\n")
	zh.WriteString("## 步骤类型\n\n从运行时分发(`internal/runtime`)抽取:\n\n| `type` | 作用 |\n|---|---|\n")
	for _, ty := range types {
		de, dz := stepDesc(ty)
		if de == "" {
			de, dz = "—", "—"
		}
		en.WriteString("| `" + ty + "` | " + de + " |\n")
		zh.WriteString("| `" + ty + "` | " + dz + " |\n")
	}

	en.WriteString("\n## Step fields\n\nEvery step is one JSON object; which fields apply depends on its `type`:\n\n| Field | Type | Used by |\n|---|---|---|\n")
	zh.WriteString("\n## 步骤字段\n\n每个步骤是一个 JSON 对象;哪些字段生效取决于其 `type`:\n\n| 字段 | 类型 | 用于 |\n|---|---|---|\n")
	for _, f := range stepFields {
		en.WriteString("| `" + f[0] + "` | " + f[1] + " | " + f[2] + " |\n")
		zh.WriteString("| `" + f[0] + "` | " + f[1] + " | " + f[3] + " |\n")
	}

	en.WriteString("\n```json\n// actions/addTodo.json — append a new object, then clear the input\n{ \"type\": \"action\", \"id\": \"addTodo\", \"steps\": [\n  { \"type\": \"state.appendObject\", \"path\": \"todos\",\n    \"item\": { \"id\": \"{{ now }}\", \"title\": \"{{ state.draft }}\", \"done\": \"false\" } },\n  { \"type\": \"state.set\", \"path\": \"draft\", \"value\": \"\" }\n] }\n```\n")
	zh.WriteString("\n```json\n// actions/addTodo.json — 追加一个新对象,然后清空输入\n{ \"type\": \"action\", \"id\": \"addTodo\", \"steps\": [\n  { \"type\": \"state.appendObject\", \"path\": \"todos\",\n    \"item\": { \"id\": \"{{ now }}\", \"title\": \"{{ state.draft }}\", \"done\": \"false\" } },\n  { \"type\": \"state.set\", \"path\": \"draft\", \"value\": \"\" }\n] }\n```\n")

	syncDoc(t, "../../api/actions.md", en.String())
	syncDoc(t, "../../api/zh/actions.md", zh.String())
}

// ---- 3. HTTP + SSE: http-api.md ---------------------------------------------

type routeDoc struct{ method, purpose, purposeZH string }

var routeDocs = map[string]routeDoc{
	"/":          {"GET", "the app shell — server-rendered HTML + the thin client runtime", "应用外壳——服务端渲染的 HTML + 轻量客户端运行时"},
	"/event":     {"POST", "dispatch a UI event (action / input change) and re-render", "派发一个 UI 事件(动作 / 输入变化)并重新渲染"},
	"/events":    {"GET (SSE)", "Server-Sent Events stream: the server pushes fresh HTML + log lines", "SSE 事件流:服务端推送最新 HTML + 日志行"},
	"/poll":      {"GET", "long-poll fallback when SSE is unavailable — returns the current revision + HTML if it advanced", "SSE 不可用时的长轮询兜底——返回当前修订号,若有更新则附带 HTML"},
	"/log":       {"GET / POST", "GET activity entries after `?since=`; POST forwards a client console line", "GET 拉取 `?since=` 之后的活动条目;POST 转发一条客户端控制台日志"},
	"/presence":  {"GET / POST", "collaboration presence — who (human/agent) is focused/typing where", "协作在场——谁(人 / 智能体)正聚焦或输入在何处"},
	"/console":   {"GET", "the log-window console feed page", "日志窗口的控制台信息流页面"},
	"/logwindow": {"GET", "the standalone log window that accompanies the desktop app", "伴随桌面应用的独立日志窗口"},
	"/window":    {"POST", "desktop window control (move / resize / open / close / focus)", "桌面窗口控制(移动 / 缩放 / 打开 / 关闭 / 聚焦)"},
	"/measure":   {"POST", "the browser reports the measured layout (x/y/w/h, computed style) of every node", "浏览器回报每个节点的实测布局(x/y/w/h、计算样式)"},
	"/mcp":       {"POST", "MCP JSON-RPC over HTTP — the same tools as `qorm mcp`, sharing the live runtime", "HTTP 上的 MCP JSON-RPC——与 `qorm mcp` 相同的工具,共享同一活动运行时"},
	"/update":    {"POST", "OTA: apply a new **signed** bundle to the running app", "OTA:向运行中的应用应用一个新的**已签名**捆绑包"},
	"/rollback":  {"POST", "revert to the previously running bundle", "回滚到上一个运行的捆绑包"},
	"/dev/state":     {"GET / POST", "DevTools state inspector: read or write the live app state", "DevTools 状态检查器：读取或修改运行中的应用状态"},
	"/dev/tree":      {"GET", "DevTools component tree: read the current scene's node tree JSON", "DevTools 组件树：读取当前场景的节点树 JSON"},
	"/dev/highlight": {"POST", "DevTools highlight event: broadcast a node highlight inspect signal to all clients", "DevTools 高亮事件：向所有客户端广播节点高亮检查信号"},
}

func TestAPIRefHTTP(t *testing.T) {
	routes := serverRoutes(t)

	var en, zh strings.Builder
	writeGenHeader(&en, "en", "HTTP & SSE",
		"`qorm run` serves the app and exposes a small HTTP surface: the browser talks to it, an AI agent reaches the MCP tools at `/mcp`, and OTA updates come in over `/update`. Endpoints that change state require a same-origin request.",
		"route table")
	writeGenHeader(&zh, "zh", "HTTP 与 SSE",
		"`qorm run` 提供应用服务并暴露一小组 HTTP 接口:浏览器与之通信,AI 智能体在 `/mcp` 使用 MCP 工具,OTA 更新经 `/update` 进入。改变状态的端点要求同源请求。",
		"路由表")

	en.WriteString("| Route | Method | Purpose |\n|---|---|---|\n")
	zh.WriteString("| 路由 | 方法 | 用途 |\n|---|---|---|\n")
	for _, r := range routes {
		d, ok := routeDocs[r]
		if !ok {
			t.Fatalf("route %s has no description in routeDocs — add one in apiref_pages_test.go", r)
		}
		en.WriteString("| `" + r + "` | " + d.method + " | " + d.purpose + " |\n")
		zh.WriteString("| `" + r + "` | " + d.method + " | " + d.purposeZH + " |\n")
	}

	en.WriteString("\n## The `/events` stream\n\nThe client opens `GET /events` and holds it open. The server writes one SSE message per change:\n\n```\n: connected\n\ndata: <html for the changed region>\n\n```\n\nEach `data:` frame carries the re-rendered HTML the client swaps in. Log and presence updates arrive on the same stream. When a proxy buffers SSE, the client falls back to `GET /poll?rev=<n>`.\n")
	zh.WriteString("\n## `/events` 事件流\n\n客户端打开 `GET /events` 并保持连接。服务端每次变化写入一条 SSE 消息:\n\n```\n: connected\n\ndata: <变化区域的 html>\n\n```\n\n每个 `data:` 帧携带客户端替换用的重渲染 HTML。日志与在场更新走同一条流。当代理缓冲 SSE 时,客户端回退到 `GET /poll?rev=<n>`。\n")

	syncDoc(t, "../../api/http-api.md", en.String())
	syncDoc(t, "../../api/zh/http-api.md", zh.String())
}

// serverRoutes extracts the registered mux paths from the server source, in
// registration order, so a new route can't ship undocumented.
func serverRoutes(t *testing.T) []string {
	t.Helper()
	src := readFile(t, "../../internal/server/server.go")
	re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatal("no routes found")
	}
	return out
}

// ---- 4. Go package API: go-api.md -------------------------------------------

func TestAPIRefGoAPI(t *testing.T) {
	const pkgDir = "../../pkg/qormext"
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var dpkg *doc.Package
	for _, p := range pkgs {
		dpkg = doc.New(p, "github.com/qorm/qorm/pkg/qormext", 0)
	}
	if dpkg == nil {
		t.Fatal("no package parsed")
	}

	sig := func(decl ast.Node) string {
		var b bytes.Buffer
		// print declaration without a body
		if fd, ok := decl.(*ast.FuncDecl); ok {
			cp := *fd
			cp.Body = nil
			cp.Doc = nil
			_ = printer.Fprint(&b, fset, &cp)
		} else {
			_ = printer.Fprint(&b, fset, decl)
		}
		return strings.TrimSpace(b.String())
	}

	build := func(lang, intro string) string {
		var b strings.Builder
		title := "Go package: qormext"
		if lang == "zh" {
			title = "Go 包:qormext"
		}
		writeGenHeaderFrom(&b, lang, title, intro, "github.com/qorm/qorm/pkg/qormext")
		b.WriteString("```go\nimport \"github.com/qorm/qorm/pkg/qormext\"\n```\n\n")
		b.WriteString(fenceGoDoc(dpkg.Doc) + "\n")

		if len(dpkg.Funcs) > 0 {
			b.WriteString("\n## Functions\n\n")
			funcs := append([]*doc.Func{}, dpkg.Funcs...)
			sort.Slice(funcs, func(i, j int) bool { return funcs[i].Name < funcs[j].Name })
			for _, fn := range funcs {
				b.WriteString("### `" + fn.Name + "`\n\n```go\n" + sig(fn.Decl) + "\n```\n\n")
				if d := fenceGoDoc(fn.Doc); d != "" {
					b.WriteString(d + "\n\n")
				}
			}
		}
		if len(dpkg.Types) > 0 {
			b.WriteString("## Types\n\n")
			for _, ty := range dpkg.Types {
				b.WriteString("### `" + ty.Name + "`\n\n```go\n" + sig(ty.Decl) + "\n```\n\n")
				if d := fenceGoDoc(ty.Doc); d != "" {
					b.WriteString(d + "\n\n")
				}
			}
		}
		return strings.TrimRight(b.String(), "\n") + "\n"
	}

	en := build("en", "The one public Go package. An app registers its **own** native ops (in Go) so the packager compiles them into the app's single executable — the desktop bridge dispatches unknown ops here.")
	zh := build("zh", "唯一的公开 Go 包。应用用 Go 注册**自己的**原生操作,打包器将其编译进应用的单一可执行文件——桌面桥接把未知操作分发到这里。")
	syncDoc(t, "../../api/go-api.md", en)
	syncDoc(t, "../../api/zh/go-api.md", zh)
}
