# Platform support matrix

> Auto-generated from the support registry — do not edit by hand.

What QORM supports on each target, at a glance. **`ok`** = supported and tested; **`beta`** = foundation / partial or platform-limited; **`—`** = not applicable. Per-capability hardware detail is in [capabilities.md](capabilities.md).


## Distribution

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Installable package | ok | ok | ok | ok | beta | beta | ok |
| Offline / self-contained | ok | ok | ok | ok | ok | ok | beta |
| PWA install (Add to Home Screen) | ok | ok | beta | — | — | — | — |
| Signed bundle (ed25519) | ok | ok | ok | ok | ok | ok | — |
| Over-the-air update + rollback | ok | ok | ok | ok | ok | ok | — |

## Rendering

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Declarative HTML/CSS render | ok | ok | ok | ok | ok | ok | beta |
| Full widget set | ok | ok | ok | ok | ok | ok | beta |
| Themes (Apple / Material / dark) | ok | ok | ok | ok | ok | ok | beta |
| Custom components (JSON-defined) | ok | ok | ok | ok | ok | ok | beta |
| i18n messages + RTL | ok | ok | ok | ok | ok | ok | beta |
| Native window (chromeless / transparent) | — | — | — | ok | beta | beta | — |
| System menu bar / tray / right-click menu | — | — | — | ok | beta | beta | — |

## Runtime

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| Live state + actions + bindings | ok | ok | ok | ok | ok | ok | — |
| Expression bindings ({{ ... }}) | ok | ok | ok | ok | ok | ok | — |
| Conditional render + data-bound lists | ok | ok | ok | ok | ok | ok | — |
| Go middle-layer (custom native ops) | ok | ok | ok | ok | ok | ok | — |
| Hardware / OS capabilities | ok | ok | ok | ok | beta | beta | beta |

## Agent

| Feature | Web | iOS | Android | macOS | Linux | Windows | Mini-program |
|---|---|---|---|---|---|---|---|
| MCP server (read / edit / verify a live app) | ok | ok | ok | ok | ok | ok | — |
| Live human-AI shared session (SSE) | ok | ok | ok | ok | ok | ok | — |
| Review-bound edits (preview → apply) | ok | ok | ok | ok | ok | ok | — |
| Self-verify render (qorm measure / check) | ok | ok | ok | ok | ok | ok | — |

## Notes

- **Installable package** — desktop is a per-platform cgo build; mini-program is a WeChat project
- **Offline / self-contained** — web/mobile run offline via Go→WASM; mini-program renders static UI
- **PWA install (Add to Home Screen)** — web manifest + service worker; iOS/Android add-to-home
- **Signed bundle (ed25519)** — pure-Go verify-the-bundle; mini-programs are vendor-signed
- **Over-the-air update + rollback** — mini-program updates are vendor-controlled
- **Declarative HTML/CSS render** — mini-program renders remapped WXML/WXSS
- **Full widget set** — layout, input, media, structure — see widgets.md
- **Themes (Apple / Material / dark)** — design tokens; mini-program carries the token WXSS
- **Custom components (JSON-defined)** — declared in qorm.json, {{prop.x}} templates
- **i18n messages + RTL** — ICU messages, plurals, currency, right-to-left
- **Native window (chromeless / transparent)** — -tags desktop; macOS is the reference
- **Live state + actions + bindings** — mini-program is static export only — no on-device runtime
- **Expression bindings ({{ ... }})** — arithmetic, comparisons, ternary, string ops, functions; mini-program is static export only (evaluated once at export)
- **Conditional render + data-bound lists** — if:, list repeat with {{item.*}} scope; mini-program is static export only (evaluated once at export)
- **Go middle-layer (custom native ops)** — one native/desktop.go into desktop AND mobile/web WASM
- **Hardware / OS capabilities** — per-capability support is in capabilities.md
- **MCP server (read / edit / verify a live app)** — stdio or /mcp against a running app; mini-program is static export only — no live tools apply
- **Live human-AI shared session (SSE)** — AI edits appear in the human's browser instantly; the human's clicks show in qorm_activity
- **Review-bound edits (preview → apply)** — apply_patch must carry the preview token; mini-program is static export only
- **Self-verify render (qorm measure / check)** — renders the app and reports real geometry
