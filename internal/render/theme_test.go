package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func TestTokenCSS(t *testing.T) {
	tokens := map[string]model.DesignToken{
		"color.primary": {Type: "color", Value: "#0a84ff"},
		"space.card":    {Type: "size", Value: "16px"},
	}
	got := TokenCSS("#qorm-stage", tokens)
	want := `#qorm-stage { --qorm-token-color-primary:#0a84ff; --qorm-token-space-card:16px; }`
	if got != want {
		t.Errorf("TokenCSS = %q, want %q", got, want)
	}
	// empty → nothing
	if got := TokenCSS("#qorm-stage", nil); got != "" {
		t.Errorf("TokenCSS(nil) = %q, want empty", got)
	}
	// names are normalized, values cannot break out of the block
	evil := map[string]model.DesignToken{
		`a;b"><script>`: {Type: "color", Value: "red;}body{display:none}"},
	}
	got = TokenCSS("#qorm-stage", evil)
	want = `#qorm-stage { --qorm-token-a-b---script-:redbodydisplay:none; }`
	if got != want {
		t.Errorf("TokenCSS sanitize = %q, want %q", got, want)
	}
}

func TestThemeVarsFor(t *testing.T) {
	for _, name := range []string{"", "apple", "auto"} {
		if v := ThemeVarsFor(name); v != themeVarsApple {
			t.Errorf("ThemeVarsFor(%q) should be the Apple palette", name)
		}
	}
	if v := ThemeVarsFor("material"); v != themeVarsMaterial {
		t.Error("ThemeVarsFor(material) should be the Material palette")
	}
	if v := ThemeVarsFor("dark"); v != themeVarsDark {
		t.Error("ThemeVarsFor(dark) should be the dark palette")
	}
}

// A designToken value is emitted into the shell's <style> block, so it is a
// CSS input like any other: a fetch function there phones home as soon as a
// scene reads the variable, and an unterminated comment eats the rest of the
// block. Stripping ; { } < > stops a token from ending its own declaration but
// does neither, so the value goes through the shared fetch/comment predicate.
func TestTokenCSSRejectsFetchAndComment(t *testing.T) {
	hostile := map[string]model.DesignToken{
		"beacon":  {Type: "color", Value: "url(//attacker.example/beacon.png)"},
		"upper":   {Type: "color", Value: "URL(//attacker.example/b.png)"},
		"imgset":  {Type: "color", Value: "image-set(//attacker.example/b.png 1x)"},
		"srcfn":   {Type: "color", Value: "src(//attacker.example/b.png)"},
		"comment": {Type: "color", Value: "#fff/*"},
	}
	got := TokenCSS("#qorm-stage", hostile)
	for _, bad := range []string{"url(", "URL(", "image-set(", "src(", "/*", "attacker.example"} {
		if strings.Contains(got, bad) {
			t.Errorf("token CSS carries %q:\n%s", bad, got)
		}
	}

	// Legitimate values must survive untouched, including the function forms a
	// theme really uses.
	legit := map[string]model.DesignToken{
		"primary": {Type: "color", Value: "#007aff"},
		"mix":     {Type: "color", Value: "color-mix(in srgb, #007aff 60%, white)"},
		"grad":    {Type: "color", Value: "linear-gradient(180deg, #fff, #eee)"},
		"space":   {Type: "size", Value: "calc(8px + 2vw)"},
		"font":    {Type: "font", Value: "-apple-system, 'SF Pro Text', sans-serif"},
	}
	out := TokenCSS("#qorm-stage", legit)
	for _, want := range []string{"#007aff", "color-mix(in srgb, #007aff 60%, white)", "linear-gradient(180deg, #fff, #eee)", "calc(8px + 2vw)", "-apple-system, 'SF Pro Text', sans-serif"} {
		if !strings.Contains(out, want) {
			t.Errorf("legitimate token value %q was altered:\n%s", want, out)
		}
	}
}
