package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestJSONComponents verifies app-defined JSON components: a component used
// twice with different props renders both prop sets, a {slot} fills with the
// instance's children, and repeated uses get unique ids.
func TestJSONComponents(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(`{
      "type":"app","id":"c","name":"C","entry":"main",
      "components":{
        "stat":{"type":"column","id":"sc","children":[{"type":"text","id":"sv","text":"{{prop.value}}"}]},
        "panel":{"type":"card","id":"pc","children":[{"type":"text","id":"pt","text":"{{prop.title}}"},{"type":"slot","id":"ps"}]}
      }}`), 0o644)
	os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(`{"type":"scene","id":"main","root":{"type":"scaffold","id":"root","children":[
      {"type":"stat","id":"a","value":"AAA"},
      {"type":"stat","id":"b","value":"BBB"},
      {"type":"panel","id":"p","title":"TITLE","children":[{"type":"text","id":"kid","text":"SLOTTED"}]}
    ]}}`), 0o644)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := render.Render(qrt.New(app))
	if len(res.Unknown) != 0 {
		t.Errorf("component types should be recognised, got unknown: %v", res.Unknown)
	}
	for _, want := range []string{"AAA", "BBB", "TITLE", "SLOTTED", `id="sv_a"`, `id="sv_b"`} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}
