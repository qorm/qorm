# QORM Desktop Platform

Desktop is one of QORM's first-priority runtime platforms, well suited to development previews, production desktop apps, utility apps, and high-performance UI experiments.

## Package it

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

## Architecture

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

## Features

- Runs the pure-Go core directly (frontend + framework + the user's `native/desktop.go` compiled into a single binary).
- The default path is pure Go with no cgo, so you can cross-compile from one machine to macOS/Linux/Windows x amd64/arm64.
- Access to a more complete set of Host Capabilities.
- Provides development debugging, Agent Patch, preview, and profiling.

## Host Capability

The Desktop Pack should support first:

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

Dangerous capabilities must require authorization:

```text
filesystem.write
shell
process.spawn
system.automation
```

## Desktop dangerous-capability boundaries

- `shell`, `process.spawn`, and `system.automation` must be explicitly authorized.
- The authorization scope should at minimum be bound to the target command, directory, or resource range.
- platform / app / host policy still takes precedence over what the Agent or Pack layer permits.

## Rendering

Desktop can validate these first:

```text
Display List
Render Graph
GPU-first renderer
text cache
texture atlas
```

## Development tools

Desktop should provide:

```text
qorm run
qorm inspect
qorm validate
qorm preview-patch
qorm profile
qorm build
```