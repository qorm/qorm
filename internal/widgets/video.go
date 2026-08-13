package widgets

import (
	"image"
	"image/color"
	"path/filepath"
	"sync"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("video", NewVideo())
}

// Video implements a Canvas widget that buffers decoded video frames and renders them.
type Video struct {
	mu      sync.Mutex
	frames  []image.Image
	dirty   bool
	playing bool
	src     string
}

// NewVideo creates a new Video widget.
func NewVideo() *Video {
	return &Video{
		frames: make([]image.Image, 0),
	}
}

// AppendFrame buffers a new decoded frame and flags the widget as dirty for repaint.
func (v *Video) AppendFrame(img image.Image) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.frames = append(v.frames, img)
	v.dirty = true
}

func (v *Video) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	// Store src to access later
	var src string
	if n.Value != "" {
		src = n.Value
	} else if srcVal, ok := n.Props["src"].(string); ok && srcVal != "" {
		src = srcVal
	}

	if src != "" {
		if !filepath.IsAbs(src) && rt != nil && rt.App != nil {
			src = filepath.Join(rt.App.BaseDir, src)
		}
		if absPath, err := filepath.Abs(src); err == nil {
			v.src = absPath
		} else {
			v.src = src
		}
	}

	if scale < 1 {
		scale = 1
	}
	w = 640 * scale
	h = 360 * scale
	return w, h
}

func (v *Video) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	startVideoDecoder(v, ln.Width, ln.Height)

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.dirty {
		ln.NeedsRedraw = true
		v.dirty = false
	}

	var currentFrame image.Image
	if len(v.frames) > 0 {
		currentFrame = v.frames[0]
		v.frames = v.frames[1:]
		if len(v.frames) > 0 {
			ln.NeedsRedraw = true
		}
	}

	node := &videoNode{
		Width:  float64(ln.Width),
		Height: float64(ln.Height),
		Frame:  currentFrame,
	}
	node.Init(node)
	return node
}

// videoNode is a custom graph.Node for the video widget that pipes the frame
// to the PaintOp mechanism by drawing the image and filling the clip.
type videoNode struct {
	graph.BaseNode
	Width  float64
	Height float64
	Frame  image.Image
}

func (n *videoNode) Base() *graph.BaseNode {
	return &n.BaseNode
}

func (n *videoNode) Draw(ctx *graph.Context) {
	if n.Width <= 0 || n.Height <= 0 {
		return
	}

	n.UpdateGlobalTransform()
	ctx.Save()
	ctx.Transform(n.Base().GlobalTransform)

	rect := image.Rect(0, 0, int(n.Width), int(n.Height))
	ctx.ClipRect(rect)

	if n.Frame != nil {
		if rgba, ok := n.Frame.(*image.RGBA); ok {
			ctx.DrawImageEx(rgba, rect, false)
		}
	} else {
		// Fallback to PaintOp if no frame is available (e.g. black screen)
		ctx.Fill(color.RGBA{0, 0, 0, 255})
		ctx.Paint()
	}

	ctx.Restore()
}

func (n *videoNode) HitTest(p geom.Point) graph.Node {
	return nil
}

func (n *videoNode) GetBBox() geom.BBox {
	return geom.NewBBox(0, 0, n.Width, n.Height)
}
