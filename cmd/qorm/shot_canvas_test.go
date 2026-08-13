//go:build !(darwin && desktop)

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanvasShotSafetyLimitsAndOutputPaths(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		want string
	}{
		{0, 100, "positive"},
		{maxCanvasPNGDimension + 1, 1, "per edge"},
		{4096, 4097, "per edge"},
	} {
		if err := validateCanvasPNGSize(tc.w, tc.h); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("validate %dx%d = %v, want %q", tc.w, tc.h, err, tc.want)
		}
	}

	dir := t.TempDir()
	if err := renderCanvasPNG("unused", 10, 10, filepath.Join(dir, "not-png.jpg")); err == nil || !strings.Contains(err.Error(), ".png") {
		t.Fatalf("extension error = %v", err)
	}
	target := filepath.Join(dir, "target.png")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(target, link); err == nil {
		if err := renderCanvasPNG("unused", 10, 10, link); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("symlink error = %v", err)
		}
		if got, _ := os.ReadFile(target); string(got) != "keep" {
			t.Fatalf("symlink target was modified: %q", got)
		}
	}
	missingParent := filepath.Join(dir, "missing", "out.png")
	if err := renderCanvasPNG("unused", 10, 10, missingParent); err == nil || !strings.Contains(err.Error(), "output parent") {
		t.Fatalf("missing-parent error = %v", err)
	}
}
