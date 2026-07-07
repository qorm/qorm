package mcp

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func newCounterServer(t *testing.T, in, out *bytes.Buffer) *Server {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return New(qrt.New(app), in, out)
}

// decode returns the parsed responses keyed by id.
func decodeResponses(t *testing.T, out *bytes.Buffer) map[float64]map[string]any {
	t.Helper()
	byID := map[float64]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad response line %q: %v", line, err)
		}
		if id, ok := m["id"].(float64); ok {
			byID[id] = m
		}
	}
	return byID
}

func TestMCPHandshakeAndTools(t *testing.T) {
	in := bytes.NewBufferString(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_inspect","arguments":{}}}`,
	}, "\n") + "\n")
	out := &bytes.Buffer{}
	if err := newCounterServer(t, in, out).Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	resp := decodeResponses(t, out)

	if resp[1]["result"].(map[string]any)["serverInfo"] == nil {
		t.Error("initialize should return serverInfo")
	}
	tools := resp[2]["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 5 {
		t.Errorf("expected >=5 tools, got %d", len(tools))
	}
	inspectText := resp[3]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(inspectText, "QORM Premium Counter") {
		t.Errorf("inspect should include app name, got: %s", inspectText)
	}
}

func TestMCPSimulateIsSideEffectFree(t *testing.T) {
	in := bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_simulate_action","arguments":{"action":"increment","args":{"count":5}}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_inspect","arguments":{}}}` + "\n")
	out := &bytes.Buffer{}
	if err := newCounterServer(t, in, out).Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	resp := decodeResponses(t, out)

	simText := resp[1]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var sim map[string]any
	if err := json.Unmarshal([]byte(simText), &sim); err != nil {
		t.Fatalf("simulate result not json: %v", err)
	}
	if after := sim["after"].(map[string]any); after["count"].(float64) != 6 {
		t.Errorf("simulate increment(5) should yield count 6, got %v", after["count"])
	}

	// The live app must be unchanged (count still 0).
	inspectText := resp[2]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var insp map[string]any
	_ = json.Unmarshal([]byte(inspectText), &insp)
	if state := insp["currentState"].(map[string]any); state["count"].(float64) != 0 {
		t.Errorf("simulate must not touch live state; count=%v", state["count"])
	}
}
