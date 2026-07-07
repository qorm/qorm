# Interpreting & verifying a QORM app

QORM's goal is to let an AI **completely and precisely interpret and verify**
everything a user expressed in an app — its layout, styles, behavior and
translations — using the framework itself, with no external browser.

The mechanism: the running app **measures itself** in its own runtime (browser
or native WebView). A tiny script walks every element with an id, records its
`getBoundingClientRect` and computed styles, and POSTs them to `/measure`. The
framework then joins that real rendered result with the user's **intent** (each
node's type, text, and state binding, from the app JSON). So for every component
you get both *what the user asked for* and *what actually rendered*.

Everything below works from the CLI (`-tags desktop` build, which drives a native
WebView) and from the live shared session over MCP.

## `qorm measure` — read the real render

```bash
qorm measure <app-dir> [-o report.json]
```

Renders the app, self-measures, and prints one row per component joining intent
with result:

```json
{ "id": "wifi", "type": "switchlisttile", "intent": {"label": "Wi-Fi", "binding": "{{state.wifi}}"},
  "x": 32, "y": 499, "w": 336, "h": 47, "visible": true,
  "color": "rgb(0,0,0)", "background": "rgba(0,0,0,0)", "fontSize": "15px",
  "padding": "…", "borderRadius": "…", "overflowX": false }
```

Fields per component: `id`, `type`, `intent` (text/label/binding), `x y w h`,
`visible`, `tag`, `text` (for leaf nodes), and computed `color`, `background`,
`fontSize`, `fontWeight`, `textAlign`, `padding`, `margin`, `borderRadius`,
`border`, `opacity`, `zIndex`, `position`, `overflowX`.

## `qorm check --checks` — verify expectations

```bash
qorm check <app-dir> --checks checks.json [-o report.json]
```

`checks.json` is an array of `{id, <assertion>…}`. Each assertion is verified
against the real render; the report gives per-check pass/fail with actual values.

| assertion | meaning |
|---|---|
| `visible: true\|false` | the component is / isn't actually visible |
| `type: "<widget>"` | rendered from the expected node type |
| `text: "<s>"` | contains `<s>` (matched against expressed OR rendered text) |
| `noOverflow: true` | no horizontal content overflow |
| `minW / maxW / minH / maxH: <px>` | size within bounds |
| `x / y: <px>` | position (±3px tolerance) |
| `within: "<id>"` | this component's box sits inside that id's box |
| `below: "<id>"` | starts below that id |
| `backgroundNot / colorNot: "<substr>"` | that substring is ABSENT (e.g. `"255, 255, 255"` to assert not-white in dark mode) |

```json
[
  {"id": "nav",      "type": "appbar", "visible": true, "y": 0, "text": "Today"},
  {"id": "wifi",     "type": "switchlisttile", "visible": true, "within": "settings"},
  {"id": "chart",    "noOverflow": true, "maxW": 370}
]
```

## `qorm check` step-flow — verify behavior

Pass a `{"steps":[…]}` object instead of an array to verify *interactions*: each
step applies an action, waits for the re-render + re-measure, then checks.

```json
{ "steps": [
  { "name": "increment", "do": {"dispatch": "increment"}, "checks": [{"id": "number", "text": "1"}] },
  { "name": "go dark",   "do": {"setState": {"path": "theme", "value": "dark"}},
    "checks": [{"id": "card", "backgroundNot": "255, 255, 255"}] }
] }
```

`do` is `{"dispatch": "<action>", "args": {…}}` or `{"setState": {"path": …, "value": …}}`.

## `qorm check --audit` — one-shot regression

```bash
qorm check <app-dir> --audit
```

No hand-authored checks: verifies generic invariants over every **visible**
component — non-zero size, no horizontal overflow, within the window
(horizontal-scroll/paged containers and their descendants are exempt). Returns
`{ok, visibleComponents, issues, details}`.

## In the live shared session (MCP)

While a human runs the app, an agent on the same session can call:

- **`qorm_measure`** — the complete intent + rendered result (as above).
- **`qorm_check_layout`** — pass `checks` (same schema as `--checks`), get
  per-check pass/fail with actual values.

Both read the live client's self-measurement, so the agent sees exactly what the
human sees. The tool descriptions carry the full assertion list.

## On-device live debugging (物理机联调)

```bash
qorm run <app> --lan
```

Binds to the LAN and prints how a physical phone joins the **same live
session** as the dev machine and agent:

- **Wi-Fi**: open the printed `http://<lan-ip>:PORT/` in the phone browser
  (same network). Real LAN addresses are listed first.
- **USB (Android)**: `adb reverse tcp:PORT` is set up automatically, so the
  phone opens `http://localhost:PORT/`.

Once connected, the device is just another client of the live server:

- agent edits (over MCP) hot-reload on the device instantly (SSE),
- the device's self-measurement posts back to `/measure`, so `qorm_measure`
  and `qorm_check_layout` report the **real device's** rendering — actual
  screen size, fonts and WebView — not a simulation,
- SSE connect/disconnect is written to the activity log with the client IP,
  so a device joining the session is visible.

This makes interpret-and-verify work against real hardware, closing the loop
from authoring to on-device confirmation.

## One command for everything

```bash
bash scripts/verify.sh
```

Runs `go test ./...` (render markers, actions, i18n formatting, fuzz,
determinism) plus a self-measured layout audit of every example, aggregated to a
single ALL-GREEN / regressions verdict. No external browser.

## Notes

- Measurement needs the app running in a rendering runtime. The CLI uses the
  `-tags desktop` WebView (runs headlessly); the live session uses whatever
  browser/WebView the human has open.
- `visible: false` + zero size is normal for inactive tab content, closed
  overlays (modal/dialog/sheet with `open:false`), and empty conditional text —
  the audit only flags *visible* components.
