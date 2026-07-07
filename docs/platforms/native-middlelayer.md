# 用户中间层:给 app 加自己的能力

QORM 内置了约 26 个硬件能力(摄像头/录音、定位、蓝牙、NFC、生物识别、震动/触感、手电、亮度/音量、电池、传感器、网络状态、剪贴板、分享、通知、截屏/录屏……),app 开发者也能**不改框架源码**加自己的能力。**推荐用 Go**——写一份、到处跑。Swift/Java 只留给罕见的纯原生 SDK。

## 推荐:Go 中间层(一份 Go,到处能用)

写一个 `native/desktop.go`,用 `qormext.Register` 注册你的 op。它会**编进桌面二进制**,也**编进手机/网页的离线 WASM**——iOS/Android/桌面/浏览器**同一份 Go**。

```go
//go:build ignore

package main
import "github.com/qorm/qorm/pkg/qormext"
func init() {
    qormext.Register("myBankSDK", func(data map[string]any) string {
        // 你的 Go 逻辑:算法、协议、HTTP、后端集成……data 是 qormToNative 的 payload
        return `qormOnBankSDK("done")`  // 返回一行 JS,回 app 执行
    })
}
```

组件里 `qormToNative('myBankSDK', {...})` 会**优先**交给这份 Go(WebView 里是 `window.qormWasmOp`,桌面是编入的二进制),`qormOnBankSDK` 回调更新 UI。

### Go 中间层直接调硬件/框架底层

Go 里能直接够到框架底层,不用只靠返回 JS:

```go
qormext.Native("bluetoothScan", `{}`)     // → 框架原生桥 或 Web API
qormext.Emit("orderDone", `{"id":42}`)     // → 推事件到 UI 事件总线,前端 qormOn('orderDone', fn) 收
qormext.CallJS(`navigator.vibrate(200)`)   // → 任意 JS
```

- **通用硬件**(摄像头/定位/传感器/震动)→ Web API,到处能用
- **iOS 缺的**(蓝牙/NFC,Safari 无 Web API)→ **框架内置原生桥**(框架维护,你不写)
- 结果回到 app 的 `qormOn<X>` 回调
- `qormext.Emit(event, dataJSON)` 走**原生→UI 事件总线**:底层主动向界面推信号,前端只管 `qormOn(event, fn)` 监听——不用请求-响应

> 硬件访问要走原生桥时,**离线包需带框架原生桥**(见文末边界)。

## 桥接契约(内置硬件 + 自定义,同一套)

```
组件/JS  ──①qormToNative(op, data)──►  Go 中间层 / 框架原生桥
组件/JS  ◄──③qormOn<X>(result)────────┘  回调送回 web
```

`qormHasNative()`(有原生桥)/ `qormHasMobileNative()`(iOS/Android 全量桥),浏览器/桌面自动回退到 Web API。

## 高级:罕见纯原生 SDK(Swift / Java 注入)

如果某能力**必须**用平台原生 API 且 Web API + 框架桥都够不到(某些厂商专有 SDK),再放 `native/ios.swift` / `native/android.java` 片段,打包时注入生成工程。**大多数情况不需要——先用上面的 Go。**

```
myapp/native/
    desktop.go      # 推荐:Go 中间层(桌面二进制 + 手机/网页 WASM)
    web.js          # web 侧:qormOn<X> 回调 + 把按钮接到 op
    ios.swift       # 高级:罕见 iOS 纯原生 SDK
    android.java    # 高级:罕见 Android 纯原生 SDK
```

### `native/ios.swift`

定义一个 `qormUserOp` 函数,用 `switch` 分发你的 op。iOS 桥的 `switch` 默认分支会调它。类里可用 `js(_:)` 回调、`body` 拿到 `qormToNative` 传的数据。

```swift
func qormUserOp(_ op: String, _ body: [String: Any]) {
    switch op {
    case "myBankSDK":
        let amount = body["amount"] as? Double ?? 0
        // 这里调你真实的原生 SDK……
        js("qormOnBankSDK(\"paid \\(amount) via native SDK\")")
    default:
        break
    }
}
```

### `native/android.java`

每个 op 是一个 `@JavascriptInterface` 方法(注入到 Bridge 类,挂在 `window.qormAndroid` 上)。用 `js(String)` 回调。

```java
@JavascriptInterface public void myBankSDK() {
    runOnUiThread(() -> js("qormOnBankSDK(\"paid via native SDK (Android)\")"));
}
```

### `native/web.js`

注入到页面,负责 **web 侧**两件事:定义 `qormOn<X>` 回调,以及把 app 里的按钮(按 id)接到 `qormToNative`。这样浏览器/桌面用 Web API、手机用原生桥,同一份逻辑。

```js
// 点 #payBtn → 触发自定义原生 op
document.addEventListener('click', function (e) {
  if (e.target.closest('#payBtn')) qormToNative('myBankSDK', { amount: 9.99 });
});
// 原生/Web 回调:更新 UI
function qormOnBankSDK(msg) {
  var el = document.getElementById('result');
  if (el) el.textContent = msg;
}
```

> 组件的 `id`(qorm.json 里写的)会渲染成 DOM 的 `id`,所以 `web.js` 能用 `getElementById` / `closest('#id')` 定位它们。

## 完整示例

- [`examples/middleware`](../../examples/middleware) —— **推荐**,展示 Go 中间层全貌:
  `hash`(真 `crypto/sha256`,声明式 JSON 做不到的逻辑)、`visit`(常驻 Go 内存的有状态计数)、
  `celebrate`(Go 调框架硬件桥 `qormext.Native` + 用 `qormext.Emit` 往 UI 事件总线推事件)。
  一份 `native/desktop.go`,桌面二进制 + 手机/网页 WASM 都编进去。
- [`examples/native-ext`](../../examples/native-ext) —— 最小版:一个「Pay via Native SDK」按钮,
  含 `ios.swift` / `android.java` 纯原生逃生舱片段。

```bash
qorm run examples/middleware                         # 桌面:Go 中间层编进二进制,直接跑
qorm package examples/middleware -p web              # 一份 Go 编进离线 WASM
qorm package examples/native-ext -p ios --dev URL    # 注入 ios.swift 到 dev 客户端
```

## 边界与提示

- **iOS / Android**:完全支持。生成的工程注入你的片段,和内置硬件同一套契约。
- **手机 / 网页(WASM)**:同一个 `native/desktop.go` 也**编进离线 WASM**(QORM 运行时本就是 Go→WASM),在 iOS/Android/浏览器的 WebView 里跑——**一份 Go,到处能用**。`qorm package -p web/ios/android` 时打包器把它注入 `cmd/qorm-wasm` 一起编。WebView 里 `qormToNative('op')` 优先交给 WASM 里的 Go(`window.qormWasmOp`),返回一行 JS 执行。
  - **WASM 能操作硬件**:Go 中间层可以 `qormext.Native("bluetoothScan", "{}")` / `qormext.CallJS(js)` **直接调框架底层**——通用硬件走 Web API,iOS 缺的蓝牙/NFC 走框架原生桥(框架已内置,用户不写)。结果回到 app 的 `qormOn<X>` 回调。
  - **注意(收敛中)**:离线包的 WebView 目前带 WASM,但**框架原生桥正在合并进离线 VC**。合并前,离线包硬件走 Web API(iOS 蓝牙/NFC 暂缺);dev 客户端有完整原生桥。合并后离线包 = WASM(含用户 Go)+ 全量原生桥,Go 中间层够到全部硬件。
- **桌面(macOS)**:用户中间层就是 **Go**,和前端+框架一起**编译进那一个二进制**(不是子进程、不是别的语言)。写 `native/desktop.go`:

  ```go
  //go:build ignore

  package main
  import "github.com/qorm/qorm/pkg/qormext"
  func init() {
      qormext.Register("myBankSDK", func(data map[string]any) string {
          // 你的 Go 逻辑:HTTP、计算、协议、后端集成……data 是 qormToNative 的 payload
          return `qormOnBankSDK("paid via Go middle-layer")` // 返回一行 JS,桌面 eval 回页面
      })
  }
  ```

  `qorm package -p mac` 会把它和框架一起 `go build` 成**单一二进制**(打包器把它注入 cmd/qorm 编译、完了删掉);桌面桥遇到未知 op 就查 `qormext` 注册表。`web.js` 也照常生效(桌面上摄像头/麦克风/定位直接用 Web API,localhost 即安全上下文)。
  > `//go:build ignore` 让 `go build ./...` 不单独编它;打包时打包器会剥掉这行再编进去。
- **平台一致性警告**:如果你有 `native/ios.swift` 却没有 `native/android.java`(反之亦然),`qorm package` 会警告——你的自定义 op 在缺片段的平台上不会执行。
- **权限**:原生能力涉及的系统权限(相机、蓝牙、定位…)在生成工程的 `Info.plist` / `AndroidManifest.xml` 里声明;付费 team / 特殊 entitlement(如 NFC)按平台提示处理。

相关:[移动端](mobile.md) · [桌面端](desktop.md)
