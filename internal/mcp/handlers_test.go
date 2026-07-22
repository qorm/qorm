package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// rpc sends one raw JSON-RPC line through the in-process handler and returns
// the decoded response, or nil when the server produces none (notifications,
// unparseable input).
func rpc(t *testing.T, s *Server, line string) map[string]any {
	t.Helper()
	resp := s.HandleLine([]byte(line))
	if resp == nil {
		return nil
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return m
}

// toolCallRPC invokes a tool through the JSON-RPC tools/call path (the way an
// MCP client would) and returns the decoded response.
func toolCallRPC(t *testing.T, s *Server, name string, args any) map[string]any {
	t.Helper()
	aj, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args for %s: %v", name, err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, aj)
	m := rpc(t, s, line)
	if m == nil {
		t.Fatalf("tools/call %s: nil response", name)
	}
	return m
}

// resultText extracts the text content of a tools/call result envelope.
func resultText(t *testing.T, m map[string]any) string {
	t.Helper()
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in response: %v", m)
	}
	content, _ := res["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content in result: %v", res)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	return text
}

// resultIsErr reports the isError flag of a tools/call result envelope.
func resultIsErr(m map[string]any) bool {
	res, _ := m["result"].(map[string]any)
	if res == nil {
		return false
	}
	b, _ := res["isError"].(bool)
	return b
}

// resultObj parses a tools/call result text as a JSON object.
func resultObj(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	text := resultText(t, m)
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("result text is not a JSON object: %s", text)
	}
	return v
}

// resultArr parses a tools/call result text as a JSON array.
func resultArr(t *testing.T, m map[string]any) []any {
	t.Helper()
	text := resultText(t, m)
	var v []any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("result text is not a JSON array: %s", text)
	}
	return v
}

// requireToolErr asserts a tools/call response is a tool-level error whose
// message contains want, and returns the message.
func requireToolErr(t *testing.T, m map[string]any, want string) string {
	t.Helper()
	if !resultIsErr(m) {
		t.Fatalf("expected tool error containing %q, got success: %v", want, m)
	}
	text := resultText(t, m)
	if !strings.Contains(text, want) {
		t.Fatalf("tool error %q does not contain %q", text, want)
	}
	return text
}

func counterDir() string { return filepath.Join("..", "..", "examples", "counter") }

// newCounterHandler builds an in-process counter server for HandleLine calls.
func newCounterHandler(t *testing.T) *Server {
	t.Helper()
	return newCounterServer(t, &bytes.Buffer{}, &bytes.Buffer{})
}

func TestRPCFraming(t *testing.T) {
	s := newCounterHandler(t)

	// ping answers with an empty result object.
	if m := rpc(t, s, `{"jsonrpc":"2.0","id":1,"method":"ping"}`); m["result"] == nil {
		t.Errorf("ping should return a result, got %v", m)
	}
	// Known notifications produce no response.
	if m := rpc(t, s, `{"jsonrpc":"2.0","method":"notifications/cancelled"}`); m != nil {
		t.Errorf("notifications/cancelled must not respond, got %v", m)
	}
	// Unknown method WITH an id is a -32601 error naming the method.
	m := rpc(t, s, `{"jsonrpc":"2.0","id":2,"method":"bogus/method"}`)
	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("unknown method should return an error, got %v", m)
	}
	if code, _ := errObj["code"].(float64); code != -32601 {
		t.Errorf("method-not-found code = %v, want -32601", errObj["code"])
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "bogus/method") {
		t.Errorf("method-not-found message %q should name the method", msg)
	}
	// Unknown method WITHOUT an id is treated as a notification: no response.
	if m := rpc(t, s, `{"jsonrpc":"2.0","method":"bogus/method"}`); m != nil {
		t.Errorf("unknown notification must not respond, got %v", m)
	}
	// Unparseable input is dropped silently.
	if m := rpc(t, s, `this is not json`); m != nil {
		t.Errorf("garbage input must not respond, got %v", m)
	}
	// tools/call with malformed params is a -32602 error.
	m = rpc(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":42}`)
	if errObj, ok := m["error"].(map[string]any); !ok || errObj["code"].(float64) != -32602 {
		t.Fatalf("malformed tools/call params should fail with -32602, got %v", m)
	}
}

func TestServeSkipsBlankLines(t *testing.T) {
	in := bytes.NewBufferString("\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n\n")
	out := &bytes.Buffer{}
	if err := newCounterServer(t, in, out).Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	resp := decodeResponses(t, out)
	if resp[1]["result"] == nil {
		t.Errorf("ping response missing; blank lines should be skipped, got %q", out.String())
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("stdin read failed") }

func TestServeReturnsReadError(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s := New(qrt.New(app), errReader{}, &bytes.Buffer{})
	if err := s.Serve(); err == nil || !strings.Contains(err.Error(), "stdin read failed") {
		t.Fatalf("Serve should surface the stdin read error, got %v", err)
	}
}

func TestHandleHTTP(t *testing.T) {
	s := newCounterHandler(t)

	data := s.HandleHTTP([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("HandleHTTP returned invalid JSON: %v", err)
	}
	result := m["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], protocolVersion)
	}
	if got := s.HandleHTTP([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); got != nil {
		t.Errorf("notification over HTTP must return an empty body, got %s", got)
	}
	if got := s.HandleHTTP([]byte(`{nope`)); got != nil {
		t.Errorf("unparseable body over HTTP must return an empty body, got %s", got)
	}
}

func TestReadOnlyRejectsMutatingTools(t *testing.T) {
	s := newCounterHandler(t)
	s.SetReadOnly(true)

	for _, name := range []string{"qorm_dispatch", "qorm_set_state", "qorm_apply_patch", "qorm_undo"} {
		m := toolCallRPC(t, s, name, map[string]any{})
		errObj, ok := m["error"].(map[string]any)
		if !ok {
			t.Fatalf("%s in read-only mode must return a JSON-RPC error, got %v", name, m)
		}
		if code, _ := errObj["code"].(float64); code != -32000 {
			t.Errorf("%s error code = %v, want -32000", name, errObj["code"])
		}
		msg, _ := errObj["message"].(string)
		if !strings.Contains(msg, "read-only") || !strings.Contains(msg, name) {
			t.Errorf("%s error message %q should name the mode and the tool", name, msg)
		}
	}

	// Inspection tools keep working in read-only mode.
	if m := toolCallRPC(t, s, "qorm_inspect", map[string]any{}); resultIsErr(m) {
		t.Errorf("qorm_inspect must work in read-only mode: %s", resultText(t, m))
	}
	// Nothing may have mutated.
	if got := fmt.Sprint(s.rt.State["count"]); got != "0" {
		t.Errorf("read-only mode must not mutate state; count=%v", got)
	}

	// Switching read-only off re-enables mutations.
	s.SetReadOnly(false)
	res := resultObj(t, toolCallRPC(t, s, "qorm_dispatch",
		map[string]any{"action": "increment", "args": map[string]any{"count": 0}}))
	if state := res["state"].(map[string]any); state["count"].(float64) != 1 {
		t.Errorf("dispatch after SetReadOnly(false) should mutate, state=%v", res["state"])
	}
}

func TestNewSharedAfterMutate(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	mutated := 0
	s := NewShared(qrt.New(app), &sync.Mutex{}, func() { mutated++ })

	// A successful mutating tool invokes afterMutate exactly once.
	if m := toolCallRPC(t, s, "qorm_dispatch",
		map[string]any{"action": "increment", "args": map[string]any{"count": 0}}); resultIsErr(m) {
		t.Fatalf("dispatch: %s", resultText(t, m))
	}
	if mutated != 1 {
		t.Errorf("afterMutate should fire once after a mutating tool, got %d", mutated)
	}

	// Read-only tools do not invoke it.
	if m := toolCallRPC(t, s, "qorm_inspect", map[string]any{}); resultIsErr(m) {
		t.Fatalf("inspect: %s", resultText(t, m))
	}
	if mutated != 1 {
		t.Errorf("afterMutate must not fire for read-only tools, got %d", mutated)
	}

	// A FAILED mutating tool (unknown action) does not invoke it either — the
	// live session only bumps its revision on real changes.
	if m := toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "nope"}); !resultIsErr(m) {
		t.Fatal("unknown action should fail")
	}
	if mutated != 1 {
		t.Errorf("afterMutate must not fire for a failed mutation, got %d", mutated)
	}
}
