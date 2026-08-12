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
	Background color.RGBA
	Color      color.RGBA
	// GradientStops, when len>=2, paints a gradient fill (see software RRect
	// path) instead of solid Background. GradientAngle is CSS degrees for
	// linear gradients (0 = to top, 90 = to right). GradientStopPos holds
	// optional 0..1 stop positions; empty = even spacing. GradientRadial
	// selects a circular gradient from the box center.
	GradientStops   []color.RGBA
	GradientStopPos []float64
	GradientAngle   float64
	GradientRadial  bool
	GradientConic   bool // conic-gradient from center; angle is start degrees
	// BackdropBlur is CSS backdrop-filter blur radius in px; when >0 the
	// software path frosts the pixels under the fill before compositing
	// Background / gradient (plus optional BackdropTint).
	BackdropBlur float64
	BackdropTint color.RGBA

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
	//
	// Stored as float64 (sub-pixel): a 60fps physics tick (mario) wants
	// `x: state.mario * 32` to land between two pixels so the sprite moves
	// smoothly, not in 1-pixel snaps. The renderer rounds to int at the
	// final draw (measure.go); float here is the contract, int is the
	// pixel. Old examples that pass integer x/y keep integer values
	// end-to-end — the change is a strict superset.
	PosX, PosY    float64
	HasPos        bool
	Align         string
	AlignSelf     string // CSS align-self (style/layout alignSelf) — distinct from Align (align-items)
	Justify       string
	FontSize      int
	FontWeight    int
	TextAlign     string
	LetterSpacing float64 // extra px between runes (0 = default tight tracking)
	LineHeight    float64 // line-box multiplier; 0 means the engine default 1.2
	FontStyle     string  // "italic" / "oblique" → faux-italic draw; empty/normal = roman
	BorderRadius  float64
	Opacity       float64 // 1 = fully opaque; lowered by pressedOpacity theme state

	StrokeColor color.RGBA
	StrokeWidth float64

	// Declarative interaction effects (the interaction-effect resolver in
	// applyInteractiveOverlay + performLayout): any node can declare them, so
	// hover/press feedback is DATA, not per-widget hardcoded logic. 0/absent
	// means "no effect" (a pressed/hovered scale of 0 is meaningless).
	PressedScale float64 // scale the node to this factor while pressed
	HoverScale   float64 // scale while hovered (pressed wins)
	// EffectiveScale is the resolved interaction scale (1 = no effect; set by
	// applyInteractiveOverlay from the pressed/hover scale). It joins the
	// transition tween so a node declaring `transition` animates its pressed
	// scale instead of snapping; performLayout applies the interpolated value.
	EffectiveScale float64
	// Transition is the declarative CSS-style transition duration ("0.2s",
	// "200ms" or plain ms) that animates hover/press effect changes instead of
	// snapping them (the interaction resolver routes the style through the
	// tween engine while it is non-zero).
	Transition time.Duration
	// TextOverflow "ellipsis" truncates single-line text with "…" when it
	// exceeds the laid-out width (CSS text-overflow:ellipsis + nowrap).
	TextOverflow string
	// LineClamp caps multi-line wrapped text to N lines with ellipsis on the
	// last (CSS -webkit-line-clamp). 0 = unlimited.
	LineClamp int
	// TextDecoration CSS: underline | line-through | overline (space-separated).
	TextDecoration string
	// TextTransform CSS: uppercase | lowercase | capitalize.
	TextTransform string
	// Outline (CSS outline — outside the border box).
	OutlineColor  color.RGBA
	OutlineWidth  float64
	OutlineOffset float64
	// Text stroke / shadow (CSS -webkit-text-stroke / text-shadow analogues).
	// Distinct from box StrokeColor (border) and BoxShadow* (drop shadow on
	// the chrome). Zero alpha skips the layer in the software text path.
	TextStrokeColor color.RGBA
	TextStrokeWidth float64
	TextShadowColor color.RGBA
	TextShadowBlur  float64
	TextShadowX     float64
	TextShadowY     float64
	// TransitionEasing names an anim curve ("spring", "easeOut", …) for
	// declarative transitions; empty uses the theme standard easing.
	TransitionEasing string
	// CSS filter on the node subtree (offscreen layer). Blur is px;
	// Brightness/Contrast/Saturate are multipliers (0 = unset → 1 at draw).
	FilterBlur       float64
	FilterBrightness float64
	FilterContrast   float64
	FilterSaturate   float64
	FilterGrayscale  float64 // 0..1
	FilterHueRotate  float64 // degrees
	FilterOpacity    float64 // 1 = identity
	// Drop-shadow filter (CSS filter: drop-shadow(...)).
	DropShadowX, DropShadowY, DropShadowBlur float64
	DropShadowColor                          color.RGBA
	// MixBlendMode is CSS mix-blend-mode (multiply/screen/overlay/…).
	MixBlendMode string
	// BoxShadowInset is CSS box-shadow: inset (inner shadow on the chrome).
	BoxShadowInset bool
	// Overflow "hidden" clips children to the box (optional rounded clip via
	// BorderRadius). Empty / "visible" = no clip. Scroll viewports clip always.
	Overflow string
	// LayoutMotion enables FLIP layout animation when the node moves/resizes
	// between frames (requires transition + a stable id).
	LayoutMotion bool
	// ScrollSnapType is CSS scroll-snap-type on a scroll viewport
	// ("y mandatory", "x proximity", "both mandatory", …).
	ScrollSnapType string
	// ScrollSnapAlign is CSS scroll-snap-align on a snap child
	// ("start" | "center" | "end" | "none").
	ScrollSnapAlign string
	// MaskFade is a soft edge fade mask on the subtree: "top"|"bottom"|"left"|
	// "right" (competitive list/carousel edge dissolve). MaskFadeSize is px.
	MaskFade     string
	MaskFadeSize float64
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
	s.PosX *= float64(f)
	s.PosY *= float64(f)
	s.FontSize *= f
	s.BorderRadius *= float64(f)
	s.StrokeWidth *= float64(f)
	s.LetterSpacing *= float64(f)
	s.TextStrokeWidth *= float64(f)
	s.TextShadowBlur *= float64(f)
	s.TextShadowX *= float64(f)
	s.TextShadowY *= float64(f)
	s.FilterBlur *= float64(f)
	s.DropShadowX *= float64(f)
	s.DropShadowY *= float64(f)
	s.DropShadowBlur *= float64(f)
	s.OutlineWidth *= float64(f)
	s.OutlineOffset *= float64(f)
	s.MaskFadeSize *= float64(f)
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
		if s.HoverScale > 0 {
			s.EffectiveScale = s.HoverScale
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
		if s.PressedScale > 0 {
			s.EffectiveScale = s.PressedScale
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

	// 1c. Gradients as colour values: first #hex stop as solid fallback
	// (full multi-stop paint uses applyGradientString on background/gradient).
	if strings.HasPrefix(c, "linear-gradient(") || strings.HasPrefix(c, "radial-gradient(") {
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
	s := NodeStyle{
		Opacity: 1, EffectiveScale: 1,
		// CSS filter color multipliers: 1 = identity (0 would mean black/flat).
		FilterBrightness: 1, FilterContrast: 1, FilterSaturate: 1, FilterOpacity: 1,
	}

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

	// Disabled nodes dim to half opacity by default (a per-node
	// disabledOpacity style key overrides it). Registered interactive widgets
	// (switch, checkbox, slider, select, …) handle their own disabled visuals
	// via formDisabled — the generic dimming only applies to plain nodes
	// (text, box, button, column, row, …) so it never double-dims.
	_, hasWidget := LookupWidget(n.Type)
	if !hasWidget && nodeDisabled(n, rt) {
		if do, ok := evalFloatStyle(n, "disabledOpacity", rt); ok && do >= 0 && do <= 1 {
			s.Opacity = do
		} else if s.Opacity >= 1 {
			s.Opacity = 0.5
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
	// present marks the node positioned; the missing axis reads 0. Stored
	// as float64 (see PosX/PosY doc) so a 60fps physics tick can land
	// between pixels; the renderer rounds at draw time.
	for _, key := range []string{"x", "left"} {
		v := esp(style[key])
		if f, ok := v.(float64); ok {
			s.PosX = f
			s.HasPos = true
		} else if i, ok := v.(int); ok {
			s.PosX = float64(i)
			s.HasPos = true
		}
	}
	for _, key := range []string{"y", "top"} {
		v := esp(style[key])
		if f, ok := v.(float64); ok {
			s.PosY = f
			s.HasPos = true
		} else if i, ok := v.(int); ok {
			s.PosY = float64(i)
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
	if fsStyle, ok := esp(style["fontStyle"]).(string); ok {
		s.FontStyle = strings.ToLower(strings.TrimSpace(fsStyle))
	}
	if to, ok := esp(style["textOverflow"]).(string); ok {
		s.TextOverflow = strings.ToLower(strings.TrimSpace(to))
	}
	if v := esp(style["lineClamp"]); v != nil {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				s.LineClamp = int(t)
			}
		case int:
			if t > 0 {
				s.LineClamp = t
			}
		}
	}
	if td, ok := esp(style["textDecoration"]).(string); ok {
		s.TextDecoration = strings.ToLower(strings.TrimSpace(td))
	}
	if tt, ok := esp(style["textTransform"]).(string); ok {
		s.TextTransform = strings.ToLower(strings.TrimSpace(tt))
	}
	if oc, ok := esp(style["outlineColor"]).(string); ok {
		s.OutlineColor = resolveColor(oc, rt)
	}
	if v := esp(style["outlineWidth"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.OutlineWidth = t
		case int:
			s.OutlineWidth = float64(t)
		case string:
			s.OutlineWidth = parseCSSPx(t)
		}
	}
	if v := esp(style["outlineOffset"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.OutlineOffset = t
		case int:
			s.OutlineOffset = float64(t)
		case string:
			s.OutlineOffset = parseCSSPx(t)
		}
	}
	// Shorthand outline: "2px solid #f00" or "2px #f00" (style token ignored).
	if v := esp(style["outline"]); v != nil {
		if str, ok := v.(string); ok {
			parseOutlineShorthand(s, str, rt)
		}
	}
	// letterSpacing: CSS px extra advance between runes (number or "Npx").
	if ls := esp(style["letterSpacing"]); ls != nil {
		switch v := ls.(type) {
		case float64:
			s.LetterSpacing = v
		case int:
			s.LetterSpacing = float64(v)
		case string:
			s.LetterSpacing = parseCSSPx(v)
		}
	}
	// lineHeight: unitless multiplier (1.2 default) or px absolute → convert
	// to multiplier against FontSize when both are known later.
	if lh := esp(style["lineHeight"]); lh != nil {
		switch v := lh.(type) {
		case float64:
			s.LineHeight = v
		case int:
			s.LineHeight = float64(v)
		case string:
			s.LineHeight = parseCSSPx(v) // may be unitless "1.5" parsed as 1.5
			if s.LineHeight == 0 {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					s.LineHeight = f
				}
			}
		}
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
	// or a plain number of milliseconds. Optional second token is the easing
	// name ("0.3s spring", "200ms easeOut").
	if v := esp(style["transition"]); v != nil {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				s.Transition = time.Duration(t) * time.Millisecond
			}
		case string:
			parts := strings.Fields(t)
			if len(parts) >= 1 {
				if d, err := parseCSSDuration(parts[0]); err == nil && d > 0 {
					s.Transition = d
				}
			}
			if len(parts) >= 2 {
				s.TransitionEasing = parts[1]
			}
		}
	}
	if te, ok := esp(style["transitionEasing"]).(string); ok && te != "" {
		s.TransitionEasing = te
	}
	// Text decorations (outline + drop shadow on glyphs, not the box).
	if sc, ok := esp(style["textStrokeColor"]).(string); ok {
		s.TextStrokeColor = resolveColor(sc, rt)
	}
	if tw := esp(style["textStrokeWidth"]); tw != nil {
		switch v := tw.(type) {
		case float64:
			s.TextStrokeWidth = v
		case int:
			s.TextStrokeWidth = float64(v)
		case string:
			s.TextStrokeWidth = parseCSSPx(v)
		}
	}
	if sc, ok := esp(style["textShadowColor"]).(string); ok {
		s.TextShadowColor = resolveColor(sc, rt)
	}
	if v := esp(style["textShadowBlur"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.TextShadowBlur = t
		case int:
			s.TextShadowBlur = float64(t)
		case string:
			s.TextShadowBlur = parseCSSPx(t)
		}
	}
	if v := esp(style["textShadowX"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.TextShadowX = t
		case int:
			s.TextShadowX = float64(t)
		case string:
			s.TextShadowX = parseCSSPx(t)
		}
	}
	if v := esp(style["textShadowY"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.TextShadowY = t
		case int:
			s.TextShadowY = float64(t)
		case string:
			s.TextShadowY = parseCSSPx(t)
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
	bsx := esp(style["boxShadowX"])
	if f, ok := bsx.(float64); ok {
		s.BoxShadowX = int(f)
	} else if i, ok := bsx.(int); ok {
		s.BoxShadowX = i
	}
	bsy := esp(style["boxShadowY"])
	if f, ok := bsy.(float64); ok {
		s.BoxShadowY = int(f)
	} else if i, ok := bsy.(int); ok {
		s.BoxShadowY = i
	}
	if v := esp(style["boxShadowInset"]); v != nil {
		switch t := v.(type) {
		case bool:
			s.BoxShadowInset = t
		case string:
			s.BoxShadowInset = t == "true" || t == "inset" || t == "1"
		case float64:
			s.BoxShadowInset = t != 0
		}
	}

	// CSS filter: "blur(8px) brightness(1.2) contrast(0.9) saturate(0)"
	// plus numeric blur / filterBlur shortcuts.
	if v := esp(style["filter"]); v != nil {
		if str, ok := v.(string); ok {
			applyCSSFilterString(s, str)
		}
	}
	if v := esp(style["blur"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.FilterBlur = t
		case int:
			s.FilterBlur = float64(t)
		case string:
			s.FilterBlur = parseCSSPx(t)
		}
	}
	if v := esp(style["filterBlur"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.FilterBlur = t
		case int:
			s.FilterBlur = float64(t)
		case string:
			s.FilterBlur = parseCSSPx(t)
		}
	}
	if v := esp(style["overflow"]); v != nil {
		if str, ok := v.(string); ok {
			s.Overflow = strings.ToLower(strings.TrimSpace(str))
		}
	}
	if v := esp(style["mixBlendMode"]); v != nil {
		if str, ok := v.(string); ok {
			s.MixBlendMode = strings.ToLower(strings.TrimSpace(str))
		}
	}
	if v := esp(style["layoutMotion"]); v != nil {
		switch t := v.(type) {
		case bool:
			s.LayoutMotion = t
		case string:
			s.LayoutMotion = t == "true" || t == "1" || t == "flip"
		}
	}
	if v := esp(style["scrollSnapType"]); v != nil {
		if str, ok := v.(string); ok {
			s.ScrollSnapType = strings.ToLower(strings.TrimSpace(str))
		}
	}
	if v := esp(style["scrollSnapAlign"]); v != nil {
		if str, ok := v.(string); ok {
			s.ScrollSnapAlign = strings.ToLower(strings.TrimSpace(str))
		}
	}
	if v := esp(style["maskFade"]); v != nil {
		if str, ok := v.(string); ok {
			s.MaskFade = strings.ToLower(strings.TrimSpace(str))
		}
	}
	if v := esp(style["maskFadeSize"]); v != nil {
		switch t := v.(type) {
		case float64:
			s.MaskFadeSize = t
		case int:
			s.MaskFadeSize = float64(t)
		case string:
			s.MaskFadeSize = parseCSSPx(t)
		}
	}
	// CSS mask-image: linear-gradient(to bottom, black, transparent) → maskFade.
	if v := esp(style["maskImage"]); v != nil {
		if str, ok := v.(string); ok {
			applyMaskImage(s, str)
		}
	}

	// HTML "gradient" / background linear-gradient(...): parse multi-stop
	// fills for the software rasterizer; fall back to first-stop solid.
	if g, ok := esp(style["gradient"]).(string); ok && g != "" {
		applyGradientString(s, g, rt)
	}
	if bg, ok := esp(style["background"]).(string); ok {
		bt := strings.TrimSpace(bg)
		if strings.HasPrefix(bt, "linear-gradient") || strings.HasPrefix(bt, "radial-gradient") ||
			strings.HasPrefix(bt, "conic-gradient") {
			applyGradientString(s, bg, rt)
		}
	}
	if bb := esp(style["backdropBlur"]); bb != nil {
		switch v := bb.(type) {
		case float64:
			s.BackdropBlur = v
		case int:
			s.BackdropBlur = float64(v)
		case string:
			s.BackdropBlur = parseCSSPx(v)
		}
	}
	if bt, ok := esp(style["backdropTint"]).(string); ok && bt != "" {
		s.BackdropTint = resolveColor(bt, rt)
	}
}

// applyGradientString parses a CSS linear-gradient(...), radial-gradient(...),
// or conic-gradient(...) into s.GradientStops (+ optional positions) and sets
// Background to the first stop for solid fallbacks.
func applyGradientString(s *NodeStyle, g string, rt *runtime.Runtime) {
	g = strings.TrimSpace(g)
	if strings.HasPrefix(g, "radial-gradient(") {
		stops, pos := parseRadialGradient(g, rt)
		if len(stops) == 0 {
			return
		}
		s.GradientStops = stops
		s.GradientStopPos = pos
		s.GradientRadial = true
		s.GradientConic = false
		s.Background = stops[0]
		return
	}
	if strings.HasPrefix(g, "conic-gradient(") {
		stops, pos, angle := parseConicGradient(g, rt)
		if len(stops) == 0 {
			return
		}
		s.GradientStops = stops
		s.GradientStopPos = pos
		s.GradientAngle = angle
		s.GradientConic = true
		s.GradientRadial = false
		s.Background = stops[0]
		return
	}
	stops, pos, angle := parseLinearGradient(g, rt)
	if len(stops) == 0 {
		return
	}
	s.GradientStops = stops
	s.GradientStopPos = pos
	s.GradientAngle = angle
	s.GradientRadial = false
	s.GradientConic = false
	s.Background = stops[0]
}

// parseConicGradient extracts stops and optional "from Ndeg" start angle.
func parseConicGradient(g string, rt *runtime.Runtime) (stops []color.RGBA, pos []float64, angle float64) {
	inner := strings.TrimSpace(g)
	if i := strings.Index(inner, "("); i >= 0 {
		inner = inner[i+1:]
	}
	inner = strings.TrimSuffix(strings.TrimSpace(inner), ")")
	parts := splitCSSList(inner)
	start := 0
	if len(parts) > 0 {
		p0 := strings.TrimSpace(strings.ToLower(parts[0]))
		if strings.HasPrefix(p0, "from ") {
			rest := strings.TrimSpace(strings.TrimPrefix(p0, "from "))
			rest = strings.TrimSuffix(rest, "deg")
			if f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64); err == nil {
				angle = f
			}
			start = 1
		}
	}
	for _, p := range parts[start:] {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// "color" or "color 25%"
		fields := strings.Fields(p)
		if len(fields) == 0 {
			continue
		}
		c := resolveColor(fields[0], rt)
		if c.A == 0 && !strings.HasPrefix(fields[0], "#") && !strings.HasPrefix(fields[0], "rgb") {
			// try full first token group for hex-with-alpha etc.
			c = resolveColor(fields[0], rt)
		}
		if c.A == 0 && fields[0] != "transparent" {
			// still accept for multi-stop presence
		}
		stops = append(stops, c)
		if len(fields) >= 2 && strings.HasSuffix(fields[1], "%") {
			if f, err := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64); err == nil {
				pos = append(pos, f/100)
			}
		}
	}
	if len(pos) != len(stops) {
		pos = nil
	}
	return stops, pos, angle
}

// applyMaskImage maps a simple CSS mask-image linear-gradient to maskFade.
// Supported: linear-gradient(to bottom|top|left|right, … transparent …).
func applyMaskImage(s *NodeStyle, raw string) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if !strings.Contains(raw, "linear-gradient") {
		return
	}
	switch {
	case strings.Contains(raw, "to bottom") || strings.Contains(raw, "to top"):
		if strings.Contains(raw, "to top") {
			s.MaskFade = "top"
		} else {
			s.MaskFade = "bottom"
		}
	case strings.Contains(raw, "to right") || strings.Contains(raw, "to left"):
		if strings.Contains(raw, "to left") {
			s.MaskFade = "left"
		} else {
			s.MaskFade = "right"
		}
	default:
		s.MaskFade = "bottom"
	}
	if s.MaskFadeSize <= 0 {
		s.MaskFadeSize = 48
	}
}

// parseOutlineShorthand parses "2px solid #f00" / "2px #f00" into outline fields.
func parseOutlineShorthand(s *NodeStyle, raw string, rt *runtime.Runtime) {
	parts := strings.Fields(raw)
	for _, p := range parts {
		pl := strings.ToLower(p)
		if pl == "solid" || pl == "dashed" || pl == "dotted" || pl == "none" {
			if pl == "none" {
				s.OutlineWidth = 0
				s.OutlineColor = color.RGBA{}
			}
			continue
		}
		if strings.HasPrefix(pl, "#") || strings.HasPrefix(pl, "rgb") || strings.HasPrefix(pl, "var(") {
			s.OutlineColor = resolveColor(p, rt)
			continue
		}
		if f := parseCSSPx(p); f > 0 || strings.HasSuffix(pl, "px") {
			if s.OutlineWidth == 0 {
				s.OutlineWidth = f
			}
		}
	}
}

// parseLinearGradient extracts colour stops (with optional % positions) and a
// rough angle from a CSS linear-gradient() string.
func parseLinearGradient(g string, rt *runtime.Runtime) (stops []color.RGBA, pos []float64, angle float64) {
	g = strings.TrimSpace(g)
	angle = 180 // default "to bottom"
	if !strings.HasPrefix(g, "linear-gradient(") || !strings.HasSuffix(g, ")") {
		if c := resolveColor(g, rt); c.A > 0 {
			return []color.RGBA{c}, nil, angle
		}
		return nil, nil, angle
	}
	inner := strings.TrimSpace(g[len("linear-gradient(") : len(g)-1])
	// Split on commas not inside parentheses (var(...)).
	parts := splitCSSList(inner)
	if len(parts) == 0 {
		return nil, nil, angle
	}
	start := 0
	first := strings.TrimSpace(parts[0])
	if strings.HasPrefix(first, "to ") {
		switch strings.TrimSpace(strings.TrimPrefix(first, "to ")) {
		case "top":
			angle = 0
		case "right":
			angle = 90
		case "bottom":
			angle = 180
		case "left":
			angle = 270
		}
		start = 1
	} else if strings.HasSuffix(first, "deg") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(first, "deg"), 64); err == nil {
			angle = f
			start = 1
		}
	}
	stops, pos = parseGradientStops(parts[start:], rt)
	return stops, pos, angle
}

// parseRadialGradient extracts colour stops from radial-gradient(...). Shape
// keywords (circle/ellipse/at ...) are skipped; fill is always centered.
func parseRadialGradient(g string, rt *runtime.Runtime) (stops []color.RGBA, pos []float64) {
	g = strings.TrimSpace(g)
	if !strings.HasPrefix(g, "radial-gradient(") || !strings.HasSuffix(g, ")") {
		return nil, nil
	}
	inner := strings.TrimSpace(g[len("radial-gradient(") : len(g)-1])
	parts := splitCSSList(inner)
	start := 0
	if len(parts) > 0 {
		first := strings.ToLower(strings.TrimSpace(parts[0]))
		// Skip leading shape/position descriptors that are not colours.
		if strings.HasPrefix(first, "circle") || strings.HasPrefix(first, "ellipse") ||
			strings.HasPrefix(first, "closest-") || strings.HasPrefix(first, "farthest-") ||
			strings.HasPrefix(first, "at ") {
			start = 1
		}
	}
	return parseGradientStops(parts[start:], rt)
}

// parseGradientStops parses "#fff 0%, #000 100%" style stop lists into colours
// and 0..1 positions. When no stop has a %, pos is nil (even spacing).
func parseGradientStops(parts []string, rt *runtime.Runtime) (stops []color.RGBA, pos []float64) {
	var rawPos []float64
	anyPos := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		colPart := p
		var stopP float64 = -1
		if i := strings.LastIndexAny(p, " \t"); i > 0 {
			tail := strings.TrimSpace(p[i:])
			if strings.HasSuffix(tail, "%") {
				if f, err := strconv.ParseFloat(strings.TrimSuffix(tail, "%"), 64); err == nil {
					stopP = f / 100
					colPart = strings.TrimSpace(p[:i])
					anyPos = true
				}
			}
		}
		c := resolveColor(colPart, rt)
		if c.A == 0 && c.R|c.G|c.B == 0 && !strings.Contains(colPart, "transparent") {
			// Skip unresolvable non-colours (leftover descriptors).
			if !strings.HasPrefix(colPart, "#") && !strings.HasPrefix(colPart, "rgb") &&
				!strings.HasPrefix(colPart, "var(") {
				continue
			}
		}
		stops = append(stops, c)
		rawPos = append(rawPos, stopP)
	}
	if len(stops) == 0 {
		return nil, nil
	}
	if !anyPos {
		return stops, nil
	}
	// Fill missing positions: first 0, last 1, interpolate gaps (CSS rules).
	pos = make([]float64, len(stops))
	copy(pos, rawPos)
	if pos[0] < 0 {
		pos[0] = 0
	}
	if pos[len(pos)-1] < 0 {
		pos[len(pos)-1] = 1
	}
	// Forward: clamp non-decreasing.
	for i := 1; i < len(pos); i++ {
		if pos[i] >= 0 && pos[i] < pos[i-1] {
			pos[i] = pos[i-1]
		}
	}
	// Fill runs of -1 between known positions.
	i := 0
	for i < len(pos) {
		if pos[i] >= 0 {
			i++
			continue
		}
		j := i
		for j < len(pos) && pos[j] < 0 {
			j++
		}
		lo, hi := 0.0, 1.0
		if i > 0 {
			lo = pos[i-1]
		}
		if j < len(pos) {
			hi = pos[j]
		}
		span := j - i + 1
		for k := i; k < j; k++ {
			pos[k] = lo + (hi-lo)*float64(k-i+1)/float64(span)
		}
		i = j
	}
	return stops, pos
}

// splitCSSList splits on top-level commas (not inside parentheses).
func splitCSSList(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
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

// parseCSSPx reads a CSS length used for letter-spacing / line-height numbers:
// "2", "2px", "0.5" → float. Non-numeric returns 0.
func parseCSSPx(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// parseCSSFilterBlur extracts the first blur() radius from a CSS filter
// string (e.g. "blur(8px)", "blur(4)"). Returns ok=false when no blur is found.
func parseCSSFilterBlur(s string) (float64, bool) {
	f, ok := parseCSSFilterFunc(s, "blur")
	return f, ok
}

// applyCSSFilterString parses a space-separated CSS filter list into s.
// Supported: blur(Npx), brightness(N|N%), contrast(N|N%), saturate(N|N%).
func applyCSSFilterString(s *NodeStyle, raw string) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "none" {
		return
	}
	// Walk function tokens: name(args)
	i := 0
	for i < len(raw) {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i >= len(raw) {
			break
		}
		start := i
		for i < len(raw) && raw[i] != '(' {
			i++
		}
		if i >= len(raw) {
			break
		}
		name := strings.TrimSpace(raw[start:i])
		i++ // skip '('
		argStart := i
		depth := 1
		for i < len(raw) && depth > 0 {
			if raw[i] == '(' {
				depth++
			} else if raw[i] == ')' {
				depth--
			}
			if depth > 0 {
				i++
			}
		}
		arg := strings.TrimSpace(raw[argStart:i])
		if i < len(raw) && raw[i] == ')' {
			i++
		}
		switch name {
		case "blur":
			if f, ok := parseFilterNumber(arg, false); ok && f >= 0 {
				s.FilterBlur = f
			}
		case "brightness":
			if f, ok := parseFilterNumber(arg, true); ok {
				s.FilterBrightness = f
			}
		case "contrast":
			if f, ok := parseFilterNumber(arg, true); ok {
				s.FilterContrast = f
			}
		case "saturate":
			if f, ok := parseFilterNumber(arg, true); ok {
				s.FilterSaturate = f
			}
		case "grayscale":
			if f, ok := parseFilterNumber(arg, true); ok {
				s.FilterGrayscale = clamp01(f)
			}
		case "hue-rotate":
			// "90deg" or "90"
			arg = strings.TrimSuffix(strings.TrimSpace(arg), "deg")
			if f, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil {
				s.FilterHueRotate = f
			}
		case "opacity":
			if f, ok := parseFilterNumber(arg, true); ok {
				s.FilterOpacity = f
			}
		case "drop-shadow":
			parseDropShadowArgs(s, arg)
		}
	}
}

// parseDropShadowArgs parses CSS drop-shadow() args: offset-x offset-y blur color
// (e.g. "2px 4px 6px #000" or "2 4 6 rgba(...)").
func parseDropShadowArgs(s *NodeStyle, arg string) {
	parts := strings.Fields(arg)
	if len(parts) < 2 {
		return
	}
	// Last token that looks like a color goes to DropShadowColor; numbers to offsets.
	var nums []float64
	var colorStr string
	for _, p := range parts {
		pl := strings.ToLower(p)
		if strings.HasPrefix(pl, "#") || strings.HasPrefix(pl, "rgb") || strings.HasPrefix(pl, "var(") ||
			pl == "black" || pl == "white" || pl == "red" || pl == "transparent" {
			colorStr = p
			continue
		}
		if f, ok := parseFilterNumber(p, false); ok {
			nums = append(nums, f)
		}
	}
	if len(nums) >= 1 {
		s.DropShadowX = nums[0]
	}
	if len(nums) >= 2 {
		s.DropShadowY = nums[1]
	}
	if len(nums) >= 3 {
		s.DropShadowBlur = nums[2]
	}
	if colorStr != "" {
		s.DropShadowColor = resolveColor(colorStr, nil)
	} else if s.DropShadowColor.A == 0 {
		s.DropShadowColor = color.RGBA{0, 0, 0, 128}
	}
}

// parseCSSFilterFunc finds name(arg) and parses the number (px stripped for blur).
func parseCSSFilterFunc(s, name string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	prefix := name + "("
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return 0, false
	}
	rest := s[idx+len(prefix):]
	j := strings.IndexByte(rest, ')')
	if j < 0 {
		return 0, false
	}
	return parseFilterNumber(rest[:j], name != "blur")
}

// parseFilterNumber parses "1.2", "120%", "8px". percent=true maps 100% → 1.
func parseFilterNumber(arg string, percent bool) (float64, bool) {
	arg = strings.TrimSpace(arg)
	arg = strings.TrimSuffix(arg, "px")
	isPct := strings.HasSuffix(arg, "%")
	if isPct {
		arg = strings.TrimSpace(strings.TrimSuffix(arg, "%"))
	}
	f, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return 0, false
	}
	if isPct && percent {
		f /= 100
	} else if isPct && !percent {
		// blur(50%) is not standard; treat as px for robustness.
	}
	return f, true
}

// lineHeightMult returns the effective line-box multiplier for text layout.
// Unitless author values in (0, 4] are treated as multipliers; larger values
// are treated as absolute px and converted using fontSize (CSS-ish heuristic).
func lineHeightMult(lh float64, fontSize int) float64 {
	if lh <= 0 {
		return 1.2
	}
	if lh > 4 && fontSize > 0 {
		return lh / float64(fontSize)
	}
	return lh
}

// canvasStyleKeys is the set of node style keys the native canvas renderer
// actually consumes (parseStyle / applyStyleProps / flex / interaction).
// The loader flags keys unknown to HTML (render.KnownStyleKeys); this set
// is the canvas-specific residual — keys still unimplemented warn once per
// scene (backdropBlur, fontFamily, letterSpacing, …).
var canvasStyleKeys = map[string]bool{
	"background": true, "color": true, "gradient": true,
	"backdropBlur": true, "backdropTint": true,
	"strokeColor": true, "borderColor": true,
	"padding": true, "gap": true, "margin": true,
	"width": true, "height": true,
	"minWidth": true, "maxWidth": true, "minHeight": true, "maxHeight": true,
	// Flex (read by flex.go from style; listed so they do not false-warn).
	"flexGrow": true, "flexShrink": true, "alignSelf": true,
	// Absolute positioning (the infinite-canvas board's coordinate model):
	// x/y are native, left/top the HTML aliases.
	"x": true, "y": true, "left": true, "top": true,
	"fontSize": true, "fontWeight": true, "textAlign": true,
	"letterSpacing": true, "lineHeight": true, "fontStyle": true,
	"textOverflow": true, // "ellipsis" single-line truncate
	"lineClamp":    true, // multi-line cap + ellipsis
	"textDecoration": true, "textTransform": true,
	"outline": true, "outlineColor": true, "outlineWidth": true, "outlineOffset": true,
	"borderRadius": true, "strokeWidth": true, "borderWidth": true,
	"opacity": true, "disabled": true, "disabledOpacity": true,
	// Declarative interaction effects (any node; resolved generically by
	// applyInteractiveOverlay + performLayout).
	"hoverBackground": true, "pressedBackground": true,
	"hoverOpacity": true, "pressedOpacity": true,
	"pressedScale": true, "hoverScale": true,
	"transition":     true, // animates interaction effect changes ("0.2s")
	"transitionEasing": true, // "spring", "easeOut", …
	"boxShadowColor": true, "boxShadowBlur": true,
	"boxShadowX": true, "boxShadowY": true,
	// Glyph decorations (distinct from box border / box-shadow).
	"textStrokeColor": true, "textStrokeWidth": true,
	"textShadowColor": true, "textShadowBlur": true,
	"textShadowX": true, "textShadowY": true,
	// CSS filter on the node subtree (offscreen layer).
	"filter": true, "blur": true, "filterBlur": true,
	"boxShadowInset": true, // CSS box-shadow: inset
	"overflow":       true, // "hidden" clips children (rounded via borderRadius)
	"mixBlendMode":   true,
	"layoutMotion":   true, // FLIP layout animation (default on with transition)
	"scrollSnapType": true, "scrollSnapAlign": true,
	"maskFade": true, "maskFadeSize": true, "maskImage": true,
	// Spacer / simple widgets.
	"size": true,
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
