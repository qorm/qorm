package playcore

import "github.com/qorm/platform/internal/runtime"

// InstallSinks wires a SINGLE-THREADED host to the runtime's two host sinks:
// the frame sink (`render` steps publishing a loading state mid-action) and the
// background work sink (an `http.*` step's round trip leaving the dispatch
// goroutine). Three hosts share it because they are one binary: the standalone
// WASM runtime (cmd/qorm-wasm), the offline package that embeds it, and the
// live playground that loads the same qorm.wasm.
//
// It lives here rather than in cmd/qorm-wasm so it can be tested under the host
// GOOS: js.FuncOf cannot run outside GOOS=js, but the sink logic — the
// generation pin, the publish-after-resume ordering, the degradation when the
// page has no frame sink — is ordinary Go, and it is exactly the part that is
// worth pinning with tests.
//
// The parameters are the three things that differ between production and a
// test, and nothing else:
//
//   - live reports the runtime the host is CURRENTLY executing. A response that
//     comes back after the host swapped runtimes (an OTA update applied, a
//     rollback, a playground recompile) is DROPPED rather than written into the
//     successor's state — the same generation pin the server's spawn uses, and
//     for the same reason: values from a retired app must not resurrect in the
//     one that replaced it. The abandoned runtime keeps its raised Inflight
//     count, which is correct, because nothing reads it any more.
//   - spawn starts the blocking half on a goroutine of the host's choosing.
//     Production passes `go f()`; a test passes an inline call and gets a fully
//     deterministic completion with no sleeps and no scheduler dependency.
//   - publish renders the current runtime and hands the frame to the page (the
//     window.qormApplyFrame entry point). Without it the runtime would still
//     settle correctly and the SCREEN would silently never update, which is why
//     installing the sinks and having a push channel is one operation, not two.
//
// AsyncAll is part of the deal, not a separate knob: on this host a
// SYNCHRONOUS http step does not just stall, it deadlocks the single-threaded
// scheduler (see runtime.Runtime.AsyncAll), so every http step must go through
// the sink whether or not its JSON opted in.
//
// THE HOST CONTRACT this implementation relies on (runtime.Runtime.Async spells
// it out): resume must run to completion, and publish must follow it, without
// yielding to another dispatch in between. On a single-threaded host that is
// achieved by containing no blocking call — there is no lock to take, so
// "does not yield" IS the mutual exclusion. work, by contrast, may block for as
// long as the network takes: it touches nothing on the runtime.
func InstallSinks(rt *runtime.Runtime, live func() *runtime.Runtime, spawn func(func()), publish func()) {
	if rt == nil || spawn == nil || publish == nil {
		return
	}
	rt.Commit = publish
	rt.AsyncAll = true
	rt.Async = func(work func() any, resume func(any)) {
		spawn(func() {
			v := work()
			if live != nil && live() != rt {
				// A replaced runtime's reply: drop it, do not publish — and
				// tell the retired runtime so, because the continuation that
				// would have released its in-flight count is the one being
				// dropped. Without this its Inflight() never returns to zero
				// and anything polling that runtime for quiescence (the MCP
				// activity payload, a test waiting for the app to settle) waits
				// for a continuation that will never run.
				rt.AbandonInflight()
				return
			}
			resume(v)
			publish()
		})
	}
}
