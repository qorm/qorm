# QSS — QORM Style Sheets

QSS is QORM's stylesheet language: a CSS-like rule syntax for sharing styles
across scenes, without repeating inline `style` blocks on every node.

A stylesheet lives in `styles/<id>.qss` and is loaded by the runtime at load
time. The Tetris example (`examples/tetris/styles/app.qss`) is a complete,
runnable reference — a game's entire look driven by one stylesheet.

## Rule syntax

```qss
# This is a comment

/* selectors: type, class, id */
button { borderRadius: 12 }
.accent { background: var(--primary); color: var(--on-primary) }
#submit { fontSize: 16 }

/* nested objects stay inline on the node, like style values */
.card { margin: { top: 8, bottom: 8 } }

/* {{bindings}} evaluate like inline style values */
.statValue { color: {{ state.dark ? "#fff" : "#111" }} }
```

- **Type selectors** (`button`, `text`, `box`, …) match nodes of that type.
- **Class selectors** (`.accent`) match nodes whose `class` prop lists that
  name (space-separated; a class named later in the prop wins).
- **ID selectors** (`#submit`) match a node's `id`.
- **`#` comments** — numbers stay numbers, strings and `{{bindings}}` evaluate
  like inline style values.

## Cascade

Style resolution is a cascade, later sources winning key by key:

```text
theme component default < type rule < class rule < id rule < inline `style`
```

- Within one class name, declaration order wins; the node's own `class` list
  order wins between classes.
- Inline `style` on the node always beats every rule.

## Using a stylesheet

Scene nodes reference rules with a `class` prop:

```json
{ "type": "text", "text": "TETRIS", "class": "title" }
```

## Structure · style · logic (with canvas FX)

The three layers share the same style vocabulary:

| Layer | Where | Role |
|---|---|---|
| Structure | `scenes/*.json` | Node tree, `class`, one-off inline `style` |
| Style | `styles/*.qss` | Shared rules — **including every canvas FX key** (`filter`, `clipPath`, `layoutMotion`, `scrollSnapType`, spring `transition`, …) |
| Logic | `actions/*.qs` (or JSON steps) | Mutate `state`; QSS/`style` bindings re-evaluate |

**QSS** accepts the same keys as inline `style` (`render.KnownStyleKeys`). A rule
body may hold numbers, strings, `var(--x)`, and `{{bindings}}` — evaluated each
frame like inline values. Nested objects (`margin: {top: …}`) stay on the node.

**qscript** does not assign styles directly. Write state, bind it:

```qss
/* styles/app.qss */
.filterCard {
  filter: {{ state.filterOn ? "saturate(0.3) brightness(1.15)" : "none" }}
}
.flipChip {
  x: {{ state.flipLeft ? 16 : 280 }}
  layoutMotion: true
  transition: 0.35s spring
}
```

```
# actions/toggle_filter.qs
state.filterOn = !state.filterOn
```

```json
{ "type": "box", "id": "filter_card", "class": "filterCard", "children": [ … ] }
```

Runnable end-to-end: [`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx) (`styles/app.qss` +
`actions/*.qs`). Tetris is the same separation for game chrome:
[`examples/tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris).

## Rendering

Both backends apply QSS with the same cascade (theme component default <
type < class < id < inline). The native canvas backend (macOS default pure-Go
window; games WASM with `qorm_canvas`) merges matching rules in its measure
pass; the HTML path merges them into each node's emitted inline CSS
(`boxCSS` / `textCSS`). Widget chrome defaults on the HTML path (button
variants, shell theme vars) sit under QSS the way canvas theme component
defaults do.

## Accepted style keys

The loader whitelist is `render.KnownStyleKeys` (~100 keys). Unknown keys are
load-time **warnings** (the app still runs). The full group list lives in the
auto-generated [common style props](/api/props.md#common-style-props).

Keys that both HTML and canvas apply include box model, text color/size/weight,
pseudo-state (`hover*` / `pressed*` / `disabled*`), and `backdropBlur` /
`backdropTint`. Canvas-only visual effects (software raster) are listed below.

For flex sizing, `width: "fill"` / `height: "fill"` is accepted in inline
style, QSS, and the node's `layout` object. Canvas resolves it to the containing
content-box size minus that child's margins, including when the parent's
cross-axis alignment is not stretch. HTML emits `100%`; normal CSS sizing and
margin rules then apply. Numeric sizes remain pixels.

Canvas also resolves CSS-style geometry from either inline `style` or QSS:

- `aspectRatio` is width/height. When exactly one positive numeric axis is
  explicit, Canvas derives the other (`width: 120; aspectRatio: 1.5` gives
  height 80). With both axes or neither axis explicit, authored/intrinsic size
  remains authoritative; min/max constraints are applied afterward.
- `position: "absolute"` removes the node from container flow. Use `left`/`top`
  (or Canvas aliases `x`/`y`) to anchor from the leading edges, or `right` /
  `bottom` to anchor from the parent's content box. An explicit leading-axis
  value wins over its trailing anchor.
- `cursor` maps `pointer` (or `hand`), `text` (or `ibeam`), `not-allowed` (or
  `forbidden`), and `default` (or `arrow`) to native cursors. The full QSS
  cascade is honored; absent an authored value, Canvas derives pointer/text/
  disabled cursors from the hovered widget.

## Canvas visual effects

Declarative style keys consumed by the pure-Go canvas backend. Use them on any
node (inline `style` or QSS). Runnable showcase: [`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx).

### Chrome, shadow, outline

```json
{
  "style": {
    "background": "#1c1c1e",
    "borderRadius": 12,
    "boxShadowColor": "#00000088",
    "boxShadowBlur": 16,
    "boxShadowX": 0,
    "boxShadowY": 8,
    "boxShadowInset": false,
    "outlineColor": "#0a84ff",
    "outlineWidth": 2,
    "outlineOffset": 4
  }
}
```

- `strokeColor` / `strokeWidth` / `strokeDasharray` / `strokeDashoffset` — vector stroke on the box RRect (distinct from
  CSS border). `strokeDasharray` and `strokeDashoffset` allow dashed strokes.
- `boxShadowInset: true` — CSS inset box-shadow (inner rim).
- `outline*` — outer ring outside the border box (focus-style chrome).

### Text stroke, shadow, decoration

```json
{
  "type": "text",
  "text": "Title",
  "style": {
    "fontSize": 28,
    "fontWeight": "700",
    "textDecoration": "underline",
    "textTransform": "uppercase",
    "textStrokeColor": "#000",
    "textStrokeWidth": 2,
    "textShadowColor": "#00000066",
    "textShadowBlur": 4,
    "textShadowX": 0,
    "textShadowY": 2,
    "lineClamp": 2
  }
}
```

- `textDecoration`: `underline` / `line-through` / `overline`
- `textTransform`: `uppercase` / `lowercase` / `capitalize`
- `textOverflow: "ellipsis"` + multi-line `lineClamp`

### Gradients

`background` / `gradient` accept:

- `linear-gradient(...)` and `radial-gradient(...)` (stop percentages supported)
- `conic-gradient(from 0deg, #f00, #00f)` — sweep from box center (canvas)

Additionally, the `radialGradient` property can be used as a standalone style key.

### Filters, blend, mask, clip

```json
{
  "style": {
    "filter": "blur(8px) brightness(1.1) saturate(1.2)",
    "mixBlendMode": "multiply",
    "maskFade": "right",
    "maskFadeSize": 40,
    "clipPath": "circle(50%)",
    "layerCache": true,
    "overflow": "hidden"
  }
}
```

| Key | Meaning |
|---|---|
| `filter` | CSS filter stack: `blur()` `brightness()` `contrast()` `saturate()` `grayscale()` `hue-rotate()` `opacity()` `drop-shadow()` `invert()` `sepia()` |
| `blur` / `filterBlur` | Shorthand group blur radius (px) |
| `contrast` / `hue-rotate` | Shorthand standalone filter properties |
| `dropShadowX` / `Y` / `Blur` / `Color` | Direct properties for applying a drop-shadow filter |
| `mixBlendMode` | `multiply` / `screen` / `overlay` / `darken` / `lighten` / `difference` / `exclusion` / `color-dodge` / `color-burn` / `hard-light` / `plus-lighter` / `lighter` when compositing the offscreen layer (`lighter` is the Porter-Duff alias of `plus-lighter`: `min(1, Cs+Cb)`) |
| `maskFade` + `maskFadeSize` | Soft edge dissolve (`top` / `bottom` / `left` / `right`) |
| `maskImage` | e.g. `linear-gradient(to bottom, black, transparent)` |
| `clipPath` | `circle(50%)` / `ellipse(50% 40%)` / `inset(10px round 12px)` / `polygon(50% 0%, 100% 100%, 0% 100%)` (optional `evenodd` / `nonzero` fill-rule) |
| `layerCache` | Reuse the offscreen layer bitmap when content fingerprint is unchanged |
| `overflow: "hidden"` | Clip children to the box (rounded when `borderRadius` is set) |
| `tint` | RGB modulate on the subtree layer (Godot `modulate` / Phaser tint). Zero alpha = off |
| `imageRendering` | `pixelated` forces nearest-neighbour even on fractional scales (pixel art) |

### Transform (canvas)

Persistent visual transform — layout box is unchanged (Godot `rotation` / `scale`, Phaser `setFlip`):

```json
{
  "style": {
    "rotate": 15,
    "scale": 1.2,
    "scaleX": 1,
    "scaleY": 1,
    "flipX": true,
    "flipY": false,
    "skewX": 12,
    "skewY": 0,
    "transformOrigin": "left top"
  }
}
```

- `rotate` — degrees about `transformOrigin` (default center)
- `scale` / `scaleX` / `scaleY` — 0 means unset (treated as 1)
- `flipX` / `flipY` — negate the matching axis
- `skewX` / `skewY` — shear angles in degrees (CSS skew; graph shear). Layout box is unchanged
- `transformOrigin` — CSS pivot for rotate / scale / flip / skew: `center`, `left top`, `50% 0`, `12px 8px`. Empty / omitted = center. Layout box is unchanged
- Composes with entrance `animation`, `fx`, and `pressedScale` / `hoverScale`

### Stacking (`zIndex`, canvas)

`zIndex` was already in `render.KnownStyleKeys` (HTML emits CSS `z-index`). The canvas backend now implements it: sibling **paint and hit order**.

```json
{
  "type": "stack",
  "children": [
    { "id": "z_back", "type": "box", "style": { "zIndex": 1, "background": "#0a84ff" } },
    { "id": "z_front", "type": "box", "style": { "zIndex": 2, "background": "#ff375f" } }
  ]
}
```

- `0` (or omitted / `"auto"`) = auto — document order among equal-z siblings (later paints and hits on top)
- Higher paints (and hit-tests) above lower; negatives paint behind auto siblings
- Layout boxes are unchanged — only sibling paint/hit order
- Measure reports the numeric `zIndex`, or `"auto"` when 0

### Scroll snap

On a `scroll` / `scrollview` viewport:

```json
{ "type": "scroll", "style": { "scrollSnapType": "y mandatory", "height": 320 }, "children": [
  { "type": "box", "style": { "height": 320, "scrollSnapAlign": "start" }, "children": [ … ] }
] }
```

- `scrollSnapType`: `x|y|both` + `mandatory|proximity`
- `scrollSnapAlign` on children: `start` / `center` / `end`
- Snaps after drag release or coast (carousel / pageview style)

### Interaction + spring transition

```json
{
  "style": {
    "pressedScale": 0.96,
    "hoverScale": 1.02,
    "hoverColor": "var(--label)",
    "hoverOpacity": 0.9,
    "pressedBackground": "var(--accent)",
    "pressedOpacity": 0.8,
    "focusBorderColor": "var(--accent)",
    "transition": "0.3s spring",
    "transitionEasing": "spring"
  }
}
```

- `hoverBackground` / `pressedBackground`, `hoverColor`, and
  `hoverOpacity` / `pressedOpacity` come from the full QSS cascade (type <
  class < id < inline), not only inline style
- `focusBorderColor` colors the native canvas focus ring
- `disabled: true` prevents pointer/keyboard activation and uses
  `disabledOpacity` (default 0.5) plus the not-allowed cursor
- `transition: "0.2s"` / `"200ms"` — ease interaction and absolute `x`/`y` moves
- `transition: "0.3s spring"` or `transitionEasing: "spring"` — underdamped
  spring (overshoot then settle) on the canvas path
- See [Animation](/api/animation.md) for entrance effects and FLIP layout motion

### Game feedback FX (`fx` prop)

Canvas-only one-shots modeled on DOTween / Phaser / Godot:

```json
{
  "fx": "hit",
  "fxToken": "{{ state.hits }}",
  "fxDuration": 320,
  "fxIntensity": 12
}
```

```
# actions/on_damage.qs — restarts the clip without remounting the node
state.hits = state.hits + 1
```

Names: `shake`, `punch`, `flash`/`blink`, `hit`, `float`/`bob`, `wobble`,
`knockback`, `burst`. Full table: [Animation — Game feedback FX](/api/animation.md#game-feedback-fx-fx-prop-canvas).

Transition easings also accept game-engine names: `backOut`, `elastic`,
`bounce`, `quadOut`, `sineOut`, `expoOut`, …

Property tweens also accept DOTween-style loops: `transitionYoyo`,
`transitionLoop`, `transitionRepeat`.

### Timeline sequence

DOTween Sequence / Godot Tween chain on any node — Append by default,
`"parallel": true` Joins; bump `timelineToken` from qscript to replay:

```json
{
  "timeline": [
    { "scale": 1.3, "duration": 180, "ease": "backOut" },
    { "dx": 48, "duration": 200, "ease": "easeOut", "parallel": true },
    { "wait": 80 },
    { "scale": 1, "dx": 0, "duration": 200, "ease": "easeInOut" }
  ],
  "timelineToken": "{{ state.tlPlay }}"
}
```

Full table: [Animation — Timeline](/api/animation.md#timeline-sequence-timeline-prop-canvas).

Also: `path` steps (polyline / cubic + `orient`), `timelineOnComplete`, list
`stagger` (ms × index on entrance/fx/timeline), timeline `{ yoyo, loop }`.

### FLIP layout motion

```json
{
  "id": "chip",
  "type": "box",
  "style": {
    "position": "absolute",
    "x": "{{ state.chipX }}",
    "y": 40,
    "layoutMotion": true,
    "transition": "0.35s"
  }
}
```

When `layoutMotion` is true, the node has a stable `id`, and `transition` is
set, absolute position/size jumps ease instead of snapping (shared-element
style). Demo: `examples/canvas-fx` "FLIP" chip.

### Side-scrollers and tile worlds (`board` + `tilemap`)

Use a `board` as the **world plane**, not a `row`/`column`/`list` of tiles.
The engine camera follows a target; `tilemap` bakes a char-grid + atlas into
**one** world bitmap (cached until `rows` or a bump changes).

```json
{
  "type": "board",
  "cameraTarget": "{{ state.mario }}",
  "cameraCenter": "x",
  "cameraCell": 32,
  "cameraViewport": 16,
  "cameraDeadZone": 160,
  "cameraLockLeft": true,
  "cameraResetToken": "{{ state.cameraGen }}",
  "cameraMax": { "x": 6240 },
  "disablePan": true,
  "children": [
    {
      "type": "tilemap",
      "id": "tiles",
      "rows": "{{ state.rows }}",
      "cell": 32,
      "bumpX": "{{ state.bumpCX }}",
      "bumpY": "{{ state.bumpCY }}",
      "bumpT": "{{ state.bumpT }}",
      "atlas": { "1": "assets/ground.png", "2": "assets/brick.png" }
    }
  ]
}
```

| Prop | Meaning |
|---|---|
| `cameraTarget` | Object with `x`/`y` in px (usually `{{state.player}}`) |
| `cameraCenter` | `true` / `"x"` / `"y"` |
| `cameraCell` + `cameraViewport` | e.g. `32` and `16` → 512 px follow window |
| `cameraDeadZone` | px; NES-style left band before the camera scrolls |
| `cameraLockLeft` | never scroll back (SMB). Pair with `cameraResetToken` |
| `cameraResetToken` | bump `{{state.cameraGen}}` on restart so lock-left rewinds |
| `cameraMax` | `{ "x": (levelW - viewportW) * cell }` |
| `disablePan` | no user drag-to-pan (games) |

Actors stay as `image` / `list` children **on the same board** with absolute
`x`/`y`. HUD lives **outside** the board (stack overlay). Do **not** tween
physics `x`/`y` or put `layoutMotion` on 60 fps movers. Pixel art: set
`imageRendering: pixelated` (a QSS `image { … }` rule is enough). Canonical
app: [`examples/mario`](https://github.com/qorm/qorm/tree/main/examples/mario).
Props: [board / tilemap](/api/props.md).

## Diagnostics

Parse errors are load-time diagnostics naming BOTH the file and the line
(`[Stylesheet: app] app.qss:3: …`). Unknown style keys (against
`render.KnownStyleKeys`) are warnings. The rules parsed before and after an
error still load, exactly like a scene keeps loading alongside its own
diagnostics — one bad rule never blanks the app.

## Loader contract

- A `.qss` file is collected only inside a `styles/` directory.
- Its id is the file name (minus `.qss`): `styles/app.qss` → stylesheet `app`.
- Duplicate ids are an error diagnostic on directory load (first wins) and a
  hard refusal from `qorm build`.
- The raw source is kept on the app so the serializer writes each sheet back
  verbatim — the same fixed-point property component documents have.
