package canvas

import (
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func TestRichTextMeasureAndWrap(t *testing.T) {
	node := &model.Node{
		Type: "richtext",
		Props: map[string]any{
			"spans": []any{
				map[string]any{"text": "Hello ", "fontSize": 10.0, "color": "#ff0000"},
				map[string]any{"text": "world ", "fontSize": 20.0},
				map[string]any{"text": "test", "fontSize": 15.0},
			},
		},
	}

	ln := Measure(node, nil, nil, 1)

	if len(ln.RichTextSpans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(ln.RichTextSpans))
	}

	if ln.RichTextSpans[0].Content != "Hello " {
		t.Errorf("expected 'Hello ', got '%s'", ln.RichTextSpans[0].Content)
	}
	if ln.RichTextSpans[0].FontSize != 10 {
		t.Errorf("expected FontSize 10, got %f", ln.RichTextSpans[0].FontSize)
	}
	if ln.RichTextSpans[0].Fill != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("expected red color, got %v", ln.RichTextSpans[0].Fill)
	}

	// Test wrapping
	ln.Style.Width = 0
	ln.Style.WidthRaw = ""
	wrapTree(ln, 50) // Assuming this forces wrap

	if len(ln.RichTextSpans) < 3 {
		t.Errorf("wrap failed to produce enough spans")
	}

	for _, span := range ln.RichTextSpans {
		if span.Y < 0 {
			t.Errorf("negative Y coordinate on span: %v", span)
		}
	}
}
