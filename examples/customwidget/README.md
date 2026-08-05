# Custom Widget (native canvas)

这个示例演示**自定义 canvas 组件**:一个应用自己注册的 `rating` 组件(五点评分),
引擎事先并不知道它的存在。

## 结构

- `scenes/main.json` — 使用 `{"type": "rating", "value": 4}` 与
  `{"type": "rating", "value": "{{state.score}}"}`(绑定,逐帧求值,和普通 prop 一样)。
- `native/desktop.go` — 应用的 Go 中间层。`qorm package` 会把它注入
  cmd/qorm 编译(`userops_gen.go`),`init()` 里 `canvas.RegisterWidget("rating", …)`
  注册组件;组件只用公开绘制层 `internal/render/draw` 组合图形。
- `actions/` — 两个按钮改 `state.score`,绑定的 rating 实时联动。

## 运行

```bash
qorm package examples/customwidget        # 中间层编译进二进制(看到 "compiling your Go middle-layer…")
./dist/.../customwidget                    # 原生窗口里 rating 正常渲染
```

`qorm run` 直接跑仓库二进制,**不注入**中间层(此时 `rating` 是无渲染的未知类型);
自定义组件随包走,这是与内置组件唯一的差别。

## 写一个自己的组件

实现 `canvas.Widget`(见 `internal/render/canvas/widget.go`):

```go
type Widget interface {
    Measure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int)
    Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node
}
```

- 样式(`style` 里的颜色/圆角/padding 等)、条件渲染(`if`/`visible`)、
  `disabled`、`onPress` 由引擎横切处理,组件不用管。
- 需要点击/拖拽:加实现 `canvas.InteractiveWidget`(见 `internal/widgets/slider.go`)。
- 需要持续动画:加实现 `canvas.AnimatedWidget`(见 `internal/widgets/spinner.go`)。
- 内置组件库在 `internal/widgets/`(badge/card/checkbox/…),是最直接的参考。
