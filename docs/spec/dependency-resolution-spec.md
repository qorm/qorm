# QORM Dependency Resolution Specification

## 目标

QORM 需要一套稳定的依赖解析规则，用于引入官方与第三方的 declarative asset packs 和 executable plugin packs。

本规范定义 `dependencies` 字段、版本范围、锁定结果、解析优先级、本地缓存与冲突规则，为 Resolver、CLI、Registry 和 Portal 提供统一协议。

## 非目标

- 不在 V1 支持任意远端脚本执行
- 不把插件依赖与声明式资产依赖视为同一信任等级
- 不要求一开始支持所有语言生态的复杂 package manager 语义

## `dependencies` 字段

`qorm.json` 可声明依赖：

```json
{
  "qorm": "0.1",
  "type": "app",
  "id": "demo_app",
  "dependencies": {
    "official/button-kit": "^1.2.0",
    "community/dark-theme": "~0.3.0"
  }
}
```

规则：
- key 为 package coordinate，不含版本。
- value 为 version range。
- 依赖默认解析到 Registry metadata，再解析到 artifact。

## 包坐标

规范形式：

```text
<scope>/<name>
```

示例：

```text
official/button-kit
community/dark-theme
trusted/game-surface-plugin
```

包类型在 Registry metadata 中声明，不在 `dependencies` key 中编码。

## Version Range

V1 建议支持：

```text
1.2.3      精确版本
^1.2.0     兼容主版本更新
~0.3.0     兼容次版本补丁更新
>=1.0.0    下界
*          任意版本（不推荐在生产使用）
```

规则：
- 若多个范围冲突且无法求交，Resolver 必须报错。
- Plugin pack 建议默认使用更严格范围，不推荐 `*`。

## Lockfile

建议生成 `qorm.lock`：

```json
{
  "qorm": "0.1",
  "lockVersion": 1,
  "packages": {
    "official/button-kit": {
      "version": "1.2.3",
      "source": "registry:official",
      "integrity": "sha256-...",
      "signature": "...",
      "keyId": "publisher-2026-q2"
    }
  }
}
```

规则：
- 生产构建默认基于 lockfile。
- 更新依赖必须显式改写 lockfile。
- lockfile 是解析结果，不是声明源。

## 依赖图

Resolver 必须支持：
- direct dependencies
- transitive dependencies
- cycle detection
- duplicate package unification
- type-aware validation（asset pack vs plugin pack）

## 冲突规则

### 可合并冲突

- 同一包多个兼容范围存在交集 → 取交集后选择最高兼容版本。

### 不可合并冲突

- 无交集版本范围。
- 同一 package coordinate 对应不同 package type。
- 同一依赖在不同来源上签名/信任状态冲突。

Resolver 必须报结构化错误，例如：

```text
dependency_version_conflict
dependency_type_conflict
dependency_source_conflict
dependency_trust_conflict
```

## Source Priority

V1 支持以下源：

```text
registry
git
local path
npm / cargo bridge (optional adapter)
```

默认优先级建议：

```text
local path > locked source > registry > external bridge
```

规则：
- 若 lockfile 已锁定 source，构建时不得静默切换来源。
- external bridge 仅在策略允许时可用。

## 本地缓存

Resolver 需要支持本地缓存：
- metadata cache
- artifact cache
- trust / revocation cache

最小规则：
- 缓存必须带版本与完整性信息。
- 被 revoked 的包不得继续作为新的解析结果，即便本地已有缓存。
- 离线构建仅在 lockfile 与缓存都可验证时允许。

## 签名与信任

依赖解析前必须校验：
- package metadata signature
- artifact integrity
- keyId trust status
- revocation status

Plugin pack 额外必须校验：
- permission declaration
- platform compatibility
- review state if policy requires

## 与 Registry 的关系

Resolver 读取 Registry：
- package metadata
- version list
- dependency manifest
- compatibility matrix
- trust status

Registry 不直接决定构建是否成功；最终仍由 Resolver + Policy 裁决。

## CLI 行为

建议命令：

```text
qorm install
qorm update
qorm publish
qorm audit
```

最小语义：
- `qorm install`：解析声明并生成/更新 lockfile
- `qorm update`：按 version range 刷新锁定版本
- `qorm publish`：发布包到 Registry
- `qorm audit`：检查 trust、revocation、兼容性风险

## 与 Runtime 的关系

- Runtime 不做完整依赖解析。
- 构建产物必须在 build / resolve 阶段完成依赖收敛。
- Bundle 中应只包含已解析、已验证的依赖结果。

## Diagnostics

最小错误码：

```text
dependency_not_found
dependency_version_conflict
dependency_type_conflict
dependency_source_conflict
dependency_signature_invalid
dependency_revoked
dependency_lock_stale
```

## 验收标准

```text
qorm.json 能声明外部依赖
Resolver 能生成稳定 lockfile
版本冲突、类型冲突、信任冲突会产生结构化错误
离线构建与缓存行为有明确边界
Plugin packs 与 declarative packs 的解析规则和信任要求可区分
```