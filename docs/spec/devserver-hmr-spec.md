# QORM DevServer and HMR Specification

## 目标

QORM 需要一套正式的 DevServer 与 HMR 协议，用于支持本地预览、增量刷新、局部状态保留、诊断输出与开发期 Patch 辅助。

本规范定义 DevServer 运行模型、热更新边界、状态保留规则以及与 `preview_patch` / `qorm test` 的关系。

## 非目标

- 不把 DevServer 设计成生产运行时
- 不要求一开始支持完整多人协作编辑
- 不把 HMR 变成无约束的运行时状态迁移系统

## DevServer 定位

DevServer 是开发期服务，负责：
- 加载源文件或 bundle
- 监听文件变化
- 增量重建与推送更新
- 提供预览、诊断、日志与调试接口
- 与测试、Agent 工具共享部分子系统

## 运行模型

```text
watch source files
resolve dependencies
build typed IR
start preview runtime
open preview channel
on change -> diff -> validate -> hot update or full reload
emit diagnostics and logs
```

## 输入来源

DevServer 应支持：
- source JSON 模式
- resolved bundle 预览模式
- example / playground 受控模式

默认优先以 source JSON 模式工作。

## HMR 刷新单元

V1 建议支持以下刷新单元：

```text
scene
component
style
action
motion
resource
```

规则：
- 文件变更后先解析和校验变更单元。
- 若依赖图受影响，应向上扩展到相关 scene / component subtree。
- 若无法安全增量更新，必须回退到 full reload。

## 状态保留规则

### 默认保留

以下情况下可保留 local state：
- 仅样式变化
- 仅非结构性文本或属性变化
- action 逻辑变化但状态 schema 不变
- motion 变化

### 必须重置

以下情况下必须重置受影响 state：
- scene `state` schema 不兼容变化
- `globalState.schema` 不兼容变化
- Context Scope shape 不兼容变化
- 节点 identity 丢失或 key 变化

### Global Store

- Global Store 默认在 HMR 下保留。
- 若 `globalState.schema` 不兼容变更，则必须 reset 或执行显式迁移。

## 节点 identity

HMR 能否保留状态取决于稳定 identity：
- node `id`
- component instance `id`
- scene `id`
- context scope identity

缺失稳定 identity 时，DevServer 不应猜测保留。

## 更新协议

建议 DevServer 输出结构化更新事件：

```json
{
  "type": "hot-update",
  "scope": "scene",
  "target": "main",
  "status": "applied",
  "statePreserved": true,
  "diagnostics": []
}
```

可能状态：
- `applied`
- `full-reload-required`
- `blocked-by-validation`
- `blocked-by-schema-reset`

## 与 Patch Preview 的关系

- DevServer 的 HMR 不是 `preview_patch` 的替代。
- `preview_patch` 必须继续在隔离副本上执行。
- DevServer 可复用 preview 诊断能力展示变更影响。
- 编辑器保存文件触发的热更新，不应被视为用户已确认的 apply_patch。

## Diagnostics 与 Logs

DevServer 至少应输出：
- parse diagnostics
- schema diagnostics
- semantic diagnostics
- HMR decision logs
- state reset reasons
- preview/runtime boundary warnings

示例：

```json
{
  "code": "hmr_full_reload_required",
  "message": "globalState.schema changed incompatibly",
  "target": "qorm.json"
}
```

## 与 Test Runner 的关系

- DevServer 与 Test Runner 可以共享：
  - query engine
  - host mock registry
  - diagnostics format
  - headless runtime harness
- DevServer 不应成为 `qorm test` 的唯一执行基础。

## 与 Agent 的关系

- Agent 可读取 DevServer diagnostics。
- Agent 发起的 preview/apply 结果可以显示在 DevServer 面板。
- DevServer 不得自动信任 Agent 写入，仍需走 patch / approval 规则。

## 传输通道

V1 可使用：
- local websocket
- IDE extension bridge
- embedded preview channel

协议至少需要支持：
- update event
- diagnostics push
- log stream
- reload request
- inspect/query request

## 错误码

```text
hmr_full_reload_required
hmr_state_reset_required
hmr_validation_failed
hmr_target_not_found
devserver_channel_disconnected
devserver_runtime_boot_failed
```

## 验收标准

```text
修改 scene/style/action 后预览窗口可实时更新
符合规则的变更能保留 local/global state
不兼容 schema 变更会明确触发 reset 或 full reload
DevServer 可输出结构化 diagnostics 与更新决策
DevServer 与 Test Runner / Agent 在 query 和 diagnostics 层共享协议
```