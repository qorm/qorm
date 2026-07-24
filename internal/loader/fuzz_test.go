package loader

import (
	"encoding/json"
	"testing"
)

// FuzzFromDocs ensures app assembly from raw source documents never panics on
// arbitrary input (manifest/scene/action docs come from app JSON, so
// robustness matters). It may produce diagnostics, but must not crash.
func FuzzFromDocs(f *testing.F) {
	// The document-shaped seeds are wrapped in a one-element array because the
	// fuzz input is decoded the same way collect()'s callers hand FromDocs a
	// document set; bare objects and non-object arrays then exercise the
	// malformed-input early return.
	for _, s := range []string{
		`[{"type":"app","id":"counter","entry":"main","globalState":{"schema":{"count":"number"},"initial":{"count":0}}}]`,
		`[{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"{{count}}"}}]`,
		`[{"type":"action","id":"increment","steps":[{"type":"state.set","path":"count","value":"{{ count + 1 }}"}]}]`,
		`[1,2,3]`,
		`{"type":"scene","root":[]}`,
		`{"type":"app","globalState":{"schema":{"x":"number"}}}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		var docs []map[string]any
		if json.Unmarshal([]byte(data), &docs) != nil {
			return // not an array of objects, as collect() would skip
		}
		app := FromDocs(docs)
		_ = app.EntryRoot() // must not panic
	})
}
