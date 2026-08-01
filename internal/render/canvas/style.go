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
	return color.RGBA{0, 0, 0, 255}
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
	Align                string
	Justify              string
	FontSize             int
	FontWeight           int
	TextAlign            string
	BorderRadius         float64
	Opacity              float64 // 1 = fully opaque; lowered by pressedOpacity theme state

	StrokeColor color.RGBA
	StrokeWidth float64
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
	s.FontSize *= f
	s.BorderRadius *= float64(f)
	s.StrokeWidth *= float64(f)
}

func evalStyleProp(val any, rt *runtime.Runtime) any {
	switch v := val.(type) {
	case string:
		if rt != nil {
			return runtime.EvalBinding(v, evalCtx(rt))
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
			out[k] = evalStyleProp(item, rt)
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
func applyInteractiveOverlay(s *NodeStyle, n *model.Node, rt *runtime.Runtime, inter *Interaction) {
	if inter == nil || rt == nil || rt.Theme == nil || (inter.Pressed != n && inter.Hovered != n) {
		return
	}
	comp, ok := rt.Theme.Components[n.Type]
	if !ok {
		return
	}
	if inter.Hovered == n && comp.HoveredBackgroundColor != "" {
		s.Background = resolveColor(comp.HoveredBackgroundColor, rt)
	}
	if inter.Pressed == n {
		if comp.PressedBackgroundColor != "" {
			s.Background = resolveColor(comp.PressedBackgroundColor, rt)
		}
		if comp.PressedOpacity != nil {
			s.Opacity = *comp.PressedOpacity
		}
	}
}

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
func resolveColor(c string, rt *runtime.Runtime) color.RGBA {
	c = strings.TrimSpace(c)
	if c == "" {
		return color.RGBA{0, 0, 0, 0}
	}

	// 1. var(--name) — resolve against theme, then defaultVars
	if strings.HasPrefix(c, "var(") && strings.HasSuffix(c, ")") {
		varName := c[4 : len(c)-1]
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

	// 2. Theme palette name (e.g. "primary", "surface")
	if rt != nil && rt.Theme != nil {
		if col, ok := rt.Theme.GetColor(c); ok {
			return col
		}
	}

	// 3. Literal hex
	return parseColor(c)
}

func parseStyle(n *model.Node, rt *runtime.Runtime) NodeStyle {
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

	if n.Style != nil {
		// --- Colors (all go through resolveColor) ---
		if bg, ok := evalStyleProp(n.Style["background"], rt).(string); ok {
			s.Background = resolveColor(bg, rt)
		}
		if cStr, ok := evalStyleProp(n.Style["color"], rt).(string); ok {
			s.Color = resolveColor(cStr, rt)
		}
		if sc, ok := evalStyleProp(n.Style["strokeColor"], rt).(string); ok {
			s.StrokeColor = resolveColor(sc, rt)
		}
		if sc, ok := evalStyleProp(n.Style["borderColor"], rt).(string); ok {
			// borderColor is an alias for strokeColor
			s.StrokeColor = resolveColor(sc, rt)
		}
		if sc, ok := evalStyleProp(n.Style["boxShadowColor"], rt).(string); ok {
			s.BoxShadowColor = resolveColor(sc, rt)
		}

		// --- Numeric properties (float64 or int from JSON) ---
		pad := evalStyleProp(n.Style["padding"], rt)
		if f, ok := pad.(float64); ok {
			s.Padding = int(f)
		} else if i, ok := pad.(int); ok {
			s.Padding = i
		}

		gap := evalStyleProp(n.Style["gap"], rt)
		if f, ok := gap.(float64); ok {
			s.Gap = int(f)
		} else if i, ok := gap.(int); ok {
			s.Gap = i
		}

		width := evalStyleProp(n.Style["width"], rt)
		if f, ok := width.(float64); ok {
			s.Width = int(f)
		} else if i, ok := width.(int); ok {
			s.Width = i
		} else if str, ok := width.(string); ok && str == "fill" {
			s.WidthRaw = "fill"
		}

		height := evalStyleProp(n.Style["height"], rt)
		if f, ok := height.(float64); ok {
			s.Height = int(f)
		} else if i, ok := height.(int); ok {
			s.Height = i
		} else if str, ok := height.(string); ok && str == "fill" {
			s.HeightRaw = "fill"
		}

		// min/max size constraints (HTML: minWidth/maxWidth/minHeight/
		// maxHeight): clamped in measure after content and explicit sizes
		// resolve, matching the CSS box resolution order.
		s.MinWidth, s.MaxWidth = styleDimPair(n.Style["minWidth"], n.Style["maxWidth"], rt)
		s.MinHeight, s.MaxHeight = styleDimPair(n.Style["minHeight"], n.Style["maxHeight"], rt)

		// margin: can be { "top": N, ... } object or a single number
		mRaw := evalStyleProp(n.Style["margin"], rt)
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

		fs := evalStyleProp(n.Style["fontSize"], rt)
		if f, ok := fs.(float64); ok {
			s.FontSize = int(f)
		} else if i, ok := fs.(int); ok {
			s.FontSize = i
		}

		fw := evalStyleProp(n.Style["fontWeight"], rt)
		if f, ok := fw.(float64); ok {
			s.FontWeight = int(f)
		} else if i, ok := fw.(int); ok {
			s.FontWeight = i
		}

		if align, ok := evalStyleProp(n.Style["textAlign"], rt).(string); ok {
			s.TextAlign = align
		}

		br := evalStyleProp(n.Style["borderRadius"], rt)
		if f, ok := br.(float64); ok {
			s.BorderRadius = f
		} else if i, ok := br.(int); ok {
			s.BorderRadius = float64(i)
		}

		sw := evalStyleProp(n.Style["strokeWidth"], rt)
		if f, ok := sw.(float64); ok {
			s.StrokeWidth = f
		} else if i, ok := sw.(int); ok {
			s.StrokeWidth = float64(i)
		}

		bw := evalStyleProp(n.Style["borderWidth"], rt)
		if f, ok := bw.(float64); ok {
			s.StrokeWidth = f
		} else if i, ok := bw.(int); ok {
			s.StrokeWidth = float64(i)
		}

		// opacity: element-level alpha, clamped to [0,1] like the browser
		// (HTML emits it raw, render_style.go:285). Applies to the whole
		// subtree at draw time (PerformLayout sets the group opacity).
		op := evalStyleProp(n.Style["opacity"], rt)
		if f, ok := op.(float64); ok {
			s.Opacity = clamp01(f)
		} else if i, ok := op.(int); ok {
			s.Opacity = clamp01(float64(i))
		}

		// boxShadow numeric overrides
		bsb := evalStyleProp(n.Style["boxShadowBlur"], rt)
		if f, ok := bsb.(float64); ok {
			s.BoxShadowBlur = int(f)
		} else if i, ok := bsb.(int); ok {
			s.BoxShadowBlur = i
		}
		bsy := evalStyleProp(n.Style["boxShadowY"], rt)
		if f, ok := bsy.(float64); ok {
			s.BoxShadowY = int(f)
		} else if i, ok := bsy.(int); ok {
			s.BoxShadowY = i
		}
	}

	if n.Layout != nil {
		if align, ok := n.Layout["align"].(string); ok {
			s.Align = align
		}
		if justify, ok := n.Layout["justify"].(string); ok {
			s.Justify = justify
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
	"fontSize": true, "fontWeight": true, "textAlign": true,
	"borderRadius": true, "strokeWidth": true, "borderWidth": true,
	"opacity":        true,
	"disabled":       true,
	"boxShadowColor": true, "boxShadowBlur": true, "boxShadowY": true,
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
