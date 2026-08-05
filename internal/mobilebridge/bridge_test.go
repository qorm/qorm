package mobilebridge

import (
	"testing"
)

const counterDocs = `[
	{"type":"app","id":"c","entry":"main","globalState":{"schema":{"count":"number"},"initial":{"count":0}}},
	{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
		{"type":"text","id":"t","text":"{{state.count}}"},
		{"type":"button","id":"b","label":"+","onPress":{"name":"inc"}}]}},
	{"type":"action","id":"inc","steps":[{"type":"state.increment","path":"count"}]}
]`

func newTestSession(t *testing.T) *Session {
	t.Helper()
	s, err := NewSession(counterDocs)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s.SetViewport(200, 200, 1)
	return s
}

func TestSessionRendersAndCounts(t *testing.T) {
	s := newTestSession(t)
	pix, fresh := s.RenderFrame()
	if !fresh || len(pix) != 200*200*4 {
		t.Fatalf("first frame: fresh=%v len=%d", fresh, len(pix))
	}
	// Second render with no change reports no new frame.
	if _, fresh := s.RenderFrame(); fresh {
		t.Error("idle second frame must report fresh=false")
	}
	s.Dispatch("inc")
	if got := s.State("count"); got != "1" {
		t.Errorf("count after inc = %q, want 1", got)
	}
	if _, fresh := s.RenderFrame(); !fresh {
		t.Error("a dispatch must produce a new frame")
	}
}

func TestSessionPointerAndKeys(t *testing.T) {
	s := newTestSession(t)
	s.RenderFrame()
	// A press anywhere must not crash and reports whether visuals changed.
	_ = s.Pointer("press", 100, 100)
	_ = s.Pointer("release", 100, 100)
	if s.CursorHint() != 0 {
		t.Errorf("cursor hint with no hover = %d, want 0 (arrow)", s.CursorHint())
	}
	s.SetState("count", "41")
	if got := s.State("count"); got != "41" {
		t.Errorf("SetState/State round trip = %q", got)
	}
}

func TestSessionBadDocs(t *testing.T) {
	if _, err := NewSession(`{"not":"json"`); err == nil {
		t.Error("malformed docs must error")
	}
	if _, err := NewSession(`[]`); err == nil {
		t.Error("empty docs must error (no scenes)")
	}
}
