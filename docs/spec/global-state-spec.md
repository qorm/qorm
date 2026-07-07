# QORM Global State Specification

> **实现说明**：本文中的 `struct` 代码块是语言中立的 schema 记法（早期 Rust 风格书写），不代表运行时语言。QORM 运行时为纯 Go，Global Store 实现见 `internal/runtime` / `internal/model`。

## 目标

QORM 默认以 scene local state 为核心，但在多场景、跨组件共享数据、跨入口 UI 协调和长期运行应用中，需要一套受控的全局状态机制。

本规范定义 Global Store / Context 的最小协议，确保其与 scene local state、Patch、Agent、Host Capability 和生命周期管理保持隔离和可验证。

## 非目标

- 不把 Global Store 设计成任意脚本状态容器
- 不允许隐式打穿 scene 边界
- 不提供无限制共享可变对象
- 不在 V1 中引入复杂响应式 DSL

## 状态层级

QORM V1 定义三层状态：

```text
Local State     scene 内部状态
Global Store    app 级共享状态
Context Scope   子树级共享状态
```

### Local State

- 定义在 scene 文件的 `state` 中
- 生命周期绑定 scene
- 默认表达式上下文直接可见

### Global Store

- 生命周期绑定 app/runtime session
- 可跨 scene 共享
- 适用于用户会话、全局配置、导航上下文、缓存摘要等

### Context Scope

- 生命周期绑定组件子树或 scene subtree
- 适用于局部主题、表单上下文、局部协作状态
- 不等于 Global Store

## 命名与访问路径

表达式上下文新增：

| 名称 | 含义 |
|---|---|
| `global` | app 级全局状态根 |
| `context` | 当前子树 context 根 |

示例：

```text
global.user.id
global.session.tokenMeta
context.theme.mode
context.form.validation
```

规则：
- `global.*` 只能访问 Global Store。
- `context.*` 只能访问最近可见的 Context Scope。
- 顶层 local state 简写不自动映射到 global。

## JSON 形状

`qorm.json` 可定义全局状态入口：

```json
{
  "qorm": "0.1",
  "type": "app",
  "id": "demo_app",
  "globalState": {
    "schema": {
      "user": "object",
      "session": "object",
      "featureFlags": "object"
    },
    "initial": {
      "user": null,
      "session": null,
      "featureFlags": {}
    }
  }
}
```

Context Scope 可由组件或节点声明：

```json
{
  "type": "column",
  "id": "settings_scope",
  "context": {
    "theme": {
      "mode": "dark"
    }
  },
  "children": []
}
```

## Action 读写

V1 建议新增两类路径前缀：

```text
state.<path>
global.<path>
context.<path>
```

示例：

```json
{
  "type": "state.set",
  "path": "global.user",
  "value": {
    "id": "{{ loginResponse.body.user.id }}"
  }
}
```

规则：
- `state.<path>` 操作 local state。
- `global.<path>` 操作 Global Store。
- `context.<path>` 操作当前可见 Context Scope。
- 若未显式写前缀，默认仍然是 scene local state，以保持兼容。

## 生命周期

### Global Store

- Runtime `init` 时初始化
- Runtime `dispose` 时释放
- Bundle 热更新默认保留，除非 schema 不兼容或策略要求重建
- 用户 logout / account switch / app reset 可触发显式清空

### Context Scope

- 节点 mount 时创建
- 节点 unmount 时销毁
- HMR / Patch 导致节点重建时可按 key 规则决定是否保留

## Schema 与校验

- Global Store 必须有 schema。
- Context Scope 推荐有 schema；无 schema 时仅允许受限对象字典。
- 写入前必须做 schema 校验。
- 不兼容写入必须失败，并返回结构化诊断。

## 可见性与隔离

- scene local state 永不自动提升为 global。
- context 只对当前节点子树可见。
- scene A 不能直接访问 scene B 的 local state。
- Global Store 是唯一允许跨 scene 共享的状态层。

## Patch 与 Agent

### Patch Path

Patch 可使用：

```text
/global/<key>
/scenes/<sceneId>/context/<path>
```

示例：

```text
/global/user
/scenes/main/context/theme.mode
```

### 安全规则

- Agent 默认不应修改 Global Store，除非策略显式允许。
- 修改 Global Store 的 Patch 必须视为高影响操作。
- `preview_patch` 必须隔离 global diff，不得污染 live runtime。
- 对 Global Store 的 apply 建议默认需要 approval。

## 并发与一致性

Global Store 必须带版本号：

```text
pub struct GlobalStore {
    values: ValueMap,
    version: u64,
}
```

最小规则：
- 每次成功写入递增版本。
- Patch / Action / Host output 写入时应校验目标版本或使用最新快照序列化执行。
- V1 不要求分布式事务，但必须避免无提示覆盖。

## 与 Host Capability 的关系

- `host.call.output.path` 可以写入 `global.*`，但应受更严格权限与 schema 校验。
- 敏感数据不应默认长驻 Global Store。
- token、secret 等应优先写入宿主安全存储，而不是 global 内存状态。

## 与 HMR 的关系

- 只修改渲染结构时 Global Store 默认保留。
- 若 `globalState.schema` 发生不兼容变更，应触发 reset 或迁移。
- Context Scope 是否保留取决于节点 identity 与 HMR 规则。

## Diagnostics

最小错误：

```text
global_state_schema_violation
global_state_write_denied
context_scope_not_found
global_state_conflict
global_state_reset_required
```

## 验收标准

```text
Global Store 与 scene local state 的读写路径明确区分
跨 scene 可共享 global，但不能访问彼此 local state
Context 只在子树内可见
Patch / Agent / HMR / Host output 对 Global Store 的行为有明确边界
不兼容 schema 或越权写入会产生稳定诊断
```