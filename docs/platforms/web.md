# QORM Web Platform

The web platform connects to QORM through the WASM Runtime or the TypeScript Adapter.

## Package it

```sh
qorm package examples/dashboard -p web -o dashboard-web   # an installable, offline PWA
```

Serve the output folder and "Add to Home Screen". Any example packages to web.
See the [support matrix](support-matrix.md).

## Architecture

```text
qorm.bundle.json
  ↓
QORM WASM Runtime / Web Runtime
  ↓
Web Host Adapter
  ↓
Renderer
  ↓
Browser
```

## Host Adapter

On the web, low-level capabilities are constrained by the browser. They should be wrapped through the Web Host Adapter:

```text
network.request
storage.read/write
clipboard.read/write
navigation.go
file.open
notification.show
```

## Web security boundaries

- The browser sandbox is the outermost capability boundary.
- The QORM Web Runtime cannot exceed the capabilities granted by the browser.
- The Web Host Adapter is QORM's permission and policy enforcement point inside the browser.
- A native browser permission prompt is not equivalent to a QORM approval; if both are required, both must pass.

## Network requests

The web platform uses an HttpClient abstraction:

```text
default fetch
pluggable custom HttpClient adapter
```

### Custom HttpClient boundaries

- The custom client is responsible only for the transport implementation, not for permission decisions.
- Domain, method, header, credentials, and approval checks must be performed on the Host Adapter side.
- The custom client cannot bypass QORM's `network.request` capability constraints.
- CORS, cookie, and same-origin restrictions remain under browser control.

Action example:

```json
{
  "type": "host.call",
  "capability": "network.request",
  "input": {
    "method": "GET",
    "url": "/api/tasks",
    "responseType": "json"
  },
  "output": {
    "path": "tasksResponse"
  }
}
```

## Rendering routes

Available routes:

```text
DOM renderer
Canvas renderer
WebGPU renderer
WASM + GPU renderer
```

V1 can start with the route that is easiest to validate, then strengthen high-performance rendering later.

## Limitations

The web platform cannot assume:

```text
Arbitrary filesystem access
System-level clipboard access
Long-running background execution
Arbitrary cross-origin networking
Local Native Capability
```

## Audit visibility

Web audit records are affected by browser privacy and storage constraints. Even so, permission decisions, approval ids, and capability results should still be recorded to the available local or host logs as much as possible.