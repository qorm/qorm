# QORM Mini-program platform

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

## What's next (not in the foundation yet)

- **Full interactivity** — a JS interpreter that evaluates QORM bindings/actions
  and `setData`, so state changes re-render on device (today `onTap` is a stub).
- **Vendor profiles** — per-vendor capability differences (WeChat / Alipay /
  ByteDance …), review/debug constraints, and degraded-capability declarations.

## Constraints (vendor sandbox)

A mini-program host caps what any framework can do — complex GPU rendering,
dynamic script, filesystem, background tasks, cross-origin sockets, and full
clipboard access are typically restricted. QORM's permission policy sits *inside*
that sandbox and never widens it: an unsupported capability is refused, not
silently downgraded.
