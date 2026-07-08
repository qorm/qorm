package support

import (
	"os"
	"strings"
	"testing"
)

// TestREADMESummaryInSync keeps the top-level README's platform-support glance
// table generated from the registry. Regenerate: QORM_UPDATE_DOCS=1 go test ./internal/support/
func TestREADMESummaryInSync(t *testing.T) {
	const path = "../../README.md"
	const pathZH = "../../README.zh.md"
	const startM = "<!-- support-summary:start -->"
	const endM = "<!-- support-summary:end -->"

	// English README
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i, j := strings.Index(s, startM), strings.Index(s, endM)
	if i < 0 || j < 0 || j < i {
		t.Fatal("README support-summary markers missing")
	}
	want := s[:i+len(startM)] + "\n" + Summary() + s[j:]

	// Chinese README
	dataZH, err := os.ReadFile(pathZH)
	if err != nil {
		t.Fatal(err)
	}
	sZH := string(dataZH)
	iZH, jZH := strings.Index(sZH, startM), strings.Index(sZH, endM)
	if iZH < 0 || jZH < 0 || jZH < iZH {
		t.Fatal("README.zh.md support-summary markers missing")
	}
	wantZH := sZH[:iZH+len(startM)] + "\n" + SummaryZH() + sZH[jZH:]

	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pathZH, []byte(wantZH), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	if s != want {
		t.Errorf("README platform-support summary out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/support/")
	}
	if sZH != wantZH {
		t.Errorf("README.zh.md platform-support summary out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/support/")
	}
}

// TestSupportMatrixInSync keeps docs/platforms/support-matrix.md generated from
// the registry, so the at-a-glance "what works where" table never drifts.
// Regenerate with: QORM_UPDATE_DOCS=1 go test ./internal/support/
func TestSupportMatrixInSync(t *testing.T) {
	const path = "../../docs/platforms/support-matrix.md"
	want := Markdown()
	const pathZH = "../../docs/zh/platforms/support-matrix.md"
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
		t.Errorf("docs/platforms/support-matrix.md is out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/support/")
	}

	gotZH, err := os.ReadFile(pathZH)
	if err != nil || string(gotZH) != wantZH {
		t.Errorf("docs/zh/platforms/support-matrix.md is out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/support/")
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
