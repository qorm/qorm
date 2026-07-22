<!-- data-lang-nav --> [English](../verification.md) · 中文

# 解释并验证一个 QORM 应用

QORM 的目标是让 AI 能够**完整而精确地解释并验证**用户在应用中表达的一切——
它的布局、样式、行为和翻译——使用框架本身,无需外部浏览器。

机制如下:运行中的应用在它自己的运行时(浏览器或原生 WebView)中**自我测量**。
一段小脚本遍历每个带 id 的元素,记录其 `getBoundingClientRect` 和计算样式,
并将它们 POST 到 `/measure`。随后框架将那份真实的渲染结果与用户的**意图**
(每个节点的类型、文本和状态绑定,来自应用 JSON)连接起来。于是对于每个组件,
你都同时得到*用户所要求的*和*实际渲染出来的*。

下面的一切都可以从 CLI(`-tags desktop` 构建,它驱动一个原生 WebView)
和经由 MCP 的实时共享会话中运行。

## `qorm measure`——读取真实渲染

```bash
qorm measure <app-dir> [-o report.json]
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

## `qorm check --checks`——验证期望

```bash
qorm check <app-dir> --checks checks.json [-o report.json]
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

无障碍断言读取的是**渲染后**的 DOM,因此能捕捉渲染器隐式注入的 role 和 label,
而不只是 JSON 里声明的。`focusTrap` 目前被刻意拒绝:焦点陷阱是动态的 Tab 键序行为,
不是静态快照,验证工具绝不应为一个它实际做不到的检查背书。

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

`do` 是 `{"dispatch": "<action>", "args": {…}}` 或 `{"setState": {"path": …, "value": …}}`。

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

两者都读取实时客户端的自我测量,因此智能体看到的正是人所看到的。
工具描述中携带完整的断言列表。

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

- 测量需要应用在一个渲染运行时中运行。CLI 使用
  `-tags desktop` 的 WebView(以无头方式运行);实时会话使用人所打开的任意
  浏览器/WebView。
- `visible: false` + 零尺寸对于非活动 tab 内容、已关闭的
  覆盖层(`open:false` 的 modal/dialog/sheet)以及空的条件文本是正常的——
  审计只标记*可见*的组件。
