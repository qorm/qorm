# 平台支持矩阵 · Platform support matrix

> 本文件由 `internal/support` 自动生成(`TestSupportMatrixInSync`),请勿手改。
> Auto-generated from the support registry — do not edit by hand.

What QORM supports on each target, at a glance. **`ok`** = supported and tested; **`beta`** = foundation / partial or platform-limited; **`—`** = not applicable. Per-capability hardware detail is in [capabilities.md](capabilities.md).


## Distribution

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Installable package | ok | ok | ok | ok | beta | beta | ok |
| Offline / self-contained | ok | ok | ok | ok | ok | ok | beta |
| Signed bundle (ed25519) | ok | ok | ok | ok | ok | ok | — |
| Over-the-air update + rollback | ok | ok | ok | ok | ok | ok | — |

## Rendering

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Declarative HTML/CSS render | ok | ok | ok | ok | ok | ok | beta |
| Native window (chromeless / transparent) | — | — | — | ok | beta | beta | — |
| System menu bar / tray / right-click menu | — | — | — | ok | beta | beta | — |

## Runtime

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Live state + actions + bindings | ok | ok | ok | ok | ok | ok | beta |
| Go middle-layer (custom native ops) | ok | ok | ok | ok | ok | ok | — |
| Hardware / OS capabilities | ok | ok | ok | ok | beta | beta | beta |

## Agent

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| MCP server (read / edit / verify a live app) | ok | ok | ok | ok | ok | ok | beta |
| Self-verify render (qorm measure / check) | ok | ok | ok | ok | ok | ok | — |

## Notes

- **Installable package** — desktop is a per-platform cgo build; mini-program is a WeChat project
- **Offline / self-contained** — web/mobile run offline via Go→WASM; mini-program renders static UI
- **Signed bundle (ed25519)** — pure-Go verify-the-bundle; mini-programs are vendor-signed
- **Over-the-air update + rollback** — mini-program updates are vendor-controlled
- **Declarative HTML/CSS render** — mini-program renders remapped WXML/WXSS
- **Native window (chromeless / transparent)** — -tags desktop; macOS is the reference
- **Live state + actions + bindings** — mini-program is static in the foundation slice
- **Go middle-layer (custom native ops)** — one native/desktop.go into desktop AND mobile/web WASM
- **Hardware / OS capabilities** — per-capability support is in capabilities.md
- **MCP server (read / edit / verify a live app)** — stdio or /mcp against a running app
- **Self-verify render (qorm measure / check)** — renders the app and reports real geometry
