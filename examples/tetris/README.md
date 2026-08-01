# Tetris example

The first game-mode component for the pure-Go canvas runtime: the classic
10x20 falling-block game as a single scene node.

```json
{ "type": "tetris", "id": "game" }
```

The `tetris` widget lives in `internal/widgets/tetris.go` and combines three
canvas widget-seam extensions at once:

- **InteractiveWidget** — clicking the board focuses it (pointer semantics).
  Until it is focused the board shows a CLICK TO PLAY hint and ignores keys.
- **KeyWidget** — while focused, the widget owns the keyboard; Escape blurs
  back out (engine built-in).
- **AnimatedWidget** — the frame loop stays alive while a game runs and
  settles when paused or topped out; gravity advances off the wall clock
  inside Measure/Record.

Rules: 7 standard tetrominoes (7-bag randomizer), gravity at 800ms at level
1 (-70ms per level, floored at 100ms), line clears score 100/300/500/800 x
level for 1/2/3/4 rows, one level per 10 lines, spawn collision tops out.

## Keys

| Key               | Action                     |
| ----------------- | -------------------------- |
| Left / Right      | move the piece sideways    |
| Down              | soft drop one row          |
| Up or X           | rotate clockwise           |
| Z                 | rotate counter-clockwise   |
| Space             | hard drop                  |
| P                 | pause / resume             |
| R                 | restart after game over    |
| Escape            | unfocus the board          |

## Props

- `cell` — board cell edge in logical px (default 18). The widget measures
  `10*cell + 90` wide (board + sidebar) and `20*cell` tall.

## Run

`qorm run examples/tetris` — the game needs the native canvas window (the
pure-Go `-tags desktop` / purego path); the HTML renderer shows the scene
chrome and degrades the unknown node gracefully.
