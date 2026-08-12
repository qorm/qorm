# Canvas FX Showcase

Runnable demo of recent pure-Go canvas rendering features:

| Feature | Where in the scene |
|---|---|
| **scroll-snap** | Horizontal strip (`scrollSnapType: x mandatory`) |
| **maskFade** | Purple gradient row fades on the right edge |
| **conic-gradient** + **outline** | Color disc with outer outline ring |
| **textDecoration / textTransform / lineClamp** | Title underline, uppercase subtitle, clamped caption |
| **CSS filter** | Red card toggles `saturate` + `brightness` |
| **mix-blend-mode** | Orange panel multiplies over green stage |
| **layoutMotion FLIP** | Blue chip eases between left/right on tap |
| **spring press** | Buttons use `pressedScale` + `transition: … spring` |

## Run

```bash
go run ./cmd/qorm run examples/canvas-fx
```

Headless canvas verification (CI / agent):

```bash
go test ./internal/render/canvas/ -run TestCanvasFxExample -count=1 -v
```

Measure the live layout:

```bash
go run ./cmd/qorm measure examples/canvas-fx
```
