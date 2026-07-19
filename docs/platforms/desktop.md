# QORM Desktop Platform

Desktop is one of QORM's first-priority runtime platforms, well suited to development previews, production desktop apps, utility apps, and high-performance UI experiments.

## Package it

```sh
qorm package examples/menus -p mac         # a macOS .app (per-platform cgo build)
qorm package app -p mac --release [--notarize]   # Developer ID + hardened runtime + DMG
./scripts/build-desktop.sh                 # native-window binary for this OS (-tags desktop)
qorm-desktop-... run examples/menus --app  # opens a native window
```

On Linux the tray, notification click-through and secure storage speak DBus
directly (StatusNotifierItem / org.freedesktop.Notifications / Secret
Service — GNOME needs the AppIndicator extension for the tray; keys land in
GNOME Keyring or KWallet).

Examples that exercise desktop features: [`menus`](https://github.com/qorm/qorm/tree/main/examples/menus) (system
menu bar / tray / right-click menus, with icons + submenus),
[`floating`](https://github.com/qorm/qorm/tree/main/examples/floating) (chromeless + transparent, custom-shape
window), [`desktop-hardware`](https://github.com/qorm/qorm/tree/main/examples/desktop-hardware). See the
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
- The default path is pure Go, so you can cross-compile from one machine to macOS/Linux/Windows x amd64/arm64.
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

Today desktop has a single render path: `internal/render` produces HTML/CSS,
which the native WebView displays. A GPU-first renderer (display list, render
graph, text cache, texture atlas) is roadmap work — see `planning/`.

## Development tools

Desktop provides:

```text
qorm run
qorm build
qorm preview
qorm measure
qorm check
```

See [the CLI reference](/api/cli.md). A desktop inspect / validate / profile
toolchain is roadmap work — see `planning/`.