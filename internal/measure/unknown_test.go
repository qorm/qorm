package measure

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestAuditFlagsUnknownWidget verifies the self-verify audit reports a
// mistyped/unrecognised widget type, so a typo is caught without a device.
func TestAuditFlagsUnknownWidget(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "colunm", ID: "typo"}}},
	}}
	out, err := Audit(qrt.New(app), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "unknown-widget") || !strings.Contains(s, "colunm") {
		t.Errorf("audit should flag the unknown widget 'colunm'; got:\n%s", s)
	}
	// a clean app has no unknown-widget issue
	clean := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "column", ID: "ok"}}},
	}}
	out2, _ := Audit(qrt.New(clean), nil)
	if strings.Contains(string(out2), "unknown-widget") {
		t.Errorf("clean app should have no unknown-widget issue; got:\n%s", out2)
	}
}
