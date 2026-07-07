package support

import (
	"os"
	"testing"
)

// TestSupportMatrixInSync keeps docs/platforms/support-matrix.md generated from
// the registry, so the at-a-glance "what works where" table never drifts.
// Regenerate with: QORM_UPDATE_DOCS=1 go test ./internal/support/
func TestSupportMatrixInSync(t *testing.T) {
	const path = "../../docs/platforms/support-matrix.md"
	want := Markdown()
	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("docs/platforms/support-matrix.md is out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/support/")
	}
}

// TestMatrixShape guards the row/column alignment.
func TestMatrixShape(t *testing.T) {
	if len(Targets) != 7 {
		t.Fatalf("expected 7 targets, got %d", len(Targets))
	}
	for _, f := range Matrix {
		if len(f.Cells) != 7 {
			t.Errorf("%q has %d cells, want 7", f.Name, len(f.Cells))
		}
	}
}
