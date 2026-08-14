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
	"fmt"
	"testing"

	"github.com/qorm/platform/internal/model"
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
	// return rather than spin — and it must not settle on either member of the
	// cycle, since both are scenes a guard just refused. With no back stack and
	// a refused entry scene there is nowhere left to go, so the runtime parks on
	// the blocked scene: nothing renders.
	app := guardApp()
	app.Entry = "dashboard"
	app.SceneGuards["login"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "dashboard"}
	rt := New(app)
	rt.RunPendingEnter() // must terminate
	if !rt.Blocked() {
		t.Errorf("scene = %q, want the blocked scene: both members of the cycle were refused", rt.Scene)
	}
}

// refusingApps returns one app per shape of an OUTRIGHT guard refusal — the
// three ways guardResolve reports ok=false. Each app's entry scene is the
// protected one and declares an onEnter that would load private data, so a
// refusal that leaks shows up twice: in the scene on screen and in the state.
// Each entry is a BUILDER, so every subtest gets a pristine app and can retune
// the entry scene without leaking into the next one.
func refusingApps() map[string]func() (app *model.App, protected string) {
	// The protected scene always declares the hook a leak would fire.
	withLoader := func(app *model.App, protected string) (*model.App, string) {
		app.SceneEnter = map[string]*model.Invoke{protected: {Name: "loadSecrets"}}
		app.Actions["loadSecrets"] = &model.Action{ID: "loadSecrets", Steps: []model.Step{
			{Type: "state.set", Path: "secrets", Value: "{{ 'ssn-and-salaries' }}"},
		}}
		app.Entry = protected
		return app, protected
	}
	return map[string]func() (*model.App, string){
		"guard without a redirect": func() (*model.App, string) {
			app := guardApp()
			app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
			return withLoader(app, "dashboard")
		},
		// The most natural login page there is: the protected scene sends you to
		// login, and login sends you back once you are signed in.
		"redirect cycle": func() (*model.App, string) {
			app := guardApp()
			app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "login"}
			app.SceneGuards["login"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "dashboard"}
			return withLoader(app, "dashboard")
		},
		// A chain of DISTINCT scenes longer than maxGuardRedirects: nothing
		// repeats, so it is the length cap that refuses it.
		"redirect chain too long": func() (*model.App, string) {
			app := guardApp()
			for i := 0; i <= maxGuardRedirects+1; i++ {
				id := fmt.Sprintf("hop%d", i)
				app.Scenes[id] = &model.Node{Type: "scaffold"}
				app.SceneGuards[id] = &model.SceneGuard{
					Condition: "{{ state.user != null }}",
					Redirect:  fmt.Sprintf("hop%d", i+1),
				}
			}
			return withLoader(app, "hop0")
		},
	}
}

func TestGuardRefusalBlocksEntry(t *testing.T) {
	// THE fail-open bug: on Navigate an ok=false guard means "stay where you
	// are", which is safe — but on the entry path "where you are" IS the scene
	// the guard just refused. Falling through rendered it in full AND fired its
	// onEnter. All three refusal shapes must instead leave, and leave before the
	// hook runs.
	for name, newApp := range refusingApps() {
		t.Run(name, func(t *testing.T) {
			app, protected := newApp()
			rt := New(app)
			rt.RunPendingEnter()

			if rt.sameScene(rt.Scene, protected) {
				t.Errorf("scene = %q: the refused scene is still on screen", rt.Scene)
			}
			if got := rt.State["secrets"]; got != nil {
				t.Errorf("the refused scene's onEnter ran: state.secrets = %v", got)
			}
			// Nothing on the back stack, and the entry scene is the refused one,
			// so there is nowhere at all this session may be: render nothing.
			if !rt.Blocked() {
				t.Errorf("scene = %q, want the blocked scene (no permitted frame exists)", rt.Scene)
			}
			// A blocked runtime is not addressable: a URL naming it would deep-link
			// straight back into a state the guard produced.
			if p := rt.RoutePath(); p != "/" {
				t.Errorf("RoutePath() = %q, want / while blocked", p)
			}
		})
	}
}

func TestGuardRefusalRetreatsToThePermittedFrame(t *testing.T) {
	// Same refusal, but the session has somewhere to fall back to. The host put
	// the runtime on the protected scene directly — a restored session, a
	// hot-reload carrying the scene across, an OTA activate — which is exactly
	// the case RunPendingEnter exists to police.
	for name, newApp := range refusingApps() {
		t.Run(name+" (back stack)", func(t *testing.T) {
			app, protected := newApp()
			app.Entry = "home" // a public entry, so the stack has a real frame
			app.SceneEnter["home"] = &model.Invoke{Name: "markHome"}
			app.Actions["markHome"] = &model.Action{ID: "markHome", Steps: []model.Step{
				{Type: "state.set", Path: "sawHome", Value: "{{ true }}"},
			}}
			rt := New(app)
			rt.Scene = protected
			rt.NavStack = []navFrame{{Scene: "home"}}
			rt.pendingEnter = true
			rt.RunPendingEnter()

			if !rt.sameScene(rt.Scene, "home") {
				t.Errorf("scene = %q, want home — the nearest frame the guards admit", rt.Scene)
			}
			if got := rt.State["secrets"]; got != nil {
				t.Errorf("the refused scene's onEnter ran: state.secrets = %v", got)
			}
			if rt.State["sawHome"] != true {
				t.Error("the scene actually entered must run its own onEnter")
			}
			// The frame we retreated to is consumed, not left as a place Back
			// could walk into the refusal from again.
			if len(rt.NavStack) != 0 {
				t.Errorf("back stack = %#v, want empty after a retreat", rt.NavStack)
			}
		})

		t.Run(name+" (entry fallback)", func(t *testing.T) {
			app, protected := newApp()
			app.Entry = "home"
			rt := New(app)
			rt.Scene = protected // no history at all: only the entry is left
			rt.pendingEnter = true
			rt.RunPendingEnter()

			if !rt.sameScene(rt.Scene, "home") {
				t.Errorf("scene = %q, want the entry scene", rt.Scene)
			}
			if got := rt.State["secrets"]; got != nil {
				t.Errorf("the refused scene's onEnter ran: state.secrets = %v", got)
			}
		})
	}
}

func TestEntryRedirectWithoutParamsDropsTheRefusedOnes(t *testing.T) {
	// A host put the runtime on a protected scene with route params (a deep link
	// carrying an id), and the guard's redirect declares none of its own. The
	// redirect target must be entered with an EMPTY route, never with the params
	// of the navigation it just refused.
	app := guardApp()
	app.Entry = "dashboard"
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "login"}
	rt := New(app)
	rt.RouteParams = map[string]any{"accountId": "acct-42"}
	rt.RunPendingEnter()

	if rt.Scene != "login" {
		t.Fatalf("scene = %q, want login", rt.Scene)
	}
	if len(rt.RouteParams) != 0 {
		t.Errorf("route params = %#v, want empty: the refused navigation's params must not travel", rt.RouteParams)
	}
}

func TestBlockedRuntimeIsNeverSomewhereBackReturnsTo(t *testing.T) {
	// A blocked runtime is the ABSENCE of a permitted scene, not a place the
	// user visited: navigating out of it must not leave a frame that Back could
	// walk into, which would put the refusal back on screen one tap later.
	app := guardApp()
	app.Entry = "dashboard"
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
	rt := New(app)
	rt.RunPendingEnter()
	if !rt.Blocked() {
		t.Fatalf("setup: scene = %q, want blocked", rt.Scene)
	}

	rt.Navigate("login", nil)
	if rt.Scene != "login" {
		t.Fatalf("scene = %q, want login: a permitted navigation must still work while blocked", rt.Scene)
	}
	if len(rt.NavStack) != 0 {
		t.Errorf("back stack = %#v, want empty: the blocked scene must never be pushed", rt.NavStack)
	}
	rt.NavigateBack()
	if rt.Blocked() {
		t.Error("Back returned to the blocked scene")
	}
}

func TestRetreatSkipsEveryFrameTheGuardsRefuse(t *testing.T) {
	// More than one frame on the stack is no longer permitted: the retreat keeps
	// unwinding rather than stopping at the first refusal it meets.
	app := guardApp()
	app.Scenes["reports"] = &model.Node{Type: "scaffold"}
	app.SceneGuards["dashboard"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
	app.SceneGuards["reports"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
	rt := New(app)
	rt.Scene = "dashboard"
	rt.NavStack = []navFrame{{Scene: "home"}, {Scene: "reports"}}
	rt.pendingEnter = true
	rt.RunPendingEnter()

	if !rt.sameScene(rt.Scene, "home") {
		t.Errorf("scene = %q, want home — the only frame the guards still admit", rt.Scene)
	}
	if len(rt.NavStack) != 0 {
		t.Errorf("back stack = %#v, want empty", rt.NavStack)
	}
}

func TestNavigateBackIsGuarded(t *testing.T) {
	// The revocation timing the audit reproduced, with no race at all: enter the
	// protected scene legitimately, navigate on, lose the permission, then tap
	// Back. Before the fix `navigate back` was the one entry path that never
	// consulted a guard, so the protected scene rendered in full — and its
	// handlers stayed live for several revisions afterwards.
	session := func(t *testing.T, guard *model.SceneGuard) *Runtime {
		t.Helper()
		app := guardApp()
		app.SceneGuards["dashboard"] = guard
		app.SceneEnter = map[string]*model.Invoke{"dashboard": {Name: "loadSecrets"}}
		app.Actions["loadSecrets"] = &model.Action{ID: "loadSecrets", Steps: []model.Step{
			{Type: "state.set", Path: "secrets", Value: "{{ 'ssn-and-salaries' }}"},
		}}
		app.Actions["goHome"] = &model.Action{ID: "goHome", Steps: []model.Step{{Type: "navigate", To: "home"}}}
		app.Actions["back"] = &model.Action{ID: "back", Steps: []model.Step{{Type: "navigate", Back: true}}}
		rt := New(app)
		rt.Dispatch("login", nil)
		rt.Dispatch("open", nil) // home -> dashboard, legitimately
		rt.RunPendingEnter()
		if rt.Scene != "dashboard" || rt.State["secrets"] == nil {
			t.Fatalf("setup: scene=%q secrets=%v — the authorised visit must work", rt.Scene, rt.State["secrets"])
		}
		rt.Dispatch("goHome", nil) // stack is now [home, dashboard]
		rt.RunPendingEnter()
		rt.State["secrets"] = nil  // forget the legitimate load
		rt.Dispatch("logout", nil) // the permission is revoked
		return rt
	}

	t.Run("diverted to the redirect target", func(t *testing.T) {
		rt := session(t, &model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "login"})
		rt.Dispatch("back", nil)
		rt.RunPendingEnter()
		if rt.Scene != "login" {
			t.Errorf("scene = %q, want login — Back into a now-guarded scene diverts like any other entry", rt.Scene)
		}
		if got := rt.State["secrets"]; got != nil {
			t.Errorf("the guarded scene's onEnter ran on the way back: %v", got)
		}
	})

	t.Run("refused outright, unwinds further", func(t *testing.T) {
		rt := session(t, &model.SceneGuard{Condition: "{{ state.user != null }}"})
		rt.Dispatch("back", nil)
		rt.RunPendingEnter()
		if rt.sameScene(rt.Scene, "dashboard") {
			t.Errorf("scene = %q: Back walked straight into the refused scene", rt.Scene)
		}
		if !rt.sameScene(rt.Scene, "home") {
			t.Errorf("scene = %q, want home — the next frame down that may be entered", rt.Scene)
		}
		if got := rt.State["secrets"]; got != nil {
			t.Errorf("the guarded scene's onEnter ran on the way back: %v", got)
		}
		if len(rt.NavStack) != 0 {
			t.Errorf("back stack = %#v: the frames Back unwound past must not remain", rt.NavStack)
		}
	})

	t.Run("nothing permitted leaves the runtime put", func(t *testing.T) {
		// Every frame below is refused too: Back cannot find anywhere to go, so
		// it stays on the scene the user is already permitted to see rather than
		// inventing a destination.
		rt := session(t, &model.SceneGuard{Condition: "{{ state.user != null }}"})
		rt.App.SceneGuards["home"] = &model.SceneGuard{Condition: "{{ state.user != null }}"}
		rt.Dispatch("back", nil)
		if !rt.sameScene(rt.Scene, "home") {
			t.Errorf("scene = %q, want to stay on home", rt.Scene)
		}
		if rt.NavDir == "pop" {
			t.Error("a Back that went nowhere must not claim a pop transition")
		}
	})

	t.Run("an unguarded Back is unchanged", func(t *testing.T) {
		rt := New(navApp())
		rt.Dispatch("openProfile", map[string]any{"userId": "u-1"})
		rt.NavigateBack()
		if rt.Scene != "" || rt.NavDir != "pop" {
			t.Errorf("plain Back regressed: scene=%q dir=%q", rt.Scene, rt.NavDir)
		}
	})
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
