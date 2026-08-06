# Tetris example

The classic 10x20 falling-block game as a **pure QORM app** — no Go game
component. JSON declares the scenes and the data; the logic lives in
**qscript actions** (`internal/qscript`):

- `qorm.json` — the data model: the 200-cell `board`, the render-ready
  `view` (board + falling piece), the 16-cell `nextView` preview, the
  falling `piece`, the seeded LCG (`rng`), score/lines/level, the flat
  tetromino rotation table (`flat`: 7 shapes x 4 rotations x 4 `[x,y]`
  offsets) and the fixed piece palette (`colors`).
- `actions/lib.qs` — the shared core merged into every action at dispatch:
  `fits` (collision), `refreshView` / `refreshNext` (render arrays), `spawn`
  (draws the next piece with a Park-Miller LCG in `state.rng`,
  `x*48271 mod 2^31-1` — exact in float64), `lock` (merges the piece, clears
  full rows, 100/300/500/800 x level, a level every 10 lines) and
  `tickStep` (one gravity step).
- `actions/*.qs` — the rules, each a script-file action: the filename is the
  action id and the file's full text is its qscript program. `tick` is
  gravity (slide or lock); `hardDrop` falls in one `for` loop;
  `moveLeft` / `moveRight` / `moveDown` / `rotate` / `rotateCCW` guard on
  the shared `fits` collision helper; `togglePause` and `restart` round
  out the controls. `lockAndSpawn` is the locking half as a dispatchable
  action of its own (hosts, MCP agents).
- `scenes/main.json` — the `keys` map (no focus required), the gravity
  `timer` node (`every` speeds up with the level and the node hides via
  `if` when the game is not playing, so the frame loop settles), the board
  as a 10-column `gridview` over `state.view` (each cell's background is
  `{{ at(state.colors, item) }}`), the 4x4 next-piece preview, and the
  pause / game-over overlays.

The shared helpers live once in `actions/lib.qs` — the reserved library file
the loader collects and the runtime splices ahead of every script action at
dispatch — so each action body stays a few lines.

Because there is no widget code, the same JSON runs on every host: the
native canvas window (where the engine itself schedules the timer node),
the browser, and the MCP/HTTP surfaces an agent drives.

## Keys

| Key            | Action                   |
| -------------- | ------------------------ |
| Left / Right   | move the piece sideways  |
| Down           | soft drop (locks on hit) |
| Up or X        | rotate clockwise         |
| Z              | rotate counter-clockwise |
| Space          | hard drop                |
| P              | pause / resume           |
| R              | restart                  |

## Run

`qorm run examples/tetris`
