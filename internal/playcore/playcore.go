// Package playcore is the pure compile core behind the live playground's
// qormCompile WASM export: it turns a set of raw QORM source documents (a JSON
// array of doc objects) into rendered HTML plus the diagnostics and
// unknown-widget lists a normal render discards.
//
// It is deliberately free of fs/network/cgo — loader.FromDocs, runtime.New and
// render.Render are all pure — and is NOT gated on js/wasm, so the doc-array ->
// {html, diagnostics, unknown} logic can be unit-tested under the host GOOS
// (js.FuncOf itself cannot run outside GOOS=js). The js.FuncOf wrapper in
// cmd/qorm-wasm is a thin adapter over CompileDocs.
package playcore

import (
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

// Result is one compile's output, ready to be handed to the JS bridge. RT and
// Handlers are exposed so the WASM wrapper can keep its global runtime/handler
// table consistent with qormInit (so qormEvent still works after a compile);
// the HTML/theme/dir/diagnostics/unknown fields are what cross into JS.
type Result struct {
	RT       *runtime.Runtime
	Handlers []render.Handler
	HTML     string
	Theme    string
	Dir      string
	// Diagnostics are the loader's static warnings/errors for the source
	// (app.Diagnostics). Unknown are the widget types the render did not
	// recognise (render.Result.Unknown). Both are nil for a clean compile.
	Diagnostics []string
	Unknown     []string
}

// CompileDocs builds a model.App from raw source docs WITHOUT requiring a
// signature or content hash — the playground compiles live, unsigned source —
// by replicating the bundle path's doc dispatch (loader.FromDocs), rendering
// the entry scene, and returning the HTML together with the diagnostics and
// unknown-widget lists that renderNow otherwise drops.
func CompileDocs(docs []map[string]any) Result {
	app := loader.FromDocs(docs)
	rt := runtime.New(app)
	res := render.Render(rt)
	dir := "ltr"
	if rt.IsRTL() {
		dir = "rtl"
	}
	return Result{
		RT:          rt,
		Handlers:    res.Handlers,
		HTML:        res.HTML,
		Theme:       rt.CurrentTheme(),
		Dir:         dir,
		Diagnostics: app.Diagnostics,
		Unknown:     res.Unknown,
	}
}
