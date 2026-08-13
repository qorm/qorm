<!-- data-lang-nav --> [English](../verification.md) · 中文

# 解释并验证一个 QORM 应用

QORM 的目标是让 AI 能够**完整而精确地解释并验证**用户在应用中表达的一切——
它的布局、样式、行为和翻译——使用框架本身,无需外部浏览器。

测量有两个来源,但输出同一种 JSON 形状:

- 默认纯 Go CLI 在软件 canvas 中离屏渲染一次,然后从同一棵保留模式渲染图
  导出布局和计算样式。该结果确定且不需要窗口或外部浏览器。
- 实时宿主测量人正在看的内容。macOS 原生 canvas 窗口在渲染帧后导出其图;
  浏览器/WebView 则遍历 DOM,记录 `getBoundingClientRect` 和计算样式,再 POST 到 `/measure`。

框架把两种渲染结果与用户的**意图**(应用 JSON 中节点的类型、文本和状态绑定)
连接起来。因此每个组件都同时给出*用户所要求的*和*所选后端实际渲染的*。

## `qorm measure`——读取真实渲染

```bash
qorm measure <app-dir> [--width 400] [--physical] [-o report.json]
```

渲染应用、自我测量,并每个组件打印一行,将意图与结果连接起来:

```json
{ "id": "wifi", "type": "switchlisttile", "intent": {"label": "Wi-Fi", "binding": "{{state.wifi}}"},
  "x": 32, "y": 499, "w": 336, "h": 47, "visible": true,
  "color": "rgb(0,0,0)", "background": "rgba(0,0,0,0)", "fontSize": "15px",
  "padding": "…", "borderRadius": "…", "overflowX": false }
```

每个组件的字段:`id`、`type`、`intent`(text/label/binding)、`x y w h`、
`visible`、`tag`、`text`(用于叶子节点),以及计算出的 `color`、`background`、
`fontSize`、`fontWeight`、`textAlign`、`padding`、`margin`、`borderRadius`、
`border`、`opacity`、`zIndex`、`position`、`overflowX`。

普通构建下,该命令按指定逻辑宽度进行无窗口纯 Go canvas 渲染(高度来自
`qorm.json`,默认 820)。坐标默认是逻辑 CSS 像素;`--physical` 保留 canvas 设备像素。
`-tags desktop` 构建则刻意走 HTML/WebView 路径;WebView 始终回报 CSS 像素,
因此 `--physical` 在该路径无效。

## `qorm check --checks`——验证期望

```bash
qorm check <app-dir> --checks checks.json [--width 400] [--physical] [-o report.json]
```

`checks.json` 是一个 `{id, <assertion>…}` 的数组。每个断言都针对真实渲染进行验证;
报告给出每项检查的通过/失败以及实际值。

| assertion | meaning |
|---|---|
| `visible: true\|false` | 组件实际可见 / 不可见 |
| `type: "<widget>"` | 由预期的节点类型渲染而来 |
| `text: "<s>"` | 包含 `<s>`(对表达的文本或渲染的文本进行匹配) |
| `noOverflow: true` | 无水平内容溢出 |
| `minW / maxW / minH / maxH: <px>` | 尺寸在界限之内 |
| `x / y: <px>` | 位置(±3px 容差) |
| `within: "<id>"` | 该组件的盒子位于那个 id 的盒子之内 |
| `below: "<id>"` | 起始位置在那个 id 的下方 |
| `backgroundNot / colorNot: "<substr>"` | 那个子串**不存在**(例如用 `"255, 255, 255"` 来断言深色模式下非白色) |
| `role: "<role>"` | 渲染后的 ARIA role(含渲染器隐式注入的,如 root→`main`、modal→`dialog`) |
| `hasAriaLabel: true` | 元素带有 `aria-label` |
| `contrastRatio: <n>` | 文本/背景对比度至少为 `n`(WCAG AA:正文 4.5、大字号 3.0),针对有效背景色计算 |

后端边界:几何、可见性、文本、溢出和上述盒模型/计算样式在两个来源都可用。
HTML/WebView 的无障碍断言读取**渲染后的 DOM**,所以 `role` 可包含渲染器
隐式注入的语义,`contrastRatio` 按有效背景计算。canvas 图当前回报作者显式声明的
`role` / `ariaLabel`,尚不计算对比度;因此 `contrastRatio` 会明确报不可用,
而不是静默通过。`focusTrap` 在所有后端都被刻意拒绝:它是动态 Tab 键序行为,
不是静态快照。

检查明确失败:无法识别的断言键(如拼写错误)会判定为失败,未被测量的
`within`/`below` 目标 id 会以 'not found' 判定为失败 —— 绝不会有检查项被
静默通过。

```json
[
  {"id": "nav",      "type": "appbar", "visible": true, "y": 0, "text": "Today"},
  {"id": "wifi",     "type": "switchlisttile", "visible": true, "within": "settings"},
  {"id": "chart",    "noOverflow": true, "maxW": 370}
]
```

## `qorm check` 步骤流——验证行为

传入一个 `{"steps":[…]}` 对象而非数组,以验证*交互*:每一步应用一个 action,
等待重新渲染 + 重新测量,然后进行检查。

```json
{ "steps": [
  { "name": "increment", "do": {"dispatch": "increment"}, "checks": [{"id": "number", "text": "1"}] },
  { "name": "go dark",   "do": {"setState": {"path": "theme", "value": "dark"}},
    "checks": [{"id": "card", "backgroundNot": "255, 255, 255"}] }
] }
```

默认纯 Canvas 驱动器要求每一步的 `do` 恰好包含一个操作:

- `{"dispatch":"<动作>", "args":{…}}`
- `{"setState":{"path":…, "value":…}}`(或更短的 `state` 别名)
- `{"press":"id"}` 或 `{"press":{"id":"id"}}`
- `{"type":{"id":"field", "text":"hello", "clear":true}}`
- `{"key":"Enter"}` 或 `{"key":{"key":"Tab", "id":"field",
  "shift":false, "ctrl":false, "alt":false, "meta":false}}`
- `{"scroll":{"id":"list", "dx":0, "dy":120, "ctrl":false}}`
- `{"wait":250}`(毫秒)或 `{"wait":"250ms"}`(Go duration)

press/type/key/scroll 驱动与原生窗口相同的保留图和输入处理器,并会先把目标
滚动到可见。Canvas `wait` 确定性推进动画/定时器时钟,不会真实 sleep。
`-tags desktop` 构建保留既有 WebView 步骤子集(`dispatch` 与 `setState`)。

## `qorm check --audit`——一次性回归

```bash
qorm check <app-dir> --audit
```

无需手写检查:针对每个**可见**组件验证通用的不变量——非零尺寸、无水平溢出、
在窗口之内(水平滚动/分页容器及其后代不在此列)。返回
`{ok, visibleComponents, issues, details}`。

## 在实时共享会话中(MCP)

当一个人在运行应用时,同一会话上的智能体可以调用:

- **`qorm_measure`**——完整的意图 + 渲染结果(如上)。
- **`qorm_check_layout`**——传入 `checks`(与 `--checks` 相同的 schema),
  得到每项检查的通过/失败以及实际值。
- **`qorm_capture_canvas`**——把原生 Canvas 宿主最近一次实际呈现的像素平面
  捕获为 base64 PNG。传可选 `id` 时 PNG 仍是完整画布,`clip` 用物理像素定位该节点;
  这不是隔离子树渲染。没有 Canvas 宿主/已呈现帧、节点不存在或不可见时会明确失败。
  浏览器/WebView 会话不会伪造 Canvas 像素。

```text
qorm_capture_canvas({})
qorm_capture_canvas({"id":"card"})  # 完整 PNG + card 裁剪元数据
```

两者都读取当前宿主的最新测量,因此智能体看到的正是人所看到的:
原生 canvas 窗口的当前保留渲染图,或浏览器/WebView 的当前 DOM。应连接运行中
`qorm run` 会话的 HTTP `/mcp`。单独的 `qorm mcp <app>` stdio 服务没有渲染宿主,
这两个工具会报测量不可用;无窗口 canvas 验证请用 CLI `qorm measure` / `qorm check`。

若需要不依赖实时宿主的文件证据,用 `qorm shot <app> -o out.png` 通过同一个纯 Go
Canvas 后端无窗口渲染(默认 `440x720`,安全上限为单边 4096 px、总计 16 MP)。若目标是
隔离 HTML 而非原生像素,请使用 `qorm_capture_subtree`。

## 设备上实时调试

```bash
qorm run <app> --lan
```

绑定到局域网,并打印一台物理手机如何加入与开发机器和智能体**同一个实时
会话**:

- **Wi-Fi**:在手机浏览器(同一网络)中打开打印出的 `http://<lan-ip>:PORT/`。
  真实的局域网地址会排在最前。
- **USB(Android)**:`adb reverse tcp:PORT` 会自动设置好,因此手机打开
  `http://localhost:PORT/`。

一旦连接,设备就只是实时服务器的又一个客户端:

- 智能体的编辑(经 MCP)即时热重载到设备上(SSE),
- 设备的自我测量回传到 `/measure`,因此 `qorm_measure`
  和 `qorm_check_layout` 报告的是**真实设备的**渲染——实际的
  屏幕尺寸、字体和 WebView——而非模拟,
- SSE 的连接/断开会带客户端 IP 写入活动日志,
  因此一台设备加入会话是可见的。

这让解释并验证得以针对真实硬件进行,闭合了从编写到设备上确认的循环。

## 一条命令搞定一切

```bash
bash scripts/verify.sh
```

运行 `go test ./...`(渲染标记、actions、i18n 格式化、fuzz、
确定性)外加对每个示例的自我测量布局审计,汇总成单一的
ALL-GREEN / 有回归 判定。无需外部浏览器。

## 说明

- 默认 CLI 是离屏 canvas 渲染,也支持原生输入步骤流。只在明确需要
  WebView/DOM 对等性时才使用 `-tags desktop` 构建。
- 实时 MCP 测量需要活跃的渲染宿主。macOS 默认 `qorm run` canvas 窗口直接提供;
  浏览器或 `-tags desktop` WebView 提供 DOM 测量。仅 stdio 的 `qorm mcp` 不提供。
- `visible: false` + 零尺寸对于非活动 tab 内容、已关闭的
  覆盖层(`open:false` 的 modal/dialog/sheet)以及空的条件文本是正常的——
  审计只标记*可见*的组件。
