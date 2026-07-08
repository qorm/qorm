<!-- data-lang-nav --> [English](../../platforms/mobile.md) · 中文

# QORM 移动平台

移动端需要专门的适配,不能简单复用桌面实现。

## 打包

```sh
qorm package examples/hardware -p ios     -o hardware-ios      # an Xcode project
qorm package examples/hardware -p android -o hardware-android  # an Android project
```

应用通过 Go→WASM 在 WebView 中于设备上离线运行。示例:[`hardware`](https://github.com/qorm/qorm/tree/main/examples/hardware)(演示能力目录)、[`i18n`](https://github.com/qorm/qorm/tree/main/examples/i18n)(语言环境、复数、货币、RTL)。有关各能力的平台支持情况,参见[支持矩阵](../../platforms/support-matrix.md)。

## 架构

```text
Mobile App (WebView)
  ↓
Go QORM Runtime, compiled to WASM (cmd/qorm-wasm, Go→WASM shipped with the app)
  ↓
qormToNative op
  ↓
Native bridge (iOS: package_native.go iosBridgeBody() / Android: androidMainActivity())
  ↓
Swift / Kotlin thin bridge
  ↓
iOS / Android system APIs
```

## 动态 Bundle

移动端可以通过内置解释器动态加载 Bundle:

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

## Bundle 签名强制要求

- 设备必须验证 `hash`、`signature`、`keyId` 和 `minRuntimeVersion`。
- 未知或已吊销的签名密钥不得激活。
- 如果新的 Bundle 激活失败,必须回滚到 `last known-good bundle`。
- 在离线场景下,使用最近一次可信的信任元数据 / 吊销快照。

## 需要专门处理

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

iOS 的建议:

```text
Go runtime compiled to WASM (cmd/qorm-wasm), packaged with the app
Swift thin bridge (native bridge fills capabilities missing from the Web API, such as Bluetooth/NFC)
WebView with a built-in Runtime, running the Bundle locally
Bundle as UI description data
A fixed Host Capability allowlist
```

Bundle 不应引入未经审查的底层 Native API。

## Android

Android 的建议:

```text
Go runtime compiled to WASM (cmd/qorm-wasm), packaged with the app
Kotlin thin bridge (native bridge fills capabilities missing from the Web API)
Minimal JNI calls
Host Capability registration
Bundle cache
```

## 移动端授权持久化

- 默认情况下,授权应绑定到应用会话或用户会话。
- 在应用更新、账户切换、Bundle 切换或策略变更之后,之前的授权应被重新评估。
- 对于文件写入、系统分享或跨域访问等危险能力,旧的授权不应无限期复用。

## 无 JIT

移动端不使用 Native JIT。当前唯一的渲染路径是 `internal/render` 生成的 HTML/CSS,由平台 WebView 渲染显示(Go 运行时以 WASM 形式随应用分发)。GPU-first 渲染(Typed IR、Execution Plan、Dirty Tree、纹理图集)属于 roadmap(见 `planning/`)。
