# Middle-Layer example

Shows the **user middle-layer** (`pkg/qormext`): an app's OWN native operations,
written in Go in `native/desktop.go`, compiled into the desktop binary AND the
mobile/web WASM — so one Go file runs everywhere.

- **hash** — real `crypto/sha256`, something the declarative JSON can't express.
- **visit** — a counter kept in Go memory (stateful backend, survives calls).
- **celebrate** — Go calls the framework hardware bridge (`qormext.Native`) then
  pushes an event onto the UI bus (`qormext.Emit`).

`native/web.js` holds the `qormOn<X>` callbacks and wires the buttons to
`qormToNative(op, data)` — the same contract as the built-in capabilities.

Run it: `qorm run examples/middleware` (desktop) or `qorm package examples/middleware -p web`.
