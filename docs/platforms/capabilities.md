# Capabilities

> Auto-generated from the capability registry — do not edit by hand.

QORM has 43 built-in hardware/native capabilities. For each: the widget type is the capability name, `qormToNative(op)` triggers it, and the result comes back via `qormOn<X>`. Agents discover them all with the `qorm_capabilities` MCP tool.

| Capability | Widget | Ops | Callback | Platforms | Description |
|---|---|---|---|---|---|
| `camera` | `camera` | — | `—` | ios, android, mac, linux, windows, web | Capture a photo (getUserMedia live path / file capture); binds to state directly. |
| `location` | `location` | location | `qormOnLocation` | ios, android, mac, linux, windows, web | Current GPS location. |
| `recorder` | `recorder` | recordStart<br>recordStop | `qormOnAudio` | ios, android, mac, linux, windows, web | Record microphone audio. |
| `sensors` | `sensors` | motionStart<br>motionStop | `qormOnMotion` | ios, android | Device motion sensors (accelerometer/gyroscope). |
| `biometric` | `biometric` | biometric | `qormOnBiometric` | ios, android, mac | Face ID / fingerprint authentication. (desktop is macOS Touch ID only) |
| `bluetooth` | `bluetooth` | bluetoothScan<br>bluetoothState | `qormOnBluetooth` | ios, android, mac | Scan for BLE devices + adapter state. |
| `wifi` | `wifi` | wifiInfo | `qormOnWifi` | ios, android, mac | Current Wi-Fi network info. (iOS restricts Wi-Fi info to the current network) |
| `nfc` | `nfc` | nfcRead | `qormOnNfc` | ios, android | Read an NFC/NDEF tag. (iOS requires a paid Apple Developer team) |
| `volume` | `volume` | volumeGet<br>volumeSet<br>volumeUp<br>volumeDown | `qormOnVolume` | ios, android, mac, linux, windows | System output volume. (Android volumeSet pending (get/up/down only)) |
| `brightness` | `brightness` | brightnessGet<br>brightnessSet<br>brightnessUp<br>brightnessDown | `qormOnBrightness` | ios, android, mac, linux | Screen brightness. (Linux needs brightnessctl + a backlight device; Android brightnessSet pending; Windows pending) |
| `vibrate` | `vibrate` | vibrate | `—` | ios, android, web | Basic vibration. |
| `torch` | `torch` | torchGet<br>torchToggle | `qormOnTorch` | ios, android | Flashlight / torch. |
| `battery` | `battery` | battery | `qormOnBattery` | ios, android, mac, linux, web | Battery level + charging state. |
| `notify` | `notify` | notify | `qormOnNotify` | ios, mac, linux, windows, web | Local notification. (Android falls back to the Web Notification API; Windows shows a WinRT toast (balloon fallback)) |
| `badge` | `dockbadge` | badge | `—` | ios, mac | App icon / Dock badge count. |
| `screenshot` | `screenshot` | screenshot | `qormOnScreenshot` | ios, android, mac, linux, windows, web | Capture the screen / app view. (Linux needs grim, scrot or ImageMagick) |
| `screenrecord` | `screenrecord` | screenRecordStart<br>screenRecordStop | `qormOnScreenRecord` | ios, mac, web | Record the screen. (Android needs MediaProjection (pending)) |
| `share` | `share` | share | `qormOnShare` | ios, android, mac, linux, windows, web | Open the system share sheet. (Linux/Windows fall back to copying the text to the clipboard) |
| `clipboard` | `clipboard` | clipboardSet<br>clipboardGet | `qormOnClipboard` | ios, android, mac, linux, windows, web | Read/write the clipboard. |
| `deviceinfo` | `deviceinfo` | deviceInfo | `qormOnDeviceInfo` | ios, android, mac, linux, windows, web | Device model / OS / name. |
| `network` | `network` | networkStatus | `qormOnNetwork` | ios, android, mac, linux, windows, web | Online state + connection type. |
| `keepawake` | `keepawake` | keepAwake | `—` | ios, android, mac, linux, web | Prevent the screen from sleeping. |
| `haptics` | `haptics` | haptic | `—` | ios, android, web | Fine haptic feedback (success/warning/error/selection). |
| `storage` | `storage` | storageSet<br>storageGet | `qormOnStorage` | ios, android, mac, linux, windows, web | Key/value local storage. |
| `loginitem` | `loginitem` | loginItem<br>loginItemGet | `qormOnLoginItem` | mac | Launch at login. (macOS-only (needs the installed .app)) |
| `stt` | `stt` | listenStart<br>listenStop | `qormOnSpeech` | ios, android, web | Speech to text (voice input / dictation). |
| `securestorage` | `securestorage` | secureSet<br>secureGet | `qormOnSecure` | ios, android, mac, linux, windows, web | Secure key/value storage (iOS Keychain / Android Keystore / Windows DPAPI). (web falls back to localStorage (not hardware-encrypted); Linux uses the DBus Secret Service (GNOME Keyring / KWallet)) |
| `filepicker` | `filepicker` | pickFile | `qormOnFile` | ios, android, web | Pick a file from storage (returns name/size/data URL). |
| `photopicker` | `photopicker` | pickPhoto | `qormOnPhoto` | ios, android, web | Pick an existing photo from the library (returns a data URL). |
| `orientation` | `orientation` | lockOrientation | `—` | android, web | Lock screen orientation (portrait/landscape). (iOS orientation lock needs AppDelegate support (pending)) |
| `videocapture` | `videocapture` | recordVideo | `qormOnVideo` | ios, web | Record a video with the camera. (Android via MediaRecorder pending) |
| `qrscan` | `qrscan` | scanQR | `qormOnScan` | ios, web | Scan a QR code / barcode with the camera. (Android needs CameraX+MLKit (pending)) |
| `tts` | `tts` | speak<br>speakStop | `—` | ios, android, mac, linux, windows, web | Text to speech (speak a string aloud). |
| `compass` | `compass` | headingStart<br>headingStop | `qormOnHeading` | ios, android, web | Compass heading (degrees from magnetic north). |
| `proximity` | `proximity` | proximityStart<br>proximityStop | `qormOnProximity` | ios, android | Proximity sensor (near/far). |
| `pedometer` | `pedometer` | pedometerStart<br>pedometerStop | `qormOnSteps` | ios, android | Step counter / pedometer. |
| `barometer` | `barometer` | barometerStart<br>barometerStop | `qormOnPressure` | ios, android | Barometric pressure / relative altitude. |
| `contacts` | `contacts` | pickContact | `qormOnContact` | ios, android, web | Pick a contact (name + phone). |
| `calendar` | `calendar` | addEvent | `qormOnCalendar` | ios, android | Add a calendar event. |
| `systemmodes` | `systemmodes` | getModes | `qormOnModes` | ios, android, mac, web | Read system modes: low-power, dark/appearance, airplane (Android), do-not-disturb (Android). Null where a platform has no public API. |
| `insets` | `insets` | getInsets | `qormOnInsets` | ios, android, web | Safe-area insets in points/dp (status bar, notch, home indicator, nav bar). |
| `openurl` | `openurl` | openURL | `qormOnOpenUrl` | ios, android, mac, linux, windows, web | Open a URL / deep link (http, mailto, tel, sms, maps). |
| `screens` | `screens` | screens | `qormOnScreens` | mac | Enumerate displays. (Linux/Windows enumeration pending (returns an empty list)) |

## Hardware interfaces by platform

Every capability each target implements natively or via a Web API.

- **iOS** (40) — `camera`, `location`, `recorder`, `sensors`, `biometric`, `bluetooth`, `wifi`, `nfc`, `volume`, `brightness`, `vibrate`, `torch`, `battery`, `notify`, `badge`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `videocapture`, `qrscan`, `tts`, `compass`, `proximity`, `pedometer`, `barometer`, `contacts`, `calendar`, `systemmodes`, `insets`, `openurl`
- **Android** (36) — `camera`, `location`, `recorder`, `sensors`, `biometric`, `bluetooth`, `wifi`, `nfc`, `volume`, `brightness`, `vibrate`, `torch`, `battery`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `orientation`, `tts`, `compass`, `proximity`, `pedometer`, `barometer`, `contacts`, `calendar`, `systemmodes`, `insets`, `openurl`
- **macOS** (25) — `camera`, `location`, `recorder`, `biometric`, `bluetooth`, `wifi`, `volume`, `brightness`, `battery`, `notify`, `badge`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `storage`, `loginitem`, `securestorage`, `tts`, `systemmodes`, `openurl`, `screens`
- **Linux** (17) — `camera`, `location`, `recorder`, `volume`, `brightness`, `battery`, `notify`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `storage`, `securestorage`, `tts`, `openurl`
- **Windows** (14) — `camera`, `location`, `recorder`, `volume`, `notify`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `storage`, `securestorage`, `tts`, `openurl`
- **Web** (28) — `camera`, `location`, `recorder`, `vibrate`, `battery`, `notify`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `orientation`, `videocapture`, `qrscan`, `tts`, `compass`, `contacts`, `systemmodes`, `insets`, `openurl`
