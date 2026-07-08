# QORM Mini-program platform

> **Positioning: static export only.** `qorm package -p miniapp` is a one-shot
> export of the app's **initial UI** to a WXML/WXSS project. There is no on-device
> QORM runtime: no live session, no state/actions, no `qorm measure`/self-verify,
> no action dispatch. What you export is what renders.

Mini-programs (WeChat / others) can't run Go or WASM in their render path —
they render **WXML** markup driven by a JS page model, inside a vendor sandbox. So
QORM targets them differently from web/mobile/desktop: instead of shipping the
Go→WASM runtime, it **remaps the app's rendered HTML/CSS to WXML/WXSS** and emits
a ready-to-open WeChat project.

See the [platform support matrix](support-matrix.md) for exactly what works here.

## Package one

```sh
qorm package examples/counter -p miniapp -o counter-weapp
# aliases: -p miniprogram | -p weapp
```

Open the output in **WeChat DevTools**. It's a standard project:

```
counter-weapp/
  app.json            pages list + window chrome
  app.js  app.wxss    app entry + global design tokens (QORM theme)
  project.config.json sitemap.json
  pages/index/
    index.wxml        the UI (QORM boxes → <view>, <img> → <image>)
    index.wxss  index.js  index.json
```

## What the foundation does

- **Static render** — the app's initial UI, with QORM's layout and inline styles
  reused wholesale (WXSS accepts inline `style=` and CSS variables).
- **Tap wiring** — `onclick="qorm(N)"` becomes `bindtap="onTap"` carrying the
  handler index in `data-h`, so events reach the page model.
- **Icons & charts** — inline SVG is re-encoded as a data-URI `<image>` (WXML can't render `<svg>`); chart colors convert exactly, icon color defaults to a neutral (full icon theming is a follow-up).
- **Theme** — QORM's design tokens (`--accent`, `--label`, …) go into `app.wxss`.

## Roadmap (v0.3, not committed)

None of the following exists today — the target is static export only:

- **Full interactivity** — would require a JS interpreter that evaluates QORM
  bindings/actions and `setData` on device. **Not implemented**: today `onTap`
  only logs a static-export notice; actions do not run in mini-programs.
- **Vendor profiles** — per-vendor capability differences (WeChat / Alipay /
  ByteDance …), review/debug constraints, and degraded-capability declarations.
  **Not implemented.**

## Constraints (vendor sandbox)

A mini-program host caps what any framework can do — complex GPU rendering,
dynamic script, filesystem, background tasks, cross-origin sockets, and full
clipboard access are typically restricted. QORM's permission policy sits *inside*
that sandbox and never widens it: an unsupported capability is refused, not
silently downgraded.
