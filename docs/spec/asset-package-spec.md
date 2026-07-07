# QORM Asset Package Specification

## 目标

QORM 需要一种正式的资产包规范，用于封装与分发 declarative 资产和 executable 插件，并与 Registry、Resolver、Portal、权限模型和签名体系对接。

本规范定义包分类、目录结构、元数据、兼容性字段、签名字段与最小信任边界。

## 非目标

- 不把组件、样式和可执行插件放入同一默认信任级别
- 不在 V1 支持任意脚本式资产包
- 不要求一开始支持复杂 monorepo package workspace 语义

## 包类型

### Declarative Asset Pack

适用于：
- components
- styles
- motions
- templates
- example bundles

特点：
- 主要包含 JSON、资源、schema、文档
- 不直接执行宿主代码
- 重点关注兼容性、依赖和签名

### Executable Plugin Pack

适用于：
- host plugins
- wasm plugins
- native bridge extensions

特点：
- 存在可执行逻辑
- 必须进入更严格的审核和权限链路

## 包目录结构

### Declarative Asset Pack

```text
package.json
manifest.json
components/
styles/
motions/
resources/
examples/
docs/
signature.json
```

### Executable Plugin Pack

```text
package.json
manifest.json
plugin/
permissions.json
platform-support.json
docs/
signature.json
review.json
```

## 包元数据

最小 `package.json`：

```json
{
  "qorm": "0.1",
  "type": "asset-package",
  "packageType": "declarative",
  "name": "official/button-kit",
  "version": "1.2.3",
  "publisher": "official",
  "license": "MIT",
  "dependencies": {
    "community/dark-theme": "^0.3.0"
  }
}
```

字段说明：
- `packageType`: `declarative` / `plugin`
- `name`: package coordinate
- `version`: semver
- `publisher`: publisher identity
- `dependencies`: 依赖声明，语义由 dependency-resolution spec 定义

## Manifest

`manifest.json` 用于描述包内容边界：

```json
{
  "exports": {
    "components": ["button_primary", "button_secondary"],
    "styles": ["theme.light", "theme.dark"]
  },
  "compatibility": {
    "qorm": ">=0.1.0",
    "platforms": ["desktop", "web", "mobile"],
    "profiles": ["app", "document"]
  }
}
```

## Plugin 附加元数据

`permissions.json`：

```json
{
  "capabilities": ["network.request", "filesystem.saveFile"],
  "requiresApproval": ["filesystem.saveFile"]
}
```

`platform-support.json`：

```json
{
  "platforms": ["desktop", "mobile"],
  "architectures": ["wasm", "native"]
}
```

## Compatibility

包至少应声明：
- QORM version range
- supported platforms
- render profile compatibility
- optional integration mode compatibility

Plugin pack 还应声明：
- runtime requirements
- host capability requirements

## 签名与信任字段

`signature.json` 至少包含：

```json
{
  "keyId": "publisher-2026-q2",
  "signature": "...",
  "integrity": "sha256-..."
}
```

Plugin pack 还可包含：

```json
{
  "reviewState": "audited",
  "reviewSummary": "..."
}
```

## 导出边界

规则：
- 包只能导出在 manifest 中显式列出的对象。
- Resolver 不应隐式暴露未导出内部文件。
- 同名导出冲突必须在安装或解析阶段报错。

## 与 Resolver 的关系

- Resolver 读取 package metadata、manifest、dependencies、signature。
- Resolver 负责版本收敛、来源校验、签名/信任校验。
- Plugin pack 若缺少权限声明，不得进入可安装状态。

## 与 Registry / Portal 的关系

Registry 应至少存储：
- package metadata
- manifest
- dependencies
- trust state
- compatibility matrix

Portal 至少展示：
- 包名称
- 类型
- 版本
- 导出内容摘要
- 平台兼容性
- Render Profile 兼容性
- trust / audit 状态

## 安全边界

- Declarative asset pack 不得因为被安装而自动获得运行时权限。
- Plugin pack 不得绕过 Platform + Policy + Approval。
- 被 revoked 或 blocked 的包不得作为新的解析目标。
- 示例包与生产插件包的信任等级应明确区分。

## Diagnostics

最小错误码：

```text
asset_package_invalid
asset_package_type_conflict
asset_package_export_conflict
asset_package_signature_invalid
asset_package_permission_missing
asset_package_compatibility_mismatch
```

## 验收标准

```text
Declarative pack 与 plugin pack 有清晰的结构与元数据差异
Resolver 能读取并验证包元数据、导出边界与签名
Portal 能展示兼容性、依赖和信任状态
插件包必须显式声明权限与平台支持
资产包规范能与 dependency-resolution 和 ecosystem-registry 规划直接对接
```