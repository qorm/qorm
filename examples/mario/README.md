# QORM Mario

A tiny side-scrolling platformer as a **pure QORM app** — no Go component, no
build step beyond `qorm run`: the structure is one scene, the styling one
stylesheet, and every rule (gravity, jumping, coins, stomping, win/lose) lives
in qscript actions under `actions/`.

- `qorm.json` — manifest + `globalState`: the 16x12 level kept as 16-char
  strings in `rows` (with the pristine copy `rows0` restart restores), the
  render-index `view`, the `colors` palette, Mario and the goomba as objects,
  `score` / `coins` / `status` (`playing` / `won` / `dead`).
- `scenes/main.json` — title, SCORE/COINS readouts + a Restart button, the
  16x12 `gridview` board (each cell is `at(state.colors, item)`), the
  win/game-over overlays, `keys` AND `swipes` (arrows and touch swipes drive
  the same move/jump actions, so the course plays on a phone), on-screen
  ◀ / JUMP / ▶ buttons, and the physics `timer` (250 ms) that runs gravity
  while `status` is `playing`. `onEnter` runs `restart`.
- `styles/app.qss` — HUD boxes, buttons, help text, overlay panel.
- `actions/lib.qs` — the shared core merged into every action: level access
  (`tileAt` / `isSolid` / `setTile`), `gravity` (jump-rise budget, falling,
  landing on the goomba as a STOMP, pit death), `stepGoomba` (it paces its
  platform and turns at walls/ledges), `collect` (coins, the flag), and
  `refreshView` (flattens the level + entities into `state.view`).
- `actions/tick|moveLeft|moveRight|jump|restart.qs` — thin bodies over the lib.

## Play

    qorm run examples/mario        # or: go run ./cmd/qorm run examples/mario

| Input | Action |
|---|---|
| Left / Right (keys) or swipe | walk |
| Up / Space (key) or swipe up | jump |
| R (key) or the Restart button | restart |

Rules: grab the coins (+100 each), stomp the goomba from above (+200) — but
touching it from the side, or falling into a pit, ends the run. Reach the flag
pole at the course's end to clear it (+500).
