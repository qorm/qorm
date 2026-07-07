<!-- data-lang-nav --> [English](../../agent/openclaw-pack.md) · 中文

# QORM OpenClaw Pack

OpenClaw Pack 面向连接到 QORM 的长时运行、跨渠道或自动化的 agent。

## 定位

OpenClaw 可以调用 QORM MCP Server,但 QORM 不依赖 OpenClaw。

```text
OpenClaw Agent
  ↓
QORM MCP Server
  ↓
QORM Agent Protocol
  ↓
QORM Core
```

## 默认策略

OpenClaw Pack 默认使用更严格的权限策略:

```text
inspect: allow
validate: allow
preview_patch: allow
explain: allow
apply_patch: deny by default
host.call: deny by default
shell: deny
filesystem.write: deny unless approved
```

## 权限边界

- OpenClaw Pack 只能收窄权限;它不能放松平台或系统策略。
- `preview_patch` 仍必须保持无副作用。
- 任何提权都必须绑定到一个显式的批准生命周期。

## 适用任务

```text
Remote UI inspection
Generating change suggestions
Previewing Patches
Platform compatibility checks
Layout diagnostics
Documentation generation
```

## 不建议的任务

```text
Automatic patch apply
Automatic deployment
Automatic invocation of low-level system capabilities
Automatic modification of permission configuration
```

## 权限提升

任何权限提升都必须包含:

```text
Reason
Scope
Target files
Risk
User confirmation
Expiration time
```
