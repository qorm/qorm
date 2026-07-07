# QORM Miniapp Vendor Capability Profiles Specification

## 目标

QORM 的 `miniapp` 不是单一宿主环境，而是一组相似但并不一致的小程序运行时。为了让适配可实现、可测试、可审计，需要在通用 miniapp abstraction 之上，再定义 vendor-specific capability profiles。

本规范定义 miniapp 抽象层、厂商 profile、动态更新与审核边界、调试与 degraded mode 规则。

## 非目标

- 不把所有小程序平台强行压成完全一致的能力模型
- 不绕过厂商审核与宿主安全约束
- 不在 V1 支持任意远端 Bundle 动态执行

## 两层模型

```text
miniapp abstraction
vendor capability profile
```

### Miniapp Abstraction

提供跨厂商的最小共性：
- scene/component/action/motion 基础支持
- 受限 Host Capability 集
- 基础渲染降级能力
- platform_check / capability diagnostics

### Vendor Capability Profile

针对具体厂商定义：
- 能力支持矩阵
- 网络与文件限制
- 审核与发布约束
- 动态更新限制
- 调试与预览限制

## 通用 Miniapp Abstraction

最小能力建议：

```text
network.request (restricted)
storage.read
storage.write
navigation.go
limited clipboard.write
nativeComponents render path
```

最小不保证能力：

```text
filesystem.saveFile
externalSurface
long-running background tasks
arbitrary cross-origin networking
full system clipboard access
```

## Vendor Profile 形状

建议每个厂商提供独立 profile 文件：

```json
{
  "qorm": "0.1",
  "type": "miniapp-vendor-profile",
  "vendor": "wechat",
  "version": "2026.04",
  "capabilities": {
    "network.request": {
      "supported": true,
      "permission": "network.request",
      "domains": ["api.example.com"]
    },
    "filesystem.saveFile": {
      "supported": false
    }
  },
  "reviewConstraints": {
    "remoteBundle": "restricted",
    "dynamicCode": "forbidden"
  }
}
```

## 推荐厂商切分

```text
wechat
alipay
bytedance
baidu
other-compatible
```

## 动态更新与审核约束

Vendor profile 必须声明：
- 是否允许远端资源更新
- 是否允许受限 bundle 拉取
- 是否要求白名单域名
- 是否禁止某类外部依赖
- 是否对运行时代码/资源热更有限制

规则：
- QORM 不得假设 miniapp 环境允许移动端那种完整 Bundle 动态更新。
- 若厂商不允许，Resolver / Runtime 必须降级到静态预置或明确拒绝。

## 调试与测试差异

Vendor profile 应声明：
- 本地调试支持程度
- headless test 覆盖能力
- preview 与真实宿主差异
- mock 必要性

示例字段：

```json
{
  "debug": {
    "localPreview": true,
    "headlessParity": "partial",
    "requiresVendorSimulator": true
  }
}
```

## Degraded Mode

当 QORM 在 miniapp 中因宿主限制而降级时，运行时必须可感知。

建议运行时暴露：

```json
{
  "miniappMode": {
    "vendor": "wechat",
    "degraded": true,
    "disabledCapabilities": ["filesystem.saveFile", "externalSurface"]
  }
}
```

规则：
- `platform_check` 必须能报告 degraded mode。
- Agent / DevServer / Test Runner 应可读到该状态。
- 降级不得静默扩大权限。

## 渲染与组件差异

Vendor profile 应明确：
- native component 支持
- display list / canvas 限制
- text rendering 差异
- overlay / modal / external surface 支持情况

## 权限与安全边界

- 厂商宿主权限高于 QORM policy。
- QORM approval 不能绕过 vendor sandbox。
- 不支持能力必须显式拒绝。
- 降级策略必须安全保持，不得从禁止能力跳到更宽能力路径。

## 与 Platform Pack 的关系

- `miniapp` Platform Pack 定义通用抽象。
- vendor profile 在其上做收窄和具体化。
- build / check / test 时应显式选择 vendor profile。

## 与 Tooling 的关系

建议命令：

```text
qorm check --target miniapp --vendor wechat
qorm run --target miniapp --vendor wechat
qorm test --target miniapp --vendor wechat
qorm platform-check --target miniapp --vendor wechat
```

## Diagnostics

最小错误码：

```text
miniapp_vendor_profile_missing
miniapp_capability_unsupported
miniapp_review_constraint_violation
miniapp_degraded_mode_active
miniapp_vendor_preview_mismatch
```

## 验收标准

```text
miniapp 不再被视为单一统一能力平台
vendor-specific capability profiles 可独立声明约束
动态更新、审核、调试差异被正式纳入模型
runtime / agent / test runner 可感知 degraded mode
vendor sandbox 边界不会被 QORM policy 或 approval 绕过
```