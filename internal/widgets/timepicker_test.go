package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// Tapping rows in the hour/minute columns updates the bound time; minuteStep
// snaps the minute column to its increments.
func TestTimePickerSelects(t *testing.T) {
	tp := &model.Node{Type: "timepicker", ID: "tp", Value: "{{state.time}}",
		Props: map[string]any{"minuteStep": 5.0}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{tp}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 800))
	rt.State["time"] = "09:15"
	e.DrawFrame(surf)

	// Click hour 14 (row 14) in the hour column: x 0..76, y 14*24=336..360.
	clickAt(e, 38, 348)
	if got := rt.State["time"]; got != "14:15" {
		t.Fatalf("hour pick: state.time = %v, want 14:15", got)
	}
	// Click minute 30 (row 6 in 5-min increments) in the minute column:
	// x 80..156, y 6*24=144..168.
	clickAt(e, 118, 156)
	if got := rt.State["time"]; got != "14:30" {
		t.Fatalf("minute pick: state.time = %v, want 14:30", got)
	}
}
