# QORM Raiden

Vertical shmup as a **pure QORM app** — scene JSON + qscript + QSS. Physics
and waves live in `actions/lib.qs`; the canvas motion engine adds feedback.

## Canvas motion

| Event | Token (qs) | Node `fx` |
|---|---|---|
| Player hit | `state.fxHit++` | ship **flash** (arcade blink, not a UI shake) |
| Bomb | `state.fxBomb++` | white flash + camera shake |
| Boss damaged | `state.fxBoss++` | boss `punch` |
| Explosion | frame `t` | grow-in-place 24→48px (`explode_1/2/3`) |
| Fire | `muzzle` | muzzle spark + engine flicker |
| Heli | — | slight `float` |
| Power-up | — | `float` |
| GAME OVER / CLEAR | mount | `animation: pop` + `backOut` |
| Touch buttons | — | QSS `pressedScale` + spring |

Do **not** put `transition` / `layoutMotion` on ship or enemy `x`/`y` — the
timer tick owns those. Sprites use `imageRendering: pixelated` so scaled
pixel art stays crisp.

## Play

```bash
go run ./cmd/qorm run examples/raiden
```
