package updates

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
)

func TestStagedRollout(t *testing.T) {
	s := &Server{rollouts: map[string]Rollout{
		"app0":   {Stable: "v1", Canary: "v2", CanaryPercent: 0},
		"app100": {Stable: "v1", Canary: "v2", CanaryPercent: 100},
		"app30":  {Stable: "v1", Canary: "v2", CanaryPercent: 30},
	}}

	// 0% -> everyone stable; 100% -> everyone canary.
	for _, c := range []string{"a", "b", "device-123", "xyz"} {
		if n, _ := s.Resolve("app0", c); n != "v1" {
			t.Errorf("0%% rollout should be stable, got %s", n)
		}
		if n, _ := s.Resolve("app100", c); n != "v2" {
			t.Errorf("100%% rollout should be canary, got %s", n)
		}
	}
	// deterministic per client id
	a, _ := s.Resolve("app30", "client-42")
	b, _ := s.Resolve("app30", "client-42")
	if a != b {
		t.Error("resolve should be deterministic for a client")
	}
	// roughly ~30% canary across many clients
	canary := 0
	for i := 0; i < 1000; i++ {
		if n, _ := s.Resolve("app30", "c"+string(rune(i))); n == "v2" {
			canary++
		}
	}
	if canary < 200 || canary > 400 {
		t.Errorf("canary share ~30%% expected, got %d/1000", canary)
	}
	// unknown app
	if _, ok := s.Resolve("nope", "x"); ok {
		t.Error("unknown app should not resolve")
	}
}

func TestResolveServesBundle(t *testing.T) {
	dir := t.TempDir()
	// build a real bundle to serve
	b, err := bundle.Build(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data, _ := bundle.Marshal(b)
	os.WriteFile(filepath.Join(dir, "counter.bundle"), data, 0o644)
	os.WriteFile(filepath.Join(dir, "rollout.json"),
		[]byte(`{"qorm_counter":{"stable":"counter.bundle"}}`), 0o644)

	srv, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/resolve?app=qorm_counter&client=abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	got, err := bundle.Unmarshal(body)
	if err != nil {
		t.Fatalf("served bundle not valid: %v", err)
	}
	if got.ToApp().EntryRoot() == nil {
		t.Error("served bundle should reconstruct a runnable app")
	}
	if !strings.Contains(string(body), "qorm-bundle/1") {
		t.Error("served payload should be a QORM bundle")
	}
}
