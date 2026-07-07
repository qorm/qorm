# QORM Error Boundary Specification

## 目标

QORM 需要在局部节点、组件、Action 或资源出错时保持应用可运行，并提供可诊断、可恢复、可降级的错误处理模型。

本规范定义 Error Boundary 的捕获范围、传播规则、fallback 契约以及与 Patch / Preview / Runtime 的关系。

## 非目标

- 不吞掉所有错误
- 不把权限错误伪装成普通渲染错误
- 不用 fallback 掩盖 schema 或安全校验失败
- 不在 V1 中实现任意自定义异常脚本

## Error Boundary 定位

Error Boundary 是 Runtime 层的局部隔离机制，用于：
- 捕获局部渲染或执行失败
- 阻止错误扩散到整棵 Render Tree
- 提供 fallback UI
- 生成结构化 diagnostics

## 可捕获错误类型

V1 建议支持：

```text
binding_evaluation_error
component_render_error
action_runtime_error
resource_load_error
child_node_parse_error
context_resolution_error
```

默认不由 Error Boundary 捕获：

```text
permission_denied
approval_required
signature_validation_failure
bundle_schema_failure
platform_unsupported_at_app_boot
```

原因：这类错误属于安全/装载/权限边界，应由上层拒绝流程处理，而不是局部 UI fallback。

## Boundary 粒度

支持三类粒度：

```text
node boundary
component boundary
subtree boundary
```

示例：

```json
{
  "type": "component",
  "id": "profile_panel",
  "ref": "user_card",
  "errorBoundary": {
    "fallback": {
      "type": "text",
      "value": "资料面板暂时不可用"
    }
  }
}
```

## Fallback 契约

`fallback` 必须是一个合法的 node / component subtree。

最小要求：
- 自身必须通过 schema 校验
- 不得依赖导致当前错误的同一失败源
- 不得自动请求更高权限
- 不得触发无限递归 boundary 嵌套失败

可选附加字段：

```json
{
  "errorBoundary": {
    "fallback": { "type": "text", "value": "加载失败" },
    "retryable": true,
    "report": true
  }
}
```

## 传播规则

- 若当前节点存在 boundary，则错误在本节点被截获。
- 若当前节点无 boundary，则向最近父级 boundary 冒泡。
- 若一直没有 boundary，则错误升级为 scene/runtime error。
- fallback 自身再次出错时，继续向上冒泡，不得在同一 boundary 内无限重试。

## 与 Action 的关系

以下错误可进入 boundary：
- binding 中引用非法值导致的表达式失败
- 组件内部 Action 运行时失败
- 资源缺失导致局部节点无法渲染

以下错误默认不进入 boundary：
- preview/apply 前的 schema validation 失败
- host permission denial
- app 级 bundle activation 失败

## 与 Patch / Preview 的关系

- `preview_patch` 若引入 boundary 可捕获错误，应在 diagnostics 中明确标记为“fallbacked preview”。
- `apply_patch` 不应因为 boundary 存在就忽略严重 schema / semantic 错误。
- 如果 patch 只在运行时才触发 boundary，应允许 apply，但必须留下可追踪 diagnostics。

## Diagnostics

Boundary 捕获后至少输出：

```json
{
  "code": "component_render_error",
  "message": "user_card binding failed",
  "boundary": "profile_panel",
  "fallbackApplied": true,
  "severity": "warning"
}
```

最小字段：
- `code`
- `message`
- `boundary`
- `node` 或 `component`
- `fallbackApplied`
- `severity`

## 恢复策略

V1 支持：
- 静态 fallback
- 手动 retry
- 下一次状态变更后重新尝试渲染

V1 不强制支持：
- 自动指数退避
- 异步恢复脚本
- 跨 boundary 协调恢复

## 与 Global Store 的关系

- boundary 捕获本地错误时，不应默认回滚 Global Store。
- 若错误来自对 Global Store 的非法读写，应产生单独 diagnostics。
- fallback 不得静默清空全局状态。

## 与渲染和性能的关系

- fallback 切换应只影响受影响 subtree。
- 一个局部 boundary 错误不应强制全 scene layout。
- 高频错误必须可节流，避免 diagnostics 风暴。

## 标准错误码

```text
binding_evaluation_error
component_render_error
action_runtime_error
resource_load_error
child_node_parse_error
context_resolution_error
fallback_render_error
```

## 验收标准

```text
局部组件失败不会导致全场景崩溃
fallback 是正式受约束的节点结构
权限/签名/装载失败不会被 Error Boundary 掩盖
preview / apply / runtime 对 boundary 的行为差异明确
捕获后的 diagnostics 可被 Agent、DevServer、测试工具读取
```