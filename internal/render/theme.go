package render

import (
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/model"
)

// Built-in theme palettes — the single source of truth for QORM's design
// tokens (the CSS custom properties every widget's default styling is
// expressed in). internal/server injects them into the HTML shell,
// internal/miniapp maps them into WXSS — a palette change here lands on every
// target. Keep these in sync with the widget default styles in this package.
const (
	themeVarsApple = `--accent:#007aff; --on-accent:#fff; --success:#34c759; --danger:#ff3b30; --warning:#ff9500;
	    --bg:#f2f2f7; --surface:#fff; --label:#000; --label2:#3c3c4399; --sep:#3c3c4949;
	    --fill:#78788033; --radius:12px; --radius-lg:20px; --stage-radius:38px;
	    --font:-apple-system,BlinkMacSystemFont,'SF Pro Text','SF Pro Display','Helvetica Neue',Arial,sans-serif;`
	themeVarsMaterial = `--accent:#2e7df6; --on-accent:#fff; --success:#16a34a; --danger:#dc2626; --warning:#f59e0b;
	    --bg:#eef0f4; --surface:#fff; --label:#111827; --label2:#6b7280; --sep:#e5e7eb;
	    --fill:#e5e7eb; --radius:8px; --radius-lg:12px; --stage-radius:14px;
	    --font:'Segoe UI',Roboto,-apple-system,BlinkMacSystemFont,sans-serif;`
	themeVarsDark = `--accent:#0a84ff; --on-accent:#fff; --success:#30d158; --danger:#ff453a; --warning:#ff9f0a;
	    --bg:#000; --surface:#1c1c1e; --label:#fff; --label2:#ebebf599; --sep:#54545899;
	    --fill:#7676803d; --radius:12px; --radius-lg:20px; --stage-radius:38px;
	    --font:-apple-system,BlinkMacSystemFont,'SF Pro Text','SF Pro Display','Helvetica Neue',Arial,sans-serif;`
)

// ThemeVarsFor returns the custom-property declarations of a built-in theme.
// "", "apple" and "auto" all mean the default Cupertino palette.
func ThemeVarsFor(theme string) string {
	switch theme {
	case "material":
		return themeVarsMaterial
	case "dark":
		return themeVarsDark
	default:
		return themeVarsApple
	}
}

// ThemeCSS is the <style>-ready theme block for the HTML shell: the three
// named palettes plus "auto" — the theme a stage gets when neither the
// manifest nor state.theme picks one. "auto" is the Apple palette that
// follows the OS light/dark setting via prefers-color-scheme; an explicit
// theme (including theme:"apple") opts out of that tracking.
const ThemeCSS = `/* ---- Design tokens (themes). "auto" — the default when an app picks no
	   theme — follows the OS light/dark setting; a manifest theme or state.theme
	   opts out. Switch by class on the stage. ---- */
	  :root, .qorm-theme-apple, .qorm-theme-auto {
	    ` + themeVarsApple + ` color-scheme:light; }
	  .qorm-theme-material {
	    ` + themeVarsMaterial + ` color-scheme:light; }
	  .qorm-theme-dark {
	    ` + themeVarsDark + ` color-scheme:dark; }
	  @media (prefers-color-scheme: dark) {
	    .qorm-theme-auto {
	      ` + themeVarsDark + ` color-scheme:dark; }
	  }`

// TokenCSS renders an app's manifest designTokens as CSS custom properties on
// scope ("#qorm-stage" for the HTML shell, "page" for WXSS), so scenes can
// reference them as var(--qorm-token-<name>) — token "color.primary" becomes
// var(--qorm-token-color-primary). Keys are sorted for a byte-stable page;
// names and values are stripped of anything that could break out of the
// declaration block.
func TokenCSS(scope string, tokens map[string]model.DesignToken) string {
	if len(tokens) == 0 {
		return ""
	}
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(scope)
	b.WriteString(" {")
	for _, name := range names {
		b.WriteString(" --qorm-token-")
		b.WriteString(sanitizeTokenName(name))
		b.WriteString(":")
		b.WriteString(sanitizeTokenValue(tokens[name].Value))
		b.WriteString(";")
	}
	b.WriteString(" }")
	return b.String()
}

func sanitizeTokenName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

func sanitizeTokenValue(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ';', '{', '}', '<', '>', '\n', '\r':
			return -1
		default:
			return r
		}
	}, s)
}
