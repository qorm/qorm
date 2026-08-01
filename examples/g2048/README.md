# 2048 example

The second game-mode component for the pure-Go canvas runtime (after
`examples/tetris`): the classic 4x4 sliding-tile game as a single scene
node. The scene type is `g2048` — a letter prefix, because a bare `2048`
starts with a digit and reads as a number, not a widget name.

```json
{ "type": "g2048", "id": "game" }
```

The `g2048` widget lives in `internal/widgets/g2048.go` and combines two
canvas widget-seam extensions:

- **InteractiveWidget** — clicking the board focuses it (pointer semantics).
  Until it is focused the board shows a CLICK TO PLAY hint and ignores keys.
- **KeyWidget** — while focused, the widget owns the keyboard; Escape blurs
  back out (engine built-in).
- **AnimatedWidget** is reported but never animates: slides are
  instantaneous (no tween), so `Animating()` is a constant false and the
  frame loop stays settled between key presses.

Rules: standard 2048. Arrows slide the whole 4x4 board; equal neighbors
merge once per step, left to right (no double merges: [2,2,2,2] becomes
[4,4]); every merge scores the new tile's value; each real move spawns one
tile (90% a 2, 10% a 4). Reaching 2048 raises the WIN overlay — press any
arrow to keep going — and a board with no legal move is GAME OVER. BEST
persists across restarts for as long as the node stays mounted.

## Keys

| Key    | Action                                |
| ------ | ------------------------------------- |
| Arrows | slide the tiles                       |
| R      | restart (any time; BEST survives)     |
| Escape | unfocus the board                     |

## Props

- `cell` — tile edge in logical px (default 64, clamped 16..128).
- `gap` — gap between tiles in logical px (default 8, clamped 2..32).
  The widget measures `4*cell + 5*gap` (about 300px) square plus a 48px
  SCORE/BEST header.

## Run

`qorm run examples/g2048` — the game needs the native canvas window (the
pure-Go `-tags desktop` / purego path); the HTML renderer shows the scene
chrome and degrades the unknown node gracefully.
