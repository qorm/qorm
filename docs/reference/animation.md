# Animation

QORM animations are declarative and cross-cutting: any node — a built-in widget
**or a component instance** — can carry an `animation` prop and play an entrance
effect. Because a server re-render remounts the subtree, changing state (a human
or an agent) replays the animation live.

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

## Animated widgets

For value-driven (not entrance) motion, use the Flutter-style widgets:

- `animatedcontainer` / `animatedpadding` / `animatedalign` / `animatedpositioned`
  — smoothly transition style whenever a bound value changes (`duration`, `curve`).
- `animatedopacity` — fade children to a bound `opacity` (0..1).
- `transform` / `rotatedbox` — static rotate / scale / translate.
- `motion` (and `fadetransition`, `slidetransition`, `scaletransition`,
  `rotationtransition`, `sizetransition`, `hero`, `animatedswitcher`) — the same
  entrance effects as a dedicated wrapper widget.

The plain `transition` style prop (e.g. `"transition": "all .2s"`) also applies to
any node for simple CSS transitions.
