package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func TestEvalBindingTypedVsInterpolated(t *testing.T) {
	ctx := map[string]any{
		"count": float64(3),
		"name":  "Ada",
		"flag":  true,
		"obj":   map[string]any{"inner": float64(9)},
	}

	// A whole-string binding returns the typed value (float64, bool), not text.
	if got := EvalBinding("{{ count }}", ctx); got != float64(3) {
		t.Errorf("typed number: got %v (%T)", got, got)
	}
	if got := EvalBinding("{{ flag }}", ctx); got != true {
		t.Errorf("typed bool: got %v (%T)", got, got)
	}
	if got := EvalBinding("{{ obj.inner }}", ctx); got != float64(9) {
		t.Errorf("typed member: got %v (%T)", got, got)
	}

	// Surrounding whitespace still counts as a whole-string binding.
	if got := EvalBinding("  {{ count }}\t", ctx); got != float64(3) {
		t.Errorf("padded whole binding: got %v (%T)", got, got)
	}

	// An expression that fails to parse degrades to "" for a whole binding.
	if got := EvalBinding("{{ 1 + }}", ctx); got != "" {
		t.Errorf("broken whole binding: got %v", got)
	}

	// Mixed text + bindings interpolates into a string.
	if got := EvalBinding("Hi {{ name }}, n={{ count }}!", ctx); got != "Hi Ada, n=3!" {
		t.Errorf("interpolated: got %v", got)
	}

	// A broken binding inside interpolation expands to "" but the rest survives.
	if got := EvalBinding("a {{ 1 + }} b {{ name }}", ctx); got != "a  b Ada" {
		t.Errorf("interpolated with broken token: got %q", got)
	}

	// Multiple whole-looking bindings are NOT typed — it is interpolation.
	if got := EvalBinding("{{ count }}{{ count }}", ctx); got != "33" {
		t.Errorf("two adjacent bindings: got %v (%T)", got, got)
	}

	// A plain string without bindings is returned unchanged.
	if got := EvalBinding("no bindings here", ctx); got != "no bindings here" {
		t.Errorf("plain string: got %v", got)
	}

	// An unresolved identifier reads nil (stringifies to "" in interpolation).
	if got := EvalBinding("{{ nope.key }}", ctx); got != nil {
		t.Errorf("missing identifier: got %v", got)
	}
	if got := EvalBinding("v={{ nope.key }}", ctx); got != "v=" {
		t.Errorf("missing identifier interpolated: got %q", got)
	}
}

func TestEvalArgsInSceneContext(t *testing.T) {
	app := &model.App{
		Entry:       "home",
		Scenes:      map[string]*model.Node{"home": {}},
		GlobalState: model.GlobalState{Initial: map[string]any{"n": float64(2)}},
	}
	rt := New(app)
	rt.Viewport = Viewport{W: 1000, H: 800}
	rt.Navigate("home", nil) // entry scene, empty route
	rt.RouteParams = map[string]any{"userId": "u-1"}

	out := rt.EvalArgs(map[string]string{
		"doubled": "{{ state.n * 2 }}",    // state
		"user":    "{{ route.userId }}",   // route
		"wide":    "{{ viewport.width }}", // viewport
		"static":  "literal",              // plain
	})
	if out["doubled"] != float64(4) {
		t.Errorf("EvalArgs state: %v", out["doubled"])
	}
	if out["user"] != "u-1" {
		t.Errorf("EvalArgs route: %v", out["user"])
	}
	if out["wide"] != float64(1000) {
		t.Errorf("EvalArgs viewport: %v", out["wide"])
	}
	if out["static"] != "literal" {
		t.Errorf("EvalArgs literal: %v", out["static"])
	}
}

func TestCatalogOverlayAndFormatting(t *testing.T) {
	app := &model.App{
		DefaultLocale: "en",
		Locales: map[string]map[string]string{
			"en": {"greet": "Hello, {name}", "onlyDefault": "kept", "items": "{n, plural, one {# item} other {# items}}"},
			"fr": {"greet": "Bonjour, {name}"},
		},
	}
	rt := New(app)
	rt.State = map[string]any{"locale": "fr", "name": "Ada", "n": float64(2)}

	cat := rt.Catalog()
	// The current locale overlays the default; missing keys fall back.
	if cat["greet"] != "Bonjour, Ada" {
		t.Errorf("overlay greet: got %v", cat["greet"])
	}
	if cat["onlyDefault"] != "kept" {
		t.Errorf("fallback to default locale: got %v", cat["onlyDefault"])
	}
	// ICU forms expand against top-level state keys.
	if cat["items"] != "2 items" {
		t.Errorf("plural against state: got %v", cat["items"])
	}

	// A locale with no catalog of its own renders the default catalog.
	rt.State["locale"] = "de"
	if got := rt.Catalog()["greet"]; got != "Hello, Ada" {
		t.Errorf("untranslated locale should use default: got %v", got)
	}

	// An app with no default locale still expands the active catalog.
	app2 := &model.App{Locales: map[string]map[string]string{"en": {"hi": "Hi {who}"}}}
	rt2 := New(app2)
	rt2.State = map[string]any{"locale": "en", "who": "Bob"}
	if got := rt2.Catalog()["hi"]; got != "Hi Bob" {
		t.Errorf("no-default-locale catalog: got %v", got)
	}
}

func TestCurrentLocale(t *testing.T) {
	rt := &Runtime{App: &model.App{DefaultLocale: "en"}, State: map[string]any{}}
	if got := rt.CurrentLocale(); got != "en" {
		t.Errorf("fallback to default locale: got %q", got)
	}
	rt.State["locale"] = "fr"
	if got := rt.CurrentLocale(); got != "fr" {
		t.Errorf("state locale: got %q", got)
	}
	// Empty and non-string values fall back to the default.
	rt.State["locale"] = ""
	if got := rt.CurrentLocale(); got != "en" {
		t.Errorf("empty locale: got %q", got)
	}
	rt.State["locale"] = float64(7)
	if got := rt.CurrentLocale(); got != "en" {
		t.Errorf("non-string locale: got %q", got)
	}
}

func TestCurrentTheme(t *testing.T) {
	// No theme anywhere -> "auto".
	rt := &Runtime{App: &model.App{}, State: map[string]any{}}
	if got := rt.CurrentTheme(); got != "auto" {
		t.Errorf("default theme: got %q", got)
	}
	// Manifest theme applies when state has none.
	rt.App.Theme = "material"
	if got := rt.CurrentTheme(); got != "material" {
		t.Errorf("manifest theme: got %q", got)
	}
	// state.theme overrides the manifest.
	rt.State["theme"] = "dark"
	if got := rt.CurrentTheme(); got != "dark" {
		t.Errorf("state theme: got %q", got)
	}
	// An empty state theme falls back to the manifest.
	rt.State["theme"] = ""
	if got := rt.CurrentTheme(); got != "material" {
		t.Errorf("empty state theme: got %q", got)
	}
	// An explicit "auto" in state is a real value (returns "auto").
	rt.State["theme"] = "auto"
	if got := rt.CurrentTheme(); got != "auto" {
		t.Errorf("explicit auto: got %q", got)
	}
	// Nil app is tolerated.
	rt2 := &Runtime{State: map[string]any{}}
	if got := rt2.CurrentTheme(); got != "auto" {
		t.Errorf("nil app theme: got %q", got)
	}
}

func TestIsRTL(t *testing.T) {
	rtl := []string{"ar", "he", "fa", "ur", "ps", "sd", "ug", "yi", "dv", "ckb",
		"he-IL", "ar-EG", "fa_IR", "AR", "HE"}
	for _, loc := range rtl {
		if !IsRTL(loc) {
			t.Errorf("IsRTL(%q) = false, want true", loc)
		}
	}
	ltr := []string{"en", "de", "fr", "zh", "", "es-419", "english", "-ar"}
	for _, loc := range ltr {
		if IsRTL(loc) {
			t.Errorf("IsRTL(%q) = true, want false", loc)
		}
	}

	// The runtime method follows the active locale.
	rt := &Runtime{App: &model.App{DefaultLocale: "en"}, State: map[string]any{}}
	if rt.IsRTL() {
		t.Error("en runtime should be LTR")
	}
	rt.State["locale"] = "ar"
	if !rt.IsRTL() {
		t.Error("ar runtime should be RTL")
	}
}

func TestViewportVars(t *testing.T) {
	cases := []struct {
		vp     Viewport
		w, h   float64
		orient string
	}{
		{Viewport{}, 0, 0, ""}, // unknown
		{Viewport{W: 1024, H: 768}, 1024, 768, "landscape"},
		{Viewport{W: 375, H: 812}, 375, 812, "portrait"},
		{Viewport{W: 500, H: 500}, 500, 500, "landscape"}, // square counts as landscape
		{Viewport{W: 100, H: 0}, 100, 0, "landscape"},     // only width known
		{Viewport{W: 0, H: 100}, 0, 100, "portrait"},      // only height known
	}
	for _, c := range cases {
		rt := &Runtime{Viewport: c.vp}
		vars := rt.ViewportVars()
		if vars["width"] != c.w || vars["height"] != c.h || vars["orientation"] != c.orient {
			t.Errorf("ViewportVars(%+v) = %#v, want %g/%g/%q", c.vp, vars, c.w, c.h, c.orient)
		}
	}
}

func TestCurrentScene(t *testing.T) {
	rt := &Runtime{Scene: "profile"}
	if got := rt.CurrentScene(); got != "profile" {
		t.Errorf("CurrentScene = %q", got)
	}
	rt.Scene = ""
	if got := rt.CurrentScene(); got != "" {
		t.Errorf("CurrentScene entry = %q", got)
	}
}

func TestCloneIndependence(t *testing.T) {
	app := &model.App{
		Entry:       "home",
		Scenes:      map[string]*model.Node{"home": {}, "detail": {}},
		GlobalState: model.GlobalState{Initial: map[string]any{"count": float64(0)}},
		Actions: map[string]*model.Action{
			"inc": {ID: "inc", Steps: []model.Step{{Type: "state.increment", Path: "count"}}},
		},
	}
	live := New(app)
	live.State["user"] = map[string]any{"name": "Ada", "tags": []any{"a", "b"}}
	live.Viewport = Viewport{W: 800, H: 600}
	live.Navigate("detail", map[string]any{"id": "x-1"})

	sim := live.Clone()

	// Shared app pointer, copied scalars.
	if sim.App != live.App {
		t.Error("Clone must share the App pointer")
	}
	if sim.Scene != "detail" || sim.Viewport != (Viewport{W: 800, H: 600}) {
		t.Errorf("Clone scene/viewport: %q %+v", sim.Scene, sim.Viewport)
	}
	if sim.RouteParams["id"] != "x-1" {
		t.Errorf("Clone route params: %v", sim.RouteParams)
	}

	// Deep independence: nested state and route params must not alias.
	sim.State["user"].(map[string]any)["name"] = "Grace"
	sim.State["user"].(map[string]any)["tags"].([]any)[0] = "zzz"
	sim.RouteParams["id"] = "changed"
	if live.State["user"].(map[string]any)["name"] != "Ada" {
		t.Error("Clone state aliases the live nested map")
	}
	if live.State["user"].(map[string]any)["tags"].([]any)[0] != "a" {
		t.Error("Clone state aliases the live nested slice")
	}
	if live.RouteParams["id"] != "x-1" {
		t.Error("Clone route params alias the live frame")
	}

	// Dispatching on the clone does not touch live state.
	sim.Dispatch("inc", nil)
	sim.Dispatch("inc", nil)
	if sim.State["count"] != float64(2) {
		t.Errorf("clone dispatch: count = %v", sim.State["count"])
	}
	if live.State["count"] != float64(0) {
		t.Errorf("live state must not change from clone dispatch, count = %v", live.State["count"])
	}

	// Navigating on the clone leaves the live scene and stack alone. NOTE: Clone
	// does not copy NavStack, so the simulation starts with an empty back stack
	// and NavigateBack is a no-op there — see the bug note on Clone. This test
	// pins the current behavior.
	sim.NavigateBack()
	if sim.Scene != "detail" || len(sim.NavStack) != 0 {
		t.Errorf("clone should start with an empty back stack: scene=%q stack=%v", sim.Scene, sim.NavStack)
	}
	sim.Navigate("home", nil)
	if sim.Scene != "home" {
		t.Errorf("clone forward nav: %q", sim.Scene)
	}
	if live.Scene != "detail" || len(live.NavStack) != 1 {
		t.Errorf("live navigation state changed by clone: scene=%q stack=%d", live.Scene, len(live.NavStack))
	}
}

func TestNewSeedsInitialDeepCopied(t *testing.T) {
	initial := map[string]any{"cfg": map[string]any{"level": float64(1)}, "xs": []any{"a"}}
	app := &model.App{GlobalState: model.GlobalState{Initial: initial}}

	rt := New(app)
	// Mutating the seeded state must not corrupt the manifest's initial values.
	rt.State["cfg"].(map[string]any)["level"] = float64(99)
	rt.State["xs"].([]any)[0] = "zzz"
	if initial["cfg"].(map[string]any)["level"] != float64(1) {
		t.Error("New must deep-copy initial state (map)")
	}
	if initial["xs"].([]any)[0] != "a" {
		t.Error("New must deep-copy initial state (slice)")
	}
}
