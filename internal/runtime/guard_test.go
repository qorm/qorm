package runtime

// Runtime behaviour of SCENE ROUTE GUARDS.
//
// A guard is the precondition for entering a scene. The property that makes it
// worth having — as opposed to an `if` at the top of every navigating action —
// is that it holds on EVERY entry path: an action's `navigate` step, a browser
// Back/Forward (NavigateTo), a deep link (NavigateToPath), and the entry scene
// itself (RunPendingEnter). These tests drive all four.
//
// The second property is termination. Guards chain (the redirect target is
// itself guarded) and a redirect lands on a scene whose onEnter may navigate
// again, so the tests below pin both caps: a redirect chain that revisits a
// scene refuses the navigation, and a guard/onEnter ping-pong stops.

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// guardApp is a three-scene app: a public home, a dashboard that requires a
// logged-in user, and the login scene the guard diverts to (carrying where the
// user was trying to go as a route param).
func guardApp() *model.App {
	return &model.App{
		Entry: "home",
		Scenes: map[string]*model.Node{
			"home":      {Type: "scaffold"},
			"dashboard": {Type: "scaffold"},
			"login":     {Type: "scaffold"},
		},
		GlobalState: model.GlobalState{Initial: map[string]any{"user": nil}},
		SceneGuards: map[string]*model.SceneGuard{
			"dashboard": {
				Condition: "{{ state.user != null }}",
				Redirect:  "login",
				Params:    map[string]string{"next": "{{ 'dashboard' }}"},
			},
		},
		Actions: map[string]*model.Action{
			"open":   {ID: "open", Steps: []model.Step{{Type: "navigate", To: "dashboard"}}},
			"login":  {ID: "login", Steps: []model.Step{{Type: "state.set", Path: "user", Value: "{{ 'ada' }}"}}},
			"logout": {ID: "logout", Steps: []model.Step{{Type: "state.set", Path: "user", Value: "{{ null }}"}}},
		},
	}
}

func TestGuardDivertsNavigateStep(t *testing.T) {
	rt := New(guardApp())
	rt.Dispatch("open", nil)
	if rt.Scene != "login" {
		t.Fatalf("scene = %q, want login (the guard must divert the navigate step)", rt.Scene)
	}
	if rt.RouteParams["next"] != "dashboard" {
		t.Errorf("redirect route params = %#v, want next=dashboard", rt.RouteParams)
	}
	// The back stack records where we CAME FROM, never the refused scene: Back
	// must not drop the user onto a page they were just denied.
	if len(rt.NavStack) != 1 || !rt.sameScene(rt.NavStack[0].Scene, "home") {
		t.Errorf("back stack = %#v, want a single home frame", rt.NavStack)
	}

	// Satisfy the condition and the very same navigation goes through.
	rt.Dispatch("login", nil)
	rt.Dispatch("open", nil)
	if rt.Scene != "dashboard" {
		t.Errorf("scene = %q, want dashboard once the guard passes", rt.Scene)
	}
}

func TestGuardProtectsDeepLinkAndBrowserNavigation(t *testing.T) {
	// A deep link straight into a protected scene is the case a per-action
	// `if` cannot cover at all.
	rt := New(guardApp())
	rt.NavigateToPath("scene=dashboard&tab=stats")
	if rt.Scene != "login" {
		t.Fatalf("deep link scene = %q, want login", rt.Scene)
	}
	if _, leaked := rt.RouteParams["tab"]; leaked {
		t.Errorf("the refused navigation's own params leaked into the redirect: %#v", rt.RouteParams)
	}

	// NavigateTo (browser Back/Forward) is guarded on the same terms.
	rt2 := New(guardApp())
	rt2.NavigateTo("dashboard", map[string]any{"tab": "stats"})
	if rt2.Scene != "login" {
		t.Errorf("NavigateTo scene = %q, want login", rt2.Scene)
	}

	// And it stops being a diversion the moment the condition holds.
	rt2.Dispatch("login", nil)
	rt3 := New(guardApp())
	rt3.State["user"] = "ada"
	rt3.NavigateToPath("scene=dashboard&tab=stats")
	if rt3.Scene != "dashboard" || rt3.RouteParams["tab"] != "stats" {
		t.Errorf("authorised deep link: scene=%q params=%#v", rt3.Scene, rt3.RouteParams)
	}
}

func TestGuardProtectsTheEntryScene(t *testing.T) {
	// The entry scene is entered by nobody's navigate step, so it can only be
	// protected at the render choke point — where onEnter is also drained.
	app := guardApp()
	app.Entry = "dashboard"
	rt := New(app)
	rt.RunPendingEnter()
	if rt.Scene != "login" {
		t.Fatalf("entry scene = %q, want the guard's login redirect", rt.Scene)
	}
	if rt.RouteParams["next"] != "dashboard" {
		t.Errorf("entry redirect params = %#v, want next=dashboard", rt.RouteParams)
	}
	// A redirect REPLACES the entry frame rather than pushing one: the refused
	// scene must not become somewhere Back can return to.
	if len(rt.NavStack) != 0 {
		t.Errorf("back stack = %#v, want empty after an entry redirect", rt.NavStack)
	}
}

func TestGuardRunsBeforeOnEnter(t *testing.T) {
	// A guard that fires must prevent the guarded scene's onEnter from running
	// at all — otherwise a "load the private data" hook fires for a user who
	// was just refused the page.
	app := guardApp()
	app.Entry = "dashboard"
	app.SceneEnter = map[string]*model.Invoke{
		"dashboard": {Name: "markPrivate"},
		"login":     {Name: "markLogin"},
	}
	app.Actions["markPrivate"] = &model.Action{ID: "markPrivate", Steps: []model.Step{
		{Type: "state.set", Path: "leaked", Value: "{{ true }}"},
	}}
	app.Actions["markLogin"] = &model.Action{ID: "markLogin", Steps: []model.Step{
		{Type: "state.set", Path: "sawLogin", Value: "{{ true }}"},
	}}
	rt := New(app)
	rt.RunPendingEnter()
	if rt.State["leaked"] == true {
		t.Error("the guarded scene's onEnter ran despite the guard refusing entry")
	}
	if rt.State["sawLogin"] != true {
		t.Error("the redirect target's onEnter must run: it is the scene actually entered")
	}
}

func TestGuardChainsThroughSeveralScenes(t *testing.T) {
	// The redirect target is itself guarded, so protections compose: dashboard
	// needs a user, login needs the app to be initialised, and an uninitialised
	// visitor lands on splash.
	app := guardApp()
	app.Scenes["splash"] = &model.Node{Type: "scaffold"}
	app.SceneGuards["login"] = &model.SceneGuard{Condition: "{{ state.ready }}", Redirect: "splash"}
	rt := New(app)
	rt.Dispatch("open", nil)
	if rt.Scene != "splash" {
		t.Fatalf("scene = %q, want splash (dashboard -> login -> splash)", rt.Scene)
	}
	rt.State["ready"] = true
	rt2 := New(app)
	rt2.State["ready"] = true
	rt2.Dispatch("open", nil)
	if rt2.Scene != "login" {
		t.Errorf("scene = %q, want login once splash's precondition holds", rt2.Scene)
	}
}

func TestGuardRedirectCycleRefusesTheNavigation(t *testing.T) {
	// Two guards that redirect to each other: the navigation is refused (the
	// runtime stays put) instead of ping-ponging. This is the boundary the
	// loader also warns about at load time.
	app := guardApp()
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "login"}
	app.SceneGuards["login"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "dashboard"}
	rt := New(app)
	rt.Dispatch("open", nil)
	if !rt.sameScene(rt.Scene, "home") {
		t.Errorf("scene = %q, want home — a cyclic redirect must refuse the navigation, not loop", rt.Scene)
	}
	if len(rt.NavStack) != 0 {
		t.Errorf("a refused navigation must not touch the back stack: %#v", rt.NavStack)
	}
}

func TestGuardEntryCycleTerminates(t *testing.T) {
	// The same cycle reached through the ENTRY path: RunPendingEnter must
	// return rather than spin. guardResolve refuses, so the scene stays where
	// the host put it (nothing better exists at entry, and the loader flags the
	// configuration).
	app := guardApp()
	app.Entry = "dashboard"
	app.SceneGuards["login"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "dashboard"}
	rt := New(app)
	rt.RunPendingEnter() // must terminate
	if rt.Scene != "" && rt.Scene != "dashboard" && rt.Scene != "login" {
		t.Errorf("scene = %q, want one of the two cycle members", rt.Scene)
	}
}

func TestGuardWithoutRedirectRefusesNavigation(t *testing.T) {
	app := guardApp()
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
	rt := New(app)
	rt.Dispatch("open", nil)
	if !rt.sameScene(rt.Scene, "home") {
		t.Errorf("scene = %q, want home — a redirect-less guard refuses the navigation", rt.Scene)
	}
	rt.Dispatch("login", nil)
	rt.Dispatch("open", nil)
	if rt.Scene != "dashboard" {
		t.Errorf("scene = %q, want dashboard once the condition holds", rt.Scene)
	}
}

func TestGuardRedirectToTheCurrentSceneIsANoOp(t *testing.T) {
	// Already on login, tapping a link to the protected scene: the guard sends
	// us back to login, which must NOT push a duplicate frame.
	rt := New(guardApp())
	rt.Navigate("login", nil)
	before := len(rt.NavStack)
	rt.Dispatch("open", nil)
	if rt.Scene != "login" {
		t.Fatalf("scene = %q, want login", rt.Scene)
	}
	if len(rt.NavStack) != before {
		t.Errorf("back stack grew from a self-redirect: %d -> %d", before, len(rt.NavStack))
	}
}

func TestGuardsAreInertWithoutDeclarations(t *testing.T) {
	// The whole feature is skipped for an app that declares no guards, so
	// navigation behaves exactly as it did before guards existed.
	rt := New(navApp())
	rt.Dispatch("openProfile", map[string]any{"userId": "u-1"})
	if rt.Scene != "profile" || rt.RouteParams["userId"] != "u-1" {
		t.Errorf("unguarded navigation changed: scene=%q params=%#v", rt.Scene, rt.RouteParams)
	}
}

func TestGuardSeesStateWrittenEarlierInTheSameAction(t *testing.T) {
	// The sign-in shape: one action writes the credential and then navigates.
	// A guard on plain state.* sees the write because state is live; a guard on
	// a DERIVED value must agree with it, even though the published derived view
	// only turns over at frame boundaries. If it did not, the two spellings of
	// the same condition would behave differently — the surprise this test
	// exists to prevent.
	// A guard is evaluated in SCENE scope, so a derived value is read there the
	// way a scene binding reads it: state-rooted. (The bare `computed.x`
	// spelling belongs to action scope, which flattens top-level state keys.)
	for _, condition := range []string{
		"{{ state.user != null }}",
		"{{ state.computed.signedIn }}",
	} {
		app := guardApp()
		app.Computed = map[string]string{"signedIn": "{{ state.user != null }}"}
		app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: condition, Redirect: "login"}
		app.Actions["signInAndGo"] = &model.Action{ID: "signInAndGo", Steps: []model.Step{
			{Type: "state.set", Path: "user", Value: "{{ 'ada' }}"},
			{Type: "navigate", To: "dashboard"},
		}}
		rt := New(app)
		rt.Dispatch("signInAndGo", nil)
		if rt.Scene != "dashboard" {
			t.Errorf("condition %q: scene = %q, want dashboard", condition, rt.Scene)
		}
	}
}

func TestGuardRefreshDoesNotLeakIntoTheDispatchView(t *testing.T) {
	// Guards re-derive privately: the PUBLISHED namespace must still only turn
	// over at the frame boundary, so the steps after a navigate keep reading the
	// same derived view as the steps before it.
	app := guardApp()
	app.Computed = map[string]string{"signedIn": "{{ state.user != null }}"}
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.computed.signedIn }}", Redirect: "login"}
	app.Actions["signInAndGo"] = &model.Action{ID: "signInAndGo", Steps: []model.Step{
		{Type: "state.set", Path: "user", Value: "{{ 'ada' }}"},
		{Type: "navigate", To: "dashboard"},
		{Type: "state.set", Path: "seen", Value: "{{ computed.signedIn }}"},
	}}
	rt := New(app)
	rt.Dispatch("signInAndGo", nil)
	if rt.State["seen"] != false {
		t.Errorf("post-navigate read = %v, want the frame-stable false — the guard's private refresh must not republish", rt.State["seen"])
	}
	if got := rt.ComputedVars()["signedIn"]; got != true {
		t.Errorf("published value after the dispatch = %v, want true", got)
	}
}

func TestGuardConditionReadsComputed(t *testing.T) {
	// Guards evaluate in scene context, so a derived value is a legal — and
	// natural — way to express the precondition once.
	app := guardApp()
	app.Computed = map[string]string{"signedIn": "{{ state.user != null }}"}
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.computed.signedIn }}", Redirect: "login"}
	rt := New(app)
	rt.Dispatch("open", nil)
	if rt.Scene != "login" {
		t.Fatalf("scene = %q, want login", rt.Scene)
	}
	rt.Dispatch("login", nil)
	rt.Dispatch("open", nil)
	if rt.Scene != "dashboard" {
		t.Errorf("scene = %q, want dashboard once the derived condition flips", rt.Scene)
	}
}
