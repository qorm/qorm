<!-- data-lang-nav --> [English](../../platforms/desktop.md) · 中文

# QORM 桌面平台

桌面是 QORM 的首要运行时平台之一,非常适合开发预览、生产桌面应用、工具类应用以及高性能 UI 实验。

## 打包

```sh
qorm package examples/menus -p mac         # a macOS .app (per-platform cgo build)
qorm package app -p mac --release [--notarize]   # Developer ID + 硬化运行时 + DMG
./scripts/build-desktop.sh                 # native-window binary for this OS (-tags desktop)
qorm-desktop-... run examples/menus --app  # opens a native window
```

Linux 上托盘、通知点击回路与安全存储直接走 DBus(StatusNotifierItem /
org.freedesktop.Notifications / Secret Service——GNOME 的托盘需要
AppIndicator 扩展;密钥落入 GNOME Keyring 或 KWallet)。

演示桌面特性的示例:[`menus`](https://github.com/qorm/qorm/tree/main/examples/menus)(系统菜单栏 / 托盘 / 右键菜单,带图标 + 子菜单)、[`floating`](https://github.com/qorm/qorm/tree/main/examples/floating)(无边框 + 透明、自定义形状窗口)、[`desktop-hardware`](https://github.com/qorm/qorm/tree/main/examples/desktop-hardware)。有关各操作系统的测试情况,参见[支持矩阵](../../platforms/support-matrix.md)。

## 架构

```text
qorm app (JSON) / qorm.bundle.json
  ↓
Go Runtime (loader + state + action + i18n, pure Go)
  ↓
Desktop Host Adapter (cmd/qorm window_desktop.go desktopHardware*)
  ↓
Rendered to HTML/CSS
  ↓
Native WebView (-tags desktop: WKWebView / WebView2 / WebKitGTK)
```

## 特性

- 直接运行纯 Go 核心(前端 + 框架 + 用户的 `native/desktop.go` 编译进单个二进制)。
- 默认路径是无 cgo 的纯 Go,因此你可以从一台机器交叉编译到 macOS/Linux/Windows x amd64/arm64。
- 可访问一套更完整的宿主能力(Host Capabilities)。
- 提供开发调试、Agent Patch、预览和性能剖析。

## 宿主能力(Host Capability)

桌面 Pack 应优先支持:

```text
network.request
storage.read/write
clipboard.read/write
filesystem.openFile
filesystem.saveFile
notification.show
window.resize
window.fullscreen
navigation.go
```

危险能力必须要求授权:

```text
filesystem.write
shell
process.spawn
system.automation
```

## 桌面危险能力边界

- `shell`、`process.spawn` 和 `system.automation` 必须显式授权。
- 授权范围至少应绑定到目标命令、目录或资源范围。
- 平台 / 应用 / 宿主策略仍然优先于 Agent 或 Pack 层所允许的内容。

## 渲染

当前唯一的渲染路径是 `internal/render` 生成的 HTML/CSS,由原生 WebView 渲染显示。GPU-first 渲染(Display List、Render Graph、文本缓存、纹理图集)属于 roadmap(见 `planning/`)。

## 开发工具

桌面应提供:

```text
qorm run
qorm inspect
qorm validate
qorm preview-patch
qorm profile
qorm build
```
