package capability

import (
	"sort"

	"github.com/qorm/platform/internal/model"
)

// UsedWidgets returns sorted widget type names in app scenes that map to a
// built-in capability (portable widgets are omitted).
func UsedWidgets(app *model.App) []string {
	if app == nil {
		return nil
	}
	seen := map[string]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if ForWidget(n.Type) != nil {
			seen[n.Type] = true
		}
		if n.Then != nil {
			walk(n.Then)
		}
		if n.Else != nil {
			walk(n.Else)
		}
		if n.Template != nil {
			walk(n.Template)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, root := range app.Scenes {
		walk(root)
	}
	for _, root := range app.Components {
		walk(root)
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// StemsFromApp returns sorted canonical capability stems used by the app's UI.
func StemsFromApp(app *model.App) []string {
	widgets := UsedWidgets(app)
	out := make([]string, 0, len(widgets))
	for _, w := range widgets {
		if c := ForWidget(w); c != nil {
			out = append(out, c.Stem)
		}
	}
	sort.Strings(out)
	return dedupeSorted(out)
}

// MergeStems unions two sorted stem lists.
func MergeStems(a, b []string) []string {
	seen := map[string]bool{}
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		seen[s] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	j := 0
	for i := 1; i < len(out); i++ {
		if out[i] != out[j] {
			j++
			out[j] = out[i]
		}
	}
	return out[:j+1]
}
