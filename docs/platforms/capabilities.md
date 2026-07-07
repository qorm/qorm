# 能力清单 · Capabilities

> 本文件由 `internal/capability` 注册表自动生成(`TestCapabilityDocInSync`),请勿手改。
> Auto-generated from the capability registry — do not edit by hand.

QORM 内置 43 个硬件/原生能力。每个能力:组件类型 = 能力名,触发 `qormToNative(op)`,结果回 `qormOn<X>`。AI 可用 `qorm_capabilities` MCP 工具发现全部。

| 能力 Capability | Widget | Ops | 回调 Callback | 平台 Platforms | 说明 |
|---|---|---|---|---|---|
| `camera` | `camera` | — | `—` | ios, android, mac, linux, windows, web | Capture a photo (getUserMedia live path / file capture); binds to state directly. |
| `location` | `location` | location | `qormOnLocation` | ios, android, mac, linux, windows, web | Current GPS location. |
| `recorder` | `recorder` | recordStart<br>recordStop | `qormOnAudio` | ios, android, mac, linux, windows, web | Record microphone audio. |
| `sensors` | `sensors` | motionStart<br>motionStop | `qormOnMotion` | ios, android | Device motion sensors (accelerometer/gyroscope). |
| `biometric` | `biometric` | biometric | `qormOnBiometric` | ios, android, mac | Face ID / fingerprint authentication. (desktop is macOS Touch ID only) |
| `bluetooth` | `bluetooth` | bluetoothScan<br>bluetoothState | `qormOnBluetooth` | ios, android, mac | Scan for BLE devices + adapter state. |
| `wifi` | `wifi` | wifiInfo | `qormOnWifi` | ios, android, mac | Current Wi-Fi network info. (iOS restricts Wi-Fi info to the current network) |
| `nfc` | `nfc` | nfcRead | `qormOnNfc` | ios, android | Read an NFC/NDEF tag. (iOS requires a paid Apple Developer team) |
| `volume` | `volume` | volumeGet<br>volumeSet<br>volumeUp<br>volumeDown | `qormOnVolume` | ios, android, mac, linux | System output volume. |
| `brightness` | `brightness` | brightnessGet<br>brightnessSet<br>brightnessUp<br>brightnessDown | `qormOnBrightness` | ios, android, mac | Screen brightness. (desktop is macOS-only for now) |
| `vibrate` | `vibrate` | vibrate | `—` | ios, android, web | Basic vibration. |
| `torch` | `torch` | torchGet<br>torchToggle | `qormOnTorch` | ios, android | Flashlight / torch. |
| `battery` | `battery` | battery | `qormOnBattery` | ios, android, mac, linux, web | Battery level + charging state. |
| `notify` | `notify` | notify | `qormOnNotify` | ios, mac, linux, web | Local notification. (Android falls back to the Web Notification API) |
| `badge` | `dockbadge` | badge | `—` | ios, mac | App icon / Dock badge count. |
| `screenshot` | `screenshot` | screenshot | `qormOnScreenshot` | ios, android, mac, web | Capture the screen / app view. |
| `screenrecord` | `screenrecord` | screenRecordStart<br>screenRecordStop | `qormOnScreenRecord` | ios, mac, web | Record the screen. (Android needs MediaProjection (pending)) |
| `share` | `share` | share | `qormOnShare` | ios, android, mac, web | Open the system share sheet. |
| `clipboard` | `clipboard` | clipboardSet<br>clipboardGet | `qormOnClipboard` | ios, android, mac, linux, windows, web | Read/write the clipboard. |
| `deviceinfo` | `deviceinfo` | deviceInfo | `qormOnDeviceInfo` | ios, android, mac, linux, windows, web | Device model / OS / name. |
| `network` | `network` | networkStatus | `qormOnNetwork` | ios, android, mac, linux, windows, web | Online state + connection type. |
| `keepawake` | `keepawake` | keepAwake | `—` | ios, android, mac, linux, web | Prevent the screen from sleeping. |
| `haptics` | `haptics` | haptic | `—` | ios, android, web | Fine haptic feedback (success/warning/error/selection). |
| `storage` | `storage` | storageSet<br>storageGet | `qormOnStorage` | ios, android, mac, linux, windows, web | Key/value local storage. |
| `loginitem` | `loginitem` | loginItem<br>loginItemGet | `qormOnLoginItem` | mac | Launch at login. (macOS-only (needs the installed .app)) |
| `stt` | `stt` | listenStart<br>listenStop | `qormOnSpeech` | ios, android, web | Speech to text (voice input / dictation). |
| `securestorage` | `securestorage` | secureSet<br>secureGet | `qormOnSecure` | ios, android, mac, web | Secure key/value storage (iOS Keychain / Android Keystore). (web falls back to localStorage (not hardware-encrypted)) |
| `filepicker` | `filepicker` | pickFile | `qormOnFile` | ios, android, web | Pick a file from storage (returns name/size/data URL). |
| `photopicker` | `photopicker` | pickPhoto | `qormOnPhoto` | ios, android, web | Pick an existing photo from the library (returns a data URL). |
| `orientation` | `orientation` | lockOrientation | `—` | android, web | Lock screen orientation (portrait/landscape). (iOS orientation lock needs AppDelegate support (pending)) |
| `videocapture` | `videocapture` | recordVideo | `qormOnVideo` | ios, web | Record a video with the camera. (Android via MediaRecorder pending) |
| `qrscan` | `qrscan` | scanQR | `qormOnScan` | ios, web | Scan a QR code / barcode with the camera. (Android needs CameraX+MLKit (pending)) |
| `tts` | `tts` | speak<br>speakStop | `—` | ios, android, mac, linux, web | Text to speech (speak a string aloud). |
| `compass` | `compass` | headingStart<br>headingStop | `qormOnHeading` | ios, android, web | Compass heading (degrees from magnetic north). |
| `proximity` | `proximity` | proximityStart<br>proximityStop | `qormOnProximity` | ios, android | Proximity sensor (near/far). |
| `pedometer` | `pedometer` | pedometerStart<br>pedometerStop | `qormOnSteps` | ios, android | Step counter / pedometer. |
| `barometer` | `barometer` | barometerStart<br>barometerStop | `qormOnPressure` | ios, android | Barometric pressure / relative altitude. |
| `contacts` | `contacts` | pickContact | `qormOnContact` | ios, android, web | Pick a contact (name + phone). |
| `calendar` | `calendar` | addEvent | `qormOnCalendar` | ios, android | Add a calendar event. |
| `systemmodes` | `systemmodes` | getModes | `qormOnModes` | ios, android, mac, web | Read system modes: low-power, dark/appearance, airplane (Android), do-not-disturb (Android). Null where a platform has no public API. |
| `insets` | `insets` | getInsets | `qormOnInsets` | ios, android, web | Safe-area insets in points/dp (status bar, notch, home indicator, nav bar). |
| `openurl` | `openurl` | openURL | `qormOnOpenUrl` | ios, android, mac, linux, windows, web | Open a URL / deep link (http, mailto, tel, sms, maps). |
| `screens` | `screens` | screens | `qormOnScreens` | mac, linux, windows | Enumerate displays. |
