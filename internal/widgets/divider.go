package widgets

import (
	"fmt"
	"image/color"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("divider", Divider{})
	canvas.RegisterWidget("verticaldivider", Divider{})
}

// Divider is the 1px separator line (HTML render_widgets.go:225: horizontal
// height:1px;width:100%;margin:8px 0, vertical width:1px;align-self:stretch;
// margin:0 8px, color var(--sep)). `verticaldivider` is the type alias.
//
// Orientation resolution: type verticaldivider → orientation prop →
// horizontal default. HTML lets CSS stretch the line across the parent's
// cross axis; the v1 Widget seam hands a widget neither its parent nor its
// siblings (canvas/widget.go), so "follow the parent's main axis" is not
// expressible and horizontal (the common column case) is the default.
//
// The same seam limits extent: there is no cross-axis stretch channel, so
// the line spans only the node's RESOLVED box. In a bare column give a
// horizontal divider an explicit style width (and a vertical one a style
// height), or it collapses to zero and draws nothing. The HTML 8px margins
// are baked into the measured box (the line is centered inside it); an
// author style margin applies generically on top.
type Divider struct{}

// dividerVertical mirrors the HTML test (render_widgets.go:226).
func dividerVertical(n *model.Node) bool {
	if n.Type == "verticaldivider" {
		return true
	}
	if raw, ok := n.Prop("orientation"); ok {
		return fmt.Sprint(raw) == "vertical"
	}
	return false
}

// Measure reports the 1px line plus the HTML default margins baked in:
// 17px tall × 0 wide horizontal, 0 tall × 17px wide vertical (× scale).
func (Divider) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	if dividerVertical(n) {
		return scale + 16*scale, 0
	}
	return 0, scale + 16*scale
}

// Record paints the separator-colored 1px line centered in the resolved box.
func (Divider) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if scale < 1 {
		scale = 1
	}
	th := scale
	line := draw.NewRect()
	line.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
	if dividerVertical(ln.Node) {
		if ln.Height <= 0 {
			return nil
		}
		line.Width = float64(th)
		line.Height = float64(ln.Height)
		line.X = float64((ln.Width - th) / 2)
		return line
	}
	if ln.Width <= 0 {
		return nil
	}
	line.Width = float64(ln.Width)
	line.Height = float64(th)
	line.Y = float64((ln.Height - th) / 2)
	return line
}
