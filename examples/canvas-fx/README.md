# Canvas FX Showcase

Full pure-Go canvas visual + **game-engine motion** stack demo.

| Layer | Path | Role |
|---|---|---|
| Structure | `scenes/main.json` | Node tree + `class` / motion props |
| Style | `styles/app.qss` | Shared QSS (filter, FLIP, spring, snap, clip, …) |
| Logic | `actions/*.qs` | Toggle state that bindings re-read |

## Sections

| # | Feature | How |
|---|---|---|
| 1 | scroll-snap | `.snapStrip` / `.snapPage` |
| 2 | conic-gradient + outline | `conic_disc` |
| 3 | maskFade | `mask_panel` |
| 4 | CSS filter (QSS + qs) | `.filterCard` + `toggle_filter.qs` |
| 5 | mix-blend-mode | `blend_overlay` |
| 6 | FLIP layoutMotion | `.flipChip` + `toggle_flip.qs` |
| 7 | clip-path | `.clipCircle` / `.clipInset` |
| 8 | layerCache | `.cacheBlur` |
| 9 | Game FX | `fx` + `fxToken` — hit/shake/punch/float/**burst** |
| 10 | Timeline + onComplete | Append/Join/wait; `timelineOnComplete` → `timeline_done.qs` |
| 11 | Path follow (polyline) | `path` + `orient` on `path_dot` |
| 12 | Stagger list | `stagger: 80` on list `renderItem` |
| 13 | Cubic Bezier path | `path` + `cubic: true` on `cubic_dot` |
| 14 | Yoyo | style `transitionYoyo` + timeline `{ yoyo: true }` |
| 15 | Text chrome | `text_chrome` — stroke / shadow / decoration / transform / italic / tracking; `ellipsis_line` + `clamp_copy` |
| 16 | Inset shadow + outline | `inset_shadow` (`boxShadowInset` + `outline*`); `overflow_clip` |
| 17 | Frosted glass | `frost_panel` — `backdropBlur` + `backdropTint` over a colorful stage |
| 18 | Hover / press spring | `press_card` — `hoverScale` / `pressedScale` / `pressedBackground` + `0.3s spring` |
| 19 | invert / sepia | `.invertCard` + `cycle_color_fx.qs` |
| 20 | tint modulate | `.tintCard` + `cycle_tint.qs` (Godot modulate / Phaser tint) |
| 21 | rotate / scale / flip | `.xformBox` + `nudge_xform.qs` / `bump_scale.qs` |
| 22 | pixelated image | `pixel_img` (`imageRendering: pixelated`) vs `smooth_img` |
| 23 | zIndex stacking | `z_back` / `z_front` + `toggle_zswap.qs` (QSS swaps `zIndex`) |
| 24 | skewX / skewY | `.skewCard` + `toggle_skew.qs` (`0` → `12` → `24` → `0`) |
| 25 | mix-blend extra | `.blendExtra` + `cycle_blend_extra.qs` (`difference` / `color-dodge` / `multiply`) |
| 26 | transformOrigin | `origin_center` (default) / `origin_corner` (`0 0`) + `toggle_origin.qs` |
| 27 | clip-path polygon | `poly_clip` — `polygon(50% 0%, 100% 100%, 0% 100%)` |
| 28 | mix-blend plus-lighter | `plus_lighter` over a colorful stage |

## Game motion quick ref

```json
{ "fx": "burst", "fxToken": "{{ state.hits }}" }
```

```json
{
  "timeline": [
    { "scale": 1.3, "duration": 180, "ease": "backOut" },
    { "path": [[0,0],[80,40],[160,0]], "duration": 600, "orient": true }
  ],
  "timelineToken": "{{ state.tlPlay }}",
  "timelineOnComplete": "timeline_done"
}
```

Docs: [api/animation.md](../../api/animation.md) · [docs/styles.md](../../docs/styles.md)

## Run

```bash
go run ./cmd/qorm run examples/canvas-fx
go test ./internal/render/canvas/ -run 'TestCanvasFx' -count=1 -v
go run ./cmd/qorm measure examples/canvas-fx
```
