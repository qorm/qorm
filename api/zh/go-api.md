# Go 包:qormext

> 由 `github.com/qorm/qorm/pkg/qormext` 自动生成(`TestAPIRef`),请勿手工编辑。

唯一的公开 Go 包。应用用 Go 注册**自己的**原生操作,打包器将其编译进应用的单一可执行文件——桌面桥接把未知操作分发到这里。

```go
import "github.com/qorm/qorm/pkg/qormext"
```

Package qormext is the user middle-layer registry. An app registers its OWN
native ops (in Go) via native/desktop.go, which the packager compiles INTO
the app's single executable. The desktop bridge dispatches unknown ops here.

Contract — the app's native/desktop.go:

```go
package main
import "github.com/qorm/qorm/pkg/qormext"
func init() {
    qormext.Register("myOp", func(data map[string]any) string {
        // your Go logic; return one line of JS to run back in the app
        return `qormOnMyOp("done")`
    })
}
```

## Functions

### `CallJS`

```go
func CallJS(script string)
```

CallJS runs a line of JS in the app's WebView (from the Go middle-layer).

### `Emit`

```go
func Emit(event, dataJSON string)
```

Emit pushes a signal onto the frontend event channel: every qormOn(event)
listener in the UI fires with dataJSON (a JSON value — "null" if empty). This
is the middle-layer's push side: Go/WASM code tells the UI something changed
and the frontend just listens, instead of the UI polling. Runs on desktop (via
the evaluator) and in WASM (via eval) alike.

### `Native`

```go
func Native(op, dataJSON string)
```

Native triggers a framework low-level op (hardware bridge / built-in) directly
from Go: e.g. Native("bluetoothScan", "{}") reaches the native bridge or Web
API. Results arrive at the app's qormOn<X> JS callback.

### `Register`

```go
func Register(name string, fn Op)
```

Register adds a custom native op (call from an init() in native/desktop.go).

### `SetEvaluator`

```go
func SetEvaluator(fn func(string))
```

SetEvaluator wires the host's JS evaluator (desktop only).

## Types

### `Op`

```go
type Op func(data map[string]any) string
```

Op handles a custom native op: it receives the qormToNative payload and
returns a line of JS (e.g. qormOnFoo(...)) to eval back in the app, or "".
