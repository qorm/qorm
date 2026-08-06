# QORM 2048

The classic 2048 sliding-tile game as a **pure QORM app** — no Go component,
no build step beyond `qorm run`: the structure is one scene, the styling one
stylesheet, and every rule (slide, merge, score, spawn, win/lose) lives in
qscript text actions under `actions/`.

- `qorm.json` — manifest + `globalState`: the flat 16-cell `board` of tile
  values, the per-cell exponent `view` (indexes `colors`/`labels`), `score`,
  `best`, the LCG seed `rng`, and `status` (`playing` / `won` / `over`).
- `scenes/main.json` — title, SCORE/BEST readouts + a Restart button, the 4x4
  `gridview` board (cell background/label come from
  `at(state.colors, at(state.view, index))`), the win/game-over overlays,
  `keys` and `swipes` (the same four slides bound to arrows and to touch
  swipes, so the game plays on a phone), and the help lines. `onEnter` runs
  `restart`, so first load opens with two tiles.
- `styles/app.qss` — board frame, score boxes, help text, overlay panel.
- `actions/lib.qs` — the shared core, merged into every action at dispatch:
  map the board to four sequences in slide order, drop zeros, merge equal
  neighbours once each (`[2,2,2,2] -> [4,4,0,0]`), score the merged values,
  spawn one tile (90% 2, 10% 4) on an LCG-picked empty cell — but only when
  the board actually changed — then settle win/lose.
- `actions/slideLeft/Right/Up/Down.qs` — one line apiece: `slide(dir)` from
  the lib. `restart` zeroes the board and score, keeps `best`, and deals the
  two opening tiles.

## Play

    qorm run examples/g2048        # or: go run ./cmd/qorm run examples/g2048

| Input | Action |
|---|---|
| Left / Right / Up / Down (keys) | slide the tiles |
| Swipe left / right / up / down (touch) | slide the tiles |
| R (key) or the Restart button | restart (keeps BEST) |

Merge rules: tiles slide as far as they can; equal neighbours merge once per
move, scoring the new value; a fresh tile (2 or 4) appears after every move
that changed the board. Reaching 2048 wins — keep playing for a higher score.
A full board with no equal neighbours is game over.
