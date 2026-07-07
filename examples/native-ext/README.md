# native-ext — 自定义原生能力示例

演示「用户中间层」:app 用 `native/` 片段加自己的原生 op(`myBankSDK`),和内置硬件同一套 `qormToNative` / `qormOn<X>` 契约。

- `scenes/main.json` — 一个 `#payBtn` 按钮 + `#result` 文本
- `native/web.js` — 点按钮 → `qormToNative('myBankSDK')`;定义 `qormOnBankSDK` 回调
- `native/ios.swift` — `qormUserOp` 里处理 `myBankSDK`
- `native/android.java` — `@JavascriptInterface myBankSDK()`
- `native/desktop.go` — 桌面 Go 中间层(qormext.Register,编译进单一二进制)

完整说明见 [docs/platforms/native-middlelayer.md](../../docs/platforms/native-middlelayer.md)。

```bash
qorm run examples/native-ext
qorm package examples/native-ext -p ios --dev http://<你的IP>:10383/
```
