# QORM Test Runner Specification

## 目标

QORM 需要一套正式的无头测试运行器协议，用于验证 scene、Action、状态更新、Patch、Host mock 和运行时诊断。

本规范定义 `qorm test` 的运行模型、事件模拟、查询/断言接口和结果格式。

## 非目标

- 不要求一开始提供完整浏览器自动化替代品
- 不把测试运行器和 DevServer 混成一个协议
- 不在 V1 支持任意脚本式测试 DSL

## 运行模型

`qorm test` 默认运行在 headless runtime 上：

```text
load bundle or source
resolve dependencies
build typed IR
start headless runtime
inject host mocks
run test steps
collect assertions and diagnostics
emit report
```

V1 运行模式：
- headless runtime required
- renderer optional
- headless layout / display list inspection optional but recommended

## 测试文件形状

建议支持独立 `type: "test"` 文件：

```json
{
  "qorm": "0.1",
  "type": "test",
  "id": "login_submit_test",
  "scene": "scene://main",
  "mocks": {},
  "steps": [],
  "assert": []
}
```

## 测试上下文

每个测试运行实例至少包含：
- selected scene
- runtime state snapshot
- global store snapshot
- host mock registry
- diagnostics buffer
- query engine

## Host Mock

测试运行器必须支持 mock Host Capability：

```json
{
  "mocks": {
    "network.request": {
      "match": {
        "method": "POST",
        "url": "/api/login"
      },
      "result": {
        "ok": true,
        "status": 200,
        "body": {
          "user": { "id": "u_1" }
        }
      }
    }
  }
}
```

规则：
- 未声明 mock 的危险 capability 默认不得真实执行。
- mock entry 可声明可选 `match` 与 `result`。没有 `match` 时按旧逻辑直接返回 mock result；没有 `result` 时整个 entry 作为 result，以兼容直接值 mock。
- `match` 为 object 时按 request input 的子集匹配：每个 key 必须存在且匹配，嵌套 object 递归匹配；数组与标量按精确相等匹配。
- capability 已声明但 `match` 不满足时，返回 `test_host_call_unmatched`，记录失败 host call，不写入 action `resultPath`。
- mock 应可记录调用次数、实际输入、成功 result 或失败诊断。

## 事件模拟

测试步骤建议支持：

```text
mount_scene
simulate_event
set_state
apply_patch
advance_time
flush_async
unmount_scene
```

### simulate_event

```json
{
  "type": "simulate_event",
  "target": { "id": "submit_button" },
  "event": "press"
}
```

### advance_time

```json
{
  "type": "advance_time",
  "ms": 500
}
```

用于测试：
- `delay`
- motion tick
- retry / debounce-like runtime behavior

## 查询与断言

测试运行器必须复用统一 Query Selector 语法，不应发明第二套节点查询模型。

Query Selector 是 node selector，只用于匹配运行时 materialized node。V1 支持的节点匹配字段包括 `id`、`semantic`、`type`、`text`，以及 scoped selector 的 `within` / `match` 组合。

`path` 不属于 node selector。任何 target selector 或 scoped selector（包括 `within` 或 `match`）包含 `path` 时，查询必须返回稳定诊断 `query_invalid_selector`，并提示使用 `state_equals` 或状态断言读取 state path。

状态读取不通过 Query Selector 扩展新的 state query DSL。local state 使用 `state_equals` 的 `path`，global state 使用同一状态断言中的 global path，例如 `global.user.id`。

断言建议支持：

```text
state_equals
global_equals
node_exists
node_not_exists
text_equals
prop_equals
diagnostic_contains
host_called
host_not_called
```

示例：

```json
{
  "assert": [
    {
      "type": "state_equals",
      "path": "loading",
      "value": false
    },
    {
      "type": "text_equals",
      "target": { "id": "status_text" },
      "value": "登录成功"
    }
  ]
}
```

## Patch 测试

测试运行器应支持：
- preview patch
- apply patch
- rollback patch
- assert diagnostics / diff

规则：
- preview 仍必须无副作用。
- apply 后可继续执行事件和断言。
- rollback 后应允许再次断言恢复结果。

## 全局状态与 Context

测试运行器必须能：
- 读取 local state
- 读取 global store
- 读取 context-visible state when query target is inside subtree
- 断言写入边界是否被遵守

## 结果格式

建议最小 JSON 输出：

```json
{
  "status": "passed",
  "tests": 1,
  "passed": 1,
  "failed": 0,
  "diagnostics": [],
  "hostCalls": [],
  "durationMs": 42
}
```

失败时至少包含：
- failed assertion
- actual / expected
- target selector or path
- diagnostics snapshot

## CLI

建议命令：

```text
qorm test
qorm test path/to/test.json
qorm test --target web
qorm test --report json
```

## 诊断与错误

最小错误码：

```text
test_scene_not_found
test_mock_missing
test_assertion_failed
query_invalid_selector
test_query_ambiguous
test_host_call_unmocked
test_host_call_unmatched
test_runtime_error
```

## 与 DevServer 的关系

- Test Runner 是离线 / CI 友好的执行协议。
- DevServer 可以复用其 query/assert/mock 子系统。
- DevServer 不应成为 `qorm test` 的唯一实现基础。

## 验收标准

```text
qorm test 能加载 scene 并执行事件模拟
Host Capability 可被受控 mock
本地状态、全局状态、节点文本和 diagnostics 可被断言
时间推进可测试 delay 和 motion
测试运行器与统一 Query Selector 共享同一查询语义
```
