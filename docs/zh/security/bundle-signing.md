<!-- data-lang-nav --> [English](../../security/bundle-signing.md) · 中文

# QORM Bundle 签名

QORM 应用以单个 JSON **Bundle** 的形式分发:内容寻址、可选 ed25519 签名。
签名是空中下载(OTA)UI 交付背后的信任原语 —— Runtime 在运行 Bundle 之前
先验证它,而不是信任投递它的服务器。实现位于 `internal/bundle`(格式与验证)、
`internal/keys`(密钥生成与存储)和 `internal/ota`(拉取与验证)。

本页描述的是代码中实际实现的格式与信任模型。旧文档中出现过的、代码里并不存在
的字段(`bundleVersion`、`minRuntimeVersion`、顶层 `hash`),以下文的代码行为
为准。

## Bundle 格式

一个真实的 Bundle,由 `qorm build examples/counter --key qorm_key --version 1.0.0`
生成(content 部分有省略):

```json
{
  "format": "qorm-bundle/1",
  "content": {
    "app": {
      "id": "qorm_counter",
      "type": "app",
      "name": "QORM Premium Counter",
      "version": "1.0.0",
      "entry": "main"
    },
    "scenes": {
      "main": { "id": "main", "type": "scene", "root": { "...": "..." } }
    },
    "actions": {
      "increment": { "id": "increment", "type": "action", "...": "..." }
    }
  },
  "contentHash": "sha256:FoElQYNuc1V8SWyz8F3g53INEnriY9wR7w1PM1jfzck=",
  "signature": {
    "algorithm": "ed25519",
    "keyId": "fod5SMmyqtJJ",
    "value": "ziVsnGtOX6oiB/sNBq2D18mRJKYgoGx2y6y6+YGlGpn8rao1DPbk7NsL/4CLZiYfe1IRjonB0lZ7v0IckMEADA=="
  }
}
```

顶层字段:

- `format` —— 固定为 `"qorm-bundle/1"`。其他值在解码阶段即被拒绝。
- `content` —— hash 与签名所覆盖的规范载荷:
  - `app` —— 清单文档(来自 `qorm.json`)。用 `qorm build --version` 盖入的
    应用 `version` 就位于这里,处于被签名内容之内。
  - `scenes` —— 场景 id 到场景文档的映射。
  - `actions` —— 动作 id 到动作文档的映射。
  - `locales` —— 可选的 i18n 目录(`locale -> key -> string`)。
  - `requiredCapabilities` —— 可选的能力名称列表(如 `"camera"`),声明应用
    运行所需能力;平台缺少其中任何一项时,Runtime 拒绝激活该 Bundle。用
    `qorm build --require-capability` 盖入。
- `contentHash` —— `"sha256:" + base64(sha256(content 的规范 JSON))`。
- `signature` —— 可选;未签名的 Bundle 省略此字段。

不存在多文件形态:构建 Bundle 时会先把所有源文档(`qorm.json`、
`scenes/*.json`、`actions/*.json`)收集进 `content`,因此单个 hash 即覆盖
清单、全部场景、全部动作与语言目录。"只对根文件签名"的问题在格式层面就不存在
—— 没有根文件,只有 `content`。

## 规范化与内容哈希

哈希是对 `content` 的 Go `encoding/json` 序列化结果计算的,该序列化是确定性的:

- 对象键按字典序排序,
- 数组保持原有顺序,
- 不含无关空白字符。

同一个 `content` 值重新序列化总是得到相同的字节,因此哈希跨机器稳定。
`signature` 与 `contentHash` 本身位于被哈希区域之外;`content` 内的一切 ——
包括盖入的 `version` 与 `requiredCapabilities` —— 都被哈希以及后续的任何签名
覆盖。

## 签名

签名是对 `contentHash` 字符串(即 `"sha256:FoEl..."` 的 ASCII 字节,而非原始
摘要)的**ed25519** 分离签名:

```text
hash:      对规范化 content JSON 做 SHA-256,base64 编码,加 "sha256:" 前缀
signature: 对 contentHash 字符串做 ed25519 签名,base64 编码存入 signature.value
```

`signature.algorithm` 为 `"ed25519"`;其他任何值都会验证失败。
`signature.keyId` 用于标识签名密钥,供展示与诊断使用。key id 由公钥派生 ——
取原始公钥字节 base64 编码的前 12 个字符 —— `qorm build --key` 与
`qorm sign` 会自动填写。由于签名只覆盖 `contentHash`,`keyId` 本身**并未被
签名**;它只能当作提示,绝不能作为信任决策的输入(见下文"吊销")。

## 验证流程

`bundle.VerifyWithRevocation` 按以下顺序执行:

```text
1. 解码 JSON;format != "qorm-bundle/1" 则拒绝
2. 重新计算内容哈希;不匹配则拒绝(内容被篡改)
3. 若未提供可信公钥:到此为止 —— 完整性已验证,真实性未验证
4. 要求 signature 存在
5. 要求 signature.algorithm == "ed25519"
6. base64 解码 signature.value
7. 用可信公钥对 contentHash 做 ed25519 验证
8. 若提供了吊销列表:验证密钥已被吊销则拒绝
```

每一步失败都是硬错误 —— 在 `Verify` 内部,验证绝不会降级为"带警告继续运行"。
(`qorm run` 在你不带 `--trust` 加载 Bundle 时会自己打印一条警告,因为该运行
模式只验证完整性;传入 `--trust` 才会强制要求签名。)

### 完整性与真实性

- **不带 `--trust <key.pub>`** 时,验证仅是防篡改检测:它证明 Bundle 自哈希
  计算之后未被损坏,但对制作者一无所知。`qorm verify` 报告
  `OK ... (integrity)`。
- **带 `--trust`** 时,额外证明该 Bundle 由持有对应私钥的人签名:
  `OK ... (integrity + signature (key fod5SMmyqtJJ))`。
- **`--revoked <list.json>`** 追加吊销检查。

## 威胁模型

Bundle 签名防御的是:

- **被攻陷或恶意的更新服务器 / CDN / 中间人** —— 被修改的 Bundle 过不了哈希
  检查;由可信密钥之外的人重新签名的 Bundle 过不了签名检查。服务器只是传输
  通道,不是信任根。
- **意外损坏** —— 哈希不匹配。
- **签名密钥泄漏** —— 将该 key id 加入本地吊销列表;此后即使签名完全有效也
  会被拒绝。

它不能防御未被吊销的私钥泄漏,也不能替代平台权限:已签名的 Bundle 仍然只能获得
平台与策略授予的能力(见 `permission-model.md`)。

## 密钥

`qorm keygen [--out-dir .]` 生成 ed25519 密钥对并写入两个文件(权限 `0600`):

```text
qorm_key       QORM-ED25519-PRIVATE-KEY\n<base64 私钥>
qorm_key.pub   QORM-ED25519-PUBLIC-KEY\n<base64 公钥>
```

密钥管理建议:

- 私钥不要进仓库、不要随构建产物分发;按发布签名密钥对待(CI secret、硬件
  令牌,或至少是受访问控制的文件)。`qorm sign` 与 `qorm build --key` 只在
  签名时刻需要私钥文件。
- 把**公钥**分发给 Bundle 的消费方 —— 它就是信任根(`--trust qorm_key.pub`,
  或通过 `qorm package --update-url ... --trust qorm_key.pub` 内嵌进包)。
- 轮换方式:生成新密钥对,先把新公钥下发到客户端,再发布用新密钥签名的
  Bundle;过渡期间客户端更新后即信任新密钥,切换完成后可把旧密钥加入吊销
  列表。

## 吊销

吊销列表是一个本地 JSON 文件 —— 裸数组或对象均可:

```json
["fod5SMmyqtJJ"]
```

```json
{ "revoked": ["fod5SMmyqtJJ"] }
```

吊销检查刻意针对**实际验证密钥**派生出的 id,而不是 Bundle 自我声明的
`signature.keyId`:签名只覆盖 `contentHash`,否则持有被吊销密钥的人可以把
`keyId` 改成任意未被吊销的字符串,在签名仍然有效的情况下绕过吊销。

当前实现没有远程吊销刷新;吊销列表就是你通过 `--revoked` 传入(或在打包时
内嵌)的本地文件。更新后的列表与公钥走同一个分发渠道。

## OTA 更新

`internal/ota` 是传输的一半:先拉取字节,**先验证、后激活** —— 顺序从不颠倒。

- `Fetch` 从 `http(s)` URL(30 秒超时,32 MiB 上限)或本地文件路径读取。
- `FetchVerified` 拉取、解码并执行 `VerifyWithRevocation`。任何错误都不会
  产出 Bundle,调用方只需继续运行当前 Bundle。这就是回滚策略:**不作为式
  回滚(rollback by inaction)** —— 失败的更新从不触碰正在运行的应用,因此
  最近一个已知良好的 Bundle 就是当前正在运行的那个。

### 在 Bundle 上执行 `qorm run`

以 Bundle 文件(而非源码目录)运行时,服务器具备 OTA 能力,多两个端点(均对
跨域调用者屏蔽):

- `POST /update {"source": "https://example.com/app.qorm.bundle"}` —— 拉取、
  验证、按本平台检查 `requiredCapabilities`,然后激活。服务器必须以 `--trust`
  启动,否则该端点拒绝服务(403),因为真实性无法验证。任何失败都返回 409,
  正在运行的应用继续保持之前的 Bundle。
- `POST /rollback` —— 重新激活内存中保存的上一个 Bundle(仅一级:即最近一次
  成功 `/update` 所替换掉的那个)。

### 打包应用

`qorm package --update-url <url> --trust <key.pub>` 把 OTA 能力烘焙进打包的
Web/移动应用;这两个参数强制成对出现。信任的划分是刻意的:

- **随包载荷**(包内的 `bundle.json`)经由安装渠道获得信任 —— 应用商店签名 /
  TLS 源站 —— 与投递 Runtime 本身的是同一渠道,因此启动时不再重复验证;
- 每个 **OTA 来源的 Bundle**(从更新服务器拉取的,或从早前更新持久化到的本地
  存储中恢复的)在激活前都要用内嵌的信任公钥做 ed25519 验证。验证失败的
  Bundle 被丢弃,应用回退一层(当前更新 → 上一次更新 → 随包载荷)。

## CLI 参考

```text
qorm keygen [--out-dir .]                                     生成 ed25519 签名密钥对
qorm build <app-dir> [-o out] [--key priv] [--version v] [--require-capability a,b]
                                                              编译 Bundle;给出 --key 时签名
qorm sign <bundle> --key priv [-o out]                        为已有(如 agent 导出的)Bundle 签名
qorm verify <bundle> [--trust pub] [--revoked list.json]      验证完整性(+ 签名,+ 吊销)
qorm run <bundle> [--trust pub] [--revoked list.json]         运行 Bundle;运行前先验证,启用 /update 与 /rollback
```

`qorm sign` 在签名前会重新计算内容哈希,因此给被篡改的 Bundle 重新签名并不能
洗白它 —— 此时签名覆盖的是被篡改后的哈希,只有当篡改者持有私钥时验证方才
会接受。保证链始终是:可信公钥决定谁的内容可以运行。

## 本格式不做的事

明确列出,以免被误当作保证:

- 没有 `minRuntimeVersion` / Runtime 版本门控 —— `qorm-bundle/1` 的 Runtime
  接受任何带该格式标记的 Bundle;版本信息(`content.app.version`)仅供展示
  (出现在更新/回滚状态行中),不是激活条件。
- 签名中没有过期时间或时间戳 —— 有效签名的 Bundle 在其密钥被吊销之前一直
  有效。
- 没有远程吊销或信任元数据刷新 —— 信任根与吊销列表都是你自行分发的本地
  文件。
- 没有按字段签名 —— 签名一次覆盖整个 `content`;无法单独签名某个场景。
