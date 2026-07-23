// Package runtime holds application state and executes actions. It binds
// {{...}} expressions in scene props and dispatches action steps that mutate
// the global state store.
package runtime

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/expr"
	"github.com/qorm/qorm/internal/model"
)

// Viewport is a client viewport size in CSS pixels. The zero value means
// "unknown" — e.g. the server's first frame before the browser has reported
// its size — in which case viewport.width/height evaluate to 0, so a `when`
// condition like `{{ viewport.width >= 768 }}` is falsy and the `else` branch
// renders.
type Viewport struct{ W, H int }

// Runtime is a live instance of an app: its state plus a reference to the app.
type Runtime struct {
	App   *model.App
	State map[string]any
	// Viewport is the size of the client viewport driving this runtime (pushed
	// by the browser via POST /viewport, or read from the JS globals in the
	// WASM build). Exposed to expressions as viewport.width / viewport.height /
	// viewport.orientation for responsive `when` nodes.
	Viewport Viewport
	// Scene is the id of the scene currently shown ("" = the manifest entry).
	// NavStack holds the frames (scene + its route params) to return to for
	// navigate-back, so popping restores both the previous scene and the route
	// params it was shown with.
	Scene    string
	NavStack []navFrame
	// RouteParams are the parameters the current scene was navigated with,
	// exposed to scene bindings as `route.*` (e.g. `{{ route.userId }}`). It is
	// scene/frame-local — distinct from the cross-scene GlobalState store — and
	// travels with the navigation stack: navigating back restores the previous
	// frame's params. Never nil for a live runtime (empty map at the entry scene).
	RouteParams map[string]any
	// NavDir records the direction of the most recent navigation ("push" / "pop")
	// so the client can play the matching page transition; cleared after it ships.
	NavDir string
}

// navFrame is one entry on the navigation back stack: the scene to return to
// and the route params it was shown with.
type navFrame struct {
	Scene  string
	Params map[string]any
}

// CurrentScene is the scene id to render ("" falls back to the entry scene).
func (r *Runtime) CurrentScene() string { return r.Scene }

// Navigate pushes the current scene (and its route params) onto the back stack
// and shows `to` with the given params. params may be nil (→ an empty route).
// Unknown scenes and no-op navigations are ignored.
func (r *Runtime) Navigate(to string, params map[string]any) {
	if to == "" || to == r.Scene {
		return
	}
	if _, ok := r.App.Scenes[to]; !ok {
		return
	}
	r.NavStack = append(r.NavStack, navFrame{Scene: r.Scene, Params: r.RouteParams})
	r.Scene = to
	if params == nil {
		params = map[string]any{}
	}
	r.RouteParams = params
	r.NavDir = "push"
}

// NavigateBack returns to the previous scene, restoring its route params.
func (r *Runtime) NavigateBack() {
	if len(r.NavStack) == 0 {
		return
	}
	top := r.NavStack[len(r.NavStack)-1]
	r.Scene = top.Scene
	r.RouteParams = top.Params
	if r.RouteParams == nil {
		r.RouteParams = map[string]any{}
	}
	r.NavStack = r.NavStack[:len(r.NavStack)-1]
	r.NavDir = "pop"
}

// TakeNavDir returns and clears the pending navigation direction.
func (r *Runtime) TakeNavDir() string { d := r.NavDir; r.NavDir = ""; return d }

// RoutePath renders the current scene + route params as a deep-link URL path —
// the inverse of NavigateToPath. The entry scene with no params is "/"; any
// other scene is "/?scene=<id>&k=v" with the scene id and every route param
// (values stringified). url.Values.Encode sorts keys, so the path is stable.
func (r *Runtime) RoutePath() string {
	scene := r.Scene
	if scene == r.App.Entry { // the entry scene is addressed as "/", not by id
		scene = ""
	}
	q := url.Values{}
	if scene != "" {
		q.Set("scene", scene)
	}
	for k, v := range r.RouteParams {
		q.Set(k, expr.Stringify(v))
	}
	if len(q) == 0 {
		return "/"
	}
	return "/?" + q.Encode()
}

// NavigateTo drives navigation to an explicit scene id with typed params — the
// engine behind URL routing (deep-link entry and browser back/forward). Unlike
// Navigate (the action-step push), it can return to the entry scene (addressed
// as "" or the entry id) and, when the target is the frame directly below the
// top of the stack, it unwinds via NavigateBack so a browser Back matches the
// in-memory stack (pop transition included). Unknown scenes and a no-op
// navigation (already showing the target) are ignored.
func (r *Runtime) NavigateTo(scene string, params map[string]any) {
	if scene == r.App.Entry {
		scene = ""
	}
	// The entry scene has two spellings ("" and the entry id); treat them as one
	// so a no-op / Back is detected however the current frame recorded it.
	isEntry := func(s string) bool { return s == "" || s == r.App.Entry }
	if scene == r.Scene || (isEntry(scene) && isEntry(r.Scene)) {
		return
	}
	if scene != "" {
		if _, ok := r.App.Scenes[scene]; !ok {
			return
		}
	}
	if n := len(r.NavStack); n > 0 {
		if top := r.NavStack[n-1].Scene; top == scene || (isEntry(top) && isEntry(scene)) {
			r.NavigateBack() // URL points at the previous frame: this is a Back
			return
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	r.NavStack = append(r.NavStack, navFrame{Scene: r.Scene, Params: r.RouteParams})
	r.Scene = scene
	r.RouteParams = params
	r.NavDir = "push"
}

// NavigateToPath drives navigation from a URL query string (the part after "?"
// produced by RoutePath): `scene` selects the target scene (absent = entry) and
// every other parameter becomes a string route param. It is the inverse of
// RoutePath, used for deep links and browser history sync.
func (r *Runtime) NavigateToPath(rawQuery string) {
	vals, _ := url.ParseQuery(rawQuery)
	params := map[string]any{}
	for k, vs := range vals {
		if k == "scene" || len(vs) == 0 {
			continue
		}
		params[k] = vs[0] // route param values are strings when they come from a URL
	}
	r.NavigateTo(vals.Get("scene"), params)
}

// New creates a runtime with state seeded from the manifest's initial values.
func New(app *model.App) *Runtime {
	state := deepCopyMap(app.GlobalState.Initial)
	if state == nil {
		state = map[string]any{}
	}
	return &Runtime{App: app, State: state, RouteParams: map[string]any{}}
}

// Stringify renders a value as display text (re-exported from expr).
func Stringify(v any) string { return expr.Stringify(v) }

// Clone returns a runtime sharing the same app but with a deep copy of state,
// so simulations can run without touching the live instance. The navigation
// stack and direction are copied too (each frame's params deep-copied), so a
// clone can navigate back exactly like the live runtime without aliasing it.
func (r *Runtime) Clone() *Runtime {
	var stack []navFrame
	if r.NavStack != nil {
		stack = make([]navFrame, len(r.NavStack))
		for i, f := range r.NavStack {
			stack[i] = navFrame{Scene: f.Scene, Params: deepCopyMap(f.Params)}
		}
	}
	return &Runtime{
		App:         r.App,
		State:       deepCopyMap(r.State),
		Viewport:    r.Viewport,
		Scene:       r.Scene,
		NavStack:    stack,
		RouteParams: deepCopyMap(r.RouteParams),
		NavDir:      r.NavDir,
	}
}

// ViewportVars exposes the viewport to expressions: viewport.width,
// viewport.height (CSS px, 0 while unknown) and viewport.orientation
// ("landscape" when W >= H, "portrait" otherwise, "" while unknown).
func (r *Runtime) ViewportVars() map[string]any {
	orientation := ""
	if r.Viewport.W > 0 || r.Viewport.H > 0 {
		if r.Viewport.W >= r.Viewport.H {
			orientation = "landscape"
		} else {
			orientation = "portrait"
		}
	}
	return map[string]any{
		"width":       float64(r.Viewport.W),
		"height":      float64(r.Viewport.H),
		"orientation": orientation,
	}
}

var bindingRe = regexp.MustCompile(`\{\{(.*?)\}\}`)

// sceneCtx is the evaluation context for scene bindings: `state.*`, the
// active-locale message catalog `t.*`, the responsive `viewport.*` vars and the
// current scene's navigation parameters `route.*`.
func (r *Runtime) sceneCtx() map[string]any {
	return map[string]any{"state": r.State, "t": r.Catalog(), "viewport": r.ViewportVars(), "route": r.RouteParams}
}

// CurrentLocale is state.locale, falling back to the app's default locale.
func (r *Runtime) CurrentLocale() string {
	if l, ok := r.State["locale"].(string); ok && l != "" {
		return l
	}
	return r.App.DefaultLocale
}

// CurrentTheme is the active design theme: state.theme, else the manifest
// theme, else "auto" — the default Cupertino look that follows the OS
// light/dark setting. An explicit theme:"auto" means the same.
func (r *Runtime) CurrentTheme() string {
	if t, ok := r.State["theme"].(string); ok && t != "" {
		return t
	}
	if r.App != nil && r.App.Theme != "" {
		return r.App.Theme
	}
	return "auto"
}

// rtlLangs are the base language codes that render right-to-left.
var rtlLangs = map[string]bool{
	"ar": true, "he": true, "fa": true, "ur": true, "ps": true,
	"sd": true, "ug": true, "yi": true, "dv": true, "ckb": true,
}

// IsRTL reports whether a locale (e.g. "ar", "he-IL") is right-to-left.
func IsRTL(locale string) bool {
	base := locale
	if i := strings.IndexAny(locale, "-_"); i > 0 {
		base = locale[:i]
	}
	return rtlLangs[strings.ToLower(base)]
}

// IsRTL reports whether the active locale is right-to-left.
func (r *Runtime) IsRTL() bool { return IsRTL(r.CurrentLocale()) }

// Catalog returns the active message catalog: the default locale overlaid by
// the current locale (missing keys fall back to the default translation), with
// each value expanded via ICU-lite MessageFormat against state — so
// `{{ t.greeting }}` fills `{name}` params and `{n, plural, ...}` from state.
func (r *Runtime) Catalog() map[string]any {
	merged := map[string]string{}
	if def := r.App.DefaultLocale; def != "" {
		for k, v := range r.App.Locales[def] {
			merged[k] = v
		}
	}
	for k, v := range r.App.Locales[r.CurrentLocale()] {
		merged[k] = v
	}
	// message context: bare {key} resolves to state.key; {state.key} also works.
	msgCtx := map[string]any{"state": r.State, "__locale": r.CurrentLocale()}
	for k, v := range r.State {
		msgCtx[k] = v
	}
	out := make(map[string]any, len(merged))
	for k, v := range merged {
		out[k] = fillMessage(v, msgCtx)
	}
	return out
}

// EvalBinding evaluates a possibly-bound string. If the whole string is a
// single {{expr}}, the typed value is returned; if it mixes text and bindings,
// an interpolated string is returned; a plain string is returned as-is.
func EvalBinding(s string, ctx map[string]any) any {
	trimmed := strings.TrimSpace(s)
	if m := bindingRe.FindStringSubmatch(trimmed); m != nil && m[0] == trimmed {
		v, err := expr.Eval(m[1], ctx)
		if err != nil {
			return ""
		}
		return v
	}
	if !strings.Contains(s, "{{") {
		return s
	}
	return bindingRe.ReplaceAllStringFunc(s, func(tok string) string {
		inner := bindingRe.FindStringSubmatch(tok)[1]
		v, err := expr.Eval(inner, ctx)
		if err != nil {
			return ""
		}
		return expr.Stringify(v)
	})
}

// EvalArgs evaluates an invoke's argument expressions in scene context.
func (r *Runtime) EvalArgs(args map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		out[k] = EvalBinding(v, r.sceneCtx())
	}
	return out
}

// BuiltinDismiss is the runtime's reserved built-in action: it sets the state
// path named by its "path" arg to false. The renderer registers it for
// default overlay behaviors (backdrop tap / Escape / an un-wired cancel
// button), so a plainly state-bound `open` closes without the app writing an
// action file. It works identically over the server, WASM and MCP dispatch.
const BuiltinDismiss = "__dismiss"

// BuiltinSort is the reserved built-in action for default table sorting. Args:
// "data" (bound array path), "column" (clicked column key), "field" and "dir"
// (the sortField/sortDir state paths). Clicking the already-sorted column
// flips its direction; a new column sorts ascending. A dispatch without a
// column is a no-op — it never reorders the data or erases a recorded sort.
// It works identically over the server, WASM and MCP dispatch.
const BuiltinSort = "__sort"

// Dispatch runs a named action with the given evaluated args. Missing actions
// are ignored (with no state change) so partially-authored apps still run.
func (r *Runtime) Dispatch(name string, args map[string]any) {
	if name == BuiltinDismiss {
		if p, ok := args["path"].(string); ok && p != "" {
			setPath(r.State, p, false)
		}
		return
	}
	if name == BuiltinSort {
		col := Stringify(args["column"])
		dataPath := Stringify(args["data"])
		fieldPath := Stringify(args["field"])
		dirPath := Stringify(args["dir"])
		// A column is required to sort: without one this is a no-op, so a
		// stray column-less dispatch never reorders the data or clobbers a
		// previously-recorded sort field/direction with an empty column.
		if col == "" {
			return
		}
		dir := "asc"
		if Stringify(getPath(r.State, fieldPath)) == col {
			if Stringify(getPath(r.State, dirPath)) == "asc" {
				dir = "desc"
			}
		} else if fieldPath != "" {
			setPath(r.State, fieldPath, col)
		}
		if dirPath != "" {
			setPath(r.State, dirPath, dir)
		}
		if dataPath != "" {
			sortArray(getPath(r.State, dataPath), col, dir)
		}
		return
	}
	act, ok := r.App.Actions[name]
	if !ok {
		return
	}
	ctx := map[string]any{"state": r.State, "t": r.Catalog(), "viewport": r.ViewportVars()}
	// Expose top-level state keys so a bare `count` in an action expression
	// resolves to state.count (as the message-format context already does);
	// otherwise `{{ count + 1 }}` reads nil and never accumulates.
	for k, v := range r.State {
		ctx[k] = v
	}
	for k, v := range args { // args still win over state
		ctx[k] = v
	}
	for _, step := range act.Steps {
		r.applyStep(step, ctx)
	}
}

// evalParams evaluates a navigate step's route-parameter expressions against
// the action context, returning the typed values to attach to the target
// scene's frame (exposed there as `route.*`). Returns nil when there are none.
func evalParams(params map[string]string, ctx map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, e := range params {
		out[k] = EvalBinding(e, ctx)
	}
	return out
}

func (r *Runtime) applyStep(step model.Step, ctx map[string]any) {
	switch step.Type {
	case "navigate":
		if step.Back {
			r.NavigateBack()
		} else {
			r.Navigate(Stringify(EvalBinding(step.To, ctx)), evalParams(step.Params, ctx))
		}
	case "state.set":
		setPath(r.State, step.Path, EvalBinding(step.Value, ctx))
	case "state.append":
		cur := getPath(r.State, step.Path)
		arr, _ := cur.([]any)
		arr = append(arr, EvalBinding(step.Value, ctx))
		setPath(r.State, step.Path, arr)
	case "state.appendObject":
		cur := getPath(r.State, step.Path)
		arr, _ := cur.([]any)
		obj := map[string]any{}
		for field, expr := range step.Object {
			obj[field] = EvalBinding(expr, ctx)
		}
		arr = append(arr, obj)
		setPath(r.State, step.Path, arr)
	case "state.toggle":
		if arr, ok := getPath(r.State, step.Path).([]any); ok {
			setPath(r.State, step.Path, toggleInArray(arr, step.MatchKey, EvalBinding(step.Match, ctx), step.Field))
		}
	case "state.increment":
		by := 1.0
		if step.Value != "" {
			by = toNum(EvalBinding(step.Value, ctx))
		}
		setPath(r.State, step.Path, toNum(getPath(r.State, step.Path))+by)
	case "state.remove":
		want := expr.Stringify(EvalBinding(step.Match, ctx))
		arr, _ := getPath(r.State, step.Path).([]any)
		out := arr[:0:0]
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok && expr.Stringify(m[step.MatchKey]) == want {
				continue // drop matching element
			}
			out = append(out, it)
		}
		setPath(r.State, step.Path, out)
	case "state.updateWhere":
		want := expr.Stringify(EvalBinding(step.Match, ctx))
		arr, _ := getPath(r.State, step.Path).([]any)
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok || expr.Stringify(m[step.MatchKey]) != want {
				continue
			}
			for field, e := range step.Object {
				m[field] = EvalBinding(e, ctx)
			}
		}
	case "state.merge":
		cur, _ := getPath(r.State, step.Path).(map[string]any)
		if cur == nil {
			cur = map[string]any{}
		}
		for field, e := range step.Object {
			cur[field] = EvalBinding(e, ctx)
		}
		setPath(r.State, step.Path, cur)
	case "state.sort":
		field := step.Field
		if strings.Contains(field, "{{") { // sort key can be dynamic (e.g. clicked column)
			field = expr.Stringify(EvalBinding(field, ctx))
		}
		sortArray(getPath(r.State, step.Path), field, EvalBinding(step.Value, ctx))
	case "state.move":
		if arr, ok := getPath(r.State, step.Path).([]any); ok {
			from := int(toNum(EvalBinding(step.From, ctx)))
			to := int(toNum(EvalBinding(step.To, ctx)))
			setPath(r.State, step.Path, moveElem(arr, from, to))
		}
	case "state.clear":
		// Reset to the value's own type zero so a cleared flag stays a boolean
		// (not the string ""): arrays empty, numbers go to 0, booleans to false,
		// everything else to "".
		switch getPath(r.State, step.Path).(type) {
		case []any:
			setPath(r.State, step.Path, []any{})
		case float64:
			setPath(r.State, step.Path, 0.0)
		case bool:
			setPath(r.State, step.Path, false)
		default:
			setPath(r.State, step.Path, "")
		}
	case "state.reset":
		// Restore the manifest's initial values: with `path` just that one key,
		// otherwise every key declared in globalState (a form reset). Values are
		// deep-copied so state never aliases the manifest.
		if step.Path != "" {
			if v := getPath(r.App.GlobalState.Initial, step.Path); v != nil {
				setPath(r.State, step.Path, deepCopy(v))
			}
		} else {
			for k, v := range r.App.GlobalState.Initial {
				r.State[k] = deepCopy(v)
			}
		}
	case "http.get", "http.post", "http.put", "http.delete", "http.request":
		r.applyHTTP(step, ctx)
	}
}

// httpClient is the shared client for backend calls (overridable in tests).
var httpClient = &http.Client{Timeout: 20 * time.Second}

// applyHTTP calls a backend and stores the parsed JSON response into state.
// The URL, body and header values may contain {{bindings}}. A body that
// resolves to a string is sent verbatim (an inline JSON template is not
// double-encoded); a body that resolves to a non-string value (map/list/number/
// bool) is JSON-encoded so it is valid under the JSON Content-Type. On success (a 2xx
// status) the response (JSON decoded, or raw string if not JSON) is written to
// Result (or Path) and any stale Error is cleared; on any other status the body
// is discarded and the status text is written to Error. Blocks until it returns.
func (r *Runtime) applyHTTP(step model.Step, ctx map[string]any) {
	method := strings.ToUpper(step.Method)
	if method == "" {
		switch step.Type {
		case "http.post":
			method = "POST"
		case "http.put":
			method = "PUT"
		case "http.delete":
			method = "DELETE"
		default:
			method = "GET"
		}
	}
	url := expr.Stringify(EvalBinding(step.URL, ctx))
	resultPath := step.Result
	if resultPath == "" {
		resultPath = step.Path
	}
	fail := func(msg string) {
		if step.Error != "" {
			setPath(r.State, step.Error, msg)
		}
	}
	var body io.Reader
	if step.Body != "" {
		bv := EvalBinding(step.Body, ctx)
		if s, ok := bv.(string); ok {
			// A string body is sent verbatim: the documented pattern is an inline
			// JSON template (e.g. `{"name":"{{ name }}"}`), which must not be
			// double-encoded.
			body = strings.NewReader(s)
		} else if enc, err := json.Marshal(bv); err == nil {
			// A whole structured value bound as the body (a map/list/number/bool)
			// is JSON-encoded so it is valid on the wire under the JSON
			// Content-Type — not Go's %v syntax ("map[a:1]").
			body = strings.NewReader(string(enc))
		} else {
			body = strings.NewReader(expr.Stringify(bv)) // unmarshalable fallback
		}
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		fail(err.Error())
		return
	}
	for k, v := range step.Headers {
		req.Header.Set(k, expr.Stringify(EvalBinding(v, ctx)))
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		fail(err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fail(resp.Status) // non-success: record the status, never the body
		return
	}
	if resultPath != "" {
		var parsed any
		if json.Unmarshal(data, &parsed) == nil {
			setPath(r.State, resultPath, parsed)
		} else {
			setPath(r.State, resultPath, string(data)) // non-JSON body → raw text
		}
	}
	if step.Error != "" {
		setPath(r.State, step.Error, "") // clear stale error on success
	}
}

// sortArray sorts an array of objects in place by key; dir "desc" reverses.
// moveElem returns arr with the element at `from` relocated to index `to`
// (drag-to-reorder). Out-of-range or no-op moves return arr unchanged.
func moveElem(arr []any, from, to int) []any {
	n := len(arr)
	if from < 0 || from >= n || from == to {
		return arr
	}
	v := arr[from]
	rest := make([]any, 0, n-1)
	rest = append(rest, arr[:from]...)
	rest = append(rest, arr[from+1:]...)
	if to < 0 {
		to = 0
	}
	if to > len(rest) {
		to = len(rest)
	}
	out := make([]any, 0, n)
	out = append(out, rest[:to]...)
	out = append(out, v)
	out = append(out, rest[to:]...)
	return out
}

func sortArray(v any, key string, dir any) {
	arr, ok := v.([]any)
	if !ok || key == "" {
		return
	}
	desc := expr.Stringify(dir) == "desc"
	sort.SliceStable(arr, func(i, j int) bool {
		a, b := fieldOf(arr[i], key), fieldOf(arr[j], key)
		if desc {
			// Swap the operands rather than negating: !less returns true for
			// equal keys in both directions (an invalid comparator) and reverses
			// equal runs, breaking the stability SliceStable is chosen for.
			return lessValue(b, a)
		}
		return lessValue(a, b)
	})
}

func fieldOf(v any, key string) any {
	if m, ok := v.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func lessValue(a, b any) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return af < bf
	}
	return expr.Stringify(a) < expr.Stringify(b)
}

// toNum coerces a value to float64.
func toNum(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

// toggleInArray flips a boolean field on the array element whose matchKey
// equals matchVal, returning the array. For an array of scalars (e.g. a
// selection of row keys) it toggles membership instead: matchVal is removed
// when present and appended when absent; an empty match is a no-op there, so
// one action can branch on its args (e.g. a select-all key).
func toggleInArray(arr []any, matchKey string, matchVal any, field string) []any {
	want := expr.Stringify(matchVal)
	scalar := true
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		scalar = false
		if expr.Stringify(m[matchKey]) == want {
			b, _ := m[field].(bool)
			m[field] = !b
			return arr
		}
	}
	if !scalar || want == "" {
		return arr
	}
	for i, it := range arr {
		if expr.Stringify(it) == want {
			return append(arr[:i:i], arr[i+1:]...) // capped append copies — never clobbers the shared tail
		}
	}
	return append(arr, matchVal)
}

// ---- path helpers (dotted) ----

func setPath(root map[string]any, path string, val any) {
	parts := strings.Split(path, ".")
	m := root
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = val
}

func getPath(root map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopy(v)
	}
	return out
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopy(e)
		}
		return out
	default:
		return v
	}
}
