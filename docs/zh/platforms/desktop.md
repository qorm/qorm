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

演示桌面特性的示例:[`menus`](https://github.com/qorm/platform/tree/main/examples/menus)(系统菜单栏 / 托盘 / 右键菜单,带图标 + 子菜单)、[`floating`](https://github.com/qorm/platform/tree/main/examples/floating)(无边框 + 透明、自定义形状窗口)、[`desktop-hardware`](https://github.com/qorm/platform/tree/main/examples/desktop-hardware)。有关各操作系统的测试情况,参见[支持矩阵](../../platforms/support-matrix.md)。

## 架构

```text
qorm app (JSON) / qorm.bundle.json
  ↓
Go Runtime (loader + state + action + i18n, pure Go)
  ↓
由构建标签/平台选择渲染宿主
  ├─ 纯 Go 保留模式 canvas(macOS 默认,无 `desktop` 标签)
  │    度量 → 布局 → 显示列表 → 软件光栅
  ├─ 原生 WebView(`-tags desktop`)
  │    HTML/CSS → WKWebView / WebView2 / WebKitGTK
  └─ 浏览器回退(其他无标签桌面平台)
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

桌面端在同一 JSON/状态/动作模型之上有两个具体渲染器:

- macOS 默认无标签 `qorm run` 打开纯 Go 保留模式软件 canvas,支持原生输入、
  滚动、文本编辑、QSS 和视觉效果,并把实时渲染图导出给 MCP 测量。
  原生指针/键盘输入分发的动作也会以人类身份进入共享 DevTool 和
  `qorm_activity` 事件流,并提供隐私安全的 focus/typing presence。DevTool
  组件树悬停会高亮对应原生节点。
- `-tags desktop` 选择平台 WebView 中的 HTML 路径
  (WKWebView/WebView2/WebKitGTK),它不是 canvas 渲染器。

两者共享应用模型,但不共享布局/绘制实现,所以应验证目标后端:默认
`qorm measure` / 静态 `qorm check` 使用无窗口 canvas;`-tags desktop`
二进制度量 WebView/DOM。参见[验证](../verification.md)与
[QSS / canvas 效果](../styles.md)。

## 开发工具

桌面提供:

```text
qorm run
qorm build
qorm preview
qorm measure
qorm check
qorm shot
```

见 [CLI 参考](/api/zh/cli.md)。DevTool 组件树可悬停高亮原生 Canvas 节点;
`GET /dev/canvas` 返回物理视口及最新 layout/render/present/total 帧耗时。
MCP 可用 `qorm_capture_canvas` 获取最近实际呈现的原生像素。
