# QORM Mario — NES Super Mario Bros. World 1-1 (2x)

A side-scroller that targets **Super Mario Bros. (NES) World 1-1**, not a
generic platformer. One consistent scale: **1 NES pixel = 2 canvas pixels**.

| NES | This app |
|---|---|
| 16 px tile | 32 px tile |
| 256×240 screen | 512×480 playfield |
| small Mario ~16 px tall | 32 px sprite, 24×32 hitbox |
| 60 Hz physics | 16 ms timer, dt-scaled 2x NES numbers |

The first version mixed a 16 px hitbox, 1x NES gravity, and a 2x tile world,
plus a side-panel HUD. That is why it did not feel like Mario. This pass
keeps one scale and NES rules.

## Feel (2x NES)

Walk cap 187 px/s, run (Shift) 307 px/s. Jump starts at 540 px/s (faster
when running). Held jump uses lighter gravity (1040) and reaches about
4.4 tiles; a tap uses 2080 and still clears about 2 tiles. Camera follows
after a 160 px dead zone and **never scrolls left**. TIME counts down at
~2.5 ticks/s (400 ≈ 160 s).

## World 1-1

`rows0` is a 15×211 tile overworld (ground on rows 13–14). `?` blocks: `7`
is a coin, `q` is the mushroom. Goombas first, then a green Koopa (stomp
to shell, kick the shell). Flag / castle ends the course.

## Files

- `qorm.json` — level + schema
- `scenes/main.json` — 512×480 board + NES-style HUD overlay (MARIO / COIN / WORLD / TIME)
- `actions/lib.qs` — tiles, SMB physics, enemies, blocks
- `assets/` — pixel art (`go run ./tools/genmariosprites`)
- `audio/` — chiptune (`go run ./tools/genmarioaudio`)

## Play

```
go run ./cmd/qorm run examples/mario
```

| Input | Action |
|---|---|
| Left / Right or A / D | walk |
| Shift | run |
| Up / W / Space | jump (hold for height) |
| R | restart |

Stomp from above. Side contact hurts (big Mario shrinks, then death bounce).
Reach the flag.
