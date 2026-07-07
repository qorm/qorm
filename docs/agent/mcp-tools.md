# QORM MCP Tools

QORM MCP Server 用于让 Agent 调用 QORM 的检查、验证、Patch、模拟和解释能力。

## 工具清单

```text
qorm.inspect_scene
qorm.inspect_node
qorm.validate_bundle
qorm.preview_patch
qorm.apply_patch
qorm.rollback_patch
qorm.simulate_event
qorm.explain_node
qorm.platform_check
qorm.layout_debug
qorm.render_profile_check
qorm.list_capabilities
qorm.build_bundle
```

## MCP 不进入渲染热路径

MCP 用于 Agent 级操作，不用于：

```text
每帧渲染
鼠标移动热路径
动画 tick
文本 shaping
GPU submit
```

## 工具安全分级

```text
read-only             inspect_scene inspect_node validate_bundle explain_node platform_check layout_debug render_profile_check list_capabilities
preview-only          preview_patch simulate_event
mutating              apply_patch rollback_patch build_bundle
external-side-effect  host-affecting build/deploy style tools if introduced later
```

## 默认权限

默认允许：

```text
inspect
validate
preview_patch
explain
platform_check
layout_debug
```

默认禁止：

```text
apply_patch
host.call
filesystem.saveFile
shell
deploy
```

其中 `filesystem.saveFile` 对应文件写入类 host capability。

## 审批门控

- `preview_patch` 默认允许，但必须无副作用。
- `apply_patch` 必须绑定 preview 结果，并在需要时要求有效 `approvalId`。
- `build_bundle` 若会覆盖现有产物或触发外部发布流程，应按策略要求确认。

## 审计要求

每次 MCP 调用至少应记录：

```text
tool name
caller id
input summary
result status
approval id
audit event id
timestamp
```

## 示例：validate_bundle

```json
{
  "tool": "qorm.validate_bundle",
  "input": {
    "path": "qorm.json",
    "target": "mobile"
  }
}
```

## 示例：preview_patch

```json
{
  "tool": "qorm.preview_patch",
  "input": {
    "patches": [
      {
        "op": "replace",
        "path": "/scenes/main/nodes/title/value",
        "value": "新的标题"
      }
    ]
  }
}
```