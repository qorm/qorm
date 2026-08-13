//go:build !desktop && darwin

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"

	"github.com/qorm/qorm/internal/mcp"
)

func encodeLiveCanvasCapture(frame image.Image, scale int, measure []byte, id string) (mcp.CanvasCapture, error) {
	if frame == nil {
		return mcp.CanvasCapture{}, fmt.Errorf("no canvas frame has been presented yet")
	}
	bounds := frame.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 || w > mcp.MaxCanvasCaptureDimension || h > mcp.MaxCanvasCaptureDimension || int64(w)*int64(h) > mcp.MaxCanvasCapturePixels {
		return mcp.CanvasCapture{}, fmt.Errorf("presented frame %dx%d exceeds capture safety limits", w, h)
	}
	if scale < 1 {
		scale = 1
	}
	capture := mcp.CanvasCapture{Width: w, Height: h, Scale: scale}
	if id != "" {
		var rows []struct {
			ID      string  `json:"id"`
			X       float64 `json:"x"`
			Y       float64 `json:"y"`
			W       float64 `json:"w"`
			H       float64 `json:"h"`
			Visible bool    `json:"visible"`
		}
		if err := json.Unmarshal(measure, &rows); err != nil {
			return mcp.CanvasCapture{}, fmt.Errorf("read live node geometry: %w", err)
		}
		found := false
		for _, row := range rows {
			if row.ID != id {
				continue
			}
			found = true
			if !row.Visible || row.W <= 0 || row.H <= 0 {
				return mcp.CanvasCapture{}, fmt.Errorf("node %q is not visibly capturable", id)
			}
			x0, y0 := int(math.Floor(row.X)), int(math.Floor(row.Y))
			x1, y1 := int(math.Ceil(row.X+row.W)), int(math.Ceil(row.Y+row.H))
			clipped := x0 < 0 || y0 < 0 || x1 > w || y1 > h
			x0, y0 = max(0, x0), max(0, y0)
			x1, y1 = min(w, x1), min(h, y1)
			if x1 <= x0 || y1 <= y0 {
				return mcp.CanvasCapture{}, fmt.Errorf("node %q is outside the presented frame", id)
			}
			capture.Clip = &mcp.CanvasCaptureRect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0, ClippedToSurface: clipped}
			break
		}
		if !found {
			return mcp.CanvasCapture{}, fmt.Errorf("node %q is absent from the presented canvas graph", id)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		return mcp.CanvasCapture{}, fmt.Errorf("encode canvas PNG: %w", err)
	}
	if encoded.Len() > mcp.MaxCanvasCapturePNGBytes {
		return mcp.CanvasCapture{}, fmt.Errorf("encoded PNG exceeds %d-byte safety limit", mcp.MaxCanvasCapturePNGBytes)
	}
	capture.PNG = encoded.Bytes()
	return capture, nil
}
