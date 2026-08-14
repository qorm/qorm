package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func TestParseImageRenderingPixelated(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "image", Style: map[string]any{"imageRendering": "pixelated"}}
	s := parseStyle(n, rt)
	if s.ImageRendering != "pixelated" {
		t.Errorf("imageRendering = %q, want pixelated", s.ImageRendering)
	}
	n2 := &model.Node{Type: "image", Style: map[string]any{"image-rendering": "pixelated"}}
	s2 := parseStyle(n2, rt)
	if s2.ImageRendering != "pixelated" {
		t.Errorf("image-rendering = %q, want pixelated", s2.ImageRendering)
	}
}

func TestImagePixelatedKeepsTexelColor(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	red := color.RGBA{220, 20, 30, 255}
	green := color.RGBA{20, 220, 30, 255}
	blue := color.RGBA{20, 30, 220, 255}
	white := color.RGBA{240, 240, 240, 255}
	src.SetRGBA(0, 0, red)
	src.SetRGBA(1, 0, green)
	src.SetRGBA(0, 1, blue)
	src.SetRGBA(1, 1, white)

	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	ops := &op.Ops{}
	ops.Add(op.ImageOp{Src: src, Dest: image.Rect(0, 0, 16, 16), Pixelated: true})
	SoftwareRenderer{}.Render(ops, img)

	// Each source texel is an 8×8 block. Center of a block must stay the
	// exact source colour — bilinear would blend toward the neighbour.
	if c := img.RGBAAt(4, 4); c != red {
		t.Fatalf("pixelated texel center (4,4) = %v, want exact %v", c, red)
	}
	if c := img.RGBAAt(12, 4); c != green {
		t.Fatalf("pixelated texel center (12,4) = %v, want exact %v", c, green)
	}
	if c := img.RGBAAt(4, 12); c != blue {
		t.Fatalf("pixelated texel center (4,12) = %v, want exact %v", c, blue)
	}
	if c := img.RGBAAt(12, 12); c != white {
		t.Fatalf("pixelated texel center (12,12) = %v, want exact %v", c, white)
	}
}

func TestRecordImagePixelatedFlag(t *testing.T) {
	resetImageCache(t)
	dir := t.TempDir()
	src := solidRGBA(2, 2, color.RGBA{200, 10, 20, 255})
	writeTestPNG(t, dir, "px.png", src)
	rt := imageTestRuntime(dir)
	n := imageNode(map[string]any{"src": "px.png", "fit": "fill"})
	n.Style = map[string]any{"imageRendering": "pixelated"}
	node := RecordImage(n, rt, 16, 16, 0, nil)
	gi, ok := node.(*graph.Image)
	if !ok {
		t.Fatalf("RecordImage type = %T, want *graph.Image", node)
	}
	if !gi.Pixelated {
		t.Fatal("imageRendering:pixelated must set graph.Image.Pixelated")
	}
	ops := &op.Ops{}
	gi.Draw(graph.NewContext(ops))
	for _, o := range ops.Operations() {
		if io, ok := o.(op.ImageOp); ok {
			if !io.Pixelated {
				t.Fatal("ImageOp.Pixelated must be true")
			}
			return
		}
	}
	t.Fatal("no ImageOp recorded")
}
