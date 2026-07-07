# QORM Query Selector Specification

## 目标

QORM 需要统一的查询选择器语法，以支撑测试运行器、Agent inspect/explain、DevServer 调试、Patch 目标辅助和运行时诊断。

本规范定义 node 查询、状态查询、语义查询和断言目标选择的统一协议。

## 非目标

- 不提供完整 CSS selector 兼容层
- 不提供任意脚本查询
- 不为不同工具定义多套不兼容 selector 语法

## 查询对象

V1 支持查询：

```text
node
component instance
scene state
global state
context-visible state
diagnostics
host call records
```

## 选择器形式

QORM V1 推荐结构化 selector，而不是纯字符串 DSL。

### Node by id

```json
{ "id": "submit_button" }
```

### Node by semantic role

```json
{ "semantic": "primary_action" }
```

### Node by type

```json
{ "type": "button" }
```

### Node by text

```json
{ "text": "提交" }
```

### Component instance

```json
{ "component": "user_card" }
```

### Scoped selector

```json
{
  "within": { "id": "settings_panel" },
  "match": { "semantic": "primary_action" }
}
```

## 查询语义

### 唯一查询

默认 `queryOne` 语义：
- 0 个结果 → `query_not_found`
- >1 个结果 → `query_ambiguous`
- 1 个结果 → success

### 多结果查询

`queryAll` 返回数组，可用于测试列表和批量断言。

## 节点查询优先级

V1 建议优先使用：

```text
id > semantic > type > text
```

原因：
- `id` 最稳定
- `semantic` 适合业务级断言
- `type` / `text` 更易歧义

## 状态路径查询

状态断言可直接引用路径：

```json
{ "path": "count" }
{ "path": "global.user.id" }
{ "path": "context.theme.mode" }
```

规则：
- 未加前缀默认指 local state。
- `global.*` 查询全局状态。
- `context.*` 必须依附可见作用域；否则返回 `context_scope_not_found`。

## 查询 API 约定

建议统一抽象：

```text
queryOne(selector)
queryAll(selector)
getState(path)
getDiagnostics(filter)
getHostCalls(filter)
```

## 断言目标模型

以下系统应共享同一 selector 结构：
- `qorm test`
- Agent `inspect_node`
- Agent `simulate_event` 目标定位
- DevServer inspector
- diagnostics 跳转辅助

## 返回结构

查询成功建议最小返回：

```json
{
  "id": "submit_button",
  "type": "button",
  "semantic": "primary_action",
  "text": "提交"
}
```

多结果返回数组。

## 文本与语义查询规则

- `text` 匹配默认基于当前解析后的可见文本值。
- 模板插值后的最终字符串用于文本匹配。
- `semantic` 匹配基于 node semantic 字段，不基于组件名猜测。

## 作用域查询

支持 `within`：
- scene 内 scoped query
- component subtree query
- overlay / modal subtree query

示例：

```json
{
  "within": { "id": "login_form" },
  "match": { "type": "button" }
}
```

## 与 Patch 的关系

Selector 与 Patch Path 相关但不同：
- selector 用于“找到目标节点”
- patch path 用于“精确修改目标字段”

可选工具层行为：
- 先通过 selector 找到唯一节点
- 再映射到逻辑 patch path

## Diagnostics 与 Host Call 查询

建议支持：

```json
{ "diagnosticCode": "component_render_error" }
{ "capability": "network.request" }
```

用于：
- 测试断言
- DevServer 面板过滤
- Agent explain / inspect 补充输出

## 错误码

```text
query_not_found
query_ambiguous
query_invalid_selector
query_scope_invalid
context_scope_not_found
state_path_invalid
```

## 验收标准

```text
测试、Agent、DevServer 使用同一套 selector 结构
可稳定按 id、semantic、type、text 查询节点
可查询 local state、global state、context-visible state
歧义与缺失会返回结构化错误，而不是静默失败
selector 可辅助映射到 patch path，但不与 patch path 混用
```