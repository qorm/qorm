<!-- data-lang-nav --> [English](../../platforms/native-middlelayer.md) · 中文

# 用户中间层:为应用添加你自己的能力

QORM 内置了约 26 项硬件能力(相机/录制、定位、蓝牙、NFC、生物识别、震动/触觉、手电筒、亮度/音量、电池、传感器、网络状态、剪贴板、分享、通知、截屏/录屏等),应用开发者可以**无需修改框架源码**添加自己的能力。**推荐使用 Go** —— 一次编写,处处运行。Swift/Java 仅保留给罕见的纯原生 SDK。

## 推荐:Go 中间层(一份 Go 源码,处处可用)

编写一个 `native/desktop.go`,并用 `qormext.Register` 注册你的 op。它会**编译进桌面二进制**,同时也**编译进面向移动/web 的离线 WASM** —— iOS/Android/桌面/浏览器用的是**同一份 Go 源码**。

```go
//go:build ignore

package main
import "github.com/qorm/qorm/pkg/qormext"
func init() {
    qormext.Register("myBankSDK", func(data map[string]any) string {
        // Your Go logic: algorithms, protocols, HTTP, backend integration... data is the qormToNative payload
        return `qormOnBankSDK("done")`  // return one line of JS, executed back in the app
    })
}
```

在组件中,`qormToNative('myBankSDK', {...})` 会**首先**交给这份 Go(在 WebView 中是 `window.qormWasmOp`,在桌面是编译进去的二进制),而 `qormOnBankSDK` 回调更新 UI。

### Go 中间层直接调用硬件 / 框架内部

从 Go 中你可以直接触及框架内部,而不必只依赖返回 JS:

```go
qormext.Native("bluetoothScan", `{}`)     // → framework native bridge or Web API
qormext.Emit("orderDone", `{"id":42}`)     // → push an event to the UI event bus; the frontend receives it via qormOn('orderDone', fn)
qormext.CallJS(`navigator.vibrate(200)`)   // → any JS
```

- **常见硬件**(相机/定位/传感器/震动)→ Web API,处处可用
- **iOS 缺失的能力**(蓝牙/NFC,Safari 中没有 Web API)→ **框架内置的原生桥接**(由框架维护,你无需编写)
- 结果返回到应用的 `qormOn<X>` 回调
- `qormext.Emit(event, dataJSON)` 使用 **native→UI 事件总线**:下层主动向 UI 推送信号,前端只需用 `qormOn(event, fn)` 监听 —— 无需请求-响应

> 当硬件访问必须经过原生桥接时,**离线包必须包含框架原生桥接**(参见末尾的边界说明)。

## 桥接契约(内置硬件 + 自定义,同一机制)

```
component/JS  ──①qormToNative(op, data)──►  Go middle layer / framework native bridge
component/JS  ◄──③qormOn<X>(result)────────┘  callback returns to web
```

`qormHasNative()`(存在原生桥接)/ `qormHasMobileNative()`(完整的 iOS/Android 桥接);浏览器/桌面会自动回退到 Web API。

## 进阶:罕见的纯原生 SDK(Swift / Java 注入)

如果某项能力**必须**使用平台原生 API,而 Web API 与框架桥接都无法触及(某些厂商专有 SDK),可以添加 `native/ios.swift` / `native/android.java` 片段,它们会在打包时注入到生成的项目中。**大多数情况下这是没有必要的 —— 先用上面的 Go。**

```
myapp/native/
    desktop.go      # recommended: Go middle layer (desktop binary + mobile/web WASM)
    web.js          # web side: qormOn<X> callbacks + wiring buttons to ops
    ios.swift       # advanced: rare iOS pure-native SDK
    android.java    # advanced: rare Android pure-native SDK
```

### `native/ios.swift`

定义一个 `qormUserOp` 函数,并用 `switch` 分派你的 op。iOS 桥接 `switch` 的 default 分支会调用它。在类内部你可以使用 `js(_:)` 回调和 `body` 来获取 `qormToNative` 传入的数据。

```swift
func qormUserOp(_ op: String, _ body: [String: Any]) {
    switch op {
    case "myBankSDK":
        let amount = body["amount"] as? Double ?? 0
        // call your real native SDK here...
        js("qormOnBankSDK(\"paid \\(amount) via native SDK\")")
    default:
        break
    }
}
```

### `native/android.java`

每个 op 是一个 `@JavascriptInterface` 方法(注入到 Bridge 类中,暴露在 `window.qormAndroid` 上)。使用 `js(String)` 回调。

```java
@JavascriptInterface public void myBankSDK() {
    runOnUiThread(() -> js("qormOnBankSDK(\"paid via native SDK (Android)\")"));
}
```

### `native/web.js`

注入到页面中,在**web 一侧**负责两件事:定义 `qormOn<X>` 回调,以及把应用的按钮(按 id)接线到 `qormToNative`。这样浏览器/桌面使用 Web API,移动端使用原生桥接,逻辑相同。

```js
// click #payBtn → trigger the custom native op
document.addEventListener('click', function (e) {
  if (e.target.closest('#payBtn')) qormToNative('myBankSDK', { amount: 9.99 });
});
// native/Web callback: update the UI
function qormOnBankSDK(msg) {
  var el = document.getElementById('result');
  if (el) el.textContent = msg;
}
```

> 组件的 `id`(在 qorm.json 中声明)渲染为 DOM 的 `id`,因此 `web.js` 可以用 `getElementById` / `closest('#id')` 定位它。

## 完整示例

- [`examples/middleware`](../../../examples/middleware) —— **推荐**,展示完整的 Go 中间层:
  `hash`(真正的 `crypto/sha256`,声明式 JSON 无法表达的逻辑)、`visit`(保存在 Go 内存中的有状态计数)、
  `celebrate`(Go 调用框架硬件桥接 `qormext.Native` + 使用 `qormext.Emit` 向 UI 事件总线推送事件)。
  单个 `native/desktop.go`,既编译进桌面二进制,也一并编译进移动/web WASM。
- [`examples/native-ext`](../../../examples/native-ext) —— 最小版本:一个「Pay via Native SDK」按钮,
  带有 `ios.swift` / `android.java` 纯原生逃生舱片段。

```bash
qorm run examples/middleware                         # desktop: Go middle layer compiled into the binary, runs directly
qorm package examples/middleware -p web              # one Go source compiled into the offline WASM
qorm package examples/native-ext -p ios --dev URL    # inject ios.swift into the dev client
```

## 边界与提示

- **iOS / Android**:完全支持。生成的项目会在与内置硬件相同的契约下注入你的片段。
- **移动 / web (WASM)**:同一份 `native/desktop.go` 也会**编译进离线 WASM**(QORM 运行时本身就是 Go→WASM),在 iOS/Android/浏览器的 WebView 内运行 —— **一份 Go 源码,处处可用**。在 `qorm package -p web/ios/android` 期间,打包器会将其注入 `cmd/qorm-wasm` 并一起编译。在 WebView 中,`qormToNative('op')` 会首先交给 WASM 内的 Go(`window.qormWasmOp`),后者返回一行 JS 供执行。
  - **WASM 可以驱动硬件**:Go 中间层可以 `qormext.Native("bluetoothScan", "{}")` / `qormext.CallJS(js)` 来**直接调用框架内部** —— 常见硬件走 Web API,而 iOS 缺失的蓝牙/NFC 走框架原生桥接(内置于框架,用户无需编写)。结果返回到应用的 `qormOn<X>` 回调。
  - **注意(进行中)**:离线包的 WebView 目前搭载 WASM,但**框架原生桥接正在合并进离线 VC**。在该合并完成之前,离线包的硬件走 Web API(iOS 蓝牙/NFC 暂不可用);开发客户端拥有完整的原生桥接。合并之后,离线包 = WASM(含用户 Go)+ 完整的原生桥接,Go 中间层即可触及所有硬件。
- **桌面 (macOS)**:用户中间层就是**纯 Go**,与前端 + 框架一起**编译进那一个二进制**(不是子进程,也不是另一种语言)。编写 `native/desktop.go`:

  ```go
  //go:build ignore

  package main
  import "github.com/qorm/qorm/pkg/qormext"
  func init() {
      qormext.Register("myBankSDK", func(data map[string]any) string {
          // Your Go logic: HTTP, computation, protocols, backend integration... data is the qormToNative payload
          return `qormOnBankSDK("paid via Go middle-layer")` // return one line of JS, which desktop evals back into the page
      })
  }
  ```

  `qorm package -p mac` 会对它连同框架一起运行 `go build`,产出一个**单一二进制**(打包器将其注入 cmd/qorm 以供编译,之后再移除);当桌面桥接遇到未知 op 时,会在 `qormext` 注册表中查找。`web.js` 也照常工作(在桌面上,相机/麦克风/定位直接使用 Web API,因为 localhost 是安全上下文)。
  > `//go:build ignore` 使 `go build ./...` 不会将其单独编译;在打包时,打包器会在编译前剥除这一行。
- **平台一致性警告**:如果你有 `native/ios.swift` 却没有 `native/android.java`(或反之),`qorm package` 会警告你 —— 你的自定义 op 将无法在缺少其片段的平台上运行。
- **权限**:原生能力所需的系统权限(相机、蓝牙、定位等)在生成项目的 `Info.plist` / `AndroidManifest.xml` 中声明;付费团队 / 特殊授权(如 NFC)按平台指引处理。

相关:[Mobile](mobile.md) · [Desktop](desktop.md)
