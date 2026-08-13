# Animation

QORM animations are declarative and cross-cutting: any node — a built-in widget
**or a component instance** — can carry an `animation` prop and play an entrance
effect. Entrance effects fire when a node mounts. The live update morphs the DOM in
place, so an effect replays when a node is newly created (e.g. an item appended to
a bound list), not on every state change.

## The `animation` property (any node)

```json
{ "type": "card", "animation": "fadeup", "duration": 450, "children": [ … ] }
```

Works the same on a component instance:

```json
{ "type": "ProductCard", "animation": "pop", "props": { "name": "Cup" } }
```

Tuning props (all optional):

| prop | default | meaning |
|---|---|---|
| `animation` | — | the effect name (below); **bindable** — `"{{state.effect}}"` lets an agent swap the animation by changing state |
| `duration` | `450` | milliseconds |
| `delay` | `0` | milliseconds before it starts (stagger a list by binding the index) |
| `curve` | `cubic-bezier(.34,1.2,.64,1)` | easing |
| `repeat` | `1` | play count (`infinite` for attention loops) |

## Effects

- **Enter**: `fade`, `fadeup`, `fadedown`, `slideup`, `slidedown`, `slideleft`,
  `slideright`, `scale`, `zoomout`, `rotate`, `flip`, `pop`.
- **Attention**: `bounce`, `shake`, `pulse`, `spin` (pair with `repeat`).

### `curve` (entrance easing)

Optional. Named easing for entrance interpolation — the same registry as
`transitionEasing` (game-engine vocabulary included):

`linear` · `easeIn` / `easeOut` / `easeInOut` · `spring` · `back` / `backOut` ·
`elastic` / `elasticOut` · `bounce` / `bounceOut` · `quadOut` · `sineOut` ·
`expoOut` · …

```json
{ "type": "card", "animation": "pop", "duration": 500, "curve": "backOut" }
```

## Game feedback FX (`fx` prop, canvas)

One-shot / short-loop **game-style feedback**, modeled after common 2D engine
APIs (DOTween `DOShake` / `DOPunchScale`, Phaser camera shake, Godot Tween
one-shots). Unlike entrance `animation` (mount-time), **`fx` restarts when the
effect name or `fxToken` changes** — fire it from qscript by bumping a counter:

```json
{
  "type": "box",
  "id": "enemy",
  "fx": "hit",
  "fxToken": "{{ state.hits }}",
  "fxDuration": 320,
  "fxIntensity": 12,
  "style": { "width": 48, "height": 48, "background": "#ff375f" }
}
```

```
# actions/on_damage.qs
state.hits = state.hits + 1
```

| prop | default | meaning |
|---|---|---|
| `fx` | — | effect name (below); bindable; `none` / empty clears |
| `fxToken` / `fxKey` | — | restart token — change restarts the same effect |
| `fxDuration` | per-effect | milliseconds (falls back to `duration` if set) |
| `fxIntensity` | per-effect | amplitude (px, scale delta, or degrees) |
| `fxDelay` | `0` | ms before the effect starts |
| `fxLoop` | auto | `true` / `infinite` forces loop; `float`/`bob`/`blink` loop by default |

### FX names

| name | Engine analogue | Motion |
|---|---|---|
| `shake` | DOTween DOShake · Phaser cameras.shake | Position jitter, decaying |
| `punch` | DOTween DOPunchScale | Scale pop then settle |
| `flash` / `blink` | DOFade blink | Opacity pulses |
| `hit` | damage pack | shake + punch + flash combo |
| `float` / `bob` | idle pickup bob | Looping vertical sine |
| `wobble` | rotation wiggle | Decaying rotation |
| `knockback` | platformer hit shove | Horizontal shove + return |
| `burst` | explosion pack | Radial knock + scale + flash (no multi-sprite emitter) |

Composable with entrance `animation`, `transition` / spring press, FLIP, and
style `rotate` / `scale` / `flipX` / `skewX` / `skewY` — offsets stack on the same transform
channels (pivot: style `transformOrigin`, default center). Persistent style transform does not change the layout box.

Runnable: [`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx) (FX section). Live
games: [`examples/tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris) (local clear flash +
SINGLE/DOUBLE/TRIPLE/TETRIS banner + gold outline; NEXT/SCORE/LINES punch;
the board does not shake or burst), [`examples/g2048`](https://github.com/qorm/qorm/tree/main/examples/g2048)
(local spawn/merge color flashes only; SCORE punch; the board does not
move), [`examples/mario`](https://github.com/qorm/qorm/tree/main/examples/mario) (`fxJump`/`fxCoin`/`fxDeath`),
[`examples/raiden`](https://github.com/qorm/qorm/tree/main/examples/raiden) (`fxHit`/`fxBomb`/`fxBoss`, explosions
`burst`). Physics owns `x`/`y`; `fx` is a visual offset only. Puzzle grids
keep motion on cells and HUD, not the whole board.

## Timeline sequence (`timeline` prop, canvas)

A **DOTween Sequence / Godot Tween** chain on any node. Steps **Append** by
default; `"parallel": true` **Joins** the previous step (start together).
Restart by bumping `timelineToken` from qscript.

```json
{
  "id": "hero",
  "timeline": [
    { "scale": 1.35, "duration": 180, "ease": "backOut" },
    { "dx": 56, "dy": -6, "duration": 220, "ease": "easeOut", "parallel": true },
    { "wait": 80 },
    { "scale": 1, "dx": 0, "dy": 0, "duration": 240, "ease": "easeInOut" }
  ],
  "timelineToken": "{{ state.tlPlay }}"
}
```

```
# actions/play_timeline.qs
state.tlPlay = state.tlPlay + 1
```

Object form (loop / yoyo of the whole sequence):

```json
{
  "timeline": {
    "yoyo": true,
    "repeat": 2,
    "steps": [
      { "scale": 1.2, "duration": 200, "ease": "sineOut" },
      { "opacity": 0.5, "duration": 200, "ease": "linear", "parallel": true }
    ]
  },
  "timelineToken": "{{ state.tlPlay }}"
}
```

| prop | meaning |
|---|---|
| `timeline` | step array, or `{ steps, loop, yoyo, repeat, token }` |
| `timelineToken` / `timelineKey` | restart token |
| `timelineLoop` / `timelineYoyo` / `timelineRepeat` | node-level overrides |

### Step fields

| field | meaning |
|---|---|
| `duration` / `ms` | milliseconds (CSS `"0.2s"` also ok) |
| `delay` | pre-step wait inside the step |
| `wait` | pure hold (no channel change) |
| `ease` / `curve` | named easing (`backOut`, `linear`, …) |
| `parallel` / `join` | Join previous group (DOTween Join) |
| `scale` `opacity` `dx`/`x` `dy`/`y` `rotation`/`rotate` | end values (degrees for rotation) |
| `path` | polyline `[[x,y],…]` or cubic with `"cubic": true` + 4 points (DOTween DOPath) |
| `orient` / `orientToPath` | rotate to path tangent |

Channels not listed in a step **hold** the previous pose. After the sequence
finishes, the **end pose is held** (DOTween default) until the next token bump.

### `timelineOnComplete` / `onComplete`

When a **finite** timeline finishes (not infinite loop/yoyo), the engine
dispatches an action once — DOTween `OnComplete` / Godot `finished`:

```json
{
  "timeline": [ { "scale": 1.2, "duration": 200 } ],
  "timelineToken": "{{ state.tlPlay }}",
  "timelineOnComplete": "timeline_done"
}
```

```
# actions/timeline_done.qs
state.tlDone = state.tlDone + 1
```

Also accepts `{ "name": "act", "args": { … } }`. Seeds include `timeline` (node
id) and `token`.

### Path follow example

```json
{
  "timeline": [
    {
      "path": [[0, 20], [60, -10], [120, 30], [160, 10]],
      "duration": 700,
      "ease": "easeInOut",
      "orient": true
    }
  ],
  "timelineToken": "{{ state.pathPlay }}"
}
```

### Stagger (lists)

`stagger` (ms × list index) delays entrance `animation`, `fx`, and `timeline`
— GSAP stagger / DOTween `SetDelay(i * step)`:

```json
{
  "type": "list",
  "data": "{{ state.items }}",
  "renderItem": {
    "type": "box",
    "animation": "fadeup",
    "stagger": 80,
    "duration": 400,
    "curve": "backOut"
  }
}
```

### Extra FX: `burst`

`fx: "burst"` — lightweight explosion pack (radial knock + scale + flash)
when a full particle system is not needed.

### Style transition yoyo / loop

Property tweens (`transition` on style) accept DOTween-style loop flags:

```json
{
  "style": {
    "opacity": "{{ state.pulse ? 0.4 : 1 }}",
    "transition": "0.35s",
    "transitionEasing": "sineInOut",
    "transitionYoyo": true,
    "transitionRepeat": 2
  }
}
```

| style key | meaning |
|---|---|
| `transitionYoyo` | ping-pong begin↔target |
| `transitionLoop` | forward repeat (infinite if no count) |
| `transitionRepeat` | count (`2`) or `"infinite"` / `-1` |

## Animated widgets

For value-driven (not entrance) motion, use the Flutter-style widgets:

- `animatedcontainer` / `animatedpadding` / `animatedalign` / `animatedpositioned`
  — smoothly transition style whenever a bound value changes (`duration`, `curve`).
- `animatedopacity` — fade children to a bound `opacity` (0..1).
- `transform` / `rotatedbox` — static rotate / scale / translate.
- `motion` (and `fadetransition`, `slidetransition`, `scaletransition`,
  `rotationtransition`, `sizetransition`, `hero`, `animatedswitcher`) — the same
  entrance effects as a dedicated wrapper widget.

The plain `transition` style prop (e.g. `"transition": "0.2s"` or
`"200ms"`) also applies to any node for simple transitions. On the native
**canvas** backend it drives interaction effects (`pressedScale`,
`hoverScale`, color/opacity swaps), absolute `x`/`y` (and left/top) moves,
and FLIP layout motion — not only CSS on the HTML path.

### Spring easing (canvas)

```json
{ "style": { "pressedScale": 0.95, "transition": "0.3s spring" } }
```

or equivalently:

```json
{ "style": { "pressedScale": 0.95, "transition": "0.3s", "transitionEasing": "spring" } }
```

Underdamped spring: the value overshoots then settles. Named CSS easings
(`easeOut`, `easeInOut`, …) and theme motion tokens still work as usual.

### FLIP layout motion (canvas)

When a node jumps in absolute position or size (e.g. a bound `x` changes),
set `layoutMotion: true` with a stable `id` and a `transition` so the
canvas eases the jump instead of snapping:

```json
{
  "id": "chip",
  "type": "box",
  "style": {
    "position": "absolute",
    "x": "{{ state.chipX }}",
    "layoutMotion": true,
    "transition": "0.35s"
  }
}
```

Runnable demo: [`examples/canvas-fx`](https://github.com/qorm/qorm/tree/main/examples/canvas-fx) (scroll-snap,
filters, mask, clip-path, spring press, and FLIP in one scene). Full style
key list: [common style props](props.md#common-style-props) and
[QSS / canvas effects](../docs/styles.md#canvas-visual-effects).

## Theme motion tokens

Skins carry the motion vocabulary alongside their colors. Each `themes/*.json`
may declare a `motion` section; the native canvas backend consumes it directly,
and the HTML/WebView backend exposes the same values as CSS custom properties:

```json
"motion": {
  "durationFast": 120,
  "durationNormal": 250,
  "durationSlow": 400,
  "easingStandard": "easeOutCubic",
  "easingEmphasized": "easeInOutCubic"
}
```

- `animatedcontainer` / `animatedopacity` default to `durationNormal` +
  `easingStandard` when the node sets no `duration` / `curve` prop — explicit
  props still win. On the HTML path this lands as `var(--qorm-motion-normal)` /
  `var(--qorm-motion-standard)`, which hand-written `transition` styles can
  reference too.
- Easing names: `linear`, `easeIn`, `easeOut`, `easeInOut` (plus cubic
  spellings), `spring`, game-engine families (`back` / `elastic` / `bounce` /
  `quad` / `sine` / `expo`, and `*In` / `*Out` / `*InOut` forms), and theme
  token aliases `standard` / `emphasized`.
- The built-in skins deliberately differ: the Apple palettes move at ~250 ms,
  the WinUI palettes at ~167 ms — switching skins changes the app's tempo.
- Note: the HTML backend does not read `themes/*.json`; it ships matching
  *defaults* for the `--qorm-motion-*` variables. Per-skin JSON values apply on
  the native canvas backend.
