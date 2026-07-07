# QORM Desktop Platform

Desktop 是 QORM 的第一优先级运行平台之一，适合开发预览、生产桌面应用、工具类应用和高性能 UI 实验。

## Package it · 打包与试用

```sh
qorm package examples/menus -p mac         # a macOS .app (per-platform cgo build)
./scripts/build-desktop.sh                 # native-window binary for this OS (-tags desktop)
qorm-desktop-... run examples/menus --app  # opens a native window
```

Examples that exercise desktop features: [`menus`](../../examples/menus) (system
menu bar / tray / right-click menus, with icons + submenus),
[`floating`](../../examples/floating) (chromeless + transparent, custom-shape
window), [`desktop-hardware`](../../examples/desktop-hardware). See the
[support matrix](support-matrix.md) for what's tested per OS.

## 架构

```text
qorm app (JSON) / qorm.bundle.json
  ↓
Go Runtime（loader + state + action + i18n，纯 Go）
  ↓
Desktop Host Adapter（cmd/qorm window_desktop.go desktopHardware*）
  ↓
Render 为 HTML/CSS
  ↓
原生 WebView（-tags desktop：WKWebView / WebView2 / WebKitGTK）
```

## 特点

- 直接运行纯 Go 核心（前端 + 框架 + 用户 `native/desktop.go` 一起编进单个二进制）。
- 默认路径纯 Go 无 cgo，可从一台机器交叉编译到 macOS/Linux/Windows × amd64/arm64。
- 可访问更完整的 Host Capability。
- 可提供开发调试、Agent Patch、预览和性能分析。

## Host Capability

Desktop Pack 应优先支持：

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

危险能力必须要求授权：

```text
filesystem.write
shell
process.spawn
system.automation
```

## 桌面危险能力边界

- `shell`、`process.spawn`、`system.automation` 必须显式授权。
- 授权 scope 应至少绑定目标命令、目录或资源范围。
- platform / app / host policy 仍然高于 Agent 或 Pack 层允许。

## 渲染

Desktop 可以优先验证：

```text
Display List
Render Graph
GPU-first renderer
text cache
texture atlas
```

## 开发工具

Desktop 应提供：

```text
qorm run
qorm inspect
qorm validate
qorm preview-patch
qorm profile
qorm build
```