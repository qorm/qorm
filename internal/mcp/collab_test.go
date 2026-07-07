package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// callResult extracts the tools/call text payload for a response id, parsed as
// JSON when possible.
func callResult(t *testing.T, resp map[float64]map[string]any, id float64) map[string]any {
	t.Helper()
	res, ok := resp[id]["result"].(map[string]any)
	if !ok {
		t.Fatalf("id %v: no result (%v)", id, resp[id])
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("id %v: result not json object: %s", id, text)
	}
	return m
}

func TestMCPOperateAndTest(t *testing.T) {
	in := bytes.NewBufferString(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_set_state","arguments":{"path":"status","value":"live"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_assert","arguments":{"checks":[{"kind":"stateEquals","path":"count","value":1},{"kind":"stateEquals","path":"status","value":"live"},{"kind":"nodeExists","id":"btn_plus"}]}}}`,
	}, "\n") + "\n")
	out := &bytes.Buffer{}
	if err := newCounterServer(t, in, out).Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	resp := decodeResponses(t, out)

	if state := callResult(t, resp, 1)["state"].(map[string]any); state["count"].(float64) != 1 {
		t.Fatalf("dispatch should mutate live state to 1, got %v", state["count"])
	}
	if !callResult(t, resp, 3)["pass"].(bool) {
		t.Errorf("all assertions should pass: %v", callResult(t, resp, 3)["checks"])
	}
}

func TestMCPDesignPreviewApplyBinding(t *testing.T) {
	ops := []PatchOp{{Op: "setProp", Target: "title", Key: "text", Value: "AI EDITED"}}
	opsJSON, _ := json.Marshal(ops)
	token := patchToken(ops) // deterministic; the client would read it from preview

	in := bytes.NewBufferString(strings.Join([]string{
		// apply WITHOUT a preview must be refused.
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_apply_patch","arguments":{"ops":` + string(opsJSON) + `,"previewToken":"` + token + `"}}}`,
		// preview (side-effect-free), then apply bound to it.
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_preview_patch","arguments":{"ops":` + string(opsJSON) + `}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_apply_patch","arguments":{"ops":` + string(opsJSON) + `,"previewToken":"` + token + `"}}}`,
	}, "\n") + "\n")
	out := &bytes.Buffer{}
	srv := newCounterServer(t, in, out)
	if err := srv.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	resp := decodeResponses(t, out)

	// 1) apply-before-preview is an error result.
	res1 := resp[1]["result"].(map[string]any)
	if res1["isError"] != true {
		t.Errorf("apply without preview must be refused, got %v", res1)
	}

	// 2) preview is side-effect-free but shows the edit + returns the token.
	prev := callResult(t, resp, 2)
	if prev["previewToken"] != token {
		t.Errorf("preview token mismatch: %v vs %v", prev["previewToken"], token)
	}
	if !strings.Contains(prev["html"].(string), "AI EDITED") {
		t.Error("preview html should reflect the edit")
	}

	// 3) apply bound to the preview commits to the live app.
	applied := callResult(t, resp, 3)
	if !strings.Contains(applied["html"].(string), "AI EDITED") {
		t.Error("apply should commit the edit to the live app")
	}
}
