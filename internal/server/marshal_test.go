package server

// Regression tests for round-3 red-team findings F1/F2
// (planning/rendering-engine/reports/redteam/round2-review.md): the two
// state-touching paths the canvas host's HTTP middleware cannot reach must be
// marshalled onto the render thread — the async-http completion (spawn) and
// the SSE catch-up render (serveEvents).

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
)

func marshalTestServer(t *testing.T) *Server {
	t.Helper()
	app := loader.FromDocs(docsFrom(t,
		`{"type":"app","id":"m","entry":"main","globalState":{"schema":{"n":"number"},"initial":{"n":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[{"type":"text","id":"t","text":"hi"}]}}`,
	))
	return New(runtime.New(app))
}

// F1: the async-http completion writes rt.State, so under a canvas host it
// must run through SetMarshal (previously it took s.mu on its own goroutine
// and raced the render thread — R3 reproduced 7 -race pairs).
func TestAsyncCompletionIsMarshalled(t *testing.T) {
	s := marshalTestServer(t)
	marshalled := make(chan struct{}, 1)
	s.SetMarshal(func(fn func()) { marshalled <- struct{}{}; fn() })

	resumed := make(chan any, 1)
	s.mu.Lock() // spawn's caller holds s.mu (generation snapshot)
	s.spawn(func() any { return 42 }, func(v any) { resumed <- v })
	s.mu.Unlock()

	select {
	case <-marshalled:
	case <-time.After(2 * time.Second):
		t.Fatal("async completion was not marshalled onto the render thread")
	}
	select {
	case v := <-resumed:
		if v != 42 {
			t.Fatalf("resume got %v, want 42", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resume never ran")
	}
}

// Without a canvas host (browser host), the completion runs inline on the
// spawn goroutine — the marshal hook must not change that scheduling.
func TestAsyncCompletionInlineWithoutMarshal(t *testing.T) {
	s := marshalTestServer(t)
	resumed := make(chan any, 1)
	s.mu.Lock()
	s.spawn(func() any { return 7 }, func(v any) { resumed <- v })
	s.mu.Unlock()
	select {
	case v := <-resumed:
		if v != 7 {
			t.Fatalf("resume got %v, want 7", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resume never ran")
	}
}

// F2: serveEvents is exempted from the host's HTTP middleware (the stream is
// long-lived), but its bounded catch-up render reads rt.State — a client
// sending ?rev=0 forced a state read on the HTTP goroutine (R3 reproduced 11
// -race pairs). The catch-up must go through SetMarshal.
func TestEventsCatchUpIsMarshalled(t *testing.T) {
	s := marshalTestServer(t)
	marshalled := make(chan struct{}, 1)
	s.SetMarshal(func(fn func()) { marshalled <- struct{}{}; fn() })

	s.mu.Lock()
	s.bump() // rev 1: a ?rev=0 viewer is behind and gets a catch-up snapshot
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/events?rev=0", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(rec, req); close(done) }()

	select {
	case <-marshalled:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("SSE catch-up render was not marshalled onto the render thread")
	}
	cancel()
	<-done
	if body := rec.Body.String(); !strings.Contains(body, "\"rev\":1") {
		t.Errorf("catch-up snapshot missing from stream, got %q", body)
	}
}

func TestReloadIsMarshalled(t *testing.T) {
	s := marshalTestServer(t)
	next := runtime.New(s.rt.App)
	called := 0
	s.SetMarshal(func(fn func()) {
		called++
		// Reload must not have swapped state before entering the serializer.
		if s.Runtime() == next {
			t.Fatal("Reload swapped runtime before marshal hook")
		}
		fn()
	})
	s.Reload(next)
	if called != 1 {
		t.Fatalf("marshal calls = %d, want 1", called)
	}
	if s.Runtime() != next {
		t.Fatal("Reload did not install next runtime")
	}
}

func TestRecordHumanDispatchUsesSharedActivityShape(t *testing.T) {
	s := marshalTestServer(t)
	s.RecordHumanDispatch("increment")
	if len(s.activity) != 1 || s.activity[0].Source != "human" || s.activity[0].Detail != "dispatch increment" {
		t.Fatalf("activity = %+v", s.activity)
	}
}
