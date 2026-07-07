// Package loader reads a QORM application from a directory (manifest + scenes
// + actions), skipping test fixtures and nested projects, and builds a
// model.App from the JSON scene format.
package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorm/qorm/internal/model"
)

// skipDirs are directories that never contain renderable QORM sources.
var skipDirs = map[string]bool{
	"target": true, "qorm_standalone": true, "src": true,
	"assets": true, "node_modules": true, ".git": true,
}

// LoadDir loads an app from a directory.
func LoadDir(dir string) (*model.App, error) {
	docs, err := CollectDocs(dir)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no QORM source documents found under %s", dir)
	}
	app := FromDocs(docs)
	loadLocales(dir, app)
	app.BaseDir = dir
	return app, nil
}

// loadLocales reads <dir>/locales/<lang>.json message catalogs into the app.
func loadLocales(dir string, app *model.App) {
	if locales := LoadLocales(dir); locales != nil {
		app.Locales = locales
	}
}

// LoadLocales reads <dir>/locales/<lang>.json into a lang -> key -> string map
// (nil if there is no locales directory).
func LoadLocales(dir string) map[string]map[string]string {
	entries, err := os.ReadDir(filepath.Join(dir, "locales"))
	if err != nil {
		return nil
	}
	out := map[string]map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "locales", e.Name()))
		if err != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		cat := make(map[string]string, len(raw))
		for k, v := range raw {
			cat[k] = asString(v)
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = cat
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CollectDocs returns the raw (parsed) QORM source documents under dir,
// skipping test fixtures and nested projects. Used by both the loader and the
// bundle builder.
func CollectDocs(dir string) ([]map[string]any, error) {
	return collect(dir)
}

// FromDocs assembles a model.App from a set of raw source documents.
func FromDocs(docs []map[string]any) *model.App {
	app := &model.App{
		Scenes:  map[string]*model.Node{},
		Actions: map[string]*model.Action{},
	}
	for _, doc := range docs {
		switch asString(doc["type"]) {
		case "app":
			applyManifest(app, doc)
		case "scene":
			if root, ok := doc["root"].(map[string]any); ok {
				app.Scenes[asString(doc["id"])] = buildNode(root)
			}
		case "action":
			act := buildAction(doc)
			if act.ID != "" {
				app.Actions[act.ID] = act
			}
		}
	}
	if app.Entry == "" {
		app.Entry = "main"
	}
	return app
}

// LoadFile loads a single scene file (no app-level state binding).
func LoadFile(path string) (*model.App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	app := &model.App{Scenes: map[string]*model.Node{}, Actions: map[string]*model.Action{}, Entry: "main"}
	if asString(doc["type"]) == "scene" {
		if root, ok := doc["root"].(map[string]any); ok {
			app.Scenes[asString(doc["id"])] = buildNode(root)
			app.Entry = asString(doc["id"])
		}
	}
	return app, nil
}

func collect(dir string) ([]map[string]any, error) {
	var out []map[string]any
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			return nil // ignore malformed / non-object json
		}
		if asString(doc["type"]) == "test" {
			return nil
		}
		out = append(out, doc)
		return nil
	})
	return out, err
}

// parseMenuItems reads a JSON array of menu items.
func parseMenuItems(raw any) []model.MenuItem {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []model.MenuItem
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, model.MenuItem{
			ID: asString(m["id"]), Title: asString(m["title"]), Icon: asString(m["icon"]),
			Shortcut: asString(m["shortcut"]), Role: asString(m["role"]), Separator: asBool(m["separator"]), Items: parseMenuItems(m["items"]),
		})
	}
	return out
}

// parseMenuGroups reads the menu-bar groups.
func parseMenuGroups(raw any) []model.MenuGroup {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []model.MenuGroup
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, model.MenuGroup{Title: asString(m["title"]), Items: parseMenuItems(m["items"])})
	}
	return out
}

func applyManifest(app *model.App, doc map[string]any) {
	app.ID = asString(doc["id"])
	app.Name = asString(doc["name"])
	app.Entry = asString(doc["entry"])
	app.DefaultLocale = asString(doc["defaultLocale"])
	app.Theme = asString(doc["theme"])
	app.Branding = true // default on; qorm.json "branding":false removes the metadata note
	if v, ok := doc["branding"]; ok {
		app.Branding = asBool(v)
	}
	if gs, ok := doc["globalState"].(map[string]any); ok {
		app.GlobalState.Schema = map[string]string{}
		if sch, ok := gs["schema"].(map[string]any); ok {
			for k, v := range sch {
				app.GlobalState.Schema[k] = asString(v)
			}
		}
		if init, ok := gs["initial"].(map[string]any); ok {
			app.GlobalState.Initial = init
		}
	}
	if ws, ok := doc["widgets"].([]any); ok {
		for _, it := range ws {
			if m, ok := it.(map[string]any); ok {
				w := model.Widget{ID: asString(m["id"]), Name: asString(m["name"]), Title: asString(m["title"])}
				if ls, ok := m["lines"].([]any); ok {
					for _, it := range ls {
						if lm, ok := it.(map[string]any); ok {
							w.Lines = append(w.Lines, model.WidgetLine{Label: asString(lm["label"]), Value: asString(lm["value"])})
						}
					}
				}
				app.Widgets = append(app.Widgets, w)
			}
		}
	}
	if comps, ok := doc["components"].(map[string]any); ok {
		app.Components = map[string]*model.Node{}
		for name, def := range comps {
			if m, ok := def.(map[string]any); ok {
				app.Components[name] = buildNode(m)
			}
		}
	}
	if scs, ok := doc["shortcuts"].([]any); ok {
		for _, it := range scs {
			if m, ok := it.(map[string]any); ok {
				app.Shortcuts = append(app.Shortcuts, model.Shortcut{
					ID:       asString(m["id"]),
					Title:    asString(m["title"]),
					Subtitle: asString(m["subtitle"]),
					Icon:     asString(m["icon"]),
				})
			}
		}
	}
	if plats, ok := doc["platforms"].(map[string]any); ok {
		if desk, ok := plats["desktop"].(map[string]any); ok {
			app.DesktopMenu = parseMenuGroups(desk["menu"])
			if tr, ok := desk["tray"].(map[string]any); ok {
				app.Tray = model.TrayConfig{Icon: asString(tr["icon"]), Tip: asString(tr["tip"]), Items: parseMenuItems(tr["items"])}
			}
			if w, ok := desk["window"].(map[string]any); ok {
				app.Window = model.Window{
					Width:       int(asFloat(w["width"])),
					Height:      int(asFloat(w["height"])),
					Title:       asString(w["title"]),
					Resizable:   asBool(w["resizable"]),
					Chromeless:  asBool(w["chromeless"]),
					Transparent: asBool(w["transparent"]),
				}
			}
		}
	}
}

// BuildNode builds a node tree from a raw JSON object (exported for patch ops).
func BuildNode(m map[string]any) *model.Node { return buildNode(m) }

func buildNode(m map[string]any) *model.Node {
	n := &model.Node{
		Type:        asString(m["type"]),
		ID:          asString(m["id"]),
		Text:        asString(m["text"]),
		Label:       asString(m["label"]),
		Placeholder: asString(m["placeholder"]),
		Value:       asString(m["value"]),
		Props:       m,
	}
	if s, ok := m["style"].(map[string]any); ok {
		n.Style = s
	}
	if l, ok := m["layout"].(map[string]any); ok {
		n.Layout = l
	}
	n.OnPress = parseInvoke(m["onPress"])
	n.OnChange = parseInvoke(m["onChange"])
	if ri, ok := m["renderItem"].(map[string]any); ok {
		n.Template = buildNode(ri)
	}
	n.Data = asString(m["data"])
	if kids, ok := m["children"].([]any); ok {
		for _, k := range kids {
			if km, ok := k.(map[string]any); ok {
				n.Children = append(n.Children, buildNode(km))
			}
		}
	}
	return n
}

func parseInvoke(v any) *model.Invoke {
	// String shorthand: "onPress": "increment" invokes that action with no args.
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		return &model.Invoke{Name: s, Args: map[string]string{}}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	inv := &model.Invoke{Name: asString(m["name"]), Args: map[string]string{}}
	if args, ok := m["args"].(map[string]any); ok {
		for k, v := range args {
			inv.Args[k] = asString(v)
		}
	}
	return inv
}

func buildAction(doc map[string]any) *model.Action {
	act := &model.Action{ID: asString(doc["id"])}
	if steps, ok := doc["steps"].([]any); ok {
		for _, s := range steps {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			step := model.Step{
				Type:     asString(sm["type"]),
				Path:     asString(sm["path"]),
				Value:    asString(sm["value"]),
				MatchKey: asString(sm["matchKey"]),
				Match:    asString(sm["match"]),
				Field:    asString(sm["field"]),
				URL:      asString(sm["url"]),
				Method:   asString(sm["method"]),
				Body:     asString(sm["body"]),
				Result:   asString(sm["result"]),
				Error:    asString(sm["error"]),
				To:       asString(sm["to"]),
				Back:     sm["back"] == true,
				From:     asString(sm["from"]),
			}
			if item, ok := sm["item"].(map[string]any); ok {
				step.Object = map[string]string{}
				for k, v := range item {
					step.Object[k] = asString(v)
				}
			}
			if hdr, ok := sm["headers"].(map[string]any); ok {
				step.Headers = map[string]string{}
				for k, v := range hdr {
					step.Headers[k] = asString(v)
				}
			}
			act.Steps = append(act.Steps, step)
		}
	}
	return act
}

// ---- coercion helpers ----

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return formatNumber(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func asFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
