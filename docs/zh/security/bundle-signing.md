<!-- data-lang-nav --> [English](../../security/bundle-signing.md) · 中文

# QORM Bundle 签名

Bundle 签名用于保障动态更新的安全性。

## Bundle 元数据

```json
{
  "qorm": "0.1",
  "type": "bundle",
  "id": "demo_bundle",
  "bundleVersion": "1.0.0",
  "minRuntimeVersion": "0.1.0",
  "hash": "...",
  "signature": "...",
  "keyId": "release-2026-q2"
}
```

## 签名算法

V1 必须至少固定一种签名算法,以避免实现之间的分歧。推荐:

```text
hash: SHA-256
signature: Ed25519
```

如果某个实现支持额外的算法,它们必须在 Bundle 元数据中显式标识,并由 Runtime 的允许列表控制。

## 规范化(Canonicalization)

签名必须基于稳定的字节序列,而非任意的 JSON 文本。

最低规则:
- UTF-8 编码。
- 对象键按字典序排序。
- 数组保持其原始顺序。
- 不得依赖于空白字符、换行或缩进的差异。
- 多文件 Bundle 必须先展开为规范化的 Bundle 结构,然后再计算 hash / signature。

## Bundle 元数据与被签名对象

签名应覆盖:

```text
manifest
resolved scenes
resolved components
resolved styles
resolved actions
resolved motions
resources
capability requirements
compiled execution plans
```

仅对根文件签名是不够的。

`signature` 应覆盖规范化后的 Bundle 内容;`hash` 应为同一规范化内容的内容哈希。

## 验证流程

```text
1. Download Bundle
2. Verify size and basic JSON
3. Canonicalize Bundle
4. Verify hash
5. Verify signature
6. Verify keyId / trust root / revocation status
7. Verify bundleVersion / minRuntimeVersion
8. Verify capability requirements
9. Pre-resolution and semantic validation
10. Activate
11. Roll back on failure
```

## 信任根与 keyId

- `keyId` 标识签名密钥。
- Runtime 只能信任本地或内置信任库所允许的签名者。
- 未知的 `keyId`、不受信任的签名者或被吊销的密钥必须被拒绝激活。

## 密钥轮换

最低规则:
- 在有限的时间窗口内,可以同时信任旧密钥和新密钥。
- 在轮换期间,必须先分发新的信任元数据,然后再分发仅由新密钥签名的 Bundle。
- 信任库更新后,旧密钥可以被标记为已吊销或已弃用。

## 吊销

必须支持某种密钥吊销或签名者吊销机制。

最低语义:
- 吊销信息可以来自本地信任元数据或远程刷新。
- 在无法刷新吊销信息的离线环境中,应使用最近一次可信的吊销快照。
- 签名已被吊销的缓存 Bundle 不得继续作为新的激活目标。

## 回滚策略

移动端和生产环境应保留:

```text
current bundle
previous bundle
last known-good bundle
```

### known-good 定义

一个 `last known-good bundle` 必须至少满足:
- 签名和版本验证通过。
- 预解析和激活成功完成。
- 未触发致命的启动错误。

如果一个新 Bundle 激活失败,则自动回滚到 `last known-good bundle`。

## 处理验证失败

- hash 不匹配:拒绝激活。
- signature 不匹配:拒绝激活。
- keyId 未知或已吊销:拒绝激活。
- 不满足 `minRuntimeVersion`:拒绝激活。
- 不满足能力要求(capability requirements):拒绝激活。
- 预解析或语义验证失败:拒绝激活并回滚。

任何失败都不得被降级为"带警告继续运行"。
