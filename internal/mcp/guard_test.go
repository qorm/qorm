package mcp

// What an MCP TOOL RESULT is allowed to contain.
//
// The agent surface renders the live app straight into a tool result, which
// makes every render point a publication: whatever HTML lands here has left the
// runtime. Two properties are pinned below.
//
//   - A scene a route guard refuses is never in it. The guard used to run AFTER
//     the render — the host drained it in its afterMutate, one step too late —
//     so a single qorm_dispatch whose action navigated into a protected scene
//     answered with that scene's full HTML, and fired its onEnter besides. No
//     forged token, no race, one call.
//   - A derived value is never written through it. qorm_set_state assigned into
//     the state map directly, which walked past both the loader's refusal and
//     the runtime's, and then reported the forged namespace back as if it were
//     derived.
//
// Assertions are made on the rendered HTML and on state the app itself can
// read, not on runtime internals: what matters is what the agent is handed.

import (
	"strings"
	"sync"
	"testing"

	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

const (
	publicMarker = "PUBLIC-LOBBY"
	secretMarker = "TOP-SECRET-SALARIES"
)

// guardedApp is a two-scene app: a public lobby and a vault whose guard admits
// only a signed-in user. The vault's onEnter loads the private data, so a leak
// shows up twice — in the HTML and in the state the tool returns beside it.
func guardedApp(guard *model.SceneGuard) *model.App {
	return &model.App{
		Entry: "lobby",
		Scenes: map[string]*model.Node{
			"lobby": {Type: "column", ID: "lobby_root", Children: []*model.Node{
				{Type: "text", ID: "lobby_txt", Text: publicMarker},
			}},
			"vault": {Type: "column", ID: "vault_root", Children: []*model.Node{
				{Type: "text", ID: "vault_txt", Text: secretMarker},
			}},
			"login": {Type: "column", ID: "login_root", Children: []*model.Node{
				{Type: "text", ID: "login_txt", Text: "SIGN-IN"},
			}},
		},
		GlobalState: model.GlobalState{Initial: map[string]any{"user": nil}},
		SceneGuards: map[string]*model.SceneGuard{"vault": guard},
		SceneEnter:  map[string]*model.Invoke{"vault": {Name: "loadSecrets"}},
		Actions: map[string]*model.Action{
			"open": {ID: "open", Steps: []model.Step{{Type: "navigate", To: "vault"}}},
			// Enters the vault while still permitted and drops the permission
			// before the dispatch ends — the ordering that put a guarded scene
			// into a tool result with no race at all.
			"openThenSignOut": {ID: "openThenSignOut", Steps: []model.Step{
				{Type: "navigate", To: "vault"},
				{Type: "state.set", Path: "user", Value: "{{ null }}"},
			}},
			"login": {ID: "login", Steps: []model.Step{{Type: "state.set", Path: "user", Value: "{{ 'ada' }}"}}},
			"loadSecrets": {ID: "loadSecrets", Steps: []model.Step{
				{Type: "state.set", Path: "payroll", Value: "{{ 'every salary' }}"},
			}},
		},
	}
}

func guardedServer(app *model.App) *Server {
	return &Server{rt: qrt.New(app), mu: &sync.Mutex{}}
}

func TestMCPResultsAreGuardResolved(t *testing.T) {
	// Both refusal shapes: a guard with no redirect, and the most ordinary login
	// wiring there is — the vault sends you to login, login sends you back once
	// you are signed in — which guardResolve refuses as a cycle.
	guards := map[string]*model.SceneGuard{
		"redirect to login":  {Condition: "{{ state.user != null }}", Redirect: "login"},
		"no redirect at all": {Condition: "{{ state.user != null }}"},
	}
	for name, guard := range guards {
		t.Run(name, func(t *testing.T) {
			s := guardedServer(guardedApp(guard))
			s.rt.State["user"] = "ada" // signed in when the navigation starts

			// One dispatch: it enters the vault, then signs the user out. The
			// scene is marked entered but no longer permitted, and the guard
			// that notices runs at the render choke point — which the tool used
			// to render ahead of.
			res := resultObj(t, toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "openThenSignOut"}))
			html, _ := res["html"].(string)
			if strings.Contains(html, secretMarker) {
				t.Errorf("qorm_dispatch handed the agent the guarded scene:\n%s", html)
			}
			state, _ := res["state"].(map[string]any)
			if state["payroll"] != nil {
				t.Errorf("the guarded scene's onEnter ran: payroll = %v", state["payroll"])
			}

			// Every other render point answers the same way.
			if h := resultText(t, toolCallRPC(t, s, "qorm_render_html", map[string]any{})); strings.Contains(h, secretMarker) {
				t.Errorf("qorm_render_html leaked the guarded scene:\n%s", h)
			}
			checks := []any{map[string]any{"kind": "htmlContains", "text": secretMarker}}
			a := resultObj(t, toolCallRPC(t, s, "qorm_assert", map[string]any{"checks": checks}))
			if a["pass"] != false {
				t.Errorf("qorm_assert saw the guarded scene: %v", a["checks"])
			}
		})
	}
}

func TestMCPRendersTheGuardedSceneOnceItIsAllowed(t *testing.T) {
	// The control for the test above: the same navigation, permitted. Without
	// this, "the HTML never contains the marker" would also hold if the tools
	// had simply stopped rendering the current scene.
	s := guardedServer(guardedApp(&model.SceneGuard{Condition: "{{ state.user != null }}", Redirect: "login"}))
	toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "login"})
	res := resultObj(t, toolCallRPC(t, s, "qorm_dispatch", map[string]any{"action": "open"}))
	html, _ := res["html"].(string)
	if !strings.Contains(html, secretMarker) {
		t.Errorf("an authorised visit must render the scene:\n%s", html)
	}
	state, _ := res["state"].(map[string]any)
	if state["payroll"] != "every salary" {
		t.Errorf("and must run its onEnter: payroll = %v", state["payroll"])
	}
	if h := resultText(t, toolCallRPC(t, s, "qorm_render_html", map[string]any{})); !strings.Contains(h, secretMarker) {
		t.Errorf("qorm_render_html must follow the session's scene:\n%s", h)
	}
}

func TestMCPRendersNothingWhenEveryEntryIsRefused(t *testing.T) {
	// The entry scene is the protected one and nothing else may be entered, so
	// the runtime is blocked. Falling back to the entry scene — which is what a
	// renderer does with a scene id it does not know — would render exactly the
	// scene the guard just refused.
	app := guardedApp(&model.SceneGuard{Condition: "{{ state.user != null }}"})
	app.Entry = "vault"
	s := guardedServer(app)

	html := resultText(t, toolCallRPC(t, s, "qorm_render_html", map[string]any{}))
	if strings.Contains(html, secretMarker) {
		t.Errorf("a blocked runtime rendered the refused entry scene:\n%s", html)
	}
	if !strings.Contains(html, "no scene available") {
		t.Errorf("a blocked runtime must render an explicit empty frame, got:\n%s", html)
	}
	if got := s.rt.State["payroll"]; got != nil {
		t.Errorf("the refused entry scene's onEnter ran: %v", got)
	}
}

func TestSetStateRefusesComputedNamespace(t *testing.T) {
	computedApp := func() *model.App {
		app := guardedApp(&model.SceneGuard{Condition: "{{ true }}"})
		app.Computed = map[string]string{"greeting": "{{ 'hi ' + state.name }}"}
		app.GlobalState.Initial = map[string]any{"name": "ada"}
		return app
	}

	t.Run("the namespace is read-only", func(t *testing.T) {
		s := guardedServer(computedApp())
		for _, path := range []string{"computed.greeting", "computed"} {
			requireToolErr(t, toolCallRPC(t, s, "qorm_set_state",
				map[string]any{"path": path, "value": "OWNED"}), "computed")
			// The refusal must also be true of what the tool would report next:
			// the derived view stays derived.
			if got := s.rt.ComputedVars()["greeting"]; got != "hi ada" {
				t.Errorf("path %q: derived value is now %v", path, got)
			}
		}
	})

	t.Run("computed stays an ordinary key without declarations", func(t *testing.T) {
		// An app that declares no computed values never had a reserved
		// namespace, so the refusal must not appear out of nowhere for it.
		s := guardedServer(guardedApp(&model.SceneGuard{Condition: "{{ true }}"}))
		res := resultObj(t, toolCallRPC(t, s, "qorm_set_state",
			map[string]any{"path": "computed", "value": "just a key"}))
		if state, _ := res["state"].(map[string]any); state["computed"] != "just a key" {
			t.Errorf("plain key write refused: %v", res["state"])
		}
	})

	t.Run("a dotted path nests", func(t *testing.T) {
		// The same spelling the state.set step uses. The raw assignment this
		// replaced created a literal "user.name" key, which no binding can read
		// — the value was written somewhere the app could never see it.
		app := guardedApp(&model.SceneGuard{Condition: "{{ true }}"})
		app.Scenes["lobby"].Children[0].Text = "hello {{ state.user.name }}"
		s := guardedServer(app)
		res := resultObj(t, toolCallRPC(t, s, "qorm_set_state",
			map[string]any{"path": "user.name", "value": "ada"}))
		user, _ := res["state"].(map[string]any)["user"].(map[string]any)
		if user["name"] != "ada" {
			t.Fatalf("dotted path did not nest: %#v", res["state"])
		}
		if html, _ := res["html"].(string); !strings.Contains(html, "hello ada") {
			t.Errorf("the app cannot read what the agent wrote:\n%s", html)
		}
		// And the matching read: qorm_assert resolves the same dotted path.
		a := resultObj(t, toolCallRPC(t, s, "qorm_assert", map[string]any{"checks": []any{
			map[string]any{"kind": "stateEquals", "path": "user.name", "value": "ada"},
		}}))
		if a["pass"] != true {
			t.Errorf("stateEquals must read the path it was written at: %v", a["checks"])
		}
	})
}
