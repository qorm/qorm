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

// whenKeys are the fields of a "when" node held in struct fields
// (Condition/Then/Else); on a when node they are emitted from the struct, not
// carried through Props verbatim.
var whenKeys = map[string]bool{"condition": true, "then": true, "else": true}

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
		if typedKeys[k] || (n.Type == "when" && whenKeys[k]) {
			continue
		}
		m[k] = v
	}
	if n.Type == "when" {
		putIf(m, "condition", n.Condition)
		if n.Then != nil {
			m["then"] = NodeToJSON(n.Then)
		}
		if n.Else != nil {
			m["else"] = NodeToJSON(n.Else)
		}
	}
	if len(n.Children) > 0 {
		kids := make([]any, len(n.Children))
		for i, c := range n.Children {
			kids[i] = NodeToJSON(c)
		}
		m["children"] = kids
	}
	// `data` is a typed field (it is in typedKeys, so the extra-props loop
	// above never carries it through) and is valid with or without a
	// renderItem template — emit it whenever it is set.
	putIf(m, "data", n.Data)
	if n.Template != nil {
		m["renderItem"] = NodeToJSON(n.Template)
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
	putIf(m, "theme", app.Theme)
	// The loader defaults branding to true when the key is absent, so only
	// an explicit false must be written out for the round trip to preserve
	// the white-label opt-out.
	if !app.Branding {
		m["branding"] = false
	}
	putIf(m, "pluginABI", app.PluginABI)
	gs := map[string]any{}
	if len(app.GlobalState.Schema) > 0 {
		gs["schema"] = copyStrMap(app.GlobalState.Schema)
	}
	if app.GlobalState.Initial != nil {
		gs["initial"] = app.GlobalState.Initial
	}
	if len(gs) > 0 {
		m["globalState"] = gs
	}
	if len(app.DesignTokens) > 0 {
		toks := map[string]any{}
		for name, dt := range app.DesignTokens {
			tok := map[string]any{}
			putIf(tok, "type", dt.Type)
			putIf(tok, "value", dt.Value)
			if dt.Enforce {
				tok["enforce"] = true
			}
			toks[name] = tok
		}
		m["designTokens"] = toks
	}
	if len(app.Widgets) > 0 {
		ws := make([]any, 0, len(app.Widgets))
		for _, w := range app.Widgets {
			item := map[string]any{}
			putIf(item, "id", w.ID)
			putIf(item, "name", w.Name)
			putIf(item, "title", w.Title)
			if len(w.Lines) > 0 {
				lines := make([]any, 0, len(w.Lines))
				for _, ln := range w.Lines {
					lines = append(lines, map[string]any{"label": ln.Label, "value": ln.Value})
				}
				item["lines"] = lines
			}
			ws = append(ws, item)
		}
		m["widgets"] = ws
	}
	desktop := map[string]any{}
	if len(app.DesktopMenu) > 0 {
		desktop["menu"] = menuGroupsToJSON(app.DesktopMenu)
	}
	if app.Tray.Icon != "" || app.Tray.Tip != "" || len(app.Tray.Items) > 0 {
		tray := map[string]any{}
		putIf(tray, "icon", app.Tray.Icon)
		putIf(tray, "tip", app.Tray.Tip)
		if len(app.Tray.Items) > 0 {
			tray["items"] = menuItemsToJSON(app.Tray.Items)
		}
		desktop["tray"] = tray
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
		desktop["window"] = win
	}
	if len(desktop) > 0 {
		m["platforms"] = map[string]any{"desktop": desktop}
	}
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
		// navigate (and state.move) targeting fields: without these a
		// re-serialised navigate step loses its target scene.
		putIf(s, "to", st.To)
		if st.Back {
			s["back"] = true
		}
		putIf(s, "from", st.From)
		if len(st.Params) > 0 {
			s["params"] = copyStrMap(st.Params)
		}
		if st.Object != nil {
			s["item"] = copyStrMap(st.Object)
		}
		if st.Headers != nil {
			s["headers"] = copyStrMap(st.Headers)
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
	return map[string]any{"type": "invoke", "name": inv.Name, "args": copyStrMap(inv.Args)}
}

// menuGroupsToJSON serialises menu-bar groups — the inverse of
// parseMenuGroups.
func menuGroupsToJSON(groups []model.MenuGroup) []any {
	out := make([]any, 0, len(groups))
	for _, g := range groups {
		item := map[string]any{"title": g.Title}
		if len(g.Items) > 0 {
			item["items"] = menuItemsToJSON(g.Items)
		}
		out = append(out, item)
	}
	return out
}

// menuItemsToJSON serialises menu items — the inverse of parseMenuItems.
func menuItemsToJSON(items []model.MenuItem) []any {
	out := make([]any, 0, len(items))
	for _, it := range items {
		item := map[string]any{}
		putIf(item, "id", it.ID)
		putIf(item, "title", it.Title)
		putIf(item, "icon", it.Icon)
		putIf(item, "shortcut", it.Shortcut)
		putIf(item, "role", it.Role)
		if it.Separator {
			item["separator"] = true
		}
		if len(it.Items) > 0 {
			item["items"] = menuItemsToJSON(it.Items)
		}
		out = append(out, item)
	}
	return out
}

// copyStrMap copies a map[string]string into a fresh map[string]any, so the
// emitted document shares no nested map with the live model.
func copyStrMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func putIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}
