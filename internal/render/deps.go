package render

import (
	"regexp"
	"strings"

	"github.com/qorm/platform/internal/model"
)

var stateRefRe = regexp.MustCompile(`state\.([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)`)

// DepIndex maps state paths to the node ids whose bindings read them.
type DepIndex struct {
	byPath    map[string][]string // e.g. "count" -> ["number"]
	patchable map[string]bool     // node id -> safe for id-splice partial render
}

// BuildDepIndex walks a scene tree and indexes state.* references per node.
func BuildDepIndex(root *model.Node) *DepIndex {
	if root == nil {
		return &DepIndex{byPath: map[string][]string{}, patchable: map[string]bool{}}
	}
	idx := &DepIndex{byPath: map[string][]string{}, patchable: map[string]bool{}}
	walkDeps(root, idx)
	return idx
}

func walkDeps(n *model.Node, idx *DepIndex) {
	if n == nil {
		return
	}
	paths := nodeStateRefs(n)
	if n.ID != "" && len(paths) > 0 {
		idx.patchable[n.ID] = patchableType(n.Type)
		for _, p := range paths {
			idx.byPath[p] = appendUnique(idx.byPath[p], n.ID)
		}
	}
	if n.Type == "when" {
		if n.Then != nil {
			walkDeps(n.Then, idx)
		}
		if n.Else != nil {
			walkDeps(n.Else, idx)
		}
		return
	}
	if n.Template != nil {
		walkDeps(n.Template, idx)
	}
	for _, c := range n.Children {
		walkDeps(c, idx)
	}
}

func patchableType(t string) bool {
	switch t {
	case "text", "badge", "progress", "icon":
		return true
	default:
		return false
	}
}

func nodeStateRefs(n *model.Node) []string {
	var refs []string
	collectRefs(n.Text, &refs)
	collectRefs(n.Label, &refs)
	collectRefs(n.Value, &refs)
	collectRefs(n.Condition, &refs)
	collectRefs(n.Data, &refs)
	for _, v := range n.Props {
		collectRefs(v, &refs)
	}
	collectStyleRefs(n.Style, &refs)
	return uniqueStrings(refs)
}

func collectStyleRefs(style map[string]any, refs *[]string) {
	for _, v := range style {
		collectRefs(v, refs)
	}
}

func collectRefs(v any, refs *[]string) {
	switch t := v.(type) {
	case string:
		for _, m := range stateRefRe.FindAllStringSubmatch(t, -1) {
			*refs = append(*refs, m[1])
		}
	case map[string]any:
		for _, vv := range t {
			collectRefs(vv, refs)
		}
	case []any:
		for _, vv := range t {
			collectRefs(vv, refs)
		}
	}
}

// NodesForDirty returns node ids that should be re-rendered for the given
// dirty state paths (prefix match: dirty "items" covers "items.0.name").
func (idx *DepIndex) NodesForDirty(dirty []string) []string {
	if idx == nil {
		return nil
	}
	var out []string
	for _, d := range dirty {
		for path, ids := range idx.byPath {
			if path == d || strings.HasPrefix(path, d+".") || strings.HasPrefix(d, path+".") {
				out = appendUniqueSlice(out, ids...)
			}
		}
	}
	return out
}

// CanPartial reports whether every affected node is a simple patchable widget.
func (idx *DepIndex) CanPartial(dirty []string) bool {
	ids := idx.NodesForDirty(dirty)
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !idx.patchable[id] {
			return false
		}
	}
	return true
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func appendUniqueSlice(list []string, vs ...string) []string {
	for _, v := range vs {
		list = appendUnique(list, v)
	}
	return list
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
