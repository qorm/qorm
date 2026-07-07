// Package model defines the in-memory representation of a QORM application:
// the manifest, its scenes (node trees) and actions. It is intentionally
// language-neutral and mirrors the QORM JSON format so the same example apps
// run unchanged on this runtime.
package model

// App is a fully loaded QORM application.
type App struct {
	ID          string
	Name        string
	Entry       string // entry scene id
	GlobalState GlobalState
	Scenes      map[string]*Node   // scene id -> root node
	Actions     map[string]*Action // action id -> action
	Window      Window
	// i18n: message catalogs by locale (lang -> key -> string) and the default
	// locale. Text resolves via {{ t.key }}; the active locale comes from
	// state.locale (falling back to DefaultLocale).
	Locales       map[string]map[string]string
	DefaultLocale string
	// BaseDir is the directory the app was loaded from (empty for bundles/
	// in-memory apps); used to resolve relative asset paths like image src.
	BaseDir string
	// Theme selects the design token set ("apple" default, "material", …); a
	// state.theme value overrides it at runtime.
	Theme string
	// Branding adds a "Made with QORM" generator note to the packaged app's
	// metadata (default true). Removing it (qorm.json "branding":false or
	// `qorm package --no-branding`) is a commercial white-label feature — see
	// TERMS.md. Default-true is applied by the loader.
	Branding bool
	// DesktopMenu is the macOS menu-bar (system menu); Tray is the menu-bar icon.
	// Both live under platforms.desktop in qorm.json. Selecting an item emits
	// 'menu'/'tray' on the event bus with the item id.
	DesktopMenu []MenuGroup
	Tray        TrayConfig
	// Widgets are home-screen widgets (iOS WidgetKit / Android AppWidget). The app
	// pushes their data at runtime via the updateWidget native op.
	Widgets []Widget
	// Components are app-defined reusable UI components, keyed by type name. A
	// node whose type matches a component name instantiates it: the node's props
	// become {{prop.x}} inside the component, and its children fill any {slot}.
	Components map[string]*Node
	// Shortcuts are the app's home-screen / Dock quick actions (long-press the
	// app icon). Selecting one launches the app and fires qormEmit('shortcut', id).
	Shortcuts []Shortcut
}

// MenuItem is one entry in a system / tray / context menu. Items nests a submenu.
type MenuItem struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Icon      string     `json:"icon,omitempty"`     // SF Symbol name (macOS)
	Shortcut  string     `json:"shortcut,omitempty"` // e.g. "cmd+s"
	Role      string     `json:"role,omitempty"`     // system role: quit/copy/paste/...
	Separator bool       `json:"separator,omitempty"`
	Items     []MenuItem `json:"items,omitempty"` // submenu
}

// MenuGroup is a top-level menu-bar title with its items (e.g. "File" > ...).
type MenuGroup struct {
	Title string     `json:"title"`
	Items []MenuItem `json:"items"`
}

// TrayConfig is the menu-bar tray icon + its menu.
type TrayConfig struct {
	Icon  string     `json:"icon,omitempty"`
	Tip   string     `json:"tip,omitempty"`
	Items []MenuItem `json:"items"`
}

// Widget is a home-screen widget: a title plus label/value lines the app keeps
// updated. Kept deliberately simple since widgets render natively (no webview).
type Widget struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Title string       `json:"title,omitempty"`
	Lines []WidgetLine `json:"lines,omitempty"`
}

// WidgetLine is one baked label/value row shown by a home-screen widget (the
// default content, also what renders when App Groups are unavailable — e.g. a
// free personal signing team, which can't share live data with the extension).
type WidgetLine struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Shortcut is one app-icon quick action (iOS UIApplicationShortcutItem / Android
// app shortcut / macOS Dock menu item).
type Shortcut struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

// EntryRoot returns the root node of the entry scene (or nil).
func (a *App) EntryRoot() *Node {
	if a == nil {
		return nil
	}
	if n, ok := a.Scenes[a.Entry]; ok {
		return n
	}
	// Fall back to any scene so a manifest-less directory still renders.
	for _, n := range a.Scenes {
		return n
	}
	return nil
}

// GlobalState is the app-level state schema and initial values.
type GlobalState struct {
	Schema  map[string]string
	Initial map[string]any
}

// Window describes the desktop window hints from the manifest.
type Window struct {
	Width, Height int
	Title         string
	Resizable     bool
	Chromeless    bool // no title bar / border (widget/overlay style)
	Transparent   bool // transparent background → custom-shape windows
	HideLog       bool // don't spawn the Activity-log window (HUDs default to this)
	HideTray      bool // don't show the menu-bar tray icon
}

// Node is a single UI element in a scene tree.
type Node struct {
	Type        string
	ID          string
	Text        string         // text nodes
	Label       string         // button label
	Placeholder string         // input placeholder
	Value       string         // input/bound value (may contain {{...}})
	Style       map[string]any // visual style
	Layout      map[string]any // layout hints (width/height/align/justify)
	Props       map[string]any // catch-all (src, min, max, checked, ...)
	OnPress     *Invoke        // press handler
	OnChange    *Invoke        // change handler (inputs, selects, toggles, sliders)
	Children    []*Node
	Template    *Node  // list renderItem template
	Data        string // list data binding expression
}

// Prop returns a props value with a fallback lookup into style.
func (n *Node) Prop(key string) (any, bool) {
	if n.Props != nil {
		if v, ok := n.Props[key]; ok {
			return v, true
		}
	}
	return nil, false
}

// Invoke is an action invocation with raw (unevaluated) argument expressions.
type Invoke struct {
	Name string
	Args map[string]string // arg name -> expression string
}

// Action is a named sequence of steps that mutate state.
type Action struct {
	ID    string
	Steps []Step
}

// Step is one action operation. Type is e.g. "state.set", "state.append",
// "state.toggle". Value is an expression string evaluated at dispatch time.
type Step struct {
	Type  string
	Path  string
	Value string
	// Match is used by state.toggle / removal steps to select an array element
	// (e.g. toggle the item whose "id" equals the evaluated Match expression).
	MatchKey string
	Match    string
	Field    string // field to toggle within a matched object
	// Object is used by state.appendObject: field name -> value expression.
	Object map[string]string
	// navigate step: To is the target scene id (may contain {{bindings}}); Back
	// pops the navigation stack instead. state.move also uses To (target index)
	// together with From (source index).
	To   string
	Back bool
	From string
	// http.* steps: call a backend and store the parsed JSON response.
	URL     string            // request URL (may contain {{bindings}})
	Method  string            // override for http.request (else GET/POST by type)
	Body    string            // request body (may contain {{bindings}})
	Headers map[string]string // request headers (values may contain {{bindings}})
	Result  string            // state path to store the parsed response (defaults to Path)
	Error   string            // state path to store an error message, if any
}
