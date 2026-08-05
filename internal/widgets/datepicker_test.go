package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Tapping rows in the month/day/year columns updates the bound date; a day
// beyond the selected month's length clamps to it (2024 is a leap year).
func TestDatePickerSelectsAndClamps(t *testing.T) {
	dp := &model.Node{Type: "datepicker", ID: "dp", Value: "{{state.date}}"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{dp}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 800))
	rt.State["date"] = "2026-03-15"
	e.DrawFrame(surf)

	// Click "May" (row 4) in the month column: x 0..76, row y 96..120.
	clickAt(e, 38, 108)
	if got := rt.State["date"]; got != "2026-05-15" {
		t.Fatalf("month pick: state.date = %v, want 2026-05-15", got)
	}
	// Click day 10 (row 9) in the day column: x 80..156, y 216..240.
	clickAt(e, 118, 228)
	if got := rt.State["date"]; got != "2026-05-10" {
		t.Fatalf("day pick: state.date = %v, want 2026-05-10", got)
	}
	// Click year 2024 (row 4) in the year column: x 160..236, y 96..120.
	clickAt(e, 198, 108)
	if got := rt.State["date"]; got != "2024-05-10" {
		t.Fatalf("year pick: state.date = %v, want 2024-05-10", got)
	}
	// February (row 1, y 24..48), then day 31 (row 30, y 720..744): 2024 is a
	// leap year (29 days), so the day clamps.
	clickAt(e, 38, 36)
	clickAt(e, 118, 732)
	if got := rt.State["date"]; got != "2024-02-29" {
		t.Fatalf("day clamp: state.date = %v, want 2024-02-29", got)
	}
}
