# QORM Mobile Platform

Mobile requires dedicated adaptation; it cannot simply reuse the Desktop implementation.

## Package it

```sh
qorm package examples/hardware -p ios     -o hardware-ios      # an Xcode project
qorm package examples/hardware -p android -o hardware-android  # an Android project
```

The app runs offline on device via Go→WASM in a WebView. Examples:
[`hardware`](https://github.com/qorm/qorm/tree/main/examples/hardware) (the capability catalog exercised),
[`i18n`](https://github.com/qorm/qorm/tree/main/examples/i18n) (locales, plurals, currency, RTL). See the
[support matrix](support-matrix.md) for per-capability platform support.

## Architecture

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

## Dynamic Bundle

Mobile can load bundles dynamically via a built-in interpreter:

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

## Bundle signature enforcement requirements

- The device must verify `hash`, `signature`, `keyId`, and `minRuntimeVersion`.
- Unknown or revoked signing keys must not be activated.
- If a new Bundle fails to activate, it must roll back to the `last known-good bundle`.
- In offline scenarios, use the most recent trusted trust metadata / revocation snapshot.

## Requires dedicated handling

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

Recommendations for iOS:

```text
Go runtime compiled to WASM (cmd/qorm-wasm), packaged with the app
Swift thin bridge (native bridge fills capabilities missing from the Web API, such as Bluetooth/NFC)
WebView with a built-in Runtime, running the Bundle locally
Bundle as UI description data
A fixed Host Capability allowlist
```

Bundles should not introduce unreviewed low-level Native APIs.

## Android

Recommendations for Android:

```text
Go runtime compiled to WASM (cmd/qorm-wasm), packaged with the app
Kotlin thin bridge (native bridge fills capabilities missing from the Web API)
Minimal JNI calls
Host Capability registration
Bundle cache
```

## Mobile approval persistence

- By default, approvals should be bound to an app session or a user session.
- After an app update, account switch, Bundle switch, or policy change, prior approvals should be re-evaluated.
- For dangerous capabilities such as file writes, system sharing, or external-domain access, old approvals should not be reused indefinitely.

## No JIT

Mobile does not use a Native JIT. Today the only render path is `internal/render`'s
HTML/CSS, displayed by the platform WebView (the Go runtime ships as WASM).
GPU-first rendering (typed IR, execution plan, dirty tree, texture atlas) is
roadmap work — see `planning/`.