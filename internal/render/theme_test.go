package render

import (
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
