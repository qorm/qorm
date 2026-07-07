# QORM Bundle Signing

Bundle 签名用于保证动态更新安全。

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

V1 必须至少固定一套签名算法，避免实现分叉。推荐：

```text
hash: SHA-256
signature: Ed25519
```

若实现支持额外算法，必须在 Bundle metadata 中显式标识，并由 Runtime 白名单控制。

## Canonicalization

签名必须基于稳定字节序列，而不是任意 JSON 文本。

最小规则：
- UTF-8 编码。
- 对象 key 按字典序排序。
- 数组保持原顺序。
- 不允许依赖空白、换行或缩进差异。
- 多文件 Bundle 必须先展开成规范 Bundle 结构，再计算 hash / signature。

## Bundle 元数据与签名对象

签名应覆盖：

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

不应只签名根文件。

`signature` 应覆盖 canonicalized Bundle 内容；`hash` 应是同一 canonicalized 内容的内容哈希。

## 校验流程

```text
1. 下载 Bundle
2. 校验大小和基本 JSON
3. Canonicalize Bundle
4. 校验 hash
5. 校验 signature
6. 校验 keyId / trust root / revocation status
7. 校验 bundleVersion / minRuntimeVersion
8. 校验 capability requirements
9. 预解析和语义校验
10. 激活
11. 失败回滚
```

## 信任根与 keyId

- `keyId` 标识签名密钥。
- Runtime 必须只信任本地或内置 trust store 中允许的签名者。
- 未知 `keyId`、未受信任签名者或已吊销密钥必须拒绝激活。

## 密钥轮换

最小规则：
- 新旧密钥可以在有限窗口内同时受信任。
- 轮换期间必须先分发新 trust metadata，再分发仅由新 key 签名的 Bundle。
- trust store 更新后，旧 key 可以被标记为 revoked 或 deprecated。

## 吊销

必须支持密钥吊销或签名者吊销机制。

最小语义：
- 吊销信息可以来自本地 trust metadata 或远端刷新。
- 离线环境下若无法刷新吊销信息，应使用最近一次可信的吊销快照。
- 已缓存但被吊销签名的 Bundle 不得继续作为新的激活目标。

## 回滚策略

移动端和生产环境应保存：

```text
current bundle
previous bundle
last known-good bundle
```

### known-good 定义

`last known-good bundle` 至少满足：
- 签名和版本校验通过。
- 成功完成预解析和激活。
- 未触发启动期致命错误。

如果新 Bundle 激活失败，自动回滚到 `last known-good bundle`。

## 校验失败处理

- hash 不匹配：拒绝激活。
- signature 不匹配：拒绝激活。
- keyId 未知或已吊销：拒绝激活。
- `minRuntimeVersion` 不满足：拒绝激活。
- capability requirements 不满足：拒绝激活。
- 预解析或语义校验失败：拒绝激活并回滚。

任何失败都不得降级为“带警告继续运行”。