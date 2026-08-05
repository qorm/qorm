# WebView demo (canvaswebview)

One `webview` widget in a scene, three ways it renders depending on the host:

- **canvaswebview build (macOS)** — the canvas native window covers the
  widget's box with a real **WKWebView subview** (platform-view overlay):
  `go run -tags canvaswebview ./cmd/qorm run examples/webdemo`
  The demo page loads from the inline `html` prop and posts a `probe` message
  on load plus a `networkStatus` op when you press its button — both go over
  the `qormDesktop` JS bridge and are logged to stderr by the host, and the
  bridge's `qormOnNetwork` reply updates the page in place.
- **pure-Go build (default)** — no web engine is linked, so the widget
  degrades to a bordered placeholder showing the resolved URL:
  `go run ./cmd/qorm run examples/webdemo`
- **HTML renderer** (`qorm run` in a browser) — an `<iframe>`
  (`srcdoc` for the `html` prop).

## Props

- `url` (or alias `src`) — address to load; may carry `{{state.*}}`
  bindings. This demo deliberately uses `html` instead so it works offline;
  to load a live page instead, swap the `html` prop for
  `"url": "https://example.com"`.
- `html` — inline markup loaded via `loadHTMLString` (`srcdoc` on HTML);
  used only when neither `url` nor `src` is set.
- `style.width` / `style.height` — box size; default 320×240.

## Notes / limits

- The overlay host is **darwin-only** (WKWebView). On Linux/Windows (and in
  any build without the tag) the widget shows the placeholder / iframe.
- The bridge is the same contract as the desktop window's:
  `window.qormDesktop(JSON.stringify({op, ...}))` → Go `desktopHardware` →
  `qormOn<Stem>(...)` evaled back into the same web view. A page written for
  the app window's bridge works unchanged inside a `webview` widget.
- Pointer and wheel events over the web view go to it alone (the host does
  not also forward them to the canvas engine); key events still go to the
  engine — an overlay text field cannot be typed into yet.
