# QORM 生态 Registry 与分发服务规划

## 目标

QORM 需要一套正式的生态分发服务，用于发布、解析、审核和分发第三方组件、样式、插件以及相关资产元数据。

该服务不只是“包下载地址”，而是 Resolver、权限模型、签名/信任、生态门户和开发者工作流的共同基础设施。

## 目标产物

- Registry API
- 包元数据模型
- 依赖解析规则
- 发布与审核工作流
- 签名 / 信任 / 吊销机制
- Resolver / CLI / Playground 集成
- Ecosystem Portal 数据源

## 非目标

- 不在第一阶段支持任意脚本包
- 不把 declarative 资产与 executable 插件混成同一信任等级
- 不要求一开始就建设完整商业化结算能力

## 包类型分层

### Declarative Asset Packs

适用于：
- components
- styles
- motions
- templates
- example bundles

特点：
- 主要由 JSON / 资源 / schema 描述构成
- 不直接执行宿主代码
- 重点关注依赖解析、兼容性和签名

### Executable Plugin Packs

适用于：
- host plugins
- wasm plugins
- native bridge extensions

特点：
- 存在可执行逻辑
- 必须进入更严格的权限、审核和签名链路
- 不能与 declarative packs 共享同一默认信任模型

## 包标识模型

每个包至少包含：

```text
package name
package type
version
publisher id
compatibility range
signature info
dependencies
license
source url
```

建议逻辑坐标：

```text
<scope>/<name>@<version>
```

例如：

```text
official/button-kit@1.2.0
community/dark-theme@0.3.1
trusted/game-surface-plugin@2.0.0
```

## 依赖模型

`qorm.json` 规划支持：

```json
{
  "dependencies": {
    "official/button-kit": "^1.2.0",
    "community/dark-theme": "~0.3.0"
  }
}
```

需要补充正式规范：
- version range 语法
- lockfile
- transitive dependency
- conflict resolution
- offline cache
- source priority（官方 registry / git / npm / cargo 等）

## Resolver 集成

Resolver 需要支持：
- 拉取 Registry 元数据
- 解析依赖图
- 校验 compatibility range
- 本地缓存包
- 验证签名与信任状态
- 生成锁定结果

建议新增：

```text
qorm install
qorm update
qorm publish
qorm audit
```

## 发布工作流

### Declarative Pack 发布

```text
prepare package
schema validation
compatibility validation
sign package metadata
publish metadata + artifact
index update
portal visible
```

### Plugin Pack 发布

```text
prepare package
schema validation
permission declaration validation
security review
sign package
publish metadata + artifact
trust status update
portal visible
```

## 审核与信任流

### 基础信任状态

建议至少区分：

```text
draft
published
verified
audited
deprecated
revoked
blocked
```

### 规则

- Declarative pack 与 Plugin pack 分开审核。
- Plugin pack 必须声明权限域。
- 被撤销的包不得继续作为新解析目标。
- 被标记 deprecated 的包仍可解析，但必须提示迁移风险。

## 签名与安全

Registry 层需要与 Bundle Signing 协同，但不应简单复用所有语义。

至少需要：
- 包签名元数据
- publisher key id
- trust root
- revocation status
- 审核结果摘要

插件包额外需要：
- 权限声明摘要
- 平台支持矩阵
- review report metadata

## Registry API 范围

建议最小 API：

```text
package search
package metadata read
version list
dependency manifest read
artifact download
publish upload
trust status read
revocation list read
compatibility matrix read
```

## Ecosystem Portal 集成

Portal 需要直接消费 Registry 元数据，展示：
- 包说明
- 版本
- 类型
- 平台支持
- Render Profile 兼容性
- 依赖图
- trust / audit 状态
- 下载与更新时间

## 运行时与权限边界

- Registry 不授予运行时权限。
- 包被解析成功，不代表其运行时能力自动被允许。
- Plugin pack 的能力仍需经过 Platform + Policy + Approval 裁决。
- Resolver 只能安装和链接符合策略的包。

## 分阶段实施

### Phase 1
- Declarative pack metadata model
- 基础 Registry API
- `dependencies` / lockfile 规划
- Portal 基础列表页

### Phase 2
- Resolver 集成
- 包签名与 trust status
- `qorm install` / `qorm update`

### Phase 3
- Plugin pack review flow
- revocation / audit API
- `qorm publish` / `qorm audit`

### Phase 4
- 多源分发
- 兼容性矩阵
- Portal 高级检索与可视化

## 验收标准

```text
开发者可声明第三方依赖并在编译期解析
Resolver 能校验版本、兼容性、签名与信任状态
Declarative packs 与 Plugin packs 的发布审核链路分离
Portal 能展示第三方包的版本、依赖、信任和兼容性信息
被吊销或 blocked 的包不会继续作为新的解析目标
```

## 关键风险

- 若不区分 declarative pack 与 plugin pack，供应链风险会被放大
- 若先做门户、后做 Registry 数据模型，页面结构会返工
- 若没有 lockfile 与缓存策略，依赖解析会不稳定
- 若没有 trust / revocation，Registry 很难成为生产级基础设施
