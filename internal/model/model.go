// Package model defines the in-memory representation of a QORM application:
// the manifest, its scenes (node trees) and actions. It is intentionally
// language-neutral and mirrors the QORM JSON format so the same example apps
// run unchanged on this runtime.
package model

import (
	"sort"
	"strings"
	"sync"
	"unicode"
)

// ComponentRefName normalises a component instance's `ref` to a component name.
// The canonical form is "component://panel" and the shorthand — the form used
// inside a component instance's fields — is plain "panel"
// (planning/spec/json-format-spec.md "Component 文件"). A ref that is empty or
// still carries an unresolved {{binding}} yields "", i.e. "not a static
// component reference".
func ComponentRefName(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "component://")
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "{{") {
		return ""
	}
	return ref
}

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
	// A component is defined either inline in qorm.json ("components": {name:
	// template}) or by its own `type:"component"` document (conventionally
	// components/<name>.json); both forms merge into this one map.
	Components map[string]*Node
	// ComponentSchemas holds the DECLARED contract of the components that opt
	// into one — the props they accept and the named slots they expose. Keyed
	// exactly like Components; a component that declares nothing has no entry
	// here and keeps the historical "anything goes" behavior (every instance
	// key is passed through to prop.*, every slot is optional). Declarations
	// give the loader something to check instances against (missing required
	// props/slots, statically-wrong literal types) and the renderer defaults to
	// fill in for props the instance omits.
	ComponentSchemas map[string]*ComponentSchema
	// ComponentDocs is the set of component names that were authored as their
	// OWN standalone `type:"component"` document (conventionally
	// components/<name>.json) rather than inline in the manifest's "components"
	// map. Keyed exactly like Components; absent name = declared inline.
	//
	// It exists so the serializer can write each component back to the spelling
	// it came from. Without it AppToDocs folded every component into the
	// manifest, and the same app compiled through bundle.Build (which carries
	// the raw source documents) and through bundle.FromApp (which carries the
	// serialised ones) produced two DIFFERENT contentHashes — content
	// addressing that disagreed with itself, so an exported design and a CI
	// build of the same tree could never be matched, deduplicated or pinned.
	ComponentDocs map[string]bool
	// Shortcuts are the app's home-screen / Dock quick actions (long-press the
	// app icon). Selecting one launches the app and fires qormEmit('shortcut', id).
	Shortcuts []Shortcut
	// DesignTokens are the app's design-system tokens (qorm.json "designTokens"),
	// keyed by token name (e.g. "color.primary"). A token with Type "color" and
	// Enforce true constrains the agent's style edits: apply_patch may only set a
	// color style to one of the enforced token values (see internal/mcp
	// enforcement). Non-enforced tokens are advisory. Empty/absent map = no
	// constraint (fully backward compatible).
	DesignTokens map[string]DesignToken
	// SceneEnter maps a scene id to its optional onEnter invoke (scene JSON
	// "onEnter"): the action dispatched once each time navigation enters that
	// scene — including the initial load of the entry scene and a deep link
	// straight into it. Empty/absent map = no scene lifecycle hooks.
	SceneEnter map[string]*Invoke
	// SceneKeys maps a scene id to its key bindings (scene JSON "keys": a
	// key-name → action map — the declarative control scheme for games and
	// keyboard-driven apps, dispatched by the engine without any focus
	// requirement). Empty/absent = no key bindings.
	SceneKeys map[string]map[string]string
	// SceneKeyReleases is the keyup counterpart of SceneKeys (scene JSON
	// "keyReleases"): a key-name → action map dispatched when the same key
	// is RELEASED. Games with "hold to move" controls (platformers,
	// shoot-em-ups, ...) need both — pressing sets a direction flag the
	// physics step reads, releasing clears it; without the release path
	// the app resorts to a one-shot action per press and the motion is
	// grainy. Empty/absent = no keyup bindings (the original v1 contract;
	// apps that don't need it pay nothing).
	SceneKeyReleases map[string]map[string]string
	// Display is the app's intended window size + chrome hints (qorm.json
	// "display": { "width", "height", "resizable", "title", "minWidth",
	// "minHeight" }). Side-scroller games, dashboards, and any app whose
	// layout is NOT fluid need a fixed window — without it, the host
	// browser / OS picks whatever default and the canvas is rendered into
	// a portrait-shaped viewport, ruining the game's aspect ratio. The
	// server seeds the runtime's Viewport from this at startup (so the
	// first render uses the right size before any client reports back) and
	// the desktop host uses it to set the native window's initial frame.
	// Empty/absent = fluid default (the runtime's zero-value Viewport
	// pattern, which evaluates `{{ viewport.width }}` as 0 until the
	// client reports its real size).
	Display DisplaySpec
	// SceneSwipes maps a scene id to its swipe bindings (scene JSON "swipes":
	// a direction → action map, directions "left"/"right"/"up"/"down") — the
	// TOUCH counterpart of SceneKeys: the engine's swipe recognizer dispatches
	// them when a press drags in one dominant direction and releases, so the
	// same game JSON plays with arrow keys on desktop and swipes on a phone.
	// Empty/absent = no swipe bindings.
	SceneSwipes map[string]map[string]string
	// SceneGuards maps a scene id to its optional route guard (scene JSON
	// "guard"): the condition every entry into that scene must satisfy, plus
	// where to send the user when it does not. It runs BEFORE the scene's
	// onEnter and on every entry path — an action's `navigate` step, browser
	// Back/Forward, a deep link straight into the scene, and the initial entry
	// scene — so a protected route cannot be reached by spelling a URL.
	// Empty/absent map = no guards (every scene is public).
	SceneGuards map[string]*SceneGuard
	// Computed are the app's DERIVED values (qorm.json "computed", or
	// "globalState.computed"): a name -> a {{binding}} expression over the rest
	// of the state. They are evaluated ONCE per frame and published read-only
	// under the reserved ComputedNamespace sub-map of state, so a total that
	// twelve nodes bind is computed once instead of twelve times, and lives in
	// one place instead of being copy-pasted into every binding.
	// Empty/absent map = no derived values (and then "computed" is an ordinary
	// state key, exactly as it was before this existed).
	Computed map[string]string
	// PluginABI is the qormext middle-layer contract version the app's native
	// Go code was authored against (qorm.json "pluginABI", e.g. "1"). The loader
	// warns when its major differs from the runtime's qormext.ABIVersion.
	// Empty = the app uses no versioned middle-layer.
	PluginABI string
	// Styles are the app's stylesheet rules (styles/*.qss), flattened in
	// declaration order (file order, then rule order within a file). They are
	// the third leg of the structure/logic/style separation: scenes/*.json
	// carries the structure, actions/*.qs the logic, styles/*.qss the shared
	// style. A rule applies when its selector matches a node — by widget type,
	// by a name in the node's `class` prop, or by node id — and overrides in
	// that order (type < class < id), beneath the node's own inline style.
	Styles []StyleRule
	// Stylesheets are the raw authored styles/*.qss files (id = filename minus
	// the extension), kept so the serializer can write each sheet back to the
	// spelling it came from — the same fixed-point property ComponentDocs gives
	// components (bundle.Build and bundle.FromApp must content-address the same
	// app identically).
	Stylesheets []Stylesheet
	// ScriptLib is the shared qscript function library (actions/lib.qs): the
	// fn definitions merged into EVERY script action's compilation, so games
	// and app logic keep their helpers in one file instead of copy-pasting
	// them into each action. Empty/absent = no library.
	ScriptLib string
	// Diagnostics holds static compilation warnings or syntax errors found by the loader.
	Diagnostics []string

	computedMu     *sync.Mutex // pointer so copying App (e.g. cloneApp in mcp/patch) is vet-clean
	computedOrder  []string
	computedCyclic []string
	computedCached bool
}

// ComponentSchema is one component's declared interface: the props it accepts
// and the named slots it exposes. It mirrors the `props` / `slots` objects of a
// component definition (planning/spec/json-format-spec.md "Component 文件") and
// is entirely optional — a component without one behaves exactly as before.
type ComponentSchema struct {
	// Props maps a prop name to its declaration. Present but empty means the
	// component declared `"props": {}` (an explicitly propless component).
	Props map[string]PropSpec
	// Slots maps a named slot to its declaration.
	Slots map[string]SlotSpec
}

// PropSpec declares one component prop. The JSON shorthand `"title": "string"`
// yields PropSpec{Type: "string"}; the long form
// `{"type":"number","default":0,"required":true}` fills all three fields.
type PropSpec struct {
	// Type is the normalised declared type: "string", "number", "boolean",
	// "array", "object" or "" (unconstrained — the `any` declaration and any
	// unrecognised type name both normalise to "").
	Type string
	// Default is injected into the component's prop.<name> scope when the
	// instance does not supply the prop. nil means "no default".
	Default any
	// Required makes a missing prop a load-time error diagnostic.
	Required bool
}

// SlotSpec declares one named slot of a component.
type SlotSpec struct {
	// Required makes an instance that fills no child with slot:"<name>" a
	// load-time error diagnostic.
	Required bool
}

// StyleRule kinds: the three selector shapes a styles/*.qss rule may carry.
const (
	StyleRuleType  = "type"  // `button { … }` — matches every node of that widget type
	StyleRuleClass = "class" // `.accent { … }` — matches a node whose `class` prop lists the name
	StyleRuleID    = "id"    // `#submit { … }` — matches the node with that id
)

// StyleRule is one parsed styles/*.qss rule: a selector plus the style keys it
// sets. Style values mirror a node's inline "style" object — numbers stay
// numbers, everything else stays a string (a var(--x) reference, a hex color,
// "fill", or a {{binding}} evaluated at render time exactly like an inline
// style value).
type StyleRule struct {
	Kind  string // StyleRuleType | StyleRuleClass | StyleRuleID
	Name  string // type name, class name, or node id the selector matches
	Style map[string]any
}

// Stylesheet is one authored styles/*.qss file: its id (the filename minus
// .qss) and its full source text. The text rides the document the loader and
// the bundle exchange — exactly like a script action's "script" field — so the
// packaged app is byte-for-byte the reviewed app.
type Stylesheet struct {
	ID  string
	QSS string
}

// DesignToken is one entry in an app's design-token system: a named, typed value
// the design system sanctions. Value is always stored as a string (numbers too,
// e.g. "16"). When Type is "color" and Enforce is true, the agent's apply_patch
// may only set color styles to this token's value; enforce:false tokens are
// advisory (surfaced to the agent but not enforced).
type DesignToken struct {
	Type    string `json:"type"`    // "color" | "number" | ...
	Value   string `json:"value"`   // canonical value (string form)
	Enforce bool   `json:"enforce"` // hard-constrain agent edits to this value
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

// SceneGuard is one scene's route guard: the precondition for entering it.
// Condition is a {{...}} expression evaluated in scene context (state.*, t.*,
// viewport.*, route.*, computed.*); when it is truthy the navigation proceeds
// untouched. When it is falsy the navigation is diverted to Redirect (with
// Params as that scene's route params) — or refused outright when Redirect is
// empty, which leaves the runtime on the scene it was already showing.
//
// The redirect target is itself guarded, so guards chain; the runtime caps the
// chain so a pair of guards that redirect to each other cannot spin.
type SceneGuard struct {
	// Condition is the {{...}} expression that must be truthy to enter.
	Condition string
	// Redirect is the scene id to divert to when Condition is falsy ("" =
	// refuse the navigation instead of diverting it).
	Redirect string
	// Params are the redirect target's route parameters: name -> expression,
	// evaluated when the guard fires and read there as {{ route.<name> }}.
	Params map[string]string
}

// ComputedNamespace is the reserved state sub-map the app's computed values are
// published under. A declaration named "total" is read as
// {{ state.computed.total }} in a scene binding and — since an action context
// also exposes every top-level state key bare — as {{ computed.total }} inside
// an action. Nothing may write into it: the loader reports a step that targets
// the namespace and the runtime drops the write.
const ComputedNamespace = "computed"

// IsComputedPath reports whether a dotted state path targets the read-only
// computed namespace (the namespace itself or anything beneath it).
//
// Both spellings count. A step `path` is relative to the state root, so
// `computed.total` is the literal one — but `state.computed.total` is the
// spelling a scene binding uses, and an author who reads
// `{{ state.computed.total }}` in a scene writes exactly that into the next
// action's `path` without a second thought. Taken literally that path is a
// write to a top-level state key NAMED "state", which is never what anyone
// means; refusing it here is what stops a typo from creating that key at all.
func IsComputedPath(path string) bool {
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, StateRoot+".")
	return p == ComputedNamespace || strings.HasPrefix(p, ComputedNamespace+".")
}

// StateRoot is the name of the state root in an evaluation context — the
// `state` of `{{ state.x }}`. It is reserved: a step path may not be written
// through it (paths are already relative to the root) and no state key, action
// arg or list-item alias may displace it in an evaluation context.
const StateRoot = "state"

// ComputedOrder returns the app's computed names in dependency order — every
// name after the computed values it reads — together with the names that can
// never be evaluated because they take part in (or depend on) a dependency
// cycle. Both slices are sorted-deterministic: the evaluation order of two
// independent values is their name order, so a frame never depends on Go's map
// iteration. A nil/computed-less app yields two nil slices.
//
// Callers evaluate `order` front to back and publish nothing for `cyclic` —
// which is what stops `a = b + 1, b = a + 1` from recursing forever. The loader
// reports the same cyclic set as a load-time error.
func (a *App) ComputedOrder() (order, cyclic []string) {
	if a == nil || len(a.Computed) == 0 {
		return nil, nil
	}
	if a.computedMu == nil {
		a.computedMu = &sync.Mutex{}
	}
	a.computedMu.Lock()
	if a.computedCached {
		order, cyclic := a.computedOrder, a.computedCyclic
		a.computedMu.Unlock()
		return order, cyclic
	}
	a.computedMu.Unlock()

	names := make([]string, 0, len(a.Computed))
	for n := range a.Computed {
		names = append(names, n)
	}
	sort.Strings(names)
	deps := make(map[string]map[string]bool, len(names))
	for _, n := range names {
		d := map[string]bool{}
		for _, ref := range computedRefs(a.Computed[n]) {
			if _, ok := a.Computed[ref]; ok {
				d[ref] = true // a reference to a NON-computed name is just state
			}
		}
		deps[n] = d
	}
	// Kahn's algorithm over the sorted name list: whatever is still unresolved
	// when no name can be released is exactly the cyclic set (the cycles
	// themselves plus everything downstream of one, which is equally
	// unevaluatable).
	done := make(map[string]bool, len(names))
	for {
		progress := false
		for _, n := range names {
			if done[n] {
				continue
			}
			ready := true
			for d := range deps[n] {
				if !done[d] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			done[n] = true
			order = append(order, n)
			progress = true
		}
		if !progress {
			break
		}
	}
	for _, n := range names {
		if !done[n] {
			cyclic = append(cyclic, n)
		}
	}

	a.computedMu.Lock()
	a.computedOrder = order
	a.computedCyclic = cyclic
	a.computedCached = true
	a.computedMu.Unlock()

	return order, cyclic
}

// computedRefs returns the computed names an expression references, in every
// spelling the evaluator resolves to the computed namespace: the dotted forms
// `computed.total` / `state.computed.total`, the bracket forms
// `computed['total']` / `state.computed["total"]`, and the same accessors with
// whitespace around them (`computed . total`, `computed ['total']` — the expr
// grammar allows all of these, so the scan must too).
//
// The scan re-lexes with the expr package's exact tokenization rules — dotted
// runs arrive as single identifier tokens, `.name` and `['name']` following a
// token extend the same access path — and then keeps only paths ROOTED at
// `computed` or `state.computed`. (The expr AST itself is not exported, so the
// walk cannot reuse it; mirroring its lexer is what keeps the scan's notion of
// "one reference" identical to the parser's.) A deeper path such as
// `item.computed.x` is app data, not the namespace, and yields no edge.
//
// Two deliberate precision calls, both safe for the recursion guard:
//   - A string literal's contents are scanned recursively: a map/filter/count
//     predicate is a string that evalSub re-parses at runtime, so a reference
//     inside one is a real dependency edge.
//   - A dynamic bracket key (`computed[key]`) yields no edge — the name is
//     unknowable until runtime — and neither does a string used as a plain
//     index key (`rows['computed.x']` reads a field of app data; index keys
//     are never re-parsed). Dropping such spurious edges can only remove a
//     false cycle report, never hide a real one.
func computedRefs(src string) []string {
	var out []string
	toks := refTokens(src)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case 's':
			out = append(out, computedRefs(t.text)...)
		case 'i':
			var path []string
			path, i = refPath(toks, i)
			if name := computedPathName(path); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// refToken is one lexeme of the reference scan: an identifier run (dots
// included, exactly as the expr lexer produces them), a string literal's
// decoded contents, or one of the accessor punctuation marks.
type refToken struct {
	kind rune // 'i' identifier, 's' string, or '.', '[', ']'
	text string
}

// refTokens lexes src the way the expr package's lexer does — identifiers are
// maximal letter/digit/_/$/. runs, number literals swallow their own dots,
// string literals honor backslash escapes — but it never fails: anything the
// real parser would reject simply yields no useful tokens here (reporting a
// syntax error is the loader's job, not the dependency scan's).
func refTokens(src string) []refToken {
	var toks []refToken
	r := []rune(src)
	i := 0
	for i < len(r) {
		c := r[i]
		switch {
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_' || r[j] == '$' || r[j] == '.') {
				j++
			}
			toks = append(toks, refToken{'i', string(r[i:j])})
			i = j
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(r) && unicode.IsDigit(r[i+1])):
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			i = j // a number token is not a reference; skip it
		case c == '\'' || c == '"':
			quote := c
			j := i + 1
			var sb strings.Builder
			for j < len(r) && r[j] != quote {
				if r[j] == '\\' && j+1 < len(r) {
					j++
				}
				sb.WriteRune(r[j])
				j++
			}
			toks = append(toks, refToken{'s', sb.String()})
			i = j + 1 // past the closing quote (or the end, if unterminated)
		case c == '.' || c == '[' || c == ']':
			toks = append(toks, refToken{c, ""})
			i++
		default:
			i++
		}
	}
	return toks
}

// refPath reassembles the postfix accessor chain the parser would build on the
// identifier at toks[i]: every following `.name` or `['name']` / `["name"]`
// extends the same path. It returns the full path and the index of the last
// token consumed. A `[` whose key is not a plain string literal (a dynamic
// key, an expression) ends the path: what follows is not a static name.
func refPath(toks []refToken, i int) ([]string, int) {
	path := strings.Split(toks[i].text, ".")
	for {
		switch {
		case i+2 < len(toks) && toks[i+1].kind == '.' && toks[i+2].kind == 'i':
			path = append(path, strings.Split(toks[i+2].text, ".")...)
			i += 2
		case i+3 < len(toks) && toks[i+1].kind == '[' && toks[i+2].kind == 's' && toks[i+3].kind == ']':
			path = append(path, toks[i+2].text)
			i += 3
		default:
			return path, i
		}
	}
}

// computedPathName returns the computed value a rooted access path names:
// "total" for `computed.total` and `state.computed.total` alike. A path not
// rooted at the namespace names no computed value, and the bare namespace
// itself names none either.
func computedPathName(path []string) string {
	if path[0] == ComputedNamespace && len(path) > 1 {
		return path[1]
	}
	if path[0] == StateRoot && len(path) > 2 && path[1] == ComputedNamespace {
		return path[2]
	}
	return ""
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

// DisplaySpec is the app's intended window geometry + chrome hints, parsed
// from qorm.json "display": { "width", "height", "resizable", "title",
// "minWidth", "minHeight" }. Side-scrollers and dashboards whose layout is
// NOT fluid declare a fixed window so the host doesn't render into a
// portrait viewport that ruins the aspect ratio. The server seeds the
// runtime's Viewport from this at startup and the desktop host uses it to
// size the native window. Zero-value = fluid default.
type DisplaySpec struct {
	Width, Height int
	Title         string
	Resizable     bool
	MinWidth      int
	MinHeight     int
}

// Node is a single UI element in a scene tree.
type Node struct {
	Type         string
	ID           string
	Text         string         // text nodes
	Label        string         // button label
	Placeholder  string         // input placeholder
	Value        string         // input/bound value (may contain {{...}})
	Style        map[string]any // visual style
	Layout       map[string]any // layout hints (width/height/align/justify)
	Props        map[string]any // catch-all (src, min, max, checked, ...)
	OnPress      *Invoke        // press handler
	OnCollide    *Invoke        // collision handler
	OnChange     *Invoke        // change handler (inputs, selects, toggles, sliders)
	OnKeyDown    *Invoke        // keyboard key down
	OnKeyUp      *Invoke        // keyboard key up
	OnHoverIn    *Invoke        // mouse hover enter
	OnHoverOut   *Invoke        // mouse hover leave
	OnTouchStart *Invoke        // touch start (or mouse down)
	OnTouchMove  *Invoke        // touch move (or mouse drag)
	OnTouchEnd   *Invoke        // touch end (or mouse up)
	Children     []*Node
	Template     *Node  // list renderItem template
	Data         string // list data binding expression
	// "when" nodes (responsive conditional rendering): Condition is a {{...}}
	// expression — typically over viewport.width / viewport.height /
	// viewport.orientation — that selects the Then subtree when truthy and the
	// Else subtree otherwise. This complements the `if`/`visible`/`show` prop
	// (which shows or hides ONE node in place): `when` swaps between two
	// ALTERNATIVE subtrees, e.g. a row layout on wide viewports and a column
	// on narrow ones. Only nodes with Type "when" carry these fields.
	Condition string
	Then      *Node
	Else      *Node
}

// Prop returns a props value; the second result reports whether the key was
// present. Nil-safe: a nil node has no props.
func (n *Node) Prop(key string) (any, bool) {
	if n == nil {
		return nil, false
	}
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

// Action is a named sequence of steps that mutate state — or, when Script is
// set, a qscript program that does.
type Action struct {
	ID    string
	Steps []Step
	// Script is qscript source (action JSON "script"). When non-empty the
	// runtime executes it INSTEAD of Steps: JSON keeps declaring the scenes
	// and the data, the script carries the logic. The loader compiles it at
	// load time (parse errors surface as diagnostics with line numbers) and
	// warns when Script and Steps are both declared; the script always wins.
	Script string
}

// Step is one action operation. Type is e.g. "state.set", "state.append",
// "state.toggle". Value is an expression string evaluated at dispatch time.
type Step struct {
	Type  string
	Path  string
	Value string
	// Index is used by state.setAt: the element position to write, as an
	// {{expression}} evaluated at dispatch time (board-cell writes in games).
	Index string
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
	// Params are the navigate step's route parameters: parameter name -> value
	// expression. Each value is evaluated at dispatch time and attached to the
	// target scene's stack frame, where it is read as `{{ route.<name> }}`.
	Params map[string]string
	// http.* steps: call a backend and store the parsed JSON response.
	URL     string            // request URL (may contain {{bindings}})
	Method  string            // override for http.request (else GET/POST by type)
	Body    string            // request body (may contain {{bindings}})
	Headers map[string]string // request headers (values may contain {{bindings}})
	Result  string            // state path to store the parsed response (defaults to Path)
	Error   string            // state path to store an error message, if any
	// Async requests that an http.* step run in the BACKGROUND: the dispatch
	// hands the round trip to the host's background sink and returns at once,
	// so the frame published at the dispatch boundary already shows the loading
	// state and the session stays responsive for the whole request. Defaults to
	// false — the request then blocks the dispatch, which is the original (and
	// still the only) behavior for a step that reads the response from its
	// sibling steps rather than from OnSuccess. It also degrades to false on a
	// host that installed no background sink (a bare runtime, an offline
	// render, an MCP simulation), so the same JSON stays portable.
	Async bool
	// Key names a request SLOT for an http.* step: at most one request per key
	// is ever in flight, and starting a new one supersedes whichever request
	// was still open on that key — the older request's transport is cancelled
	// and, decisively, its continuation is DROPPED (no Result/Error write, no
	// OnSuccess/OnError). This is what makes a search-as-you-type box correct:
	// the reply that lands on screen is the reply to the LAST keystroke, not
	// whichever round trip happened to finish last.
	//
	// Only the async path can be superseded — a synchronous request blocks its
	// own dispatch, so there is never a second one to supersede it — but the
	// field is harmless there and a single-threaded host (AsyncAll) makes every
	// http step async, so the same JSON keeps its meaning. Empty (the default)
	// opts out entirely: unkeyed requests never cancel each other.
	Key string
	// TimeoutMS overrides the shared client's 20s ceiling for THIS request,
	// in milliseconds. It applies to both execution modes. Expiry is an
	// ordinary failure: the Error path is written and OnError runs, with the
	// message "request timed out after <n>ms" — a stable, host-independent
	// string rather than Go's transport wording, so an app may show it (or
	// match on it) directly. Zero (the default) keeps the client ceiling.
	TimeoutMS int
	// Pending is a state path held true for exactly as long as this request is
	// open: set on launch, cleared when the outcome settles — INCLUDING the
	// failure, timeout and cap-rejection paths, which is what a hand-written
	// pair of state.set steps reliably forgets. It replaces the loading flag,
	// not the loading UI: bind a spinner to `{{ state.<path> }}` as usual.
	//
	// The path is REFERENCE-COUNTED across requests: two open requests sharing
	// one path hold it true until both have settled, and a request superseded
	// by Key releases its own reference without clearing a flag its successor
	// is still holding.
	Pending string
	// DelayMS is the `delay` step's wait, in milliseconds. The steps FOLLOWING
	// a delay in the same list run when it expires — the step suspends the rest
	// of its list, it does not merely sleep — so `render` / `delay` / `render`
	// paces an animation or a staged reveal declaratively.
	//
	// It never blocks: the wait is handed to the host's background sink, the
	// same one an async http step uses. On a host that installed no sink (a
	// bare runtime, an offline render, an MCP simulation) the pause degrades to
	// nothing and the remaining steps run immediately, so the action still
	// reaches the same final state — the sole difference is that nobody waited.
	DelayMS int
	// OnSuccess / OnError are the http.* steps' optional result branches. They
	// run once the request returns: with the default synchronous step, inline
	// on the dispatching goroutine before the http step's sibling steps, so the
	// dispatch is run-to-completion and the state it produces is readable the
	// moment Dispatch returns; with Async, on the host's background sink after
	// the dispatch has already ended, holding the host's lock, with the
	// dispatch-time args frozen but state read live (see Async).
	// OnSuccess steps see the decoded response as `{{ response }}` (in addition
	// to the Result state path, which is written first); OnError steps see the
	// failure message as `{{ error }}` (the Error state path is still written
	// first, preserving the classic error-path behavior). Either branch may
	// contain a `render` step, which publishes an intermediate frame at that
	// point without suspending the dispatch.
	OnSuccess []Step
	OnError   []Step
	// `if` step: Condition is a {{...}} expression; the Then steps run when it
	// is truthy and the Else steps otherwise. Branches may nest (guarded by a
	// depth limit at both load and dispatch time).
	Condition string
	Then      []Step
	Else      []Step
	// invoke step: Name is the target action id and Args its argument
	// expressions — evaluated in the caller's context and merged into the
	// callee's scope exactly like an event invoke's args. Call depth is
	// guarded at dispatch time, so mutual recursion cannot hang the runtime.
	Name string
	Args map[string]string
	// `forEach` step: In is a {{...}} expression yielding the array to iterate
	// and Steps is the loop body, run once per element with the element bound
	// under the As alias (default "item") plus the derived index/first/last
	// keys — the same scope shape a list's renderItem template gets, so the
	// alias style is one thing to learn rather than two. A non-array `in`
	// iterates zero times; the iteration count is capped at dispatch time and
	// the body nests under the same depth guard as `if`.
	In    string
	As    string
	Steps []Step
}
