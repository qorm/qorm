# QORM OpenClaw Pack

OpenClaw Pack 用于长期运行、跨渠道或自动化型 Agent 接入 QORM。

## 定位

OpenClaw 可以调用 QORM MCP Server，但 QORM 不依赖 OpenClaw。

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

OpenClaw Pack 默认采用更严格权限：

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

- OpenClaw Pack 只能收窄权限，不能放宽平台或系统策略。
- `preview_patch` 仍必须无副作用。
- 任何提升都必须绑定显式 approval 生命周期。

## 适用任务

```text
远程检查 UI
生成修改建议
预览 Patch
平台兼容性检查
布局诊断
文档生成
```

## 不建议任务

```text
自动 apply patch
自动部署
自动调用底层系统能力
自动修改权限配置
```

## 权限提升

任何权限提升必须包含：

```text
原因
范围
目标文件
风险
用户确认
过期时间
```