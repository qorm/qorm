# QORM MCP 工具

> 由 `internal/mcp/tools.go` (`TestMCPDocInSync`) 自动生成 —— 请勿手动修改。
> 请使用 `QORM_UPDATE_DOCS=1 go test ./internal/mcp/` 重新生成。

QORM 暴露了一个 [Model Context Protocol](https://modelcontextprotocol.io) 服务端，使 AI 智能体可以**读取、操作、设计并验证**一个运行中的 QORM 应用。可以使用 `qorm mcp <app-dir|bundle>` 启动它（基于标准输入输出的 JSON-RPC），或在运行的 `qorm run` 中通过 HTTP 的 `/mcp` 访问相同的工具 —— 此时智能体和浏览器将共享同一个运行中的运行时会话。

**安全模型**：`qorm_simulate_action`、`qorm_preview_patch` 和 `qorm_diff` 均对应用副本运行，绝不触碰运行中的应用。`qorm_apply_patch` 提交修改，但它必须携带由相同操作的 `qorm_preview_patch` 返回的 `previewToken` —— 从而保证每次提交的编辑都有前置审查。`qorm_undo` 撤销最后一次提交的操作。

| 工具 | 参数 | 描述 |
|---|---|---|
| `qorm_window` | `h` (integer), `id` (string), `js` (string), `op` (move\|open\|close\|eval\|tile\|focus\|minimize\|pin\|unpin), `url` (string), `w` (integer), `x` (integer), `y` (integer) | 控制桌面应用窗口：op=move 时需要 x,y,w,h（左上角像素坐标）；op=focus/minimize/pin/unpin 作用于窗口。控制引擎调整用户窗口的位置。支持 macOS 和 Windows 桌面应用。 |
| `qorm_inspect` | — | 检查 QORM 应用：id、名称、入口场景、场景 id 列表、状态模式 (schema)、当前状态、动作 (action) id 列表、静态编译诊断警告，以及（若已声明）设计令牌系统（designTokens：名称 -> {type,value,enforce}）。声明为 enforce 的颜色令牌会硬约束 apply_patch：颜色样式只能设为这些令牌的值。只读。 |
| `qorm_render_html` | — | 将当前应用渲染为 HTML，以便智能体查看 UI 的外观。只读。 |
| `qorm_a11y_tree` | — | 推导入口场景的无障碍（accessibility）树：每个节点的 ARIA role、可访问名称（accessible name）与语义状态（checked/disabled/required/value），并附带无障碍问题审计——会到达屏幕阅读器却没有可访问名称的交互控件与图片。用于检查无障碍覆盖或定位待修复项。只读。 |
| `qorm_capabilities` | — | 列出所有内置的硬件/原生能力：每个能力的规范名称 + 组件类型、它接受的 qormToNative 操作字符串、它的 qormOn<Name> 回调，以及实现它的平台（ios/android/mac/linux/windows/web）。只读 —— 用于智能体发现存在哪些硬件以及如何调用它们。 |
| `qorm_get_node` | `id`* (string) | 通过节点 id 返回节点的类型、属性（props）和子节点 id 列表。只读。 |
| `qorm_query` | `hasProp` (string), `idContains` (string), `textContains` (string), `type` (string) | 查找与选择器匹配 of：type、textContains、idContains、hasProp（通过 AND 组合）。返回每个匹配项的 id、类型、标签和祖先路径。在应用补丁 (patch) 前使用此工具定位节点。只读。 |
| `qorm_list_actions` | — | 列出可用动作以及每个动作步骤的摘要。只读。 |
| `qorm_activity` | — | 读取共享会话的实时状态：返回 {events:[谁（人类/智能体）做了什么，从旧到新], humanFocus:{元素, 秒数前}, humanTyping:{输入内容, 秒数前}, humanFilled:{字段, 秒数前}} —— 从而使智能体看到人类刚刚做了什么、当前聚焦在哪个元素、最后输入的文本，以及填写了哪些隐藏（密码）字段（仅标签；密码值绝不会被捕获），实现上下文协同。仅在运行中的 `qorm run` session中可用。只读。 |
| `qorm_export_scene` | — | 将当前（可能已被应用补丁的）入口场景序列化回 QORM JSON，以便保存或交付通过 apply_patch 完成的设计工作。只读。 |
| `qorm_export_bundle` | — | 将整个当前应用（清单 + 场景 + 动作）序列化为一个未签名的包（包含内容哈希）。人类/CI 在 OTA 部署前对其进行签名（`qorm sign`） —— 智能体绝不会持有签名密钥。只读。 |
| `qorm_simulate_action` | `action`* (string), `args` (object) | 对状态的副本分发动作，并返回 before/after/changed 信息。无副作用：绝不会修改运行中的应用。 |
| `qorm_dispatch` | `action`* (string), `args` (object) | 操作运行中的应用：分发动作（修改状态）并返回新状态和渲染后的 HTML。 |
| `qorm_set_state` | `path`* (string), `value`* | 操作运行中的应用：将状态路径设为特定值，并返回新状态和渲染后的 HTML。 |
| `qorm_assert` | `checks`* (array) | 测试应用：对当前状态和渲染后的 HTML 评估检查项。每个检查项为 {kind: 'stateEquals'\|'htmlContains'\|'nodeExists', ...}。返回每个检查项的通过/失败状态以及总体结果。 |
| `qorm_preview_patch` | `ops`* (array) | 设计（安全）：将补丁操作应用到应用的副本，并返回生成的 HTML 以及一个 previewToken。无副作用 —— 运行中的应用不会被改变。操作类型：{op:'setProp',target,key,value} \| {op:'addChild',target,node} \| {op:'insertBefore'\|'insertAfter',target,node} \| {op:'replace',target,node} \| {op:'wrap',target,node} \| {op:'move',target,into} \| {op:'remove',target}。 |
| `qorm_diff` | `ops`* (array) | 设计（安全）：在不接触运行中应用的前提下，显示补丁将会产生的结构差异（新增/删除的节点 id，以及每个改变的节点中被修改的字段）。在应用前进行评审。 |
| `qorm_apply_patch` | `ops`* (array), `previewToken`* (string) | 设计（提交）：将补丁操作应用到运行中的应用。必须传递由相同操作的 qorm_preview_patch 返回 of previewToken —— 提交应用绑定于评审。会对当前状态进行快照备份以便后续撤销。若应用声明了 enforce 的颜色设计令牌（见 qorm_inspect 的 designTokens），将颜色样式设为非令牌值的 setProp style 操作会被拒绝（预览阶段亦然）。 |
| `qorm_undo` | — | 设计：撤销最后一次应用的补丁，将应用恢复到该应用前的状态。返回撤销后的 HTML 以及剩余的撤销深度。 |
| `qorm_measure` | — | 精确解析运行中的渲染：返回连接用户表达（类型、文本、状态绑定）与实际渲染方式的每一个组件细节 —— x,y,w,h, visible, 以及计算出的 color/background/fontSize/fontWeight/padding/borderRadius/border/opacity/zIndex/position/x-overflow —— 由运行中应用在其窗口中自行测量。要求应用在窗口/浏览器中打开（它在加载时以及每次更改后会进行自我测量）。用于查看用户应用的确切渲染情况。 |
| `qorm_check_layout` | `checks`* (array), `viewportH` (integer), `viewportW` (integer) | 根据预期校验运行中的渲染；返回每个检查项的通过/失败状态以及实际值。`checks` 是由 {id, <assertions>} 组成的数组。断言：visible(bool) \| type(组件类型 string) \| text(组件必须包含的子字符串，与所表达或渲染后的文本匹配) \| noOverflow(bool, 无水平溢出) \| minW\|maxW\|minH\|maxH(px 数值) \| x\|y(px 数值, ±3 容差) \| within(id: 该盒子必须位于该 id 盒子的内部) \| below(id: 必须在该 id 的下方开始) \| backgroundNot\|colorNot(必须不存在的子字符串 —— 例如在暗黑模式下断言 "255, 255, 255" 即非白色) \| role(渲染后的 ARIA role 字符串，含渲染器隐式注入的 role) \| hasAriaLabel(bool) \| contrastRatio(最小 WCAG 对比度，如 AA 正文 4.5 —— 针对有效背景色计算)。示例：[{"id":"wifi","type":"switchlisttile","visible":true,"within":"settings"},{"id":"chart","noOverflow":true}]。要求应用在窗口中打开（它会自我测量）。可选的 viewportW/viewportH（px）在校验前设置运行时视口，使响应式 `when` 分支按该窗口尺寸解析 —— 注意测得的矩形仍来自客户端的真实窗口（活动客户端也会在下次加载/缩放时覆盖该视口）。 |

带有 `*` 标记的参数为必填项；其余为选填项。
