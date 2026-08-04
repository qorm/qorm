package canvas

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// defaultVars are the fallback palette when no theme JSON is loaded.
// Keys match the theme color names (prefixed with --) so var(--primary)
// resolves correctly even without a theme file.
var defaultVars = map[string]color.RGBA{
	"--primary":       {0, 122, 255, 255},   // #007AFF
	"--secondary":     {88, 86, 214, 255},   // #5856D6
	"--background":    {245, 245, 247, 255}, // #F5F5F7
	"--surface":       {255, 255, 255, 204}, // #FFFFFFCC
	"--text":          {29, 29, 31, 255},    // #1D1D1F
	"--textSecondary": {134, 134, 139, 255}, // #86868B
	"--separator":     {198, 198, 200, 255}, // #C6C6C8
	"--shadow":        {0, 0, 0, 26},        // #0000001A
	"--cardBg":        {255, 255, 255, 255}, // #FFFFFF
	"--inputBg":       {232, 232, 237, 255}, // #E8E8ED
	// Legacy aliases for backward compatibility
	"--bg":        {245, 245, 247, 255},
	"--accent":    {0, 122, 255, 255},
	"--on-accent": {255, 255, 255, 255},
	"--label":     {29, 29, 31, 255},
	"--label2":    {134, 134, 139, 255},
	"--sep":       {198, 198, 200, 255},
}

func parseColor(c string) color.RGBA {
	c = strings.TrimSpace(c)
	if c == "" {
		return color.RGBA{0, 0, 0, 0}
	}

	if strings.HasPrefix(c, "var(") && strings.HasSuffix(c, ")") {
		varName := c[4 : len(c)-1]
		if col, ok := defaultVars[varName]; ok {
			return col
		}
		return color.RGBA{255, 0, 255, 255} // debug magenta
	}

	if strings.HasPrefix(c, "#") {
		hex := c[1:]
		switch len(hex) {
		case 3: // #RGB -> #RRGGBB
			r, _ := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
			g, _ := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
			b, _ := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
			return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
		case 6: // #RRGGBB
			r, _ := strconv.ParseUint(hex[0:2], 16, 8)
			g, _ := strconv.ParseUint(hex[2:4], 16, 8)
			b, _ := strconv.ParseUint(hex[4:6], 16, 8)
			return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
		case 8: // #RRGGBBAA
			r, _ := strconv.ParseUint(hex[0:2], 16, 8)
			g, _ := strconv.ParseUint(hex[2:4], 16, 8)
			b, _ := strconv.ParseUint(hex[4:6], 16, 8)
			a, _ := strconv.ParseUint(hex[6:8], 16, 8)
			return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
		}
	}
	// An unparseable colour must not paint (the old opaque-black fallback
	// turned any author typo — and every gradient — into a black box).
	return color.RGBA{0, 0, 0, 0}
}

type NodeStyle struct {
	Background  color.RGBA
	Color       color.RGBA
	Padding     int
	MarginTop   int
	MarginBot   int
	MarginLeft  int
	MarginRight int
	Gap         int

	BoxShadowColor color.RGBA
	BoxShadowBlur  int
	BoxShadowX     int
	BoxShadowY     int

	Width     int
	Height    int
	WidthRaw  string // "fill"
	HeightRaw string // "fill"
	// Min/MaxWidth/Height clamp the resolved size in measure (0 = unset),
	// mirroring the CSS box resolution order (content/explicit first, then
	// clamp).
	MinWidth, MaxWidth   int
	MinHeight, MaxHeight int
	// PosX/PosY hold an explicit absolute position (style keys x/y, or the
	// HTML aliases left/top): when either is authored (HasPos), performLayout
	// places the child at the container content-box origin + (PosX, PosY)
	// instead of flowing it — the coordinate model of an infinite-canvas
	// board. Out of flow, so it neither consumes flex space nor reflows
	// siblings.
	PosX, PosY   int
	HasPos       bool
	Align        string
	AlignSelf    string // CSS align-self (style/layout alignSelf) — distinct from Align (align-items)
	Justify      string
	FontSize     int
	FontWeight   int
	TextAlign    string
	BorderRadius float64
	Opacity      float64 // 1 = fully opaque; lowered by pressedOpacity theme state

	StrokeColor color.RGBA
	StrokeWidth float64

	// Declarative interaction effects (the interaction-effect resolver in
	// applyInteractiveOverlay + performLayout): any node can declare them, so
	// hover/press feedback is DATA, not per-widget hardcoded logic. 0/absent
	// means "no effect" (a pressed/hovered scale of 0 is meaningless).
	PressedScale float64 // scale the node to this factor while pressed
	HoverScale   float64 // scale while hovered (pressed wins)
	// Transition is the declarative CSS-style transition duration ("0.2s",
	// "200ms" or plain ms) that animates hover/press effect changes instead of
	// snapping them (the interaction resolver routes the style through the
	// tween engine while it is non-zero).
	Transition time.Duration
}

// scaleBy multiplies every pixel-valued field by f (a device-pixel ratio), so
// the layout produced from logical design values lands in physical pixels —
// the basis of crisp HiDPI rendering. f<=1 is a no-op (scale 1 == current
// behaviour, bit-for-bit). Unit-less fields (opacity, font weight, alignment)
// and colours are intentionally untouched.
func (s *NodeStyle) scaleBy(f int) {
	if f <= 1 {
		return
	}
	s.Padding *= f
	s.MarginTop *= f
	s.MarginBot *= f
	s.MarginLeft *= f
	s.MarginRight *= f
	s.Gap *= f
	s.BoxShadowBlur *= f
	s.BoxShadowX *= f
	s.BoxShadowY *= f
	s.Width *= f
	s.Height *= f
	s.PosX *= f
	s.PosY *= f
	s.FontSize *= f
	s.BorderRadius *= float64(f)
	s.StrokeWidth *= float64(f)
}

func evalStyleProp(val any, rt *runtime.Runtime, sc ...*listScope) any {
	var scope *listScope
	if len(sc) > 0 {
		scope = sc[0]
	}
	switch v := val.(type) {
	case string:
		if rt != nil {
			return runtime.EvalBinding(v, evalCtxScope(rt, scope))
		}
		return v
	case map[string]any:
		// Nested style objects (margin/padding) hold bindings in their INNER
		// values — margin:{top:"{{…}}"} — and the web path resolves those;
		// evaluate recursively (copying, never mutating the shared model) or a
		// bound margin silently collapses to 0.
		if rt == nil {
			return v
		}
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = evalStyleProp(item, rt, scope)
		}
		return out
	}
	return val
}

// applyInteractiveOverlay layers the theme ComponentStyles interactive-state
// fields (pressedBackgroundColor / hoveredBackgroundColor / pressedOpacity)
// over the resolved style when the node is Pressed/Hovered. Pressed wins over
// hovered for the background; opacity applies in either state. This is what
// makes the theme's interactive keys live — previously they were dead fields.
// parseCSSDuration parses a CSS-style duration ("0.2s", "200ms") or plain
// milliseconds.
func parseCSSDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "ms") {
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "ms")), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(f * float64(time.Millisecond)), nil
	}
	if strings.HasSuffix(s, "s") {
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "s")), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(f * float64(time.Second)), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(f * float64(time.Millisecond)), nil
}

// evalColorStyle reads a node's declarative interaction color key (hover/
// pressedBackground), evaluating bindings.
func evalColorStyle(n *model.Node, key string, rt *runtime.Runtime) (color.RGBA, bool) {
	v, ok := evalStyleProp(n.Style[key], rt).(string)
	if !ok {
		return color.RGBA{}, false
	}
	c := resolveColor(v, rt)
	return c, c.A > 0
}

// evalFloatStyle reads a node's declarative interaction float key (hover/
// pressedOpacity), evaluating bindings.
func evalFloatStyle(n *model.Node, key string, rt *runtime.Runtime) (float64, bool) {
	switch v := evalStyleProp(n.Style[key], rt).(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// applyInteractiveOverlay is the declarative interaction-effect resolver: any
// node can declare hover/press feedback (hoverBackground, pressedBackground,
// hoverOpacity, pressedOpacity) and the engine layers it over the base style
// here — effects are DATA, not per-widget hardcoded logic. The theme's
// component styles are the baseline; per-node declarations win; pressed is
// applied last so it beats hovered. The pressed/hover SCALE lands in
// performLayout (it is a graph transform, not a style field).
func applyInteractiveOverlay(s *NodeStyle, n *model.Node, rt *runtime.Runtime, inter *Interaction) {
	if inter == nil || rt == nil || rt.Theme == nil || (inter.Pressed != n && inter.Hovered != n) {
		return
	}
	if inter.Hovered == n {
		// The theme component's hovered color is the baseline...
		if comp, ok := rt.Theme.Components[n.Type]; ok && comp.HoveredBackgroundColor != "" {
			s.Background = resolveColor(comp.HoveredBackgroundColor, rt)
		}
		// ...and a per-node declaration wins over it.
		if c, ok := evalColorStyle(n, "hoverBackground", rt); ok {
			s.Background = c
		}
		if o, ok := evalFloatStyle(n, "hoverOpacity", rt); ok && o >= 0 && o <= 1 {
			s.Opacity = o
		}
	}
	// Pressed is applied LAST so it wins over hovered.
	if inter.Pressed == n {
		if c, ok := evalColorStyle(n, "pressedBackground", rt); ok {
			s.Background = c
		} else if comp, ok := rt.Theme.Components[n.Type]; ok && comp.PressedBackgroundColor != "" {
			s.Background = resolveColor(comp.PressedBackgroundColor, rt)
		}
		if o, ok := evalFloatStyle(n, "pressedOpacity", rt); ok && o >= 0 && o <= 1 {
			s.Opacity = o
		} else if comp, ok := rt.Theme.Components[n.Type]; ok && comp.PressedOpacity != nil {
			s.Opacity = *comp.PressedOpacity
		}
	}
}

// resolveSelectionColor returns the theme's text-selection highlight:
// palette "selection" → palette "primary" → a translucent primary fallback.
func resolveSelectionColor(rt *runtime.Runtime) color.RGBA {
	if rt != nil && rt.Theme != nil {
		if c, ok := rt.Theme.GetColor("selection"); ok {
			return c
		}
		if c, ok := rt.Theme.GetColor("primary"); ok {
			return c
		}
	}
	return color.RGBA{0, 122, 255, 90}
}

// SelectionColor exports resolveSelectionColor for the widgets library (the
// textarea widget paints its own per-line selection highlight).
func SelectionColor(rt *runtime.Runtime) color.RGBA { return resolveSelectionColor(rt) }

// resolveFocusColor returns the theme's focus ring color:
// palette "focus" → palette "primary" → literal #007AFF.
func resolveFocusColor(rt *runtime.Runtime) color.RGBA {
	if rt != nil && rt.Theme != nil {
		if c, ok := rt.Theme.GetColor("focus"); ok {
			return c
		}
		if c, ok := rt.Theme.GetColor("primary"); ok {
			return c
		}
	}
	return color.RGBA{0, 122, 255, 255}
}

// resolveColor resolves a color string against the live Theme first, then
// defaultVars, then literal hex. This is the single authoritative color
// resolver for the canvas engine.
// ResolveColor exports resolveColor for the widgets library: author props
// carry var(--…) spellings (design tokens, HTML theme-var aliases) that the
// widgets-side themeColor cannot unwrap.
func ResolveColor(c string, rt *runtime.Runtime) color.RGBA { return resolveColor(c, rt) }

func resolveColor(c string, rt *runtime.Runtime) color.RGBA {
	c = strings.TrimSpace(c)
	if c == "" {
		return color.RGBA{0, 0, 0, 0}
	}

	// 1. var(--name) — design tokens, HTML theme-var aliases, theme, then
	// defaultVars; only then the debug magenta.
	if strings.HasPrefix(c, "var(") && strings.HasSuffix(c, ")") {
		varName := c[4 : len(c)-1]
		// 1a. Manifest design tokens render as var(--qorm-token-<dotted.name>)
		// with dots flattened to hyphens (color.bg → --qorm-token-color-bg).
		// Match by forward-mapping each token key (exact for nested dots).
		if rt != nil && rt.App != nil && strings.HasPrefix(varName, "--qorm-token-") {
			for name, tok := range rt.App.DesignTokens {
				if tok.Type == "color" && varName == "--qorm-token-"+strings.ReplaceAll(name, ".", "-") {
					return parseColor(tok.Value)
				}
			}
		}
		// 1b. HTML theme-var aliases (render/theme.go palettes) → canvas theme
		// tokens, so scenes written for the HTML shell keep their colors.
		if alias, ok := themeVarAliases[varName]; ok {
			if rt != nil && rt.Theme != nil {
				if col, ok := rt.Theme.GetColor(alias); ok {
					return col
				}
			}
			if col, ok := defaultVars["--"+alias]; ok {
				return col
			}
		}
		// Check active theme first
		if rt != nil && rt.Theme != nil {
			// Strip leading -- for theme lookup (theme keys don't have --)
			lookupName := strings.TrimPrefix(varName, "--")
			if col, ok := rt.Theme.GetColor(lookupName); ok {
				return col
			}
		}
		// Fallback to hardcoded defaultVars
		if col, ok := defaultVars[varName]; ok {
			return col
		}
		return color.RGBA{255, 0, 255, 255} // debug magenta
	}

	// 1c. Gradient: the software rasterizer has no gradient paint — degrade a
	// linear-gradient(...) to its first #hex stop instead of nothing/black.
	if strings.HasPrefix(c, "linear-gradient(") {
		if i := strings.Index(c, "#"); i >= 0 {
			rest := c[i+1:]
			end := strings.IndexAny(rest, ",%) ")
			if end > 0 {
				rest = rest[:end]
			}
			if col := parseColor("#" + rest); col.A > 0 {
				return col
			}
		}
	}

	// 2. Theme palette name (e.g. "primary", "surface")
	if rt != nil && rt.Theme != nil {
		if col, ok := rt.Theme.GetColor(c); ok {
			return col
		}
	}

	// 3. Literal hex
	return parseColor(c)
}

func parseStyle(n *model.Node, rt *runtime.Runtime, sc ...*listScope) NodeStyle {
	var scope *listScope
	if len(sc) > 0 {
		scope = sc[0]
	}
	s := NodeStyle{Opacity: 1}

	// Apply Theme Defaults
	if rt != nil && rt.Theme != nil {
		if compStyle, ok := rt.Theme.Components[n.Type]; ok {
			if compStyle.BackgroundColor != "" {
				if c, ok := rt.Theme.GetColor(compStyle.BackgroundColor); ok {
					s.Background = c
				}
			}
			if compStyle.Color != "" {
				if c, ok := rt.Theme.GetColor(compStyle.Color); ok {
					s.Color = c
				}
			}
			if compStyle.BorderRadius != nil {
				s.BorderRadius = *compStyle.BorderRadius
			}
			if compStyle.Padding != nil {
				s.Padding = *compStyle.Padding
			}
			if compStyle.Margin != nil {
				// We set all margins to the theme margin for simplicity
				s.MarginTop = *compStyle.Margin
				s.MarginBot = *compStyle.Margin
				s.MarginLeft = *compStyle.Margin
				s.MarginRight = *compStyle.Margin
			}
			if compStyle.Gap != nil {
				s.Gap = *compStyle.Gap
			}
			if compStyle.FontSize != nil {
				s.FontSize = *compStyle.FontSize
			}
			if compStyle.FontWeight != nil {
				s.FontWeight = *compStyle.FontWeight
			}
			if compStyle.TextAlign != "" {
				s.TextAlign = compStyle.TextAlign
			}
			if compStyle.StrokeColor != "" {
				if c, ok := rt.Theme.GetColor(compStyle.StrokeColor); ok {
					s.StrokeColor = c
				}
			}
			if compStyle.StrokeWidth != nil {
				s.StrokeWidth = *compStyle.StrokeWidth
			}
			if compStyle.BoxShadowColor != "" {
				if c, ok := rt.Theme.GetColor(compStyle.BoxShadowColor); ok {
					s.BoxShadowColor = c
				}
			}
			if compStyle.BoxShadowBlur != nil {
				s.BoxShadowBlur = *compStyle.BoxShadowBlur
			}
			if compStyle.BoxShadowX != nil {
				s.BoxShadowX = *compStyle.BoxShadowX
			}
			if compStyle.BoxShadowY != nil {
				s.BoxShadowY = *compStyle.BoxShadowY
			}
		}
	}

	// Default color if still empty
	if s.Color.A == 0 {
		s.Color = color.RGBA{255, 255, 255, 255}
	}

	// Stylesheet cascade (styles/*.qss): type rules, then class rules, then id
	// rules — each overrides the theme component defaults above and is in turn
	// overridden by the node's inline style below. Rule values evaluate at the
	// same moment inline values do (here, per measure), so a {{binding}} in a
	// rule tracks state exactly like an inline one.
	matched := matchingStyleRules(n, rt)
	authorBackground := false
	for _, m := range matched {
		applyStyleProps(&s, m, rt, scope)
		if _, ok := m["background"]; ok {
			authorBackground = true
		}
	}

	if n.Style != nil {
		applyStyleProps(&s, n.Style, rt, scope)
		if _, author := n.Style["background"]; author {
			authorBackground = true
		}
	}
	// Browser parity for bare text fields: the HTML path emits a plain
	// <input>/<textarea> with no background styling, so the user-agent
	// chrome is WHITE (render_input.go). The theme's inputBg only shows
	// when the author sets background explicitly — inline or via a matched
	// stylesheet rule. (Like before, the default only engages once the author
	// styled the field at all — inline or by rule — an untouched field keeps
	// the theme background.)
	if n.Type == "input" || n.Type == "textarea" {
		if !authorBackground && (n.Style != nil || len(matched) > 0) {
			s.Background = color.RGBA{255, 255, 255, 255}
		}
	}

	if n.Layout != nil {
		if align, ok := n.Layout["align"].(string); ok {
			s.Align = align
		}
		if justify, ok := n.Layout["justify"].(string); ok {
			s.Justify = justify
		}
		if as, ok := n.Layout["alignSelf"].(string); ok {
			s.AlignSelf = as
		}
		if width, ok := n.Layout["width"].(string); ok && width == "fill" {
			s.WidthRaw = "fill"
		}
		if height, ok := n.Layout["height"].(string); ok && height == "fill" {
			s.HeightRaw = "fill"
		}
	}

	return s
}

// matchingStyleRules returns the styles/*.qss rule bodies that apply to n, in
// cascade order: every matching type rule first (declaration order), then
// class rules, then id rules — so a later map in the slice overrides an
// earlier one key by key. Class rules follow the node's own `class` list
// order first (a class named later in the prop wins over an earlier one) and
// declaration order within one class name. Zero matches (or no stylesheets)
// costs one slice header and no map lookups.
func matchingStyleRules(n *model.Node, rt *runtime.Runtime) []map[string]any {
	if rt == nil || rt.App == nil || len(rt.App.Styles) == 0 {
		return nil
	}
	var typeRules, idRules []map[string]any
	classRules := map[string][]map[string]any{}
	for _, r := range rt.App.Styles {
		switch r.Kind {
		case model.StyleRuleType:
			if r.Name == n.Type {
				typeRules = append(typeRules, r.Style)
			}
		case model.StyleRuleID:
			if r.Name == n.ID {
				idRules = append(idRules, r.Style)
			}
		case model.StyleRuleClass:
			classRules[r.Name] = append(classRules[r.Name], r.Style)
		}
	}
	var classes []string
	if len(classRules) > 0 {
		if cs, _ := n.Props["class"].(string); cs != "" {
			classes = strings.Fields(cs)
		}
	}
	total := len(typeRules) + len(idRules)
	for _, c := range classes {
		total += len(classRules[c])
	}
	if total == 0 {
		return nil
	}
	out := make([]map[string]any, 0, total)
	out = append(out, typeRules...)
	for _, c := range classes {
		out = append(out, classRules[c]...)
	}
	out = append(out, idRules...)
	return out
}

// applyStyleProps layers one style map — a node's inline style or a matched
// stylesheet rule body — over s. It is the exact key set parseStyle has always
// consumed; both callers share it so a rule and an inline style can never
// drift into two interpretations of the same key.
func applyStyleProps(s *NodeStyle, style map[string]any, rt *runtime.Runtime, sc ...*listScope) {
	var scope *listScope
	if len(sc) > 0 {
		scope = sc[0]
	}
	esp := func(val any) any { return evalStyleProp(val, rt, scope) }
	// --- Colors (all go through resolveColor) ---
	if bg, ok := esp(style["background"]).(string); ok {
		s.Background = resolveColor(bg, rt)
	}
	if as, ok := esp(style["alignSelf"]).(string); ok {
		s.AlignSelf = as
	}
	if cStr, ok := esp(style["color"]).(string); ok {
		s.Color = resolveColor(cStr, rt)
	}
	if sc, ok := esp(style["strokeColor"]).(string); ok {
		s.StrokeColor = resolveColor(sc, rt)
	}
	if sc, ok := esp(style["borderColor"]).(string); ok {
		// borderColor is an alias for strokeColor
		s.StrokeColor = resolveColor(sc, rt)
	}
	if sc, ok := esp(style["boxShadowColor"]).(string); ok {
		s.BoxShadowColor = resolveColor(sc, rt)
	}

	// --- Numeric properties (float64 or int from JSON) ---
	pad := esp(style["padding"])
	if f, ok := pad.(float64); ok {
		s.Padding = int(f)
	} else if i, ok := pad.(int); ok {
		s.Padding = i
	}

	gap := esp(style["gap"])
	if f, ok := gap.(float64); ok {
		s.Gap = int(f)
	} else if i, ok := gap.(int); ok {
		s.Gap = i
	}

	width := esp(style["width"])
	if f, ok := width.(float64); ok {
		s.Width = int(f)
	} else if i, ok := width.(int); ok {
		s.Width = i
	} else if str, ok := width.(string); ok && str == "fill" {
		s.WidthRaw = "fill"
	}

	height := esp(style["height"])
	if f, ok := height.(float64); ok {
		s.Height = int(f)
	} else if i, ok := height.(int); ok {
		s.Height = i
	} else if str, ok := height.(string); ok && str == "fill" {
		s.HeightRaw = "fill"
	}

	// Absolute position: x/y (native) or left/top (HTML alias). Either key
	// present marks the node positioned; the missing axis reads 0.
	for _, key := range []string{"x", "left"} {
		v := esp(style[key])
		if f, ok := v.(float64); ok {
			s.PosX = int(f)
			s.HasPos = true
		} else if i, ok := v.(int); ok {
			s.PosX = i
			s.HasPos = true
		}
	}
	for _, key := range []string{"y", "top"} {
		v := esp(style[key])
		if f, ok := v.(float64); ok {
			s.PosY = int(f)
			s.HasPos = true
		} else if i, ok := v.(int); ok {
			s.PosY = i
			s.HasPos = true
		}
	}

	// min/max size constraints (HTML: minWidth/maxWidth/minHeight/
	// maxHeight): clamped in measure after content and explicit sizes
	// resolve, matching the CSS box resolution order.
	s.MinWidth, s.MaxWidth = styleDimPair(style["minWidth"], style["maxWidth"], rt)
	s.MinHeight, s.MaxHeight = styleDimPair(style["minHeight"], style["maxHeight"], rt)

	// margin: can be { "top": N, ... } object or a single number
	mRaw := esp(style["margin"])
	if margin, ok := mRaw.(map[string]any); ok {
		if top, ok := margin["top"].(float64); ok {
			s.MarginTop = int(top)
		}
		if bot, ok := margin["bottom"].(float64); ok {
			s.MarginBot = int(bot)
		}
		if l, ok := margin["left"].(float64); ok {
			s.MarginLeft = int(l)
		}
		if r, ok := margin["right"].(float64); ok {
			s.MarginRight = int(r)
		}
	} else if mf, ok := mRaw.(float64); ok {
		m := int(mf)
		s.MarginTop = m
		s.MarginBot = m
		s.MarginLeft = m
		s.MarginRight = m
	}

	fs := esp(style["fontSize"])
	if f, ok := fs.(float64); ok {
		s.FontSize = int(f)
	} else if i, ok := fs.(int); ok {
		s.FontSize = i
	}

	fw := esp(style["fontWeight"])
	if f, ok := fw.(float64); ok {
		s.FontWeight = int(f)
	} else if i, ok := fw.(int); ok {
		s.FontWeight = i
	}

	if align, ok := esp(style["textAlign"]).(string); ok {
		s.TextAlign = align
	}

	br := esp(style["borderRadius"])
	if f, ok := br.(float64); ok {
		s.BorderRadius = f
	} else if i, ok := br.(int); ok {
		s.BorderRadius = float64(i)
	}

	// Declarative interaction effects: hover/pressed scale (the interaction
	// resolver applies them; 0 = unset).
	if f, ok := esp(style["pressedScale"]).(float64); ok && f > 0 {
		s.PressedScale = f
	}
	if f, ok := esp(style["hoverScale"]).(float64); ok && f > 0 {
		s.HoverScale = f
	}

	// The declarative transition duration: CSS spellings ("0.2s", "200ms")
	// or a plain number of milliseconds.
	if v := esp(style["transition"]); v != nil {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				s.Transition = time.Duration(t) * time.Millisecond
			}
		case string:
			if d, err := parseCSSDuration(t); err == nil && d > 0 {
				s.Transition = d
			}
		}
	}

	sw := esp(style["strokeWidth"])
	if f, ok := sw.(float64); ok {
		s.StrokeWidth = f
	} else if i, ok := sw.(int); ok {
		s.StrokeWidth = float64(i)
	}

	bw := esp(style["borderWidth"])
	if f, ok := bw.(float64); ok {
		s.StrokeWidth = f
	} else if i, ok := bw.(int); ok {
		s.StrokeWidth = float64(i)
	}

	// opacity: element-level alpha, clamped to [0,1] like the browser
	// (HTML emits it raw, render_style.go:285). Applies to the whole
	// subtree at draw time (PerformLayout sets the group opacity).
	op := esp(style["opacity"])
	if f, ok := op.(float64); ok {
		s.Opacity = clamp01(f)
	} else if i, ok := op.(int); ok {
		s.Opacity = clamp01(float64(i))
	}

	// boxShadow numeric overrides
	bsb := esp(style["boxShadowBlur"])
	if f, ok := bsb.(float64); ok {
		s.BoxShadowBlur = int(f)
	} else if i, ok := bsb.(int); ok {
		s.BoxShadowBlur = i
	}
	bsy := esp(style["boxShadowY"])
	if f, ok := bsy.(float64); ok {
		s.BoxShadowY = int(f)
	} else if i, ok := bsy.(int); ok {
		s.BoxShadowY = i
	}
}

// clamp01 constrains an author opacity to [0,1] (CSS clamps likewise).
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// canvasStyleKeys is the set of node style keys the native canvas renderer
// actually consumes (parseStyle above, plus `disabled` — read by interaction.go
// for pointer/focus suppression). The loader already flags keys unknown
// to the HTML renderer (render.KnownStyleKeys) at load time; what remains to
// surface HERE is the canvas-specific gap — keys the HTML path implements but
// the native engine does not yet (gradient, flexGrow, min/max sizes, position,
// boxShadowX at node level, ...) — so an author or agent learns about the
// silent degradation instead of guessing.
var canvasStyleKeys = map[string]bool{
	"background": true, "color": true,
	"strokeColor": true, "borderColor": true,
	"padding": true, "gap": true, "margin": true,
	"width": true, "height": true,
	"minWidth": true, "maxWidth": true, "minHeight": true, "maxHeight": true,
	// Absolute positioning (the infinite-canvas board's coordinate model):
	// x/y are native, left/top the HTML aliases.
	"x": true, "y": true, "left": true, "top": true,
	"fontSize": true, "fontWeight": true, "textAlign": true,
	"borderRadius": true, "strokeWidth": true, "borderWidth": true,
	"opacity":         true,
	"disabled":        true,
	// Declarative interaction effects (any node; resolved generically by
	// applyInteractiveOverlay + performLayout).
	"hoverBackground": true, "pressedBackground": true,
	"hoverOpacity": true, "pressedOpacity": true,
	"pressedScale": true, "hoverScale": true,
	"transition": true, // animates interaction effect changes ("0.2s")
	"boxShadowColor":  true, "boxShadowBlur": true, "boxShadowY": true,
}

// styleWarn* implement one-shot unsupported-style-key warnings: each key is
// reported once per scene tree (keyed by the scene root pointer — a scene
// switch or hot reload re-arms the warnings), so the per-frame Measure pass
// never spams. The writer is a var so tests can capture it.
var (
	styleWarnMu   sync.Mutex
	styleWarnRoot *model.Node
	styleWarnSeen           = map[string]bool{}
	styleWarnOut  io.Writer = os.Stderr
)

// warnUnsupportedStyleKeys reports each style key on n that the canvas
// renderer does not consume — once per key per scene, sorted for stable
// output. Called from the measure pass; root identifies the scene tree.
func warnUnsupportedStyleKeys(root, n *model.Node) {
	if len(n.Style) == 0 {
		return
	}
	styleWarnMu.Lock()
	defer styleWarnMu.Unlock()
	if root != styleWarnRoot {
		styleWarnRoot = root
		styleWarnSeen = map[string]bool{}
	}
	var keys []string
	for k := range n.Style {
		if !canvasStyleKeys[k] && !styleWarnSeen[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)
	for _, k := range keys {
		styleWarnSeen[k] = true
		fmt.Fprintf(styleWarnOut, "[qorm canvas] style key %q (node id: %q, type: %q) is not supported by the native renderer; ignoring it\n", k, n.ID, n.Type)
	}
}

// styleDimPair parses one (min, max) dimension pair — plain numbers only
// ("fill" is meaningless for a constraint).
func styleDimPair(minV, maxV any, rt *runtime.Runtime) (min, max int) {
	num := func(v any) int {
		switch t := evalStyleProp(v, rt).(type) {
		case float64:
			return int(t)
		case int:
			return t
		}
		return 0
	}
	return num(minV), num(maxV)
}

// themeVarAliases maps the HTML shell's theme variable names
// (render/theme.go palettes) onto canvas theme tokens, so scenes authored
// for the HTML path keep their colors in the native renderer.
var themeVarAliases = map[string]string{
	"--label":  "text",
	"--label2": "textSecondary",
	"--bg":     "background",
	"--sep":    "separator",
	"--fill":   "inputBg",
	"--accent": "primary",
}
