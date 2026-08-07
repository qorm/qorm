package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"

	"github.com/qorm/qorm/internal/anim"
)

// Theme represents a complete UI styling skin
type Theme struct {
	Name       string                     `json:"name"`
	Colors     map[string]string          `json:"colors"`
	Components map[string]ComponentStyles `json:"components"`
	Motion     *Motion                    `json:"motion,omitempty"`

	// Precomputed for fast access
	ParsedColors map[string]color.RGBA `json:"-"`
}

// Motion defines the skin's animation tokens: shared durations (milliseconds)
// and named easing curves consumed by both render backends (canvas reads this
// struct directly; the HTML backend exposes matching --qorm-motion-* CSS vars).
type Motion struct {
	DurationFast     *int   `json:"durationFast,omitempty"`
	DurationNormal   *int   `json:"durationNormal,omitempty"`
	DurationSlow     *int   `json:"durationSlow,omitempty"`
	EasingStandard   string `json:"easingStandard,omitempty"`
	EasingEmphasized string `json:"easingEmphasized,omitempty"`
}

// Default motion token values used when a theme omits them.
const (
	DefaultDurationFast   = 120
	DefaultDurationNormal = 250
	DefaultDurationSlow   = 400
)

// DurationMs returns a named duration ("fast", "normal", "slow") in
// milliseconds. Nil-safe: a nil Theme or omitted field yields the defaults.
func (t *Theme) DurationMs(name string) int {
	if t != nil && t.Motion != nil {
		switch name {
		case "fast":
			if t.Motion.DurationFast != nil {
				return *t.Motion.DurationFast
			}
		case "normal":
			if t.Motion.DurationNormal != nil {
				return *t.Motion.DurationNormal
			}
		case "slow":
			if t.Motion.DurationSlow != nil {
				return *t.Motion.DurationSlow
			}
		}
	}
	switch name {
	case "fast":
		return DefaultDurationFast
	case "slow":
		return DefaultDurationSlow
	default:
		return DefaultDurationNormal
	}
}

// Easing returns a named curve ("standard" or "emphasized"). Nil-safe;
// unknown easing names never panic and fall back to anim.EaseOutCubic.
func (t *Theme) Easing(name string) anim.Curve {
	if t != nil && t.Motion != nil {
		easingName := t.Motion.EasingStandard
		if name == "emphasized" {
			easingName = t.Motion.EasingEmphasized
		}
		if c, ok := anim.CurveByName(easingName); ok {
			return c
		}
	}
	if name == "emphasized" {
		return anim.EaseInOutCubic
	}
	return anim.EaseOutCubic
}

// ComponentStyles defines default styles for a component type
type ComponentStyles struct {
	BorderRadius    *float64 `json:"borderRadius,omitempty"`
	Padding         *int     `json:"padding,omitempty"`
	Margin          *int     `json:"margin,omitempty"`
	BackgroundColor string   `json:"backgroundColor,omitempty"`
	Color           string   `json:"color,omitempty"`
	FontSize        *int     `json:"fontSize,omitempty"`
	FontWeight      *int     `json:"fontWeight,omitempty"`
	TextAlign       string   `json:"textAlign,omitempty"`
	Gap             *int     `json:"gap,omitempty"`
	StrokeColor     string   `json:"strokeColor,omitempty"`
	StrokeWidth     *float64 `json:"strokeWidth,omitempty"`
	BoxShadowColor  string   `json:"boxShadowColor,omitempty"`
	BoxShadowBlur   *int     `json:"boxShadowBlur,omitempty"`
	BoxShadowX      *int     `json:"boxShadowX,omitempty"`
	BoxShadowY      *int     `json:"boxShadowY,omitempty"`
	// Interactive state styles
	PressedBackgroundColor string   `json:"pressedBackgroundColor,omitempty"`
	HoveredBackgroundColor string   `json:"hoveredBackgroundColor,omitempty"`
	PressedOpacity         *float64 `json:"pressedOpacity,omitempty"`
}

// IsBuiltinTheme reports whether a theme name has a built-in CSS palette
// shipped in render.ThemeCSS (apple, dark, material, win11-light, win11-dark,
// and the "auto" alias). Custom themes loaded from themes/<name>.json return
// false here so the host knows it must inject the skin's colors itself.
func IsBuiltinTheme(name string) bool {
	switch name {
	case "", "auto", "apple", "apple-light", "apple-dark",
		"material", "win11-light", "win11-dark", "dark":
		return true
	}
	return false
}

// CSSClassRules renders a custom theme (loaded from themes/<name>.json, not
// matching any built-in) as a CSS rule that sets every color in the skin as a
// --<name> custom property on the stage. The HTML renderer stamps this block
// in addition to render.ThemeCSS so a scene's `background: var(--sky)` (etc.)
// resolves to the skin's actual color, not the empty string.
//
// Returns "" for a nil theme or one whose name is built-in (those palettes are
// already covered by render.ThemeCSS), so callers can always concatenate the
// result without conditional logic.
func (t *Theme) CSSClassRules() string {
	if t == nil || t.Name == "" || IsBuiltinTheme(t.Name) || len(t.Colors) == 0 {
		return ""
	}
	// Class names allow [a-z0-9-]; refuse anything else to keep the rule
	// well-formed even when the manifest author chose an exotic theme id.
	className := t.Name
	for _, r := range className {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return ""
		}
	}
	// Sort keys for a byte-stable page — the theme block shows up in the
	// /events broadcast's rev==0 frame, byte stability is what lets a small
	// page diff actually mean "nothing changed".
	keys := make([]string, 0, len(t.Colors))
	for k := range t.Colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(".qorm-theme-")
	sb.WriteString(className)
	sb.WriteString("{")
	for _, k := range keys {
		v := strings.TrimSpace(t.Colors[k])
		if v == "" {
			continue
		}
		// Allow only safe CSS values: hex, rgb(), rgba(), hsl(), and named
		// colors via the color: and color value lexer. Anything else
		// (selectors, semicolons, braces) would be a class-out.
		if !isSafeCSSColor(v) {
			continue
		}
		sb.WriteString("--")
		sb.WriteString(k)
		sb.WriteString(":")
		sb.WriteString(v)
		sb.WriteString(";")
	}
	sb.WriteString("}")
	return sb.String()
}

// isSafeCSSColor accepts the subset of CSS color values a theme author is
// likely to type: #RGB / #RRGGBB / #RRGGBBAA, rgb()/rgba()/hsl()/hsla() with
// number or percentage args, or a short list of named colors. Anything more
// exotic (gradients, currentColor) belongs on a node, not in a theme palette.
func isSafeCSSColor(s string) bool {
	if s == "" {
		return false
	}
	// hex
	if s[0] == '#' {
		hex := s[1:]
		switch len(hex) {
		case 3, 4, 6, 8:
			for _, r := range hex {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
					return false
				}
			}
			return true
		}
		return false
	}
	// functions rgb / rgba / hsl / hsla
	if s[0] == 'r' || s[0] == 'h' {
		open := strings.IndexByte(s, '(')
		if open <= 0 || s[len(s)-1] != ')' {
			return false
		}
		head := s[:open]
		switch head {
		case "rgb", "rgba", "hsl", "hsla":
			return true
		}
	}
	// a tiny named-color allowlist
	switch strings.ToLower(s) {
	case "transparent", "currentcolor", "inherit", "initial", "unset", "none":
		return true
	}
	return false
}

// LoadTheme loads a JSON theme file and parses its colors
func LoadTheme(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var t Theme
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	
	t.ParsedColors = make(map[string]color.RGBA)
	for k, v := range t.Colors {
		if c, ok := parseColor(v); ok {
			t.ParsedColors[k] = c
		}
	}
	
	return &t, nil
}

// GetColor resolves a color string. It first checks the theme palette,
// then tries to parse it as a hex string.
func (t *Theme) GetColor(val string) (color.RGBA, bool) {
	if val == "" {
		return color.RGBA{}, false
	}
	
	// Check Theme Palette
	if c, ok := t.ParsedColors[val]; ok {
		return c, true
	}
	
	// Fallback to literal hex parsing
	return parseColor(val)
}

// GetDefault returns a fallback default Apple HIG theme if loading fails
func GetDefault() *Theme {
	t := &Theme{
		Name: "apple-light",
		Colors: map[string]string{
			"primary":       "#007AFF",
			"selection":     "#007AFF99", // text-selection highlight (input/textarea)
			"secondary":     "#5856D6",
			"success":       "#34C759",
			"warning":       "#FF9500",
			"danger":        "#FF3B30",
			"background":    "#F5F5F7",
			"surface":       "#FFFFFFCC",
			"text":          "#1D1D1F",
			"textSecondary": "#86868B",
			"separator":     "#C6C6C8",
			"shadow":        "#0000001A",
			"shadowDeep":    "#00000033",
			"inputBg":       "#E8E8ED",
			"inputBorder":   "#C6C6C8",
			"cardBg":        "#FFFFFF",
			"focus":         "#007AFF",
		},
		Components: map[string]ComponentStyles{
			"scene": {
				BackgroundColor: "background",
				Color:           "text",
			},
			"column": {
				Color: "text",
			},
			"row": {
				Color: "text",
			},
			"button": {
				BorderRadius:           ptrFloat(12.0),
				Padding:                ptrInt(14),
				BackgroundColor:        "primary",
				Color:                  "#FFFFFF",
				FontSize:               ptrInt(16),
				FontWeight:             ptrInt(600),
				PressedBackgroundColor: "#0062CC",
				HoveredBackgroundColor: "#1A86FF",
				PressedOpacity:         ptrFloat(0.9),
			},
			"text": {
				Color:    "text",
				FontSize: ptrInt(16),
			},
			"input": {
				BorderRadius:    ptrFloat(10.0),
				Padding:         ptrInt(12),
				BackgroundColor: "inputBg",
				Color:           "text",
				FontSize:        ptrInt(16),
				StrokeColor:     "inputBorder",
				StrokeWidth:     ptrFloat(1.0),
			},
			"card": {
				BorderRadius:    ptrFloat(16.0),
				Padding:         ptrInt(16),
				BackgroundColor: "cardBg",
			},
			// Select gets the interactive pair only: the picker chrome owns
			// its base fill (white, or the author's background), the overlay
			// supplies the hover/press feedback on top of it.
			"select": {
				HoveredBackgroundColor: "inputBg",
				PressedBackgroundColor: "separator",
			},
		},
		Motion: &Motion{
			DurationFast:     ptrInt(DefaultDurationFast),
			DurationNormal:   ptrInt(DefaultDurationNormal),
			DurationSlow:     ptrInt(DefaultDurationSlow),
			EasingStandard:   "easeOutCubic",
			EasingEmphasized: "easeInOutCubic",
		},
		ParsedColors: make(map[string]color.RGBA),
	}
	
	for k, v := range t.Colors {
		if c, ok := parseColor(v); ok {
			t.ParsedColors[k] = c
		}
	}
	return t
}

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int { return &i }

// parseColor parses a hex string like #RRGGBBAA or #RRGGBB
func parseColor(hex string) (color.RGBA, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 6 {
		hex += "FF"
	}
	if len(hex) != 8 {
		return color.RGBA{}, false
	}
	
	var r, g, b, a uint8
	_, err := fmt.Sscanf(hex, "%02x%02x%02x%02x", &r, &g, &b, &a)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: r, G: g, B: b, A: a}, true
}
