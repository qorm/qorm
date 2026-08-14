package render_test

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render"
	qrt "github.com/qorm/platform/internal/runtime"
)

// blockedApp is an app whose only scene refuses every visitor: the guard's
// condition is false and it has no redirect, so the runtime has nowhere it may
// legally put the user.
func blockedApp() *model.App {
	secret := &model.Node{Type: "column", ID: "vault", Children: []*model.Node{
		{Type: "text", ID: "leak", Text: "TOP-SECRET-PAYROLL"},
	}}
	return &model.App{
		Entry:       "vault",
		GlobalState: model.GlobalState{Initial: map[string]any{"unlocked": false}},
		Scenes:      map[string]*model.Node{"vault": secret},
		SceneGuards: map[string]*model.SceneGuard{
			"vault": {Condition: "{{ state.unlocked }}"},
		},
	}
}

// RenderScene falls back to the entry scene for any scene id it does not know,
// and GuardBlocked is deliberately an id no scene map contains. Without an
// explicit check that fallback renders exactly the scene the guard refused —
// on the server, in WASM and in the playground alike, since all three call
// through here. The runtime-level refusal is only half the fix; this is the
// other half, and it is one line, so it needs a test that fails when the line
// goes away.
func TestBlockedRuntimeRendersNoScene(t *testing.T) {
	rt := qrt.New(blockedApp())
	rt.RunPendingEnter()

	if !rt.Blocked() {
		t.Fatalf("precondition: a guard refusing every entry must block the runtime, scene = %q", rt.CurrentScene())
	}

	for _, res := range []render.Result{
		render.Render(rt),
		render.RenderScene(rt, rt.CurrentScene()),
		render.RenderScene(rt, "vault"), // an explicit ask for the refused scene
	} {
		if strings.Contains(res.HTML, "TOP-SECRET-PAYROLL") {
			t.Errorf("the refused scene's content reached the HTML:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, `id="vault"`) {
			t.Errorf("the refused scene's root reached the HTML:\n%s", res.HTML)
		}
		if len(res.Handlers) != 0 {
			t.Errorf("a blocked render registered %d handler(s) — a client could dispatch them", len(res.Handlers))
		}
	}
}

// The positive control: once the guard admits the visitor the same scene must
// render normally, so the assertions above cannot pass by rendering nothing
// under every condition.
func TestPermittedRuntimeStillRendersTheScene(t *testing.T) {
	app := blockedApp()
	rt := qrt.New(app)
	rt.State["unlocked"] = true
	rt.RunPendingEnter()

	if rt.Blocked() {
		t.Fatalf("an admitted visitor must not be blocked")
	}
	if html := render.Render(rt).HTML; !strings.Contains(html, "TOP-SECRET-PAYROLL") {
		t.Errorf("the permitted scene did not render:\n%s", html)
	}
}
