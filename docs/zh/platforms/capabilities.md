# 能力清单

> 由能力注册表自动生成 —— 请勿手动修改。

QORM 内置了 43 项硬件/原生能力。对应地：组件类型即能力名称，由 `qormToNative(op)` 触发，结果通过 `qormOn<X>` 回传。智能体可通过 `qorm_capabilities` MCP 工具发现所有能力。

| 能力 | 组件 | 操作 | 回调 | 平台 | 描述 |
|---|---|---|---|---|---|
| `camera` | `camera` | — | `—` | ios, android, mac, linux, windows, web | 拍照（通过 getUserMedia 实时捕获或文件采集）；直接绑定到状态。 |
| `location` | `location` | location | `qormOnLocation` | ios, android, mac, linux, windows, web | 当前 GPS 定位。 |
| `recorder` | `recorder` | recordStart<br>recordStop | `qormOnAudio` | ios, android, mac, linux, windows, web | 录制麦克风音频。 |
| `sensors` | `sensors` | motionStart<br>motionStop | `qormOnMotion` | ios, android | 设备运动传感器（加速度计/陀螺仪）。 |
| `biometric` | `biometric` | biometric | `qormOnBiometric` | ios, android, mac | Face ID / 指纹身份验证。 (桌面端目前仅支持 macOS Touch ID) |
| `bluetooth` | `bluetooth` | bluetoothScan<br>bluetoothState | `qormOnBluetooth` | ios, android, mac | 扫描 BLE 设备 + 适配器状态。 |
| `wifi` | `wifi` | wifiInfo | `qormOnWifi` | ios, android, mac | 当前 Wi-Fi 网络信息。 (iOS 限制仅能获取当前连接网络的信息) |
| `nfc` | `nfc` | nfcRead | `qormOnNfc` | ios, android | 读取 NFC/NDEF 标签。 (iOS 需要付费的 Apple Developer 团队账号) |
| `volume` | `volume` | volumeGet<br>volumeSet<br>volumeUp<br>volumeDown | `qormOnVolume` | ios, android, mac, linux, windows | 系统输出音量。 (Android 的 volumeSet 待完成（仅支持读取/加减）) |
| `brightness` | `brightness` | brightnessGet<br>brightnessSet<br>brightnessUp<br>brightnessDown | `qormOnBrightness` | ios, android, mac, linux | 屏幕亮度。 (Linux 需要 brightnessctl 与背光设备；Android brightnessSet 待完成；Windows 待完成) |
| `vibrate` | `vibrate` | vibrate | `—` | ios, android, web | 基本振动。 |
| `torch` | `torch` | torchGet<br>torchToggle | `qormOnTorch` | ios, android | 手电筒 / 闪光灯。 |
| `battery` | `battery` | battery | `qormOnBattery` | ios, android, mac, linux, web | 电量水平 + 充电状态。 |
| `notify` | `notify` | notify | `qormOnNotify` | ios, mac, linux, windows, web | 本地通知。 (Android 会回退到 Web Notification API；Windows 为 WinRT Toast 通知（气泡回退）) |
| `badge` | `dockbadge` | badge | `—` | ios, mac | 应用图标 / Dock 徽标计数。 |
| `screenshot` | `screenshot` | screenshot | `qormOnScreenshot` | ios, android, mac, linux, windows, web | 捕获屏幕 / 应用视图。 (Linux 需要 grim、scrot 或 ImageMagick) |
| `screenrecord` | `screenrecord` | screenRecordStart<br>screenRecordStop | `qormOnScreenRecord` | ios, mac, web | 录制屏幕。 (Android 需要 MediaProjection 权限（待完成）) |
| `share` | `share` | share | `qormOnShare` | ios, android, mac, linux, windows, web | 打开系统分享面板。 (Linux/Windows 回退为复制文本到剪贴板) |
| `clipboard` | `clipboard` | clipboardSet<br>clipboardGet | `qormOnClipboard` | ios, android, mac, linux, windows, web | 读/写剪贴板。 |
| `deviceinfo` | `deviceinfo` | deviceInfo | `qormOnDeviceInfo` | ios, android, mac, linux, windows, web | 设备型号 / 操作系统 / 名称。 |
| `network` | `network` | networkStatus | `qormOnNetwork` | ios, android, mac, linux, windows, web | 在线状态 + 连接类型。 |
| `keepawake` | `keepawake` | keepAwake | `—` | ios, android, mac, linux, web | 防止屏幕休眠。 |
| `haptics` | `haptics` | haptic | `—` | ios, android, web | 精细触觉反馈（成功/警告/错误/选择）。 |
| `storage` | `storage` | storageSet<br>storageGet | `qormOnStorage` | ios, android, mac, linux, windows, web | 键值对本地存储。 |
| `loginitem` | `loginitem` | loginItem<br>loginItemGet | `qormOnLoginItem` | mac | 开机自启。 (仅限 macOS（需要安装后的 .app 包）) |
| `stt` | `stt` | listenStart<br>listenStop | `qormOnSpeech` | ios, android, web | 语音转文字（语音输入 / 听写）。 |
| `securestorage` | `securestorage` | secureSet<br>secureGet | `qormOnSecure` | ios, android, mac, linux, windows, web | 安全键值存储（iOS Keychain / Android Keystore）。 (Web 端会回退到 localStorage（无硬件加密）；Linux 走 DBus Secret Service（GNOME Keyring / KWallet）) |
| `filepicker` | `filepicker` | pickFile | `qormOnFile` | ios, android, web | 从存储中选择文件（返回名称、大小、数据 URL）。 |
| `photopicker` | `photopicker` | pickPhoto | `qormOnPhoto` | ios, android, web | 从相册选择已有照片（返回数据 URL）。 |
| `orientation` | `orientation` | lockOrientation | `—` | android, web | 锁定屏幕方向（竖屏/横屏）。 (iOS 屏幕方向锁定需要 AppDelegate 支持（待完成）) |
| `videocapture` | `videocapture` | recordVideo | `qormOnVideo` | ios, web | 用摄像头录制视频。 (Android 端的 MediaRecorder 适配待完成) |
| `qrscan` | `qrscan` | scanQR | `qormOnScan` | ios, web | 用摄像头扫描二维码 / 条形码。 (Android 端需要 CameraX+MLKit 支持（待完成）) |
| `tts` | `tts` | speak<br>speakStop | `—` | ios, android, mac, linux, windows, web | 文字转语音（大声朗读字符串）。 |
| `compass` | `compass` | headingStart<br>headingStop | `qormOnHeading` | ios, android, web | 罗盘朝向（相对于磁北的角度）。 |
| `proximity` | `proximity` | proximityStart<br>proximityStop | `qormOnProximity` | ios, android | 距离传感器（近/远）。 |
| `pedometer` | `pedometer` | pedometerStart<br>pedometerStop | `qormOnSteps` | ios, android | 计步器 / 步数计数。 |
| `barometer` | `barometer` | barometerStart<br>barometerStop | `qormOnPressure` | ios, android | 气压 / 相对高度。 |
| `contacts` | `contacts` | pickContact | `qormOnContact` | ios, android, web | 选择联系人（姓名 + 电话）。 |
| `calendar` | `calendar` | addEvent | `qormOnCalendar` | ios, android | 添加日历事件。 |
| `systemmodes` | `systemmodes` | getModes | `qormOnModes` | ios, android, mac, web | 读取系统模式：低电量、深色/外观样式、飞行模式 (Android)、免打扰 (Android)。在平台没有公开 API 时返回空值。 |
| `insets` | `insets` | getInsets | `qormOnInsets` | ios, android, web | 安全区域内边距（以 point/dp 为单位，含状态栏、刘海屏、Home 指示条、导航栏）。 |
| `openurl` | `openurl` | openURL | `qormOnOpenUrl` | ios, android, mac, linux, windows, web | 打开 URL / 深度链接（http, mailto, tel, sms, maps）。 |
| `screens` | `screens` | screens | `qormOnScreens` | mac | 枚举显示器。 (Linux/Windows 枚举待完成（返回空列表）) |

## 各平台硬件接口支持

每个运行目标原生支持或通过 Web API 实现的全部能力清单。

- **iOS** (40) — `camera`, `location`, `recorder`, `sensors`, `biometric`, `bluetooth`, `wifi`, `nfc`, `volume`, `brightness`, `vibrate`, `torch`, `battery`, `notify`, `badge`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `videocapture`, `qrscan`, `tts`, `compass`, `proximity`, `pedometer`, `barometer`, `contacts`, `calendar`, `systemmodes`, `insets`, `openurl`
- **Android** (36) — `camera`, `location`, `recorder`, `sensors`, `biometric`, `bluetooth`, `wifi`, `nfc`, `volume`, `brightness`, `vibrate`, `torch`, `battery`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `orientation`, `tts`, `compass`, `proximity`, `pedometer`, `barometer`, `contacts`, `calendar`, `systemmodes`, `insets`, `openurl`
- **macOS** (25) — `camera`, `location`, `recorder`, `biometric`, `bluetooth`, `wifi`, `volume`, `brightness`, `battery`, `notify`, `badge`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `storage`, `loginitem`, `securestorage`, `tts`, `systemmodes`, `openurl`, `screens`
- **Linux** (17) — `camera`, `location`, `recorder`, `volume`, `brightness`, `battery`, `notify`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `storage`, `securestorage`, `tts`, `openurl`
- **Windows** (14) — `camera`, `location`, `recorder`, `volume`, `notify`, `screenshot`, `share`, `clipboard`, `deviceinfo`, `network`, `storage`, `securestorage`, `tts`, `openurl`
- **Web** (28) — `camera`, `location`, `recorder`, `vibrate`, `battery`, `notify`, `screenshot`, `screenrecord`, `share`, `clipboard`, `deviceinfo`, `network`, `keepawake`, `haptics`, `storage`, `stt`, `securestorage`, `filepicker`, `photopicker`, `orientation`, `videocapture`, `qrscan`, `tts`, `compass`, `contacts`, `systemmodes`, `insets`, `openurl`
