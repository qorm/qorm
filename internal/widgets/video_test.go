package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
)

func TestVideoBuffering(t *testing.T) {
	v := NewVideo()

	// Initially not dirty and no frames
	if v.dirty {
		t.Error("Expected initially not dirty")
	}
	if len(v.frames) != 0 {
		t.Error("Expected 0 frames initially")
	}

	// Create a dummy image frame
	img1 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img2 := image.NewRGBA(image.Rect(0, 0, 10, 10))

	// Append a frame
	v.AppendFrame(img1)
	if !v.dirty {
		t.Error("Expected dirty after appending frame")
	}
	if len(v.frames) != 1 {
		t.Error("Expected 1 frame")
	}

	// Append another frame
	v.AppendFrame(img2)
	if len(v.frames) != 2 {
		t.Error("Expected 2 frames")
	}

	// Create a layout node for recording
	ln := &canvas.LayoutNode{
		Width:  100,
		Height: 100,
	}

	// First record should consume the first frame and remain dirty because another frame is buffered
	node1 := v.Record(ln, &runtime.Runtime{}, 1).(*videoNode)
	if node1.Frame != img1 {
		t.Error("Expected first frame to be consumed")
	}
	if !ln.NeedsRedraw {
		t.Error("Expected NeedsRedraw to be set to true because more frames exist")
	}
	if len(v.frames) != 1 {
		t.Error("Expected 1 frame remaining")
	}

	// Reset ln.NeedsRedraw
	ln.NeedsRedraw = false

	// Second record should consume the second frame and not need redraw (unless a new frame is appended)
	node2 := v.Record(ln, &runtime.Runtime{}, 1).(*videoNode)
	if node2.Frame != img2 {
		t.Error("Expected second frame to be consumed")
	}
	if ln.NeedsRedraw {
		t.Error("Expected NeedsRedraw to be false since no more frames")
	}
	if len(v.frames) != 0 {
		t.Error("Expected 0 frames remaining")
	}
}

func TestVideoMeasure(t *testing.T) {
	v := NewVideo()
	w, h := v.Measure(&model.Node{}, &runtime.Runtime{}, nil, 2)
	if w != 1280 || h != 720 { // 640 * 2, 360 * 2 (16:9)
		t.Errorf("Expected 1280x720, got %dx%d", w, h)
	}
}
