package model

import (
	"encoding/json"
	"testing"
)

func TestEntryRoot(t *testing.T) {
	home := &Node{Type: "scaffold", ID: "home"}
	other := &Node{Type: "scaffold", ID: "other"}

	tests := []struct {
		name string
		app  *App
		want *Node
	}{
		{"nil app", nil, nil},
		{"entry scene found", &App{Entry: "home", Scenes: map[string]*Node{"home": home, "other": other}}, home},
		{"empty scenes", &App{Entry: "home", Scenes: map[string]*Node{}}, nil},
		{"nil scenes", &App{Entry: "home"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.EntryRoot(); got != tt.want {
				t.Errorf("EntryRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryRootFallsBackToAnyScene(t *testing.T) {
	// A missing entry id must not blank the app: any scene root is returned so
	// a manifest-less directory still renders.
	only := &Node{Type: "scaffold"}
	app := &App{Entry: "missing", Scenes: map[string]*Node{"only": only}}
	if got := app.EntryRoot(); got != only {
		t.Errorf("EntryRoot with unknown entry = %v, want fallback %v", got, only)
	}
}

func TestNodeProp(t *testing.T) {
	node := &Node{Props: map[string]any{
		"src":     "a.png",
		"checked": true,
		"empty":   nil, // present but nil: found, not a miss
	}}

	tests := []struct {
		name    string
		node    *Node
		key     string
		wantVal any
		wantOK  bool
	}{
		{"string prop", node, "src", "a.png", true},
		{"bool prop", node, "checked", true, true},
		{"present nil value", node, "empty", nil, true},
		{"missing key", node, "nope", nil, false},
		{"nil props map", &Node{}, "src", nil, false},
		// Style is documented as a fallback but the code does not consult it;
		// pin the actual behavior.
		{"style is not a fallback", &Node{Style: map[string]any{"color": "red"}}, "color", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := tt.node.Prop(tt.key)
			if ok != tt.wantOK {
				t.Fatalf("Prop(%q) ok = %v, want %v", tt.key, ok, tt.wantOK)
			}
			if v != tt.wantVal {
				t.Errorf("Prop(%q) = %v, want %v", tt.key, v, tt.wantVal)
			}
		})
	}
}

func TestJSONFieldTags(t *testing.T) {
	// These structs are the wire contract for qorm.json; the tags are
	// load-bearing, so pin them with a decode of the canonical shapes.
	data := `{
		"token":    {"type": "color", "value": "#fff", "enforce": true},
		"shortcut": {"id": "new", "title": "New", "subtitle": "Create", "icon": "plus"},
		"menu":     {"title": "File", "items": [{"id": "save", "title": "Save", "shortcut": "cmd+s", "items": [{"id": "sub", "title": "Sub"}]}]},
		"tray":     {"icon": "tray", "tip": "tip", "items": [{"id": "show", "title": "Show"}]},
		"widget":   {"id": "w", "name": "W", "title": "T", "lines": [{"label": "L", "value": "V"}]}
	}`
	var doc struct {
		Token    DesignToken `json:"token"`
		Shortcut Shortcut    `json:"shortcut"`
		Menu     MenuGroup   `json:"menu"`
		Tray     TrayConfig  `json:"tray"`
		Widget   Widget      `json:"widget"`
	}
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Token != (DesignToken{Type: "color", Value: "#fff", Enforce: true}) {
		t.Errorf("DesignToken mismatch: %+v", doc.Token)
	}
	if doc.Shortcut != (Shortcut{ID: "new", Title: "New", Subtitle: "Create", Icon: "plus"}) {
		t.Errorf("Shortcut mismatch: %+v", doc.Shortcut)
	}
	if len(doc.Menu.Items) != 1 || doc.Menu.Items[0].Shortcut != "cmd+s" ||
		len(doc.Menu.Items[0].Items) != 1 || doc.Menu.Items[0].Items[0].ID != "sub" {
		t.Errorf("nested MenuItem mismatch: %+v", doc.Menu)
	}
	if doc.Tray.Tip != "tip" || len(doc.Tray.Items) != 1 || doc.Tray.Items[0].ID != "show" {
		t.Errorf("TrayConfig mismatch: %+v", doc.Tray)
	}
	if len(doc.Widget.Lines) != 1 || doc.Widget.Lines[0] != (WidgetLine{Label: "L", Value: "V"}) {
		t.Errorf("Widget lines mismatch: %+v", doc.Widget)
	}
}
