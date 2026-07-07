package loader

import "github.com/qorm/qorm/internal/model"

// typedKeys are node fields represented by struct fields; everything else in a
// node's Props is an extra (options, src, min, max, columns, checked, if,
// role, ...) that must be preserved on the way back out.
var typedKeys = map[string]bool{
	"type": true, "id": true, "text": true, "label": true, "placeholder": true,
	"value": true, "style": true, "layout": true, "onPress": true, "onChange": true,
	"children": true, "renderItem": true, "data": true,
}

// NodeToJSON serialises a node back to a QORM JSON object — the inverse of
// BuildNode. Typed fields come from the struct (so patch edits are reflected);
// unrecognised props are carried through verbatim.
func NodeToJSON(n *model.Node) map[string]any {
	if n == nil {
		return nil
	}
	m := map[string]any{"type": n.Type}
	putIf(m, "id", n.ID)
	putIf(m, "text", n.Text)
	putIf(m, "label", n.Label)
	putIf(m, "placeholder", n.Placeholder)
	putIf(m, "value", n.Value)
	if n.Style != nil {
		m["style"] = n.Style
	}
	if n.Layout != nil {
		m["layout"] = n.Layout
	}
	if n.OnPress != nil {
		m["onPress"] = invokeToJSON(n.OnPress)
	}
	if n.OnChange != nil {
		m["onChange"] = invokeToJSON(n.OnChange)
	}
	// carry through extra props not covered by typed fields
	for k, v := range n.Props {
		if !typedKeys[k] {
			m[k] = v
		}
	}
	if len(n.Children) > 0 {
		kids := make([]any, len(n.Children))
		for i, c := range n.Children {
			kids[i] = NodeToJSON(c)
		}
		m["children"] = kids
	}
	if n.Template != nil {
		m["renderItem"] = NodeToJSON(n.Template)
		putIf(m, "data", n.Data)
	}
	return m
}

// SceneToJSON serialises a scene root as a full scene document.
func SceneToJSON(id string, root *model.Node) map[string]any {
	return map[string]any{"type": "scene", "id": id, "root": NodeToJSON(root)}
}

// ManifestToJSON rebuilds an app manifest document from the model.
func ManifestToJSON(app *model.App) map[string]any {
	m := map[string]any{"type": "app"}
	putIf(m, "id", app.ID)
	putIf(m, "name", app.Name)
	putIf(m, "entry", app.Entry)
	putIf(m, "defaultLocale", app.DefaultLocale)
	gs := map[string]any{}
	if len(app.GlobalState.Schema) > 0 {
		schema := map[string]any{}
		for k, v := range app.GlobalState.Schema {
			schema[k] = v
		}
		gs["schema"] = schema
	}
	if app.GlobalState.Initial != nil {
		gs["initial"] = app.GlobalState.Initial
	}
	if len(gs) > 0 {
		m["globalState"] = gs
	}
	if app.Window != (model.Window{}) {
		win := map[string]any{}
		if app.Window.Width != 0 {
			win["width"] = app.Window.Width
		}
		if app.Window.Height != 0 {
			win["height"] = app.Window.Height
		}
		putIf(win, "title", app.Window.Title)
		if app.Window.Chromeless {
			win["chromeless"] = true
		}
		if app.Window.Transparent {
			win["transparent"] = true
		}
		if app.Window.Resizable {
			win["resizable"] = true
		}
		m["platforms"] = map[string]any{"desktop": map[string]any{"window": win}}
	}
	putIf(m, "theme", app.Theme)
	if len(app.Components) > 0 {
		comps := map[string]any{}
		for name, node := range app.Components {
			comps[name] = NodeToJSON(node)
		}
		m["components"] = comps
	}
	if len(app.Shortcuts) > 0 {
		scs := make([]any, 0, len(app.Shortcuts))
		for _, sc := range app.Shortcuts {
			item := map[string]any{"id": sc.ID, "title": sc.Title}
			putIf(item, "subtitle", sc.Subtitle)
			putIf(item, "icon", sc.Icon)
			scs = append(scs, item)
		}
		m["shortcuts"] = scs
	}
	return m
}

// ActionToJSON serialises an action document.
func ActionToJSON(a *model.Action) map[string]any {
	steps := make([]any, 0, len(a.Steps))
	for _, st := range a.Steps {
		s := map[string]any{"type": st.Type}
		putIf(s, "path", st.Path)
		putIf(s, "value", st.Value)
		putIf(s, "matchKey", st.MatchKey)
		putIf(s, "match", st.Match)
		putIf(s, "field", st.Field)
		putIf(s, "url", st.URL)
		putIf(s, "method", st.Method)
		putIf(s, "body", st.Body)
		putIf(s, "result", st.Result)
		putIf(s, "error", st.Error)
		if st.Object != nil {
			obj := map[string]any{}
			for k, v := range st.Object {
				obj[k] = v
			}
			s["item"] = obj
		}
		if st.Headers != nil {
			hdr := map[string]any{}
			for k, v := range st.Headers {
				hdr[k] = v
			}
			s["headers"] = hdr
		}
		steps = append(steps, s)
	}
	return map[string]any{"type": "action", "id": a.ID, "steps": steps}
}

// AppToDocs serialises a whole app (manifest + scenes + actions) back to the
// raw document list, the inverse of FromDocs.
func AppToDocs(app *model.App) []map[string]any {
	docs := []map[string]any{ManifestToJSON(app)}
	for id, root := range app.Scenes {
		docs = append(docs, SceneToJSON(id, root))
	}
	for _, act := range app.Actions {
		docs = append(docs, ActionToJSON(act))
	}
	return docs
}

func invokeToJSON(inv *model.Invoke) map[string]any {
	args := map[string]any{}
	for k, v := range inv.Args {
		args[k] = v
	}
	return map[string]any{"type": "invoke", "name": inv.Name, "args": args}
}

func putIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}
