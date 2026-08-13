---
title: QORM MCP 工具
description: AI 智能体用于读取、操作、设计并验证运行中 QORM 应用的 Model Context Protocol 工具，由源码自动生成。
---

# QORM MCP 工具

> 由 `internal/mcp/tools.go` (`TestMCPDocInSync`) 自动生成 —— 请勿手动修改。
> 请使用 `QORM_UPDATE_DOCS=1 go test ./internal/mcp/` 重新生成。

QORM 暴露了一个 [Model Context Protocol](https://modelcontextprotocol.io) 服务端，使 AI 智能体可以**读取、操作、设计并验证**一个运行中的 QORM 应用。可以使用 `qorm mcp <app-dir|bundle>` 启动它（基于标准输入输出的 JSON-RPC），或在运行的 `qorm run` 中通过 HTTP 的 `/mcp` 访问相同的工具 —— 此时智能体与当前 canvas、浏览器或 WebView 宿主共享同一个运行时会话。

![运行中的 QORM 应用与共享协作日志并列](../../agent/img/console.png)
*人类运行的应用，与智能体通过 MCP 读取的共享会话日志并列——同一个运行中的运行时。*

![共享会话的 QORM DevTool 活动日志](../../assets/screenshots/logwindow.png)
*DevTool 将你的点击与智能体在同一应用上的 MCP 调用按时间交错列出，并以颜色区分操作者。*

**安全模型**：`qorm_simulate_action`、`qorm_preview_patch` 和 `qorm_diff` 均对应用副本运行，绝不触碰运行中的应用。`qorm_apply_patch` 提交修改，但它必须携带由相同操作的 `qorm_preview_patch` 返回的 `previewToken` —— 从而保证每次提交的编辑都有前置审查。`qorm_undo` 撤销最后一次提交的操作。

## 工具分类

25 个工具按意图分为六组，按需取用：

- **理解（只读）：** `qorm_inspect`、`qorm_render_html`、`qorm_capture_subtree`、`qorm_capture_canvas`、`qorm_a11y_tree`、`qorm_capabilities`、`qorm_get_node`、`qorm_source_location`、`qorm_query`、`qorm_list_actions`、`qorm_activity`、`qorm_export_scene`、`qorm_export_bundle`
- **操作（修改实时状态）：** `qorm_dispatch`、`qorm_set_state`
- **模拟（无副作用）：** `qorm_simulate_action`
- **设计（preview → apply，评审绑定）：** `qorm_preview_patch`、`qorm_diff`、`qorm_apply_patch`、`qorm_undo`
- **验证（测试 + 解读实时渲染）：** `qorm_assert`、`qorm_validate`、`qorm_measure`、`qorm_check_layout`
- **窗口控制（桌面）：** `qorm_window`

| 工具 | 参数 | 描述 |
|---|---|---|
| `qorm_window` | `h` (integer), `id` (string), `js` (string), `op` (move\|open\|close\|eval\|tile\|focus\|minimize\|pin\|unpin), `url` (string), `w` (integer), `x` (integer), `y` (integer) | 控制桌面应用窗口：op=move 时需要 x,y,w,h（左上角像素坐标）；op=focus/minimize/pin/unpin 作用于窗口。控制引擎调整用户窗口的位置。支持 macOS 和 Windows 桌面应用。 |
| `qorm_inspect` | — | 检查 QORM 应用：id、名称、入口场景、场景 id 列表、状态模式 (schema)、当前状态、动作 (action) id 列表、静态编译诊断警告，以及（若已声明）设计令牌系统（designTokens：名称 -> {type,value,enforce}）。声明为 enforce 的颜色令牌会硬约束 apply_patch：颜色样式只能设为这些令牌的值。只读。 |
| `qorm_render_html` | — | 将当前应用渲染为 HTML，以便智能体查看 UI 的外观——渲染的是会话当前实际所处的场景，且已先完成该场景的路由守卫解析（会话无权进入的受保护场景绝不会被渲染）。只读。 |
| `qorm_capture_subtree` | `id`* (string) | 按节点 id 捕获指定子树：返回隔离渲染的 HTML 与子级布局层次，用于视觉反馈。只读。 |
| `qorm_capture_canvas` | `id` (string) | 把原生 Canvas 最近一次实际呈现的像素平面捕获为 base64 PNG。可选 id 仍返回完整画布，并附该节点的物理像素裁剪矩形；不会伪称已隔离或重新渲染子树。在非运行中的原生 Canvas 宿主、首帧尚未呈现、节点不存在/不可见或超过安全上限时会明确失败。只读。 |
| `qorm_a11y_tree` | — | 推导入口场景的无障碍（accessibility）树：每个节点的 ARIA role、可访问名称（accessible name）与语义状态（checked/disabled/required/value），并附带无障碍问题审计——会到达屏幕阅读器却没有可访问名称的交互控件与图片。用于检查无障碍覆盖或定位待修复项。只读。 |
| `qorm_capabilities` | — | 列出所有内置的硬件/原生能力：每个能力的规范名称 + 组件类型、它接受的 qormToNative 操作字符串、它的 qormOn<Name> 回调，以及实现它的平台（ios/android/mac/linux/windows/web）。只读 —— 用于智能体发现存在哪些硬件以及如何调用它们。小程序 (mini-program) 是静态导出目标：没有适用的实时工具。 |
| `qorm_get_node` | `id`* (string) | 通过节点 id 返回节点的类型、属性（props）和子节点 id 列表。只读。 |
| `qorm_source_location` | `id`* (string) | 反查：给定节点 id（例如用户在 devtool 里点击的、或你通过 qorm_query 找到的），返回它在应用源码中的声明位置——文件（相对应用目录）、1 起始的行号，以及该行文本。让你直接跳到 JSON 去修改。签名 bundle（无源码树）或模板化 id 不可用。只读。 |
| `qorm_query` | `hasProp` (string), `idContains` (string), `textContains` (string), `type` (string) | 查找与选择器匹配的节点，条件可任选：type、textContains、idContains、hasProp（多个条件通过 AND 组合）。返回每个匹配项的 id、类型、标签和祖先路径。在应用补丁 (patch) 前使用此工具定位节点。只读。 |
| `qorm_list_actions` | — | 列出可用动作以及每个动作步骤的摘要。只读。 |
| `qorm_activity` | — | 读取共享会话的实时状态：返回 {events:[谁（人类/智能体）做了什么，从旧到新], humanFocus:{元素, 秒数前}, humanTyping:{输入内容, 秒数前}, humanFilled:{字段, 秒数前}, inflight:N} —— 让智能体看到人类刚做了什么、当前停在哪个元素、最近输入了什么文本，以及填写了哪些隐藏（密码）字段（仅记录字段标签，绝不捕获密码值），从而在上下文中协作。浏览器/WebView 事件与原生 canvas 指针/键盘动作进入同一份人类活动流和 DevTool；两类宿主都回报隐私安全的 focus/typing/filled presence，且都绝不捕获密码值。`inflight` 是应用尚未完成的后台工作数（异步 `http.*` 请求加上等待中的 `delay` 步骤）：为 0 表示应用已静默、此刻读到的就是最终状态；大于 0 表示还有响应在路上、当前这一帧只是 loading 态，请稍后重读再下结论。仅在运行中的 `qorm run` 会话中可用。只读。 |
| `qorm_export_scene` | — | 将当前（可能已被应用补丁的）入口场景序列化回 QORM JSON，以便保存或交付通过 apply_patch 完成的设计工作。只读。 |
| `qorm_export_bundle` | — | 将整个当前应用（清单 + 场景 + 动作）序列化为一个未签名的包（包含内容哈希）。人类/CI 在 OTA 部署前对其进行签名（`qorm sign`） —— 智能体绝不会持有签名密钥。只读。 |
| `qorm_simulate_action` | `action`* (string), `args` (object) | 对状态的副本分发动作，并返回 before/after/changed 信息。无副作用：绝不会修改运行中的应用。 |
| `qorm_dispatch` | `action`* (string), `args` (object) | 操作运行中的应用：分发动作（修改状态）并返回新状态和渲染后的 HTML。 |
| `qorm_set_state` | `path`* (string), `value`* | 操作运行中的应用：将状态路径设为特定值，并返回新状态和渲染后的 HTML。点号路径按嵌套解释，与 state.set 动作步骤完全一致：路径 'user.name' 会写入 user 内部的 name，因此绑定 {{ state.user.name }} 能读回它。computed（派生）值是只读的——指向 computed 命名空间的路径会被拒绝，因为它们在每一帧都会依据声明重新发布。 |
| `qorm_assert` | `checks`* (array) | 测试应用：对当前状态和渲染后的 HTML 评估检查项。每个检查项为 {kind: 'stateEquals'\|'htmlContains'\|'nodeExists', ...}。返回每个检查项的通过/失败状态以及总体结果。 |
| `qorm_preview_patch` | `ops`* (array) | 设计（安全）：将补丁操作应用到应用的副本，并返回生成的 HTML 以及一个 previewToken。无副作用 —— 运行中的应用不会被改变。操作类型：{op:'setProp',target,key,value} \| {op:'addChild',target,node} \| {op:'insertBefore'\|'insertAfter',target,node} \| {op:'replace',target,node} \| {op:'wrap',target,node} \| {op:'move',target,into} \| {op:'remove',target}。 |
| `qorm_diff` | `ops`* (array) | 设计（安全）：在不接触运行中应用的前提下，显示补丁将会产生的结构差异（新增/删除的节点 id，以及每个改变的节点中被修改的字段）。在应用前进行评审。 |
| `qorm_apply_patch` | `ops`* (array), `previewToken`* (string) | 设计（提交）：将补丁操作应用到运行中的应用。必须传递由相同操作的 qorm_preview_patch 返回的 previewToken —— 提交应用绑定于评审。会对当前状态进行快照备份以便后续撤销。若应用声明了 enforce 的颜色设计令牌（见 qorm_inspect 的 designTokens），将颜色样式设为非令牌值的 setProp style 操作会被拒绝（预览阶段亦然）。 |
| `qorm_undo` | — | 设计：撤销最后一次应用的补丁，将应用恢复到该应用前的状态。返回撤销后的 HTML 以及剩余的撤销深度。 |
| `qorm_measure` | — | 精确解析实时渲染：返回每个组件的用户表达（类型、文本、状态绑定）与实际渲染结果 —— x,y,w,h, visible，以及计算出的 color/background/fontSize/fontWeight/padding/borderRadius/border/opacity/zIndex/position/x-overflow。测量由当前渲染宿主提供：原生 canvas 窗口导出其保留模式渲染图，浏览器/WebView 则回报 DOM。需要带渲染窗口/客户端的运行中应用；单独通过 stdio 运行 `qorm mcp` 没有渲染宿主。如需无窗口、确定性的单次 canvas 测量，请使用 CLI `qorm measure`。用于查看人类用户的实时宿主究竟渲染了什么。 |
| `qorm_check_layout` | `checks`* (array), `viewportH` (integer), `viewportW` (integer) | 根据预期校验实时渲染；返回每个检查项的通过/失败状态以及实际值。`checks` 是由 {id, <assertions>} 组成的数组。断言：visible(bool) \| type(组件类型 string) \| text(组件必须包含的子字符串，与所表达或渲染后的文本匹配) \| noOverflow(bool, 无水平溢出) \| minW\|maxW\|minH\|maxH(px 数值) \| x\|y(px 数值, ±3 容差) \| within(id: 该盒子必须位于该 id 盒子的内部) \| below(id: 必须在该 id 的下方开始) \| backgroundNot\|colorNot(必须不存在的子字符串 —— 例如在暗黑模式下断言 "255, 255, 255" 即非白色) \| role(渲染后的 ARIA role 字符串) \| hasAriaLabel(bool) \| contrastRatio(最小 WCAG 对比度，如 AA 正文 4.5)。浏览器/WebView 的 DOM 回报包含渲染器隐式注入的 role 和计算对比度；canvas 图当前回报作者声明的 role/ariaLabel，并使 contrastRatio 以不可用明确失败。示例：[{"id":"wifi","type":"switchlisttile","visible":true,"within":"settings"},{"id":"chart","noOverflow":true}]。明确失败：无法识别的断言键（如拼写错误）会判定为失败，未被测量的 within/below 目标 id 会以 'not found' 判定为失败 —— 绝不会有检查项被静默通过。需要带渲染窗口/客户端的运行中应用：原生 canvas 检查使用其保留模式渲染图，浏览器/WebView 检查使用 DOM 回报；单独通过 stdio 运行 `qorm mcp` 没有渲染宿主。可选的 viewportW/viewportH（px）在校验前设置运行时视口，使响应式 `when` 分支按该窗口尺寸解析 —— 注意测得的矩形仍来自宿主的真实窗口（活动浏览器客户端也会在下次加载/缩放时覆盖该视口）。 |
| `qorm_validate` | `node` (object), `sceneId` (string) | 在打补丁或保存之前，将 QORM 场景节点或整个应用对照组件模式 (schema)、组件目录类型规则、表达式语法和设计令牌约束进行校验。返回 valid（布尔值）以及诊断警告或错误数组。 |

带有 `*` 标记的参数为必填项；其余为选填项。

## 示例

代表性调用（JSON-RPC 2.0，方法 `tools/call`）：

**理解** —— 读取应用，然后定位节点并跳转到其源码位置：

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call",
 "params":{"name":"qorm_inspect","arguments":{}}}

{"jsonrpc":"2.0","id":2,"method":"tools/call",
 "params":{"name":"qorm_query","arguments":{"type":"button","textContains":"Save"}}}

{"jsonrpc":"2.0","id":3,"method":"tools/call",
 "params":{"name":"qorm_source_location","arguments":{"id":"saveBtn"}}}
```

**操作** —— 分发动作、写入嵌套状态路径：

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call",
 "params":{"name":"qorm_dispatch","arguments":{"action":"increment"}}}

{"jsonrpc":"2.0","id":5,"method":"tools/call",
 "params":{"name":"qorm_set_state","arguments":{"path":"user.name","value":"Ada"}}}
```

**设计** —— 预览补丁、评审差异，然后携带 token 提交：

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call",
 "params":{"name":"qorm_preview_patch","arguments":{"ops":[
   {"op":"setProp","target":"saveBtn","key":"text","value":"Save now"}]}}}

{"jsonrpc":"2.0","id":7,"method":"tools/call",
 "params":{"name":"qorm_apply_patch","arguments":{
   "ops":[{"op":"setProp","target":"saveBtn","key":"text","value":"Save now"}],
   "previewToken":"<来自 qorm_preview_patch 的 token>"}}}
```

**验证** —— 断言状态并校验实时布局：

```json
{"jsonrpc":"2.0","id":8,"method":"tools/call",
 "params":{"name":"qorm_assert","arguments":{"checks":[
   {"kind":"stateEquals","path":"count","value":3},
   {"kind":"htmlContains","text":"Saved"}]}}}

{"jsonrpc":"2.0","id":9,"method":"tools/call",
 "params":{"name":"qorm_check_layout","arguments":{"checks":[
   {"id":"saveBtn","type":"button","visible":true,"noOverflow":true}]}}}
```

