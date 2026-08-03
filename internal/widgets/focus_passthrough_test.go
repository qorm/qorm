package widgets

import (
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
)

// isFocusBlue matches the keyboard focus ring (#007AFF) — distinct from
// isBlue (pure B) and from most tab-bar chrome.
func isFocusBlue(c color.RGBA) bool { return c.R < 40 && c.B > 200 && c.G > 80 && c.G < 160 }

// A container widget (tabs) lays its panel children out with the frame's
// interaction (LayoutSinks.Inter), so keyboard focus reaches into the panel
// and the focus ring renders. Before the passthrough these were laid out with
// nil interaction and the ring never drew — a Tab-focused panel child had no
// visual at all.
func TestTabsPanelFocusRing(t *testing.T) {
	pa := &model.Node{Type: "box", ID: "pa",
		Style:   map[string]any{"width": 60.0, "height": 30.0, "background": "#888888"},
		OnPress: &model.Invoke{Name: "x"}} // focusable via OnPress
	tb := &model.Node{Type: "tabs", ID: "tb",
		Props:    map[string]any{"tabs": []any{"One"}},
		Children: []*model.Node{pa}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{tb}}

	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	before := countPixels(surf.Frame(), isFocusBlue)
	for i := 0; i < 6; i++ {
		e.HandleKey(canvas.KeyInput{Key: "tab", Down: true})
		if e.Inter.Focused == pa {
			break
		}
	}
	if e.Inter.Focused != pa {
		t.Fatal("tab must reach the tabs-panel child")
	}
	e.DrawFrame(surf)
	if after := countPixels(surf.Frame(), isFocusBlue); after <= before {
		t.Errorf("focus ring must render around the panel child (focus-blue px %d → %d)", before, after)
	}
}
