package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func testCanvasPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(2, 3, color.RGBA{R: 220, G: 40, B: 30, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCanvasCaptureUnavailableFailsLoudly(t *testing.T) {
	s := newCounterHandler(t)
	requireToolErr(t, toolCallRPC(t, s, "qorm_capture_canvas", map[string]any{}), "requires a running native Canvas host")
}

func TestCanvasCaptureReturnsActualPNGAndNodeClip(t *testing.T) {
	s := newCounterHandler(t)
	pngBytes := testCanvasPNG(t, 32, 24)
	s.SetCanvasCaptureProvider(func(id string) (CanvasCapture, error) {
		capture := CanvasCapture{PNG: pngBytes, Width: 32, Height: 24, Scale: 2}
		if id != "" {
			capture.Clip = &CanvasCaptureRect{X: 3, Y: 4, W: 10, H: 8}
		}
		return capture, nil
	})
	text := resultText(t, toolCallRPC(t, s, "qorm_capture_canvas", map[string]any{"id": "title"}))
	var got struct {
		PNGBase64 string            `json:"pngBase64"`
		Scope     string            `json:"scope"`
		NodeID    string            `json:"nodeId"`
		Clip      CanvasCaptureRect `json:"clip"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.PNGBase64)
	if err != nil || !bytes.Equal(decoded, pngBytes) {
		t.Fatalf("decoded PNG mismatch: %v", err)
	}
	if got.Scope != "full-surface" || got.NodeID != "title" || got.Clip.W != 10 {
		t.Fatalf("capture metadata = %+v", got)
	}
}

func TestCanvasCaptureRejectsProviderMetadataMismatch(t *testing.T) {
	s := newCounterHandler(t)
	s.SetCanvasCaptureProvider(func(string) (CanvasCapture, error) {
		return CanvasCapture{PNG: testCanvasPNG(t, 4, 4), Width: 5, Height: 4, Scale: 1}, nil
	})
	errText := requireToolErr(t, toolCallRPC(t, s, "qorm_capture_canvas", map[string]any{}), "invalid PNG metadata")
	if !strings.Contains(errText, "canvas capture") {
		t.Fatalf("error should identify canvas capture: %q", errText)
	}
}
