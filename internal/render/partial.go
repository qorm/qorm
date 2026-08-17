package render

import (
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// PatchScene re-renders only the nodes bound to dirty state paths and splices
// their HTML into prev. Returns ok=false when partial render is not possible.
func PatchScene(rt *runtime.Runtime, sceneID string, prev Result, dirty []string, idx *DepIndex) (Result, bool) {
	if idx == nil || prev.HTML == "" || !idx.CanPartial(dirty) {
		return Result{}, false
	}
	root := sceneRoot(rt, sceneID)
	if root == nil {
		return Result{}, false
	}
	nodeByID := indexNodes(root)
	html := prev.HTML
	for _, id := range idx.NodesForDirty(dirty) {
		n := nodeByID[id]
		if n == nil {
			return Result{}, false
		}
		frag := renderNodeFragment(rt, n)
		var ok bool
		html, ok = replaceNodeHTML(html, id, frag)
		if !ok {
			return Result{}, false
		}
	}
	return Result{HTML: html, Handlers: prev.Handlers, Unknown: prev.Unknown}, true
}

func sceneRoot(rt *runtime.Runtime, sceneID string) *model.Node {
	if sceneID != "" {
		if sc := rt.App.Scenes[sceneID]; sc != nil {
			return sc
		}
	}
	return rt.App.EntryRoot()
}

func indexNodes(root *model.Node) map[string]*model.Node {
	out := map[string]*model.Node{}
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if n.ID != "" {
			out[n.ID] = n
		}
		if n.Type == "when" {
			walk(n.Then)
			walk(n.Else)
			return
		}
		if n.Template != nil {
			walk(n.Template)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

func renderNodeFragment(rt *runtime.Runtime, n *model.Node) string {
	r := &renderer{rt: rt, scope: map[string]any{}}
	r.node(n)
	return r.sb.String()
}

// replaceNodeHTML swaps the outer HTML of the element with id=nodeID.
func replaceNodeHTML(doc, nodeID, repl string) (string, bool) {
	for _, marker := range []string{`id="` + nodeID + `"`, `id='` + nodeID + `'`} {
		pos := strings.Index(doc, marker)
		if pos < 0 {
			continue
		}
		start := strings.LastIndex(doc[:pos], "<")
		if start < 0 {
			continue
		}
		tag := tagName(doc[start:])
		if tag == "" {
			continue
		}
		end, ok := elementEnd(doc, start, tag)
		if !ok {
			continue
		}
		return doc[:start] + repl + doc[end:], true
	}
	return doc, false
}

func tagName(opening string) string {
	if len(opening) < 2 || opening[0] != '<' {
		return ""
	}
	i := 1
	if i < len(opening) && opening[i] == '/' {
		return ""
	}
	start := i
	for i < len(opening) && opening[i] != ' ' && opening[i] != '>' && opening[i] != '/' {
		i++
	}
	return strings.ToLower(opening[start:i])
}

func elementEnd(doc string, start int, tag string) (int, bool) {
	open := "<" + tag
	close := "</" + tag + ">"
	depth := 0
	i := start
	for i < len(doc) {
		nextOpen := strings.Index(doc[i:], open)
		nextClose := strings.Index(doc[i:], close)
		if nextClose < 0 {
			return 0, false
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			after := i + nextOpen + len(open)
			if after < len(doc) && (doc[after] == ' ' || doc[after] == '>' || doc[after] == '/') {
				depth++
				i = after
				continue
			}
		}
		depth--
		i = i + nextClose + len(close)
		if depth == 0 {
			return i, true
		}
	}
	return 0, false
}
