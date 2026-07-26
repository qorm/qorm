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
		if app.Window.HideLog {
			win["hideLog"] = true
		}
		if app.Window.HideTray {
			win["hideTray"] = true
		}
		desktop["window"] = win
	}
	if len(desktop) > 0 {
		m["platforms"] = map[string]any{"desktop": desktop}
	}
	if len(app.Components) > 0 {
		comps := map[string]any{}
		for name, node := range app.Components {
			comps[name] = componentToJSON(node, app.ComponentSchemas[name])
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

// componentToJSON serialises one component definition — the inverse of
// defineComponent. A component that declared nothing is written back as its
// bare template (the legacy spelling, so an app that never used declarations
// round-trips byte-identically); one that declared props or slots is written in
// the declaration form, with every prop in its canonical long form.
func componentToJSON(tmpl *model.Node, sc *model.ComponentSchema) map[string]any {
	node := NodeToJSON(tmpl)
	if sc == nil {
		return node
	}
	def := map[string]any{"template": node}
	if sc.Props != nil {
		props := map[string]any{}
		for name, spec := range sc.Props {
			p := map[string]any{}
			putIf(p, "type", spec.Type)
			if spec.Default != nil {
				p["default"] = spec.Default
			}
			if spec.Required {
				p["required"] = true
			}
			props[name] = p
		}
		def["props"] = props
	}
	if sc.Slots != nil {
		slots := map[string]any{}
		for name, spec := range sc.Slots {
			s := map[string]any{}
			if spec.Required {
				s["required"] = true
			}
			slots[name] = s
		}
		def["slots"] = slots
	}
	return def
}

// ActionToJSON serialises an action document.
func ActionToJSON(a *model.Action) map[string]any {
	return map[string]any{"type": "action", "id": a.ID, "steps": stepsToJSON(a.Steps)}
}

// stepsToJSON serialises a step list (recursively — `if` then/else and http
// onSuccess/onError nest step lists) — the inverse of buildSteps.
func stepsToJSON(steps []model.Step) []any {
	out := make([]any, 0, len(steps))
	for _, st := range steps {
		out = append(out, stepToJSON(st))
	}
	return out
}

func stepToJSON(st model.Step) map[string]any {
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
	// Only emitted when set: false is the default, so a round trip must not
	// grow an "async": false onto every http step that never asked for one.
	if st.Async {
		s["async"] = true
	}
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
	// `if` step branches + condition.
	putIf(s, "condition", st.Condition)
	if len(st.Then) > 0 {
		s["then"] = stepsToJSON(st.Then)
	}
	if len(st.Else) > 0 {
		s["else"] = stepsToJSON(st.Else)
	}
	// invoke step target + args.
	putIf(s, "name", st.Name)
	if len(st.Args) > 0 {
		s["args"] = copyStrMap(st.Args)
	}
	// http result branches.
	if len(st.OnSuccess) > 0 {
		s["onSuccess"] = stepsToJSON(st.OnSuccess)
	}
	if len(st.OnError) > 0 {
		s["onError"] = stepsToJSON(st.OnError)
	}
	return s
}

// AppToDocs serialises a whole app (manifest + scenes + actions) back to the
// raw document list, the inverse of FromDocs. Components always come back
// nested in the manifest's "components" map — the one canonical output form —
// even when they were authored as standalone type:"component" documents, since
// the two spellings are merged into a single map at load time and nothing in
// the model records which file a component came from.
func AppToDocs(app *model.App) []map[string]any {
	docs := []map[string]any{ManifestToJSON(app)}
	for id, root := range app.Scenes {
		doc := SceneToJSON(id, root)
		// Scene lifecycle: the onEnter hook lives on the scene document, not
		// the node tree — emit it here so it survives the round trip.
		if inv := app.SceneEnter[id]; inv != nil {
			doc["onEnter"] = invokeToJSON(inv)
		}
		docs = append(docs, doc)
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
