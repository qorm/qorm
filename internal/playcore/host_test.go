package playcore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// Tests for the single-threaded host's two sinks (the WASM standalone build,
// the offline package and the playground share this one implementation).
//
// They run under the HOST GOOS on purpose — js.FuncOf cannot, which is exactly
// why the sink logic lives here and not in cmd/qorm-wasm. Every one of them is
// deterministic: spawn runs its closure INLINE, so a "background" round trip
// completes before InstallSinks' caller returns and there is nothing to sleep
// on or poll for.

// hostFake is a test double for the three things InstallSinks needs from a
// host: which runtime is live, how to start background work, and where a
// published frame goes.
type hostFake struct {
	live   *runtime.Runtime
	frames int  // how many frames the host published
	ran    bool // whether spawn was asked to run anything
}

func (h *hostFake) install(rt *runtime.Runtime) {
	InstallSinks(rt,
		func() *runtime.Runtime { return h.live },
		func(f func()) { h.ran = true; f() }, // inline: deterministic, no goroutine
		func() { h.frames++ })
}

// httpApp builds a one-action app whose single step GETs url and writes the
// response to state.out. The step is NOT marked {"async": true} — proving the
// host's AsyncAll is what moves it off the dispatch goroutine, which is the
// whole point on js/wasm (a blocking round trip there deadlocks the scheduler).
func httpApp(t *testing.T, url string) *model.App {
	t.Helper()
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "a", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "t", "text": "{{state.out}}"}},
		{"type": "action", "id": "load", "steps": []any{
			map[string]any{"type": "http.get", "url": url, "result": "out"},
		}},
	})
	if len(app.Diagnostics) > 0 {
		t.Fatalf("unexpected loader diagnostics: %v", app.Diagnostics)
	}
	return app
}

func okServer(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestInstallSinksRunsHTTPInBackgroundAndPublishes is the M3 acceptance in one
// assertion: on this host a PLAIN http step leaves the dispatch goroutine (so
// the JS callback returns instead of deadlocking), its outcome still lands in
// state, and — the half that makes it visible — the host is told to publish a
// frame afterwards.
func TestInstallSinksRunsHTTPInBackgroundAndPublishes(t *testing.T) {
	rt := runtime.New(httpApp(t, okServer(t, `{"v":7}`)))
	h := &hostFake{live: rt}
	h.install(rt)

	rt.Dispatch("load", nil)

	if !h.ran {
		t.Fatal("a plain http step must go through the background sink on this host")
	}
	if rt.Inflight() != 0 {
		t.Fatalf("inflight = %d, want 0 once the continuation has run", rt.Inflight())
	}
	out, _ := rt.State["out"].(map[string]any)
	if out == nil || out["v"] != 7.0 {
		t.Fatalf("state.out = %#v, want the decoded response", rt.State["out"])
	}
	if h.frames != 1 {
		t.Fatalf("published %d frames, want exactly 1 for the completion", h.frames)
	}
}

// TestInstallSinksCommitPublishesFrame pins the other sink: a `render` step
// publishes an intermediate frame through the host, which is what makes a
// loading state visible on a host that returns its render synchronously.
func TestInstallSinksCommitPublishesFrame(t *testing.T) {
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "a", "entry": "main"},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "text", "id": "t", "text": "x"}},
		{"type": "action", "id": "go", "steps": []any{
			map[string]any{"type": "set", "path": "busy", "value": true},
			map[string]any{"type": "render"},
			map[string]any{"type": "set", "path": "busy", "value": false},
		}},
	})
	rt := runtime.New(app)
	h := &hostFake{live: rt}
	h.install(rt)

	rt.Dispatch("go", nil)

	if h.frames != 1 {
		t.Fatalf("published %d frames, want 1 from the render step", h.frames)
	}
}

// TestInstallSinksDropsReplacedRuntime pins the generation guard: an OTA
// update, a rollback or a playground recompile swaps the runtime while a
// request is open, and the reply belongs to an app that is no longer running.
// It must NOT be written into the successor's state, and must not publish a
// frame rendered against a scene the user is no longer on.
func TestInstallSinksDropsReplacedRuntime(t *testing.T) {
	url := okServer(t, `{"v":7}`)
	old := runtime.New(httpApp(t, url))
	next := runtime.New(httpApp(t, url))
	h := &hostFake{live: old}
	h.install(old)
	// The swap happens while the request is open: spawn runs inline, so doing it
	// from the "background" closure's point of view means swapping first.
	h.live = next

	old.Dispatch("load", nil)

	if _, ok := old.State["out"]; ok {
		t.Error("a replaced runtime must not receive its in-flight reply")
	}
	if _, ok := next.State["out"]; ok {
		t.Error("the successor runtime must not inherit the retired app's reply")
	}
	if h.frames != 0 {
		t.Fatalf("published %d frames for a dropped reply, want 0", h.frames)
	}
	// The abandoned runtime keeps its raised count — documented and sound,
	// because nothing reads it any more.
	if old.Inflight() != 1 {
		t.Fatalf("abandoned runtime Inflight = %d, want the raised 1", old.Inflight())
	}
}

// TestNewRuntimeHasNoHostSinks is the contract in the other direction: a bare
// runtime carries neither sink and no AsyncAll, so a plain render, a static
// export and an MCP simulation stay fully synchronous and never open a socket
// on a background goroutine.
func TestNewRuntimeHasNoHostSinks(t *testing.T) {
	rt := runtime.New(httpApp(t, "http://127.0.0.1:1/never"))
	if rt.Commit != nil || rt.Async != nil || rt.AsyncAll {
		t.Fatal("runtime.New must install no host sinks")
	}
	// …and a clone of an INSTALLED runtime is hook-free too, so simulate never
	// inherits the live host's network or frame channel.
	h := &hostFake{live: rt}
	h.install(rt)
	c := rt.Clone()
	if c.Commit != nil || c.Async != nil || c.AsyncAll {
		t.Fatal("Clone must not carry the host sinks")
	}
}

// TestInstallSinksIgnoresIncompleteHost guards the degradation path: a host
// that cannot publish frames (no DOM, an old page with no qormApplyFrame) gets
// NOTHING installed rather than an async sink with no way to show the result —
// which would trade a deadlock for a UI that silently never updates.
func TestInstallSinksIgnoresIncompleteHost(t *testing.T) {
	rt := runtime.New(httpApp(t, "http://127.0.0.1:1/never"))
	InstallSinks(rt, nil, func(f func()) { f() }, nil)
	if rt.Async != nil || rt.Commit != nil || rt.AsyncAll {
		t.Fatal("a host with no frame sink must get no hooks at all")
	}
	InstallSinks(nil, nil, func(f func()) { f() }, func() {}) // must not panic
}

// TestInstallSinksWithoutLiveFuncAlwaysResumes covers the optional generation
// pin: a host that never swaps runtimes may pass nil, and every reply lands.
func TestInstallSinksWithoutLiveFuncAlwaysResumes(t *testing.T) {
	rt := runtime.New(httpApp(t, okServer(t, `{"v":7}`)))
	frames := 0
	InstallSinks(rt, nil, func(f func()) { f() }, func() { frames++ })
	rt.Dispatch("load", nil)
	if _, ok := rt.State["out"]; !ok {
		t.Error("with no generation pin every reply must be written")
	}
	if frames != 1 {
		t.Fatalf("published %d frames, want 1", frames)
	}
}
