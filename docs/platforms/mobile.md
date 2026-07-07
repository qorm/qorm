# QORM Mobile Platform

Mobile 需要专门适配，不能简单复用 Desktop 实现。

## Package it · 打包与试用

```sh
qorm package examples/hardware -p ios     -o hardware-ios      # an Xcode project
qorm package examples/hardware -p android -o hardware-android  # an Android project
```

The app runs offline on device via Go→WASM in a WebView. Examples:
[`hardware`](../../examples/hardware) (the capability catalog exercised),
[`i18n`](../../examples/i18n) (locales, plurals, currency, RTL). See the
[support matrix](support-matrix.md) for per-capability platform support.

## 架构

```text
Mobile App (WebView)
  ↓
Go QORM Runtime，编译为 WASM（cmd/qorm-wasm，随应用一起 Go→WASM）
  ↓
qormToNative op
  ↓
原生桥（iOS: package_native.go iosBridgeBody() / Android: androidMainActivity()）
  ↓
Swift / Kotlin thin bridge
  ↓
iOS / Android system APIs
```

## 动态 Bundle

移动端可以内置解释器动态加载：

```text
qorm.bundle.json
  ↓
version/hash/signature/keyId validation
  ↓
pre-parse
  ↓
activate
  ↓
rollback on failure
```

## Bundle 签名执行要求

- 设备端必须校验 `hash`、`signature`、`keyId`、`minRuntimeVersion`。
- 未知或已吊销签名密钥不得激活。
- 新 Bundle 激活失败时必须回滚到 `last known-good bundle`。
- 离线场景下应使用最近一次可信 trust metadata / revocation snapshot。

## 需要专项处理

```text
safe area
orientation
keyboard show/hide
IME composition
touch gesture
navigation stack
permissions
lifecycle
background/foreground
memory warning
remote bundle update
rollback
```

## iOS

iOS 侧建议：

```text
Go 运行时编译为 WASM（cmd/qorm-wasm）随应用打包
Swift thin bridge（原生桥补 Web API 缺失的能力，如蓝牙/NFC）
WebView 内置 Runtime，本地运行 Bundle
Bundle 作为 UI 描述数据
固定 Host Capability 白名单
```

Bundle 不应新增未审核的底层 Native API。

## Android

Android 侧建议：

```text
Go 运行时编译为 WASM（cmd/qorm-wasm）随应用打包
Kotlin thin bridge（原生桥补 Web API 缺失的能力）
JNI 调用最小化
Host Capability 注册
Bundle cache
```

## 移动端审批存续

- 审批默认应绑定 app session 或用户 session。
- app 更新、账号切换、Bundle 切换、策略变化后应重新评估原审批。
- 涉及文件写入、系统分享、外部域名访问等危险能力时，不应无限期复用旧审批。

## 不考虑 JIT

移动端不使用 Native JIT。性能依靠：

```text
Typed IR
Execution Plan
Dirty Tree
Render Cache
Text Cache
Texture Atlas
GPU-first rendering
```