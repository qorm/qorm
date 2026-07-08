# User middle layer: adding your own capabilities to an app

QORM ships with about 26 built-in hardware capabilities (camera/recording, location, Bluetooth, NFC, biometrics, vibration/haptics, flashlight, brightness/volume, battery, sensors, network status, clipboard, sharing, notifications, screenshot/screen recording, and more), and app developers can add their own capabilities **without modifying framework source code**. **Go is recommended** -- write it once, run it everywhere. Swift/Java are reserved only for rare pure-native SDKs.

## Recommended: Go middle layer (one Go source, usable everywhere)

Write a `native/desktop.go` and register your op with `qormext.Register`. It is **compiled into the desktop binary** and also **compiled into the offline WASM for mobile/web** -- the **same Go source** for iOS/Android/desktop/browser.

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

In a component, `qormToNative('myBankSDK', {...})` is handed **first** to this Go (`window.qormWasmOp` in a WebView, the compiled-in binary on desktop), and the `qormOnBankSDK` callback updates the UI.

### The Go middle layer calls hardware / framework internals directly

From Go you can reach framework internals directly, rather than relying only on returning JS:

```go
qormext.Native("bluetoothScan", `{}`)     // → framework native bridge or Web API
qormext.Emit("orderDone", `{"id":42}`)     // → push an event to the UI event bus; the frontend receives it via qormOn('orderDone', fn)
qormext.CallJS(`navigator.vibrate(200)`)   // → any JS
```

- **Common hardware** (camera/location/sensors/vibration) → Web API, works everywhere
- **What iOS lacks** (Bluetooth/NFC, no Web API in Safari) → **the framework's built-in native bridge** (maintained by the framework; you don't write it)
- Results return to the app's `qormOn<X>` callback
- `qormext.Emit(event, dataJSON)` uses the **native→UI event bus**: the lower layer actively pushes signals to the UI, and the frontend only listens with `qormOn(event, fn)` -- no request-response needed

> When hardware access must go through the native bridge, **the offline package must include the framework native bridge** (see the boundaries at the end).

## Bridging contract (built-in hardware + custom, same mechanism)

```
component/JS  ──①qormToNative(op, data)──►  Go middle layer / framework native bridge
component/JS  ◄──③qormOn<X>(result)────────┘  callback returns to web
```

`qormHasNative()` (a native bridge is present) / `qormHasMobileNative()` (the full iOS/Android bridge); browser/desktop automatically fall back to the Web API.

## Advanced: rare pure-native SDKs (Swift / Java injection)

If a capability **must** use a platform-native API and neither the Web API nor the framework bridge can reach it (certain vendor-proprietary SDKs), add `native/ios.swift` / `native/android.java` snippets, which are injected into the generated project at package time. **Most of the time this is unnecessary -- use the Go above first.**

```
myapp/native/
    desktop.go      # recommended: Go middle layer (desktop binary + mobile/web WASM)
    web.js          # web side: qormOn<X> callbacks + wiring buttons to ops
    ios.swift       # advanced: rare iOS pure-native SDK
    android.java    # advanced: rare Android pure-native SDK
```

### `native/ios.swift`

Define a `qormUserOp` function and dispatch your ops with a `switch`. The default branch of the iOS bridge's `switch` calls it. Inside the class you can use the `js(_:)` callback and `body` to get the data passed by `qormToNative`.

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

Each op is a `@JavascriptInterface` method (injected into the Bridge class, exposed on `window.qormAndroid`). Use the `js(String)` callback.

```java
@JavascriptInterface public void myBankSDK() {
    runOnUiThread(() -> js("qormOnBankSDK(\"paid via native SDK (Android)\")"));
}
```

### `native/web.js`

Injected into the page, responsible for two things on the **web side**: defining `qormOn<X>` callbacks, and wiring the app's buttons (by id) to `qormToNative`. This way browser/desktop use the Web API and mobile uses the native bridge, with the same logic.

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

> A component's `id` (declared in qorm.json) renders as the DOM `id`, so `web.js` can locate it with `getElementById` / `closest('#id')`.

## Complete examples

- [`examples/middleware`](https://github.com/qorm/qorm/tree/main/examples/middleware) -- **recommended**, showing the full Go middle layer:
  `hash` (real `crypto/sha256`, logic that declarative JSON cannot express), `visit` (stateful counting held in Go memory),
  `celebrate` (Go calling the framework hardware bridge `qormext.Native` + using `qormext.Emit` to push events to the UI event bus).
  A single `native/desktop.go`, compiled into the desktop binary and the mobile/web WASM alike.
- [`examples/native-ext`](https://github.com/qorm/qorm/tree/main/examples/native-ext) -- minimal version: a "Pay via Native SDK" button,
  with `ios.swift` / `android.java` pure-native escape-hatch snippets.

```bash
qorm run examples/middleware                         # desktop: Go middle layer compiled into the binary, runs directly
qorm package examples/middleware -p web              # one Go source compiled into the offline WASM
qorm package examples/native-ext -p ios --dev URL    # inject ios.swift into the dev client
```

## Boundaries and tips

- **iOS / Android**: fully supported. The generated project injects your snippets under the same contract as the built-in hardware.
- **Mobile / web (WASM)**: the same `native/desktop.go` is also **compiled into the offline WASM** (the QORM runtime is itself Go→WASM), running inside the WebView on iOS/Android/browser -- **one Go source, usable everywhere**. During `qorm package -p web/ios/android`, the packager injects it into `cmd/qorm-wasm` and compiles it together. In the WebView, `qormToNative('op')` is handed first to the Go inside the WASM (`window.qormWasmOp`), which returns one line of JS to execute.
  - **WASM can drive hardware**: the Go middle layer can `qormext.Native("bluetoothScan", "{}")` / `qormext.CallJS(js)` to **call framework internals directly** -- common hardware goes through the Web API, and the Bluetooth/NFC that iOS lacks go through the framework native bridge (built into the framework; the user doesn't write it). Results return to the app's `qormOn<X>` callback.
  - **Note (in progress)**: the offline package's WebView currently ships WASM, but **the framework native bridge is being merged into the offline VC**. Before that merge, offline-package hardware goes through the Web API (iOS Bluetooth/NFC temporarily unavailable); the dev client has the full native bridge. After the merge, the offline package = WASM (including user Go) + the full native bridge, and the Go middle layer reaches all hardware.
- **Desktop (macOS)**: the user middle layer is just **Go**, **compiled into that single binary** together with the frontend + framework (not a subprocess, not another language). Write `native/desktop.go`:

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

  `qorm package -p mac` runs `go build` on it together with the framework into a **single binary** (the packager injects it into cmd/qorm for compilation, then removes it afterward); when the desktop bridge hits an unknown op, it looks it up in the `qormext` registry. `web.js` also works as usual (on desktop, camera/microphone/location use the Web API directly, since localhost is a secure context).
  > `//go:build ignore` keeps `go build ./...` from compiling it standalone; at package time the packager strips this line before compiling it in.
- **Platform consistency warning**: if you have `native/ios.swift` but no `native/android.java` (or vice versa), `qorm package` warns you -- your custom op will not run on the platform that is missing its snippet.
- **Permissions**: the system permissions that native capabilities require (camera, Bluetooth, location, and so on) are declared in the generated project's `Info.plist` / `AndroidManifest.xml`; paid-team / special entitlements (such as NFC) are handled per the platform's guidance.

Related: [Mobile](mobile.md) · [Desktop](desktop.md)
