package capability

import (
	"os"
	"testing"
)

// TestCapabilityDocInSync keeps docs/platforms/capabilities.md generated from the
// registry — so the human-facing capability reference never drifts from the code.
// Regenerate with: QORM_UPDATE_DOCS=1 go test ./internal/capability/
func TestCapabilityDocInSync(t *testing.T) {
	const path = "../../docs/platforms/capabilities.md"
	want := Markdown()
	const pathZH = "../../docs/zh/platforms/capabilities.md"
	wantZH := MarkdownZH()

	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pathZH, []byte(wantZH), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("docs/platforms/capabilities.md is out of sync with the registry — run: QORM_UPDATE_DOCS=1 go test ./internal/capability/")
	}

	gotZH, err := os.ReadFile(pathZH)
	if err != nil || string(gotZH) != wantZH {
		t.Errorf("docs/zh/platforms/capabilities.md is out of sync with the registry — run: QORM_UPDATE_DOCS=1 go test ./internal/capability/")
	}
}
