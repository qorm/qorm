package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCmdCheckRefusesErrorDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"type":"app","id":"broken","entry":"main","computed":{"k":"{{ computed[state.k] }}"}}`
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	scene := `{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"hi"}}`
	if err := os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdCheck([]string{dir, "--audit"}); code != 1 {
		t.Fatalf("cmdCheck exit = %d, want 1 (computed dynamic key is an error-level diagnostic)", code)
	}
}

func TestCmdCheckAuditCleanAppStillPasses(t *testing.T) {
	if code := cmdCheck([]string{counterDir(), "--audit"}); code != 0 {
		t.Fatalf("cmdCheck counter --audit exit = %d, want 0", code)
	}
}
