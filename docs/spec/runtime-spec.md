# QORM Runtime Specification

QORM Runtime 负责状态、事件、Action、Motion、Patch、Dirty Tracking 和 Host Capability 调度。

> **实现说明**：本文中的 `struct` 代码块是**语言中立的 schema 记法**（早期 Rust 风格书写），不代表运行时语言。QORM Runtime 为**纯 Go**（`internal/runtime`），离线场景通过 `cmd/qorm-wasm` 编译为 WASM。见 [Go 架构](../overview/go-architecture.md)。

以下高级能力的正式语义以对应专门规格为准：
- Global Store / Context：`global-state-spec.md`
- Error Boundary：`error-boundary-spec.md`
- Test Runner：`test-runner-spec.md`
- Query Selector：`query-selector-spec.md`
- DevServer / HMR：`devserver-hmr-spec.md`

## 运行时原则

- 不考虑 JIT。
- 不在每帧解析 JSON。
- 不在每次事件后全量重建 Scene。
- 不在所有状态变化后全量 layout。
- 使用 Typed IR 和 Execution Plan。
- 所有 Host Capability 调用必须通过权限检查。
- 所有预览型 Patch 必须无副作用。

## Runtime 组成

```text
State Store
Binding Engine
Event Dispatcher
Action Interpreter
Motion Runtime
Dirty Tracker
Patch Engine
Host Dispatcher
Lifecycle Manager
Global Store
Context Scope
Error Boundary
```

## State Store

State Store 保存 scene 运行时状态：

```text
pub struct StateStore {
    values: ValueMap,
    dependency_graph: DependencyGraph,
    version: StateVersion,
}
```

scene local state、Global Store 和 Context Scope 的完整层级与隔离规则以 `global-state-spec.md` 为准。

状态变更必须产生：

```text
StateChange
DirtyBinding
DirtyLayout?
DirtyPaint?
DirtyText?
DirtyHitTest?
```

## Binding Engine

Binding Engine 负责表达式求值和依赖追踪。

```json
{
  "value": "{{ player.hp }} / {{ player.maxHp }}"
}
```

依赖：

```text
player.hp
player.maxHp
```

### 求值规则

- 完整表达式字段返回 typed value。
- 模板插值字段返回字符串。
- 缺失路径结果为 `null`。
- 布尔上下文使用 JSON 格式规格中定义的 truthy / falsey 规则。
- 表达式必须纯净，可静态提取依赖。

状态变化后只重新计算相关表达式。

## Event Dispatcher

平台事件先转换为 QORM Event：

```text
PointerDown
PointerUp
Press
KeyDown
TextInput
ImeStart
ImeUpdate
ImeCommit
Scroll
Focus
Blur
Lifecycle
Custom
```

事件分发流程：

```text
platform event → qorm event → hit test → target node → handler list → action interpreter
```

V1 默认不定义 DOM 式 capture / bubble；事件命中后由目标节点 handler 顺序执行。

## Action Interpreter

Action 是受限声明式行为，不是任意脚本。

支持：

```text
state.set
state.toggle
state.append
state.remove
state.update
action.call
motion.play
host.call
plugin.call
native.call
navigation.go
toast.show
modal.open
modal.close
event.emit
condition
delay
batch
```

### 执行顺序

- 一个 handler 内的 step 按顺序执行。
- `batch` 按内部 `steps` 顺序展开执行。
- `action.call` 解析到目标 action 后按当前上下文内联执行。
- `condition` 先求值 `when`，再只执行 `then` 或 `else` 其中一支。

### 异步 step

以下 step 可能异步：

```text
host.call
plugin.call
native.call
delay
```

执行规则：
- 当前 step 必须完成后才继续后续 step。
- 若 step 配置了 `output.path`，结果会先写入状态，再继续后续 step。
- 若 step 失败且未配置可消费的输出位置，当前 action 链停止，并返回结构化错误。

## Motion Runtime

Motion Runtime 的可动画属性以 `motion-spec.md` 为主来源。

- Runtime 必须支持 `motion-spec.md` 中定义的 V1 animatable properties。
- 默认不触发布局。
- 只有 `affectsLayout: true` 的 motion 才允许触发布局。
- 同一节点同一属性上的新 motion 默认替换旧 motion。

## Dirty Tracking

Dirty 类型：

```text
DirtyState
DirtyBinding
DirtyStyle
DirtyLayout
DirtyPaint
DirtyText
DirtyResource
DirtyHitTest
```

更新策略：

```text
状态变更 → 依赖图 → 局部 binding → 局部 style/layout/render
```

### 基本规则

- 只影响文本、颜色、透明度等非布局属性的变化不得触发全量 layout。
- 文本内容、字体、宽高、padding、children 变化可能触发局部 layout。
- `opacity`、`transform`、`clip`、`progress` 等纯渲染属性变化只应标记 paint / display list 更新。
- `preview_patch` 只能在隔离副本上产生 dirty，不得污染 live runtime。

## Host Dispatcher

Host Dispatcher 负责：

```text
permission check
capability lookup
approval check
platform dispatch
structured result normalization
audit emission
```

`host.call`、`plugin.call`、`native.call` 的最终放行规则以 `permission-model.md` 和 `host-capability-spec.md` 为准。

## Patch Engine

Patch 支持：

```text
preview.patch
apply.patch
rollback.patch
history.undo
history.redo
```

Patch 必须经过：

```text
path validation
schema validation
semantic validation
permission check
platform capability check
```

### Patch 目标模型

- Patch 作用于解析后的逻辑模型。
- 规范 Patch Path 使用稳定逻辑路径，如 `/scenes/main/nodes/submit_button/text`。
- Source Map 的 JSON Pointer 只用于回源定位和诊断。

### Preview / Apply / Rollback

- `preview.patch` 必须在隔离副本上执行。
- `apply.patch` 只能应用通过 preview 的规范化 patch 集。
- `rollback.patch` 必须能回退最近一次成功 apply 的逻辑变更。
- `history.undo` / `history.redo` 作用于 patch 历史，而不是任意运行时状态变化。
- DevServer 触发的 HMR 不是 patch preview 的替代，其状态保留与 full reload 规则以 `devserver-hmr-spec.md` 为准。

## Lifecycle

Runtime 生命周期：

```text
init
loadBundle
mountScene
activate
suspend
resume
updateBundle
rollbackBundle
unmountScene
dispose
```

移动端必须处理：

```text
foreground
background
memory warning
orientation change
safe area change
keyboard show/hide
```

Bundle 更新前后都必须重新校验版本、签名、能力要求和 approval 失效条件。