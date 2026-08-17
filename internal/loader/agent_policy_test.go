package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/policy"
)

func TestAgentPolicyParsed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("qorm.json", `{
  "type": "app",
  "id": "pol",
  "entry": "main",
  "agent": {
    "policy": {
      "level": "preview-only",
      "tools": { "qorm_dispatch": false },
      "hostCall": { "allowed": true, "ops": ["move"] }
    }
  }
}`)
	write("scenes/main.json", `{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"hi"}}`)

	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if app.AgentPolicy.Level != "preview-only" {
		t.Fatalf("level: got %q", app.AgentPolicy.Level)
	}
	if app.AgentPolicy.Tools["qorm_dispatch"] {
		t.Fatal("dispatch override should be false")
	}
	if !app.AgentPolicy.HostCall.Allowed || app.AgentPolicy.HostCall.Ops[0] != "move" {
		t.Fatalf("hostCall: %+v", app.AgentPolicy.HostCall)
	}
	if !app.AgentPolicy.HasPolicy() {
		t.Fatal("HasPolicy should be true")
	}
}

func TestAgentPolicyUnknownLevelDiagnostic(t *testing.T) {
	dir := t.TempDir()
	body := `{"type":"app","id":"x","entry":"main","agent":{"policy":{"level":"bogus"}}}`
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes/main.json"), []byte(`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range app.Diagnostics {
		if strings.Contains(d, "agent.policy.level") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown level diagnostic, got %v", app.Diagnostics)
	}
}

func TestLegacyAppNoAgentPolicy(t *testing.T) {
	app, err := LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatal(err)
	}
	if app.AgentPolicy.HasPolicy() {
		t.Fatalf("counter should have no agent policy, got %+v", app.AgentPolicy)
	}
	if policy.EffectiveLevel(app.AgentPolicy) != policy.LevelFull {
		t.Fatalf("legacy app should be full access")
	}
}
