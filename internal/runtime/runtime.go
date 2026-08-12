// Package runtime holds application state and executes actions. It binds
// {{...}} expressions in scene props and dispatches action steps that mutate
// the global state store.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/audio"
	"github.com/qorm/qorm/internal/expr"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/qscript"
	"github.com/qorm/qorm/internal/theme"
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
	Theme *theme.Theme
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
	// pendingEnter marks that navigation just entered the current scene (or the
	// runtime was just created showing the entry scene) and its onEnter — if the
	// scene declares one — has not been dispatched yet. Drained explicitly by
	// RunPendingEnter at each host's chosen point (the server before rendering,
	// the WASM build after init/dispatch), so an SSE reconnect or catch-up
	// re-render can never replay it.
	pendingEnter bool
	// callDepth counts nested Dispatch calls (the `invoke` step, onEnter
	// chains), capped at maxInvokeDepth so action recursion cannot hang.
	callDepth int

	// Commit is the HOST's frame sink: the `render` step calls it to publish an
	// intermediate frame in the middle of an action, so a loading state written
	// by an earlier step actually reaches the screen before a slow step (an
	// http.* call) runs. nil means "no host installed" — `render` is then a
	// no-op and the dispatch behaves exactly as it did before the step existed.
	//
	// It is deliberately NOT set by New (nor carried by Clone): a bare runtime,
	// and every simulation clone, stay hook-free and fully synchronous. Each
	// host installs it explicitly (the server does so in initAgent), so backing
	// the feature out is one deleted line per host. The host's implementation
	// runs on the calling goroutine under whatever lock the dispatch already
	// holds; it must not re-take that lock and must not block.
	Commit func()
	// budget is the intermediate-frame allowance of the current top-level
	// interaction: the dispatch AND every continuation it spawns (an async
	// reply, a resumed delay tail) draw from one shared allowance capped at
	// MaxFrames, so a single click can never publish more than MaxFrames
	// intermediate frames however many round trips it spans. nil outside a
	// dispatch; the render step treats that as "no allowance".
	budget *frameBudget

	// Async is the HOST's background work sink: an `http.*` step marked
	// {"async": true} hands it the blocking half of the request (work) plus the
	// continuation that consumes the outcome (resume), and the dispatch returns
	// immediately instead of waiting out the round trip. nil means "no host
	// installed" — the step then runs on its ordinary synchronous path, so the
	// same JSON behaves exactly as it did before the field existed.
	//
	// The host contract, which every implementation must honour:
	//   - work runs on a goroutine of the host's choosing and touches NOTHING
	//     on the runtime, so it needs no lock;
	//   - resume is called at most once, with work's return value, while the
	//     host holds the lock it serialises dispatches with — it reads and
	//     writes state and may publish frames;
	//   - a host that decides to drop the result (the runtime was swapped out
	//     from under it by a hot reload) simply never calls resume, and calls
	//     AbandonInflight on the retired runtime to say so — the continuation it
	//     dropped is the one that would have released the in-flight count, and a
	//     count left raised makes Inflight() promise a reply that is never
	//     coming.
	//
	// Like Commit it is deliberately NOT set by New (nor carried by Clone), so
	// bare runtimes, offline renders and MCP simulations never open a socket on
	// a background goroutine.
	Async func(work func() any, resume func(any))
	// AsyncAll makes EVERY http.* step take the Async sink, not only the ones
	// that opted in with {"async": true}. It exists for one host shape: a
	// single-threaded one whose dispatch runs on the same thread that services
	// the UI event loop. On js/wasm that is literal — net/http blocks the
	// calling goroutine on the JS fetch, and when that goroutine is the one
	// re-entering Go from a js.FuncOf callback the scheduler deadlocks waiting
	// on itself (cmd/qorm-wasm/main.go promise()) — so a synchronous http step
	// there does not merely stall, it freezes the app permanently. Such a host
	// sets this flag when it installs Async (see playcore.InstallSinks) and the
	// step's own Async field becomes advisory.
	//
	// It is inert without an Async sink, so it can never make a bare runtime,
	// an offline render or an MCP simulation behave differently; and like the
	// two sinks it is not set by New and not carried by Clone.
	AsyncAll bool
	// inflight counts background units of work started but not yet resumed —
	// async requests plus pending `delay` continuations. Guarded by the host's
	// dispatch lock, like the state it lives beside.
	inflight int
	// LastScriptError holds the most recent qscript runtime error ("" when
	// the last script action ran clean). A script action has no per-step
	// error path — it is one program — so its failures (governance limits,
	// type errors) surface here, carrying the script line number; hosts and
	// agents (MCP) can read it after a dispatch. Cleared at the start of
	// every top-level dispatch and guarded by the host's dispatch lock, like
	// the state it describes.
	LastScriptError string
	// keyed maps a step's `key` to the request currently occupying that slot,
	// so a newer request on the same key can supersede it. Entries live only
	// between launch and continuation; the map is nil until the first keyed
	// request. Written and read exclusively under the host's dispatch lock (a
	// launch runs inside a dispatch, a continuation inside the host's resume),
	// which is what makes the plain map and the plain bool inside asyncSlot
	// race-free without a mutex of their own.
	keyed map[string]*asyncSlot
	// pendingRefs counts the open requests holding each `pending` state path
	// true, so overlapping requests release the flag only when the LAST of them
	// settles. Same locking discipline as keyed.
	pendingRefs map[string]int
}

// asyncSlot is one keyed request's cancellation handle. cancel tears down the
// transport (so a superseded request stops occupying a connection instead of
// running to completion unread); dropped is the authoritative decision, taken
// under the host's lock, that this request's continuation must not touch state.
//
// The two are deliberately separate. Cancellation is best-effort and racy by
// nature — a reply can already be decoded when cancel fires — so it cannot be
// what decides whether the outcome lands. `dropped` can: it is set by the
// superseding dispatch under the same lock the continuation later runs under,
// so the continuation reads a value that is already final.
type asyncSlot struct {
	cancel  context.CancelFunc
	dropped bool
}

// Inflight reports how many background units of work this runtime has started
// that have not delivered their outcome yet: async `http.*` requests plus
// `delay` steps whose remaining steps are still waiting. Zero means quiescent:
// every continuation has run, so the state (and therefore the render) has
// settled. Read it under whatever lock the host serialises dispatches with —
// the same discipline that guards State.
func (r *Runtime) Inflight() int { return r.inflight }

// releaseInflight retires one unit of background work. It floors at zero rather
// than decrementing blindly, because AbandonInflight may have zeroed the count
// out from under a continuation the host meant to drop and ran anyway: a
// negative count would read as "quiescent" by luck on the next launch and as
// nonsense to anyone printing it.
func (r *Runtime) releaseInflight() {
	if r.inflight > 0 {
		r.inflight--
	}
}

// AbandonInflight declares that this runtime is retired: the host has swapped
// it out (a hot reload, an OTA activate, a rollback) and will never call the
// continuation of any background work it started. The count drops to zero,
// every keyed slot is marked dropped and its transport cancelled.
//
// It exists because "the abandoned runtime keeps its raised count, which is
// sound because nothing reads it any more" is only true of the host that
// abandoned it. Inflight() is a PUBLISHED quiescence signal — the MCP
// qorm_activity payload, a test polling for the state to settle — and a
// discarded runtime that reports work forever in flight makes any such caller
// wait for a continuation that is never coming. Zero is the honest answer:
// nothing further will happen to this runtime.
//
// Marking the slots dropped is belt and braces: if a continuation does reach a
// retired runtime after all, it now takes the superseded path and writes
// nothing. Idempotent, and safe to call on a runtime that never started any
// background work. Call it under whatever lock the host serialises dispatches
// with, like every other write to this runtime.
func (r *Runtime) AbandonInflight() {
	r.inflight = 0
	for _, slot := range r.keyed { // claimKey only ever stores a live slot
		slot.dropped = true
		slot.cancel()
	}
	r.keyed = nil
}

// MaxFrames caps the intermediate frames one top-level interaction may publish,
// so a looping action cannot flood the live-sync channel. The allowance is
// shared by the dispatch and every continuation it spawns (see frameBudget), so
// "one click" is bounded as a whole, not per leg. Frames beyond the cap are
// dropped silently (the final frame at the dispatch boundary always ships).
//
// It is exported because hosts must size their per-revision handler rings to
// cover at least this many frames plus the boundary frame (see
// server.handlerHistory): a click from any frame of even the most
// frame-flooding interaction must stay resolvable.
const MaxFrames = 64

// frameBudget is one top-level interaction's remaining intermediate-frame
// allowance. It is heap-allocated and shared by pointer with the continuations
// the dispatch launches, so a completion that lands after the dispatch ended —
// possibly after OTHER dispatches have run — draws down the SAME allowance
// instead of starting over (which is what let one click publish 128+ frames).
type frameBudget struct{ left int }

// maxInflight caps the background work one runtime may have open at once:
// async `http.*` requests plus waiting `delay` continuations. It is the same
// class of defence as the timer floor (render.TimerMinEveryMS) — a guard
// against an app hurting ITSELF. A 250ms timer whose onTick fires an async
// request against a backend taking 5s accumulates twenty open requests and
// keeps climbing; without a ceiling that is an unbounded goroutine leak, an
// unbounded connection count, and a self-inflicted denial of service on the
// backend, all from JSON that looks entirely reasonable.
//
// Reaching the cap does NOT queue and does NOT silently drop: the step takes
// its ERROR path immediately (the `error` state path is written and OnError
// runs, with errTooManyInflight as the message), which is the one outcome an
// app already knows how to render. Queuing would trade a visible failure for
// an invisible, unbounded backlog — the very thing the cap exists to prevent —
// and dropping silently would leave a spinner up forever.
const maxInflight = 64

// errTooManyInflight is the message a step gets when it is refused by
// maxInflight. It is a stable string so an app (or a test) can match on it.
var errTooManyInflight = fmt.Sprintf("too many concurrent requests (%d in flight)", maxInflight)

// maxInvokeDepth caps nested Dispatch calls (invoke steps calling actions that
// invoke further actions); beyond it a dispatch is silently dropped.
const maxInvokeDepth = 16

// maxIfDepth caps `if` step nesting at dispatch time (the loader also warns
// beyond the same limit); deeper branches are silently skipped.
const maxIfDepth = 32

// maxEnterChain caps consecutive onEnter dispatches drained in one
// RunPendingEnter call, so two scenes whose onEnter actions navigate to each
// other cannot ping-pong forever.
const maxEnterChain = 8

// maxGuardRedirects caps the guard redirect chain resolved for ONE navigation
// (scene A's guard sends you to B, whose guard sends you to C, ...). Reaching
// the cap — or revisiting a scene already seen in the same chain — refuses the
// navigation outright rather than looping. It shares maxEnterChain's budget
// shape on purpose: a redirect landing on a scene whose onEnter navigates again
// is bounded by BOTH caps.
const maxGuardRedirects = maxEnterChain

// maxForEachIterations caps one `forEach` step's iterations. A collection
// longer than this is truncated (the extra elements are simply not visited), so
// a state array that grew unexpectedly — or was filled by an http response —
// cannot turn one dispatch into an unbounded amount of work.
const maxForEachIterations = 10000

// navFrame is one entry on the navigation back stack: the scene to return to
// and the route params it was shown with.
type navFrame struct {
	Scene  string
	Params map[string]any
}

// GuardBlocked is the reserved scene id a runtime shows when a route guard
// refuses entry OUTRIGHT (no redirect, a redirect cycle, or a chain past
// maxGuardRedirects) and nothing else may be entered either — no frame on the
// back stack and not the entry scene. It is the fail-CLOSED terminal of the
// entry path: the runtime would otherwise have to keep showing the very scene
// its guard just refused.
//
// It is deliberately not a real scene: no app can declare it (the NUL byte
// cannot appear in a scene id read from disk), it carries no onEnter and no
// guard of its own, and it is never pushed onto the back stack. A host renders
// it as an EMPTY frame — there is, by construction, nothing this session is
// allowed to see. A host that does not know the id must fall back to rendering
// nothing rather than to the entry scene, which may be the refused scene
// itself.
const GuardBlocked = "\x00guard-refused"

// Blocked reports whether the runtime is parked on GuardBlocked — every entry a
// guard would permit was refused, so there is nothing this session may render.
func (r *Runtime) Blocked() bool { return r.Scene == GuardBlocked }

// CurrentScene is the scene id to render ("" falls back to the entry scene).
func (r *Runtime) CurrentScene() string { return r.Scene }

// KeyAction resolves a scene-level key binding (scene JSON "keys") for the
// current scene — the declarative control scheme for games and
// keyboard-driven apps, dispatched by the engine with no focus required.
func (r *Runtime) KeyAction(key string) (string, bool) {
	if r.App == nil || r.App.SceneKeys == nil {
		return "", false
	}
	// CurrentScene's "" falls back to the entry scene (a single-scene app
	// never navigates, so its keys must resolve from the first frame).
	scene := r.Scene
	if scene == "" {
		scene = r.App.Entry
	}
	m := r.App.SceneKeys[scene]
	if m == nil {
		return "", false
	}
	a, ok := m[strings.ToLower(key)]
	return a, ok
}

// KeyReleaseAction resolves the keyup counterpart of KeyAction (scene JSON
// "keyReleases"): a key-name → action map dispatched on the same key being
// released. The lookup mirrors KeyAction — case-insensitive, falls back to
// the entry scene — so a press/release pair can be authored on the same
// key from one JSON block.
func (r *Runtime) KeyReleaseAction(key string) (string, bool) {
	if r.App == nil || r.App.SceneKeyReleases == nil {
		return "", false
	}
	scene := r.Scene
	if scene == "" {
		scene = r.App.Entry
	}
	m := r.App.SceneKeyReleases[scene]
	if m == nil {
		return "", false
	}
	a, ok := m[strings.ToLower(key)]
	return a, ok
}

// SwipeAction resolves a scene-level swipe binding (scene JSON "swipes") for
// the current scene — the touch counterpart of KeyAction. dir is one of
// "left", "right", "up", "down" (the loader drops any other direction).
func (r *Runtime) SwipeAction(dir string) (string, bool) {
	if r.App == nil || r.App.SceneSwipes == nil {
		return "", false
	}
	scene := r.Scene
	if scene == "" {
		scene = r.App.Entry
	}
	m := r.App.SceneSwipes[scene]
	if m == nil {
		return "", false
	}
	a, ok := m[strings.ToLower(dir)]
	return a, ok
}

// Navigate pushes the current scene (and its route params) onto the back stack
// and shows `to` with the given params. params may be nil (→ an empty route).
// Unknown scenes and no-op navigations are ignored, and the target's route
// guard runs first: a guard that fails diverts this push to its redirect target
// (so the back stack records the scene you actually came from, never the one
// you were refused) or cancels it entirely.
func (r *Runtime) Navigate(to string, params map[string]any) {
	if to == "" || to == r.Scene {
		return
	}
	if _, ok := r.App.Scenes[to]; !ok {
		return
	}
	resolved, gparams, ok := r.guardResolve(to, params)
	if !ok {
		return // the guard refused this navigation: stay put
	}
	if resolved != to {
		// The guard diverted us. Landing back on the scene already showing is
		// a no-op — pushing it would leave a duplicate frame on the back stack.
		if r.sameScene(resolved, r.Scene) {
			return
		}
		to, params = resolved, gparams
	}
	r.pushFrame()
	r.Scene = to
	if params == nil {
		params = map[string]any{}
	}
	r.RouteParams = params
	r.NavDir = "push"
	r.pendingEnter = true
}

// pushFrame records the current scene (and its route params) as the frame Back
// returns to. The blocked scene is never recorded: it is not somewhere the user
// was, it is the absence of anywhere to be.
func (r *Runtime) pushFrame() {
	if r.Scene == GuardBlocked {
		return
	}
	r.NavStack = append(r.NavStack, navFrame{Scene: r.Scene, Params: r.RouteParams})
}

// NavigateBack returns to the previous scene, restoring its route params — and
// runs that scene's route guard first, exactly like every other entry path.
//
// Back is an ENTRY into the scene below, not an undo: the frame was pushed when
// the user was allowed there, and by the time they return the permission may be
// gone (a token expired, a role revoked, a sign-out earlier in the same
// action). Re-resolving is therefore the whole point — without it Back is the
// one door into a protected scene that nobody checks, and the scene renders in
// full to every subscriber of the session.
//
// A frame whose guard now DIVERTS is followed to the redirect target, like any
// other navigation. A frame the guard refuses outright is skipped and the
// unwind continues into the frames below it: "go back" is the user's expressed
// intent to leave the current scene, and the answer to "you may not enter that"
// is the next place they may enter, never the refused scene. If no frame at all
// may be entered the stack is left untouched and the runtime stays put — the
// current scene was permitted when it was entered, and Back promises to leave,
// not to find somewhere new.
func (r *Runtime) NavigateBack() {
	for i := len(r.NavStack) - 1; i >= 0; i-- {
		f := r.NavStack[i]
		to, params, ok := r.guardResolve(r.sceneID(f.Scene), f.Params)
		if !ok {
			continue // refused outright: keep unwinding past it
		}
		if r.sameScene(to, f.Scene) {
			// Not diverted: restore the frame verbatim, including the entry
			// scene's "" spelling and the params it was shown with.
			to, params = f.Scene, f.Params
		}
		r.NavStack = r.NavStack[:i]
		r.Scene = to
		if params == nil {
			params = map[string]any{}
		}
		r.RouteParams = params
		r.NavDir = "pop"
		r.pendingEnter = true
		return
	}
}

// TakeNavDir returns and clears the pending navigation direction.
func (r *Runtime) TakeNavDir() string { d := r.NavDir; r.NavDir = ""; return d }

// RoutePath renders the current scene + route params as a deep-link URL path —
// the inverse of NavigateToPath. The entry scene with no params is "/"; any
// other scene is "/?scene=<id>&k=v" with the scene id and every route param
// (values stringified). url.Values.Encode sorts keys, so the path is stable.
func (r *Runtime) RoutePath() string {
	scene := r.Scene
	if scene == r.App.Entry || scene == GuardBlocked {
		// The entry scene is addressed as "/", not by id — and so is the blocked
		// scene, which is not addressable at all: a URL naming it would invite a
		// deep link back into a state the guard produced, not one it permits.
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
	// Route guards run BEFORE the Back detection below, so the stack unwind is
	// decided for the scene actually being entered — and so a deep link or a
	// browser Back into a protected scene is guarded exactly like an in-app
	// navigate. The guard speaks concrete scene ids; the entry scene's ""
	// spelling is restored afterwards so the frame bookkeeping is unchanged.
	target, gparams, ok := r.guardResolve(r.sceneID(scene), params)
	if !ok {
		return
	}
	if !r.sameScene(target, scene) {
		scene, params = target, gparams
		if isEntry(scene) {
			scene = ""
		}
		if scene == r.Scene || (isEntry(scene) && isEntry(r.Scene)) {
			return // the guard diverted us to the scene already on screen
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
	r.pushFrame()
	r.Scene = scene
	r.RouteParams = params
	r.NavDir = "push"
	r.pendingEnter = true
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

// loadAppTheme returns the skin a runtime should start with. The default Apple
// HIG theme is always the safe fallback (matches the rest of the built-in
// palettes' behavior); a custom skin (themes/<name>.json beside the manifest)
// wins when it exists and parses, so the server's first frame already paints
// the right colors and `background: var(--sky)` resolves instead of empty.
func loadAppTheme(app *model.App) *theme.Theme {
	if app == nil || app.Theme == "" {
		return theme.GetDefault()
	}
	// Built-in names are already covered by render.ThemeCSS — the runtime
	// still gets the default Theme struct so legacy callers that read
	// rt.Theme.ParsedColors see the right values.
	if theme.IsBuiltinTheme(app.Theme) {
		return theme.GetDefault()
	}
	if app.BaseDir != "" {
		if t, err := theme.LoadTheme(filepath.Join(app.BaseDir, "themes", app.Theme+".json")); err == nil {
			return t
		}
	}
	return theme.GetDefault()
}

// New creates a runtime with state seeded from the manifest's initial values.
func New(app *model.App) *Runtime {
	state := deepCopyMap(app.GlobalState.Initial)
	if state == nil {
		state = map[string]any{}
	}
	// Seed the active theme into state so expressions (e.g. a theme-toggle's
	// `state.theme == …` ternary) and the native canvas backend — both of which
	// read state.theme directly — see the manifest's initial theme instead of
	// an empty value that only CurrentTheme() compensated for. Only when absent:
	// an explicit initial/persisted theme wins.
	if app != nil && app.Theme != "" {
		if t, _ := state["theme"].(string); t == "" {
			state["theme"] = app.Theme
		}
	}
	rt := &Runtime{App: app, State: state, RouteParams: map[string]any{}, pendingEnter: true, Theme: loadAppTheme(app)}
	rt.refreshComputed() // derived values exist from the very first frame
	// The call() builtin's dispatch bridge: a script calling call("otherAction")
	// re-enters Dispatch on the SAME runtime, so invoke-depth governance (the
	// recursion guard below) applies to call()-chains exactly as it does to
	// human/agent dispatches.
	qscript.SetDispatchHook(func(line int, name string, args map[string]any) error {
		return rt.DispatchErr(name, args)
	})
	// Audio hook: playSound / playMusic builtins route here so qscript can
	// trigger WAV playback without expr importing the runtime.
	expr.SetAudioHandler(audioAdapter{rt: rt})
	return rt
}

// audioAdapter forwards qscript's playSound/playMusic/stopMusic to the
// runtime's audio sink. It owns the runtime reference so a swap (hot reload)
// keeps routing to the current app's BaseDir.
type audioAdapter struct{ rt *Runtime }

func (a audioAdapter) baseDir() string {
	if a.rt != nil && a.rt.App != nil {
		return a.rt.App.BaseDir
	}
	return ""
}

func (a audioAdapter) isWeb() bool {
	return a.rt != nil && a.rt.App != nil && a.rt.App.Web
}

// play resolves src and starts playback. On App.Web the browser fetches
// BaseDir+src via HTMLAudioElement (SrcPlayer); native loads a WAV from disk
// and hands PCM to the platform sink.
func (a audioAdapter) play(src string, loop bool) error {
	if a.isWeb() {
		url, err := audio.ResolveWebSrc(a.baseDir(), src)
		if err != nil {
			return err
		}
		if sp, ok := audio.ActiveSink().(audio.SrcPlayer); ok {
			return sp.PlaySrc(url, loop)
		}
		return fmt.Errorf("audio: web playback requires a SrcPlayer sink")
	}
	snd, err := audio.LoadSound(a.baseDir(), src)
	if err != nil {
		return err
	}
	return audio.ActiveSink().Play(snd, loop)
}

func (a audioAdapter) PlayOnce(src string) error { return a.play(src, false) }

func (a audioAdapter) PlayLoop(src string) error { return a.play(src, true) }

func (a audioAdapter) Stop() error { return audio.ActiveSink().Stop() }

// SwapAppPreservingState swaps out the app manifest (e.g. during a hot reload)
// while preserving active runtime state and user inputs.
func (r *Runtime) SwapAppPreservingState(newApp *model.App) {
	if newApp == nil {
		return
	}
	oldTheme := ""
	if r.App != nil {
		oldTheme = r.App.Theme
	}
	r.App = newApp
	if r.State == nil {
		r.State = map[string]any{}
	}
	if newApp.GlobalState.Initial != nil {
		for k, v := range newApp.GlobalState.Initial {
			if _, exists := r.State[k]; !exists {
				r.State[k] = deepCopy(v)
			}
		}
	}
	// Re-seed the manifest theme the way New does. state.theme holding the OLD
	// manifest's value is a seed the runtime wrote, not a user choice — without
	// this a manifest theme edit (or removal) never took effect on hot reload,
	// because the stale seed pinned CurrentTheme() to the old value. An
	// explicit initial theme wins over the manifest theme (New's precedence);
	// a theme the app/user set to something else is preserved.
	if cur, _ := r.State["theme"].(string); cur == "" || (oldTheme != "" && cur == oldTheme) {
		if t, _ := newApp.GlobalState.Initial["theme"].(string); t != "" {
			r.State["theme"] = t
		} else if newApp.Theme != "" {
			r.State["theme"] = newApp.Theme
		} else {
			delete(r.State, "theme")
		}
	}
	r.refreshComputed()
}

// sceneID normalises a scene spelling to a concrete id: the entry scene is
// addressed both as "" and by its id, and everything that reasons about a
// scene's declarations (guards, onEnter) must see the same key for both.
func (r *Runtime) sceneID(scene string) string {
	if scene == "" {
		return r.App.Entry
	}
	return scene
}

// sameScene reports whether two scene spellings denote the same scene.
func (r *Runtime) sameScene(a, b string) bool { return r.sceneID(a) == r.sceneID(b) }

// guardResolve applies the route guards protecting `scene` and returns the
// scene that may actually be entered, the route params to enter it with, and
// whether the navigation is allowed at all.
//
// A scene with no guard, or whose guard condition is truthy, is returned
// unchanged — the overwhelmingly common case, and the only one an app without
// guards ever takes. A failing guard hands over to its Redirect target, whose
// own guard is then resolved in turn (so a chain of protections composes), with
// the redirect's Params evaluated in the CURRENT scene context. A failing guard
// with no redirect, a chain that revisits a scene, and a chain longer than
// maxGuardRedirects all refuse the navigation (ok=false), which every caller
// turns into "stay where you are" — the safe outcome for a guard whose author
// wrote a loop.
func (r *Runtime) guardResolve(scene string, params map[string]any) (string, map[string]any, bool) {
	if len(r.App.SceneGuards) == 0 {
		return scene, params, true
	}
	seen := map[string]bool{}
	for i := 0; i < maxGuardRedirects; i++ {
		g := r.App.SceneGuards[r.sceneID(scene)]
		if g == nil || g.Condition == "" {
			return scene, params, true
		}
		ctx := r.guardCtx()
		if expr.Truthy(EvalBinding(g.Condition, ctx)) {
			return scene, params, true
		}
		if g.Redirect == "" || seen[r.sceneID(g.Redirect)] {
			return "", nil, false
		}
		seen[r.sceneID(scene)] = true
		params = evalParams(g.Params, ctx)
		scene = g.Redirect
	}
	return "", nil, false
}

// ComputedVars returns the app's derived values as an evaluation-scope map —
// the same map published at state.<ComputedNamespace>, so it is read-only to
// callers. A host that wants the shorter bare spelling ({{ computed.total }})
// in SCENE bindings adds it to its binding context under
// model.ComputedNamespace; inside actions the bare spelling already works,
// because an action context exposes every top-level state key. Never nil.
func (r *Runtime) ComputedVars() map[string]any {
	if m, ok := r.State[model.ComputedNamespace].(map[string]any); ok {
		return m
	}
	return noComputed
}

// noComputed is the empty namespace returned for an app that declares nothing
// derived. It is shared rather than allocated per call because the renderer
// calls ComputedVars once per SCOPED node — a list of 200 rows allocated 200
// empty maps per frame for apps that have no derived values at all. Sharing is
// safe precisely because the return value is read-only by contract (it is the
// published namespace itself in the non-empty case, so a caller that wrote to
// it would already be corrupting live state); the only writes to the namespace
// go through refreshComputed, which replaces the map wholesale.
var noComputed = map[string]any{}

// SetStatePath writes a value at a dotted state path and reports whether the
// write happened. `user.name` descends into (and creates) nested maps, exactly
// like the `state.set` action step — the one dotted-path semantics the whole
// system uses, so a value written here is readable by `{{ state.user.name }}`.
//
// It refuses a path inside the read-only computed namespace, the same refusal
// applyStep makes for a step and the loader makes at load time: those values
// are DERIVED and republished wholesale at every frame boundary, so a write
// there is overwritten within the frame and misleading until it is. As in
// applyStep the check only applies to an app that actually declares computed
// values — everywhere else `computed` is an ordinary state key and keeps
// working.
//
// It is the entry point for a host that writes state from outside a dispatch
// (the MCP qorm_set_state tool); actions go through applyStep.
func (r *Runtime) SetStatePath(path string, val any) bool {
	if path == "" {
		return false
	}
	if r.App != nil && len(r.App.Computed) > 0 && model.IsComputedPath(path) {
		return false
	}
	setPath(r.State, path, val)
	return true
}

// StatePath reads the value at a dotted state path — the read that matches
// SetStatePath's write, so a host reasoning about state uses the same spelling
// an app binding does.
func (r *Runtime) StatePath(path string) any { return getPath(r.State, path) }

// refreshComputed re-evaluates every declared computed value and republishes
// the whole namespace. It is the ONLY writer of state.<ComputedNamespace>.
//
// Evaluation happens once per frame rather than once per binding: the values
// are materialised into a plain map that every later binding just reads, so a
// total bound by a dozen nodes costs one evaluation. Names are evaluated in
// model.ComputedOrder, so a value may read another one ({{ computed.subtotal +
// computed.tax }}) and see it already filled in; names caught in a dependency
// cycle are published as NOTHING (reading one yields nil), which is what makes
// a circular declaration a diagnosable mistake instead of a stack overflow.
//
// An app that declares no computed values never touches state at all, so
// "computed" stays an ordinary state key for every app written before this
// existed.
func (r *Runtime) refreshComputed() {
	if r.App == nil || len(r.App.Computed) == 0 {
		return
	}
	// Assigned in one shot: a half-filled namespace is never observable, not
	// even by a host reading state from another goroutine mid-refresh.
	r.State[model.ComputedNamespace] = r.deriveComputed()
}

// deriveComputed evaluates every declared value and returns the namespace,
// WITHOUT publishing it. It reads the runtime but writes nothing, which is what
// lets a route guard decide against an up-to-date derived view (see guardCtx)
// while the published view still turns over only at frame boundaries.
func (r *Runtime) deriveComputed() map[string]any {
	out := make(map[string]any, len(r.App.Computed))
	// A shallow copy of state with the namespace pointed at the map being
	// filled: each value therefore sees the ones evaluated before it (the
	// dependency order guarantees those are the ones it declared a need for),
	// and the live state is not touched while that happens.
	st := make(map[string]any, len(r.State)+1)
	for k, v := range r.State {
		st[k] = v
	}
	st[model.ComputedNamespace] = out
	ctx := r.bareCtx(st)
	order, _ := r.App.ComputedOrder()
	for _, name := range order {
		out[name] = EvalBinding(r.App.Computed[name], ctx)
	}
	return out
}

// guardCtx is the evaluation context for a route-guard condition: the scene
// context, but with the derived values RE-EVALUATED first.
//
// The published derived view deliberately turns over only at frame boundaries,
// so every binding in one frame agrees. A guard is not a binding, though — it
// is a decision taken mid-dispatch, and the state it judges is live. An action
// that signs a user in and then navigates would otherwise be judged against the
// derived view from before it ran, i.e. `{{ state.user != null }}` would let
// the navigation through while the equivalent `{{ computed.signedIn }}` bounced
// it. Refreshing here is what makes those two spellings mean the same thing.
func (r *Runtime) guardCtx() map[string]any {
	ctx := r.sceneCtx()
	if r.App == nil || len(r.App.Computed) == 0 {
		return ctx
	}
	st := make(map[string]any, len(r.State)+1)
	for k, v := range r.State {
		st[k] = v
	}
	st[model.ComputedNamespace] = r.deriveComputed()
	ctx["state"] = st
	return ctx
}

// RunPendingEnter dispatches the current scene's onEnter action if navigation
// just entered it (or the runtime was just created) and it has not been
// dispatched yet. Hosts call it at their render choke point — the server before
// rendering under its mutex, the WASM build after init/event dispatch — so the
// hook fires exactly once per scene entry: an SSE reconnect, a page refresh of
// an already-entered scene, or a catch-up re-render never replays it. An
// onEnter that itself navigates marks the next scene pending, which drains in
// the same call, capped at maxEnterChain so mutually-navigating onEnter hooks
// cannot loop forever (navigating to the scene itself is already a runtime
// no-op and never re-marks).
func (r *Runtime) RunPendingEnter() {
	// One refresh per frame: every host funnels its mutation paths (human
	// events, agent writes over MCP, viewport reports, OTA activation) through
	// the call site of this method before rendering, so a derived value is
	// current even when the state under it was written without dispatching an
	// action.
	r.refreshComputed()
	last := "\x00" // sentinel: never equals a scene key
	for i := 0; i < maxEnterChain && r.pendingEnter; i++ {
		r.pendingEnter = false
		scene := r.Scene
		if scene == "" {
			scene = r.App.Entry
		}
		// Route guard, checked before the scene's onEnter and before it
		// renders — this is the path that protects the ENTRY scene and any
		// scene a host put on screen directly (a deep link, a restored
		// session). A guard that fires REPLACES the current frame rather than
		// pushing one: you were never legitimately on the guarded scene, so it
		// must not become somewhere Back can return to.
		to, params, ok := r.guardResolve(scene, r.RouteParams)
		if !ok {
			// The guard refused entry OUTRIGHT — it named no redirect, its
			// redirects cycle, or the chain ran past maxGuardRedirects.
			//
			// On Navigate/NavigateTo a refusal means "stay where you are",
			// which is safe because "where you are" is the scene the user was
			// already permitted. HERE it is the opposite: this scene IS the
			// refused one, so falling through would render it and fire its
			// onEnter — the load-the-private-data hook running for a visitor
			// the guard just turned away. A refusal on this path must
			// therefore LEAVE, and it must leave before onEnter.
			//
			// Retreat first (the nearest permitted frame, else the entry
			// scene); if nothing at all may be entered, park on GuardBlocked,
			// which renders empty. Both outcomes terminate: retreat only ever
			// shortens the stack, and the blocked scene clears pendingEnter.
			if !r.retreatToPermitted() {
				r.block()
			}
			continue
		}
		if !r.sameScene(to, scene) {
			r.Scene = to
			if params == nil {
				params = map[string]any{}
			}
			r.RouteParams = params
			r.pendingEnter = true
			continue
		}
		if scene == last {
			// The hook navigated back into the SAME scene (e.g. the entry
			// scene's "" and id spellings) — entering it again immediately
			// would re-fire forever; one fire per consecutive entry.
			break
		}
		last = scene
		inv := r.App.SceneEnter[scene]
		if inv == nil {
			continue // nothing to run for this scene; a pending re-mark would drain next
		}
		r.Dispatch(inv.Name, r.EvalArgs(inv.Args))
	}
	r.pendingEnter = false
}

// retreatToPermitted moves the runtime off a scene whose guard refused entry
// outright, to the nearest place it may legitimately be: the topmost back-stack
// frame the guards still admit (following that frame's own redirect if it has
// one), or failing that the entry scene. It reports whether it found one.
//
// Frames it walks past are DROPPED rather than kept: they lead back to the
// refusal, and leaving them on the stack would let one Back tap walk into it
// again. The landing frame is marked pending so the scene actually entered runs
// its own onEnter — the refused scene's never does.
//
// It never re-enters the scene it was called for, because that scene is not on
// the stack (it is the current one) and, if it is the entry scene, the entry
// fallback resolves the same refusal and declines.
func (r *Runtime) retreatToPermitted() bool {
	for i := len(r.NavStack) - 1; i >= 0; i-- {
		f := r.NavStack[i]
		to, params, ok := r.guardResolve(r.sceneID(f.Scene), f.Params)
		if !ok {
			continue
		}
		if r.sameScene(to, f.Scene) {
			to, params = f.Scene, f.Params
		}
		r.NavStack = r.NavStack[:i]
		r.enter(to, params)
		return true
	}
	if to, params, ok := r.guardResolve(r.App.Entry, nil); ok {
		r.NavStack = nil
		r.enter(to, params)
		return true
	}
	return false
}

// enter puts the runtime on a scene the guards have already admitted, marking
// it pending so RunPendingEnter's loop dispatches its onEnter next.
func (r *Runtime) enter(scene string, params map[string]any) {
	r.Scene = scene
	if params == nil {
		params = map[string]any{}
	}
	r.RouteParams = params
	r.pendingEnter = true
}

// block parks the runtime on GuardBlocked: there is no scene this session may
// see. The back stack goes with it — every frame on it was just refused — and
// the entry mark is cleared, because the blocked scene has no hook to run and
// nothing to re-resolve.
func (r *Runtime) block() {
	r.Scene = GuardBlocked
	r.NavStack = nil
	r.RouteParams = map[string]any{}
	r.pendingEnter = false
}

// ClearPendingEnter drops an undispatched scene-entry mark. Used by hot-reload:
// the swapped-in runtime continues the SAME session (scene and state carried
// over), so the entry hook a fresh runtime would fire must not replay.
func (r *Runtime) ClearPendingEnter() { r.pendingEnter = false }

// PendingEnter reports whether a scene-entry mark is waiting to be drained
// (the current scene's onEnter has not run yet). Hosts rendering outside a
// dispatch use it to decide whether the render must instead publish a fresh
// revision: a pending hook can write state, and a frame rendered after it
// differs from whatever the current revision last published.
func (r *Runtime) PendingEnter() bool { return r.pendingEnter }

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
		App:          r.App,
		State:        deepCopyMap(r.State),
		Viewport:     r.Viewport,
		Scene:        r.Scene,
		NavStack:     stack,
		RouteParams:  deepCopyMap(r.RouteParams),
		NavDir:       r.NavDir,
		pendingEnter: r.pendingEnter,
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

// sceneCtx is the evaluation context for scene bindings: `state.*`, the
// active-locale message catalog `t.*`, the responsive `viewport.*` vars and the
// current scene's navigation parameters `route.*`.
func (r *Runtime) sceneCtx() map[string]any {
	return map[string]any{"state": r.State, "t": r.Catalog(), "viewport": r.ViewportVars(), "route": r.RouteParams}
}

// isReservedRoot reports whether a name is one of the context roots every
// dotted spelling in an app resolves through. These are owned by the runtime:
// nothing an app can name — a state key, an action arg, a captured list-item
// alias — may take one over. See bareCtx.
func isReservedRoot(name string) bool {
	return name == "state" || name == "t" || name == "viewport"
}

// bareCtx builds an action/derived evaluation context over the given state map:
// the reserved roots (`state`, `t`, `viewport`) plus every TOP-LEVEL state key
// spelled bare, so `{{ count + 1 }}` inside an action means `{{ state.count + 1 }}`
// exactly as the message-format context already reads a bare `{count}`.
//
// The bare keys are laid down FIRST and the roots are written over them, so the
// roots always win. That ordering is the whole point: a state key that happens
// to be named `state` (or `t`, or `viewport`) is still readable bare, but it can
// never displace the root. Written the other way round — roots first, bare keys
// over them — a single top-level `state` key silently repointed every
// `{{ state.x }}` in the app at itself, so every binding and every computed
// value collapsed to nothing at once, on every frame, with no diagnostic.
//
// Callers layering args on top (Dispatch, freshCtx) skip the reserved roots for
// the same reason; freezeCtx already refuses to freeze them.
func (r *Runtime) bareCtx(st map[string]any) map[string]any {
	ctx := make(map[string]any, len(st)+3)
	for k, v := range st {
		ctx[k] = v // bare spellings, including `computed` itself
	}
	ctx["state"] = st
	ctx["t"] = r.Catalog()
	ctx["viewport"] = r.ViewportVars()
	return ctx
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
	// The bare keys go down first so the `state` root wins over a state key of
	// the same name, exactly as bareCtx does for action contexts. (This one
	// cannot call bareCtx: bareCtx resolves the catalog.)
	msgCtx := make(map[string]any, len(r.State)+2)
	for k, v := range r.State {
		msgCtx[k] = v
	}
	msgCtx["state"] = r.State
	msgCtx["__locale"] = r.CurrentLocale()
	out := make(map[string]any, len(merged))
	for k, v := range merged {
		out[k] = fillMessage(v, msgCtx)
	}
	return out
}

// EvalBinding evaluates a possibly-bound string. If the whole string is a
// single {{expr}}, the typed value is returned; if it mixes text and bindings,
// an interpolated string is returned; a plain string is returned as-is.
//
// Delimiters are found with the same quote-aware scan the loader's static
// checks use (expr.CloseIndex), so a "}}" inside a binding's string literal —
// e.g. {{ '}}' }} — does not truncate the expression at render time. A "{{"
// with no closing "}}" outside string literals is not a binding; the text is
// returned unchanged from that point.
func EvalBinding(s string, ctx map[string]any) any {
	if !strings.Contains(s, "{{") {
		return s // fast path: plain text never scans or evaluates
	}
	// Whole-string single binding -> typed value.
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{{") {
		if end := expr.CloseIndex(trimmed[2:]); end != -1 && 2+end+2 == len(trimmed) {
			v, err := expr.Eval(trimmed[2:2+end], ctx)
			if err != nil {
				return ""
			}
			return v
		}
	}
	// Mixed text and bindings -> interpolated string. A binding that fails to
	// evaluate expands to "" while the surrounding text survives.
	var sb strings.Builder
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			sb.WriteString(s)
			return sb.String()
		}
		end := expr.CloseIndex(s[start+2:])
		if end == -1 {
			sb.WriteString(s)
			return sb.String()
		}
		sb.WriteString(s[:start])
		if v, err := expr.Eval(s[start+2:start+2+end], ctx); err == nil {
			sb.WriteString(expr.Stringify(v))
		}
		s = s[start+2+end+2:]
	}
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
// Nested dispatches (the `invoke` step, onEnter chains) are capped at
// maxInvokeDepth, so recursive or mutually-recursive actions terminate instead
// of hanging the runtime.
func (r *Runtime) Dispatch(name string, args map[string]any) {
	if r.callDepth >= maxInvokeDepth {
		return
	}
	// The intermediate-frame budget is per top-level INTERACTION: nested invokes
	// share the outer dispatch's allowance, so a recursive action cannot reset
	// it and publish without bound — and the continuations the dispatch spawns
	// draw from the same allowance (they restore this pointer when they land).
	if r.callDepth == 0 {
		r.budget = &frameBudget{left: MaxFrames}
		r.LastScriptError = ""
	}
	r.callDepth++
	// Derived values are republished at the dispatch BOUNDARY, not per step:
	// one action that writes five state keys recomputes them once, and the
	// state the caller reads the moment Dispatch returns is already consistent
	// with its own derived view. Nested invokes share the outer boundary.
	defer func() {
		r.callDepth--
		if r.callDepth == 0 {
			r.refreshComputed()
		}
	}()
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
	if act.Script != "" {
		// A script action runs its qscript source INSTEAD of steps (the loader
		// warns when both are declared; the script always wins). Reads and
		// writes go straight to the state store with the dispatch args bound
		// as `args`; the script was already compiled at load time. A runtime
		// failure (governance limit, type error) is recorded on LastScriptError
		// with the script line number — dispatch stays fire-and-forget, like
		// every other action path, and a failed script never panics the host.
		// The shared function library (actions/lib.qs, loader.ScriptLib) is
		// PREPENDED at dispatch: its fn definitions join every action's
		// compilation without being stored in act.Script, so the round-trip
		// serializer never writes the merged source back out (the fixed-point
		// property holds). A lib must contain only fn definitions (its own
		// parse errors were reported at load time).
		src := act.Script
		if lib := r.App.ScriptLib; lib != "" {
			src = lib + "\n" + src
		}
		if err := qscript.Run(src, r.State, args); err != nil {
			r.LastScriptError = err.Error()
		}
		return
	}
	// bareCtx exposes top-level state keys so a bare `count` in an action
	// expression resolves to state.count (as the message-format context already
	// does); otherwise `{{ count + 1 }}` reads nil and never accumulates.
	ctx := r.bareCtx(r.State)
	for k, v := range args { // args still win over state — but never over the roots
		if isReservedRoot(k) {
			continue
		}
		ctx[k] = v
	}
	r.applySteps(act.Steps, ctx, 0)
}

// applySteps runs a step list in order. depth counts `if`/branch nesting
// levels (not action calls — that is callDepth), capped at maxIfDepth.
//
// One step is resolved here rather than in applyStep: `delay` suspends the REST
// OF ITS LIST, and this is the only place that knows what the rest of the list
// is. When the wait is accepted, the remaining steps become the continuation
// and this call returns without running them; applyStep's own `delay` case is
// the degradation for when it is not (see there).
// DispatchErr is Dispatch with a return value for the qscript call() builtin:
// it reports whether the action exists and was dispatched under the recursion
// cap. A missing action or a depth-limit refusal is an error the calling
// script sees; a nested script action's failure surfaces through
// LastScriptError (restored by the outer dispatch's boundary reset).
func (r *Runtime) DispatchErr(name string, args map[string]any) error {
	if r.App == nil || r.App.Actions[name] == nil {
		return fmt.Errorf("call(): action %q not found", name)
	}
	if r.callDepth >= maxInvokeDepth {
		return fmt.Errorf("call(): invoke depth exceeded (max %d)", maxInvokeDepth)
	}
	r.Dispatch(name, args)
	if r.LastScriptError != "" {
		return fmt.Errorf("call(%q): %s", name, r.LastScriptError)
	}
	return nil
}

func (r *Runtime) applySteps(steps []model.Step, ctx map[string]any, depth int) {
	if depth > maxIfDepth {
		return
	}
	for i, step := range steps {
		if step.Type == "delay" && r.deferRest(step, steps[i+1:], ctx, depth) {
			return
		}
		r.applyStep(step, ctx, depth)
	}
}

// deferRest hands a `delay` step's wait to the host's background sink and
// schedules `rest` — the steps that follow it in the same list — as the
// continuation. It reports whether it took ownership of them.
//
// It declines (false) when there is no host sink, when `ms` is not positive, or
// when the runtime is already at maxInflight. Declining is not an error: the
// caller then runs `rest` immediately, so the action still reaches the same
// final state and the only casualty is the pause. That is the same portability
// rule `render` and `async` follow, and it is what keeps an offline render, an
// MCP simulation and `qorm render` instantaneous instead of sleeping through
// every animation an app declares.
//
// The pause is never a Sleep on the dispatching goroutine. On the server that
// would hold the mutex serialising every request; on the single-threaded WASM
// host it would freeze the UI outright.
func (r *Runtime) deferRest(step model.Step, rest []model.Step, ctx map[string]any, depth int) bool {
	if r.Async == nil || step.DelayMS <= 0 || r.inflight >= maxInflight {
		return false
	}
	// Same context split as an async request: `{{ arg }}` keeps the value the
	// dispatch carried, `{{ state.x }}` is read when the wait expires.
	frozen := freezeCtx(r.State, ctx)
	budget := r.budget
	wait := time.Duration(step.DelayMS) * time.Millisecond
	r.inflight++
	r.Async(
		func() any { time.Sleep(wait); return nil },
		func(any) {
			r.releaseInflight()
			// A resumed tail EXTENDS the interaction that scheduled it: it draws
			// from the same frame budget (restored here — other dispatches may
			// have replaced r.budget meanwhile) and runs as a nested dispatch
			// level, so an `invoke` from the tail cannot mint a fresh budget.
			// It must still republish the derived values itself, because it
			// lands outside any Dispatch.
			r.budget = budget
			r.callDepth++
			r.applySteps(rest, freshCtx(r, frozen), depth)
			r.callDepth--
			r.refreshComputed()
		})
	return true
}

// branchCtx returns ctx plus one extra binding (e.g. "response" or "error")
// for a nested branch, without mutating the caller's context.
func branchCtx(ctx map[string]any, key string, val any) map[string]any {
	out := make(map[string]any, len(ctx)+1)
	for k, v := range ctx {
		out[k] = v
	}
	out[key] = val
	return out
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

func (r *Runtime) applyStep(step model.Step, ctx map[string]any, depth int) {
	// The computed namespace is DERIVED: it is republished wholesale at every
	// frame boundary, so a step that writes into it would be both overwritten
	// and misleading. Such a step is dropped here and reported by the loader as
	// an error. The check costs nothing for an app that declares no computed
	// values — where "computed" is just an ordinary state key and keeps working.
	if len(r.App.Computed) > 0 &&
		(model.IsComputedPath(step.Path) || model.IsComputedPath(step.Result) ||
			model.IsComputedPath(step.Error) || model.IsComputedPath(step.Pending)) {
		return
	}
	switch step.Type {
	case "delay":
		// Reached only when applySteps could NOT defer the rest of the list —
		// no host background sink, a missing/non-positive `ms`, or the
		// in-flight cap. The wait then degrades to nothing (the following steps
		// have already run, or are about to) rather than blocking the dispatch,
		// which on the server would hold the mutex every request queues behind
		// and on the single-threaded WASM host would freeze the app.
		//
		// The case is here, not only in applySteps, because this switch IS the
		// step vocabulary: the API reference extracts it (see
		// internal/integration/apiref_doc_test.go stepTypes), so a step handled
		// exclusively elsewhere would be undocumented.
	case "render":
		// Publish the state written so far as a frame, mid-action. This is what
		// makes a loading state visible: `state.set saving=true` -> `render` ->
		// a slow `http.*` -> `state.set saving=false`. Without a host frame sink
		// it is a no-op, so the same JSON runs unchanged on every host.
		if r.Commit != nil && r.budget != nil && r.budget.left > 0 {
			r.budget.left--
			// An intermediate frame is a real frame: refresh the derived values
			// so the loading state it shows carries a consistent computed view
			// (the dispatch boundary is still to come).
			r.refreshComputed()
			r.Commit()
		}
	case "if":
		// Conditional branch: the condition is a {{...}} expression evaluated
		// with the expression language's own truthiness (expr.Truthy), so it
		// branches exactly like a `{{ cond ? a : b }}`. Branches nest up to
		// maxIfDepth (applySteps enforces it).
		if expr.Truthy(EvalBinding(step.Condition, ctx)) {
			r.applySteps(step.Then, ctx, depth+1)
		} else {
			r.applySteps(step.Else, ctx, depth+1)
		}
	case "forEach":
		// Bulk step: run the body once per element of `in`, with the element
		// bound under the item alias. See applyForEach.
		r.applyForEach(step, ctx, depth)
	case "invoke":
		// Action-calls-action: evaluate the args in the CALLER's context and
		// dispatch the target action with them — the same semantics as an
		// event invoke's args. Dispatch's callDepth guard breaks recursion.
		args := make(map[string]any, len(step.Args))
		for k, e := range step.Args {
			args[k] = EvalBinding(e, ctx)
		}
		r.Dispatch(step.Name, args)
	case "navigate":
		if step.Back {
			r.NavigateBack()
		} else {
			r.Navigate(Stringify(EvalBinding(step.To, ctx)), evalParams(step.Params, ctx))
		}
	case "state.set":
		setPath(r.State, step.Path, EvalBinding(step.Value, ctx))
	case "state.setAt":
		// state.setAt writes ONE array element: path names the list, index an
		// expression yielding the position (board-cell writes in games).
		if arr, ok := getPath(r.State, step.Path).([]any); ok {
			if f, ok := EvalBinding(step.Index, ctx).(float64); ok {
				i := int(f)
				if i >= 0 && i < len(arr) {
					arr[i] = EvalBinding(step.Value, ctx)
					setPath(r.State, step.Path, arr)
				}
			}
		}
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
		r.applyHTTP(step, ctx, depth)
	}
}

// reservedScopeAliases are evaluation-context roots a `forEach` alias must
// never shadow: binding the element over one of them would break every
// {{state.x}} (etc.) inside the loop body. MUST mirror the renderer's set of
// the same name (internal/render/render_data.go) — the runtime cannot import
// the renderer (the renderer imports the runtime), and a `forEach` alias and a
// renderItem alias that resolved differently would be two rules to learn where
// the JSON shows one. TestForEachAliasNamesMatchRenderer pins the parity.
var reservedScopeAliases = map[string]bool{
	"state": true, "t": true, "viewport": true, "route": true, "prop": true,
}

// listAliasNames resolves a `forEach` step's `as` value to the four scope names
// its body binds: the element alias plus the derived index/first/last keys. The
// default alias keeps the short built-in names (`index`, `first`, `last`); a
// custom alias namespaces them (`as: "row"` → `rowIndex`, `rowFirst`,
// `rowLast`) so a nested loop keeps the outer loop's bindings visible. An `as`
// that is reserved or not a plain identifier falls back to the default names
// (the loader warns about it at load time). Mirrors render.ListAliasNames.
func listAliasNames(as string) (alias, idxKey, firstKey, lastKey string) {
	if as == "" || as == "item" || reservedScopeAliases[as] || !isIdent(as) {
		return "item", "index", "first", "last"
	}
	return as, as + "Index", as + "First", as + "Last"
}

// isIdent reports whether s is a plain identifier ([A-Za-z_][A-Za-z0-9_]*) —
// the only names the expression language can reference.
func isIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || (i > 0 && '0' <= c && c <= '9') {
			continue
		}
		return false
	}
	return s != ""
}

// applyForEach runs a `forEach` step: the body once per element of the `in`
// collection, with the element bound under the alias plus index/first/last —
// the same four names a list's renderItem template binds, resolved by the same
// rules, so "how do I read the current element" has one answer across the whole
// format.
//
// Guards, all of them deliberate:
//   - a non-array `in` (nil, a number, an object, an unevaluatable expression)
//     iterates zero times rather than guessing an iteration for it;
//   - the body nests under the SAME depth budget as `if` (applySteps enforces
//     maxIfDepth on depth+1), so loops-inside-branches-inside-loops terminate;
//   - `invoke` inside the body still goes through Dispatch's call-depth guard;
//   - the element count is capped at maxForEachIterations.
//
// The bound element is the LIVE value from state, so a body step that mutates
// the array it iterates (state.updateWhere over the same path) sees its own
// writes — the ordinary Go slice-aliasing semantics every other step already
// has. `last` is computed against the collection's REAL length, so it is simply
// never true in a run that hit the iteration cap.
func (r *Runtime) applyForEach(step model.Step, ctx map[string]any, depth int) {
	items, _ := EvalBinding(step.In, ctx).([]any)
	if len(items) == 0 {
		return
	}
	alias, idxKey, firstKey, lastKey := listAliasNames(step.As)
	n := len(items)
	if n > maxForEachIterations {
		n = maxForEachIterations
	}
	for i := 0; i < n; i++ {
		loop := make(map[string]any, len(ctx)+4)
		for k, v := range ctx {
			loop[k] = v
		}
		// Written after the copy, so the innermost loop wins a name collision
		// with an enclosing one — same precedence as nested renderItem scopes.
		loop[alias] = items[i]
		loop[idxKey] = float64(i)
		loop[firstKey] = i == 0
		loop[lastKey] = i == len(items)-1
		r.applySteps(step.Steps, loop, depth+1)
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
// is discarded and the status text is written to Error.
//
// Result branches: after the classic Result/Error writes, the optional
// OnSuccess steps run with the decoded response bound as `{{ response }}`
// (whatever Result stored — or would have stored when Result is unset), and
// the optional OnError steps run with the failure message bound as
// `{{ error }}`. A `render` step inside either branch publishes a frame at
// that point via the host's Commit sink.
//
// Two execution modes, selected by the step's Async field:
//
//   - Synchronous (the default, and the only mode when the host installed no
//     Async sink): the round trip blocks the dispatching goroutine and the
//     branch runs inline in the caller's context, so this call does not return
//     until the branch has finished and the state is readable the moment
//     Dispatch returns.
//   - Async ({"async": true} — or a host that set AsyncAll — AND a host sink):
//     the request is built here — so
//     a malformed URL still fails synchronously, before anything is handed off
//     — and only the round trip plus the continuation move to the host's
//     background sink. applyHTTP returns immediately and the step's siblings
//     run while the request is still open; the continuation later writes
//     Result/Error and runs the branch under the host's lock. It sees the LIVE
//     state (whatever the rest of the action, and any event since, has written)
//     but the FROZEN action args and handler scope, so `{{ state.x }}` reads
//     "now" while `{{ someArg }}` still reads the value the click carried.
//
// Three governance fields shape either mode (see model.Step for their exact
// contracts): Timeout bounds the round trip, Pending holds a state flag true
// for its duration, and Key gives the request a slot that a newer request on
// the same key takes over — cancelling the older transport and discarding its
// continuation outright.
func (r *Runtime) applyHTTP(step model.Step, ctx map[string]any, depth int) {
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
		// A request that cannot even be built never reaches the async sink: the
		// failure is reported inline, exactly as before, so a typo'd URL keeps
		// behaving identically in both modes.
		r.settleHTTP(step, resultPath, ctx, depth, httpOutcome{errMsg: err.Error()})
		return
	}
	for k, v := range step.Headers {
		req.Header.Set(k, expr.Stringify(EvalBinding(v, ctx)))
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	background := (step.Async || r.AsyncAll) && r.Async != nil
	if background && r.inflight >= maxInflight {
		// The cap is checked BEFORE the request is handed off, so a refused
		// step neither opens a connection nor raises the count it was refused
		// by. It takes the error path inline, like an unbuildable request.
		r.settleHTTP(step, resultPath, ctx, depth, httpOutcome{errMsg: errTooManyInflight})
		return
	}
	// One context governs both the deadline and the supersede-cancel, because
	// both are the same operation to the transport: stop this round trip. It is
	// created even when neither field is set (an always-valid cancel to release
	// on completion keeps the two paths one shape rather than two).
	reqCtx, cancel := context.Background(), context.CancelFunc(nil)
	if step.TimeoutMS > 0 {
		reqCtx, cancel = context.WithTimeout(reqCtx, time.Duration(step.TimeoutMS)*time.Millisecond)
	} else {
		reqCtx, cancel = context.WithCancel(reqCtx)
	}
	req = req.WithContext(reqCtx)
	r.holdPending(step.Pending)
	if !background {
		defer cancel()
		out := doRequest(req, step.TimeoutMS)
		r.releasePending(step.Pending)
		r.settleHTTP(step, resultPath, ctx, depth, out)
		return
	}
	// A keyed request takes over its slot here, in the dispatch — so by the
	// time this call returns, the request it superseded can no longer write
	// state no matter how its own round trip ends.
	slot := r.claimKey(step.Key, cancel)
	frozen := freezeCtx(r.State, ctx)
	budget := r.budget
	r.inflight++
	r.Async(
		// Background half: the request is already built, so this closure
		// reads nothing off the runtime and needs no lock. cancel is released
		// here rather than in the continuation, so a runtime the host retires
		// mid-flight (whose continuation is therefore never called) still
		// gives its timer back.
		func() any { defer cancel(); return doRequest(req, step.TimeoutMS) },
		// Continuation: the host calls this holding its dispatch lock.
		func(res any) {
			r.releaseInflight()
			superseded := r.releaseKey(step.Key, slot)
			r.releasePending(step.Pending)
			// The continuation is its own dispatch boundary: it lands outside
			// the Dispatch that started the request, so the derived values must
			// be republished here too — including on the superseded path, whose
			// pending release is itself a state write.
			defer r.refreshComputed()
			if superseded {
				// A newer request owns this key. Writing the older reply now
				// would be the exact bug `key` exists to prevent: the second
				// keystroke's results replaced by the first keystroke's.
				return
			}
			out, _ := res.(httpOutcome)
			// A completion EXTENDS the interaction that launched the request: it
			// draws from the same frame budget (restored here — other dispatches
			// may have replaced r.budget meanwhile) and runs as a nested dispatch
			// level, so an `invoke` from a result branch cannot mint a fresh
			// budget either. One click is bounded to MaxFrames intermediate
			// frames however many round trips it spans.
			r.budget = budget
			r.callDepth++
			r.settleHTTP(step, resultPath, freshCtx(r, frozen), 0, out)
			r.callDepth--
		})
}

// claimKey gives a launching request the slot named by key, superseding
// whatever request held it: that one's transport is cancelled and its slot is
// marked dropped, which is the decision its continuation will read. Returns the
// new slot, or nil for an unkeyed request (which supersedes nothing and is
// never superseded). Caller holds the host's dispatch lock.
func (r *Runtime) claimKey(key string, cancel context.CancelFunc) *asyncSlot {
	if key == "" {
		return nil
	}
	if prev := r.keyed[key]; prev != nil {
		prev.dropped = true
		prev.cancel()
	}
	slot := &asyncSlot{cancel: cancel}
	if r.keyed == nil {
		r.keyed = map[string]*asyncSlot{}
	}
	r.keyed[key] = slot
	return slot
}

// releaseKey retires a finished request's slot and reports whether it was
// superseded while it was open (in which case its outcome must be discarded).
// The map entry is only cleared when this request still OWNS the slot: a
// superseded request must not evict its successor. Caller holds the lock.
func (r *Runtime) releaseKey(key string, slot *asyncSlot) bool {
	if slot == nil {
		return false
	}
	if r.keyed[key] == slot {
		delete(r.keyed, key)
	}
	return slot.dropped
}

// holdPending raises a `pending` state path for one request: the first holder
// writes true, later ones only add a reference. Caller holds the lock.
func (r *Runtime) holdPending(path string) {
	if path == "" {
		return
	}
	if r.pendingRefs == nil {
		r.pendingRefs = map[string]int{}
	}
	r.pendingRefs[path]++
	setPath(r.State, path, true)
}

// releasePending drops one request's reference to a `pending` state path,
// writing false only once the last holder is gone — so a superseded request
// cannot switch off the spinner its successor is still keeping on, and two
// overlapping requests on one flag behave like one. Caller holds the lock.
func (r *Runtime) releasePending(path string) {
	if path == "" {
		return
	}
	if n := r.pendingRefs[path]; n > 1 {
		r.pendingRefs[path] = n - 1
		return
	}
	delete(r.pendingRefs, path)
	setPath(r.State, path, false)
}

// httpOutcome is a finished request normalised into the three things the
// continuation needs: it is the only value that crosses the boundary between
// the (lock-free, possibly background) request and the (locked) state writes.
type httpOutcome struct {
	response any    // decoded body on success, raw string when it is not JSON
	errMsg   string // transport error or non-2xx status text
	ok       bool
}

// doRequest performs an already-built request and normalises the result. It
// deliberately closes over no runtime state, which is what makes it safe to run
// on a background goroutine while the dispatch that started it carries on.
//
// timeoutMS is passed only to phrase the deadline failure. Go reports it as
// `Get "https://…": context deadline exceeded (Client.Timeout exceeded …)`,
// which names an implementation detail the app author never wrote; the step
// declared a timeout, so the message says exactly that, identically on every
// host and stable enough for an app to match on.
func doRequest(req *http.Request, timeoutMS int) httpOutcome {
	resp, err := httpClient.Do(req)
	if err != nil {
		if timeoutMS > 0 && errors.Is(err, context.DeadlineExceeded) {
			return httpOutcome{errMsg: fmt.Sprintf("request timed out after %dms", timeoutMS)}
		}
		return httpOutcome{errMsg: err.Error()}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpOutcome{errMsg: resp.Status} // record the status, never the body
	}
	var response any
	if json.Unmarshal(data, &response) != nil {
		response = string(data) // non-JSON body → raw text
	}
	return httpOutcome{response: response, ok: true}
}

// settleHTTP applies a finished request to state: the classic Result/Error
// writes first, then the matching result branch. Both execution modes funnel
// through it, so async and synchronous requests land identically — the only
// difference is which goroutine and which context they run in.
func (r *Runtime) settleHTTP(step model.Step, resultPath string, ctx map[string]any, depth int, out httpOutcome) {
	if !out.ok {
		if step.Error != "" {
			setPath(r.State, step.Error, out.errMsg) // classic error-path write, kept first
		}
		if len(step.OnError) > 0 {
			r.applySteps(step.OnError, branchCtx(ctx, "error", out.errMsg), depth+1)
		}
		return
	}
	if resultPath != "" {
		setPath(r.State, resultPath, out.response)
	}
	if step.Error != "" {
		setPath(r.State, step.Error, "") // clear stale error on success
	}
	if len(step.OnSuccess) > 0 {
		r.applySteps(step.OnSuccess, branchCtx(ctx, "response", out.response), depth+1)
	}
}

// freezeCtx captures the part of an action context that must survive until an
// async request completes: everything that is NOT a live view of the runtime —
// the action's args, the handler's captured list-item scope (item/index/first/
// last, which reach the action as args), an enclosing branch's `response` /
// `error`. Net effect: `{{ someArg }}` still means what it meant when the
// button was pressed, while `{{ state.x }}` means what it means now.
//
// The split is decided by NAME: state owns every name it declares, so `state`,
// `t`, `viewport` and every top-level state key are re-read live, and an arg
// that collides with a state key is read live too. The alternative — remembering
// which names the args occupied — would make `{{ draft }}` mean "dispatch-time"
// or "live" depending on an argument list written in a different file, which is
// not a rule an author (or an agent) can apply by reading the action.
func freezeCtx(state, ctx map[string]any) map[string]any {
	frozen := make(map[string]any, len(ctx))
	for k, v := range ctx {
		if k == "state" || k == "t" || k == "viewport" {
			continue
		}
		if _, live := state[k]; live {
			continue
		}
		frozen[k] = v
	}
	return frozen
}

// freshCtx rebuilds an action context for a continuation: live state (and its
// flattened top-level keys), live catalog and viewport, with the frozen
// dispatch-time bindings layered on top — the same precedence Dispatch uses
// when it merges args over state.
func freshCtx(r *Runtime, frozen map[string]any) map[string]any {
	ctx := r.bareCtx(r.State)
	for k, v := range frozen {
		if isReservedRoot(k) { // freezeCtx never captures one; belt and braces
			continue
		}
		ctx[k] = v
	}
	return ctx
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
