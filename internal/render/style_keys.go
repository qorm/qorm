package render

// KnownStyleKeys is the single source of truth for the style keys the
// renderer understands. The loader warns on any other key in a node's
// "style" object instead of silently dropping it. Box + text keys mirror
// boxCSS/textCSS in render_style.go; "size" is read by the spacer widget
// (render_widgets.go). Note: "elevated" and "animation" are node props,
// not style keys.
var KnownStyleKeys = map[string]bool{
	// Box model (boxCSS).
	"width": true, "height": true,
	"minWidth": true, "maxWidth": true, "minHeight": true, "maxHeight": true,
	"flexGrow": true, "flexShrink": true, "alignSelf": true, "aspectRatio": true,
	"zIndex":     true,
	"background": true, "gradient": true,
	"borderRadius": true, "borderWidth": true, "borderColor": true,
	"gap": true, "opacity": true, "shadow": true,
	"position": true, "top": true, "right": true, "bottom": true, "left": true,
	// Absolute positioning aliases (native canvas): x/y are the infinite-canvas
	// board's coordinate model; the canvas renderer treats left/top as aliases
	// and position:absolute as out-of-flow.
	"x": true, "y": true,
	"cursor": true, "transition": true, "transitionEasing": true,
	"transitionYoyo": true, "transitionLoop": true, "transitionRepeat": true,
	"padding": true, "margin": true,
	// Text (textCSS).
	"color": true, "fontSize": true, "fontWeight": true, "fontFamily": true,
	"lineHeight": true, "letterSpacing": true, "fontStyle": true,
	"textDecoration": true, "textTransform": true, "lineClamp": true,
	"textAlign": true, "textOverflow": true,
	// Glyph outline / drop shadow (canvas; CSS text-stroke / text-shadow).
	"textStrokeColor": true, "textStrokeWidth": true,
	"textShadowColor": true, "textShadowBlur": true,
	"textShadowX": true, "textShadowY": true,
	// Box chrome (canvas RRect path).
	"strokeColor": true, "strokeWidth": true, "strokeDasharray": true, "strokeDashoffset": true, "radialGradient": true,
	"outline": true, "outlineColor": true, "outlineWidth": true, "outlineOffset": true,
	"boxShadowColor": true, "boxShadowBlur": true,
	"boxShadowX": true, "boxShadowY": true, "boxShadowInset": true,
	"filter": true, "blur": true, "filterBlur": true,
	"tint":           true,
	"imageRendering": true, "image-rendering": true,
	"rotate": true, "scale": true, "scaleX": true, "scaleY": true,
	"flipX": true, "flipY": true,
	// Canvas-only visual skew (degrees → graph Skew radians). HTML ignores these.
	"skewX": true, "skewY": true,
	// CSS transform-origin (canvas rotate/scale/skew pivot; empty = center).
	"transformOrigin": true,
	"overflow":        true, "mixBlendMode": true, "layoutMotion": true,
	"scrollSnapType": true, "scrollSnapAlign": true,
	"maskFade": true, "maskFadeSize": true, "maskImage": true,
	"clipPath": true, "layerCache": true,
	"pressedBackground": true, "hoverScale": true,
	// Pseudo-state (pseudoStateCSS). Each key emits a CSS custom property into
	// the node's inline style; the HTML shell (internal/server/server.go) carries
	// the matching fixed :hover / :active / :focus-within rules that consume it,
	// so a state visual costs zero JS and survives any DOM morph.
	"hoverBackground": true, "hoverColor": true, "hoverOpacity": true,
	"pressedScale": true, "pressedOpacity": true,
	"focusBorderColor": true,
	"disabled":         true, "disabledOpacity": true,
	// Backdrop (backdropCSS). Frosted glass: a blur radius plus the translucent
	// fill it shows through. Emitted as custom properties and applied by the
	// shell's @supports-guarded rules, which carry a solid fallback.
	"backdropBlur": true, "backdropTint": true,
	// Widget-specific & layout extensions.
	"size":      true,                                                 // spacer
	"container": true,                                                 // container query root
	"rowHeight": true, "headerBackground": true, "stickyHeader": true, // datatable
}
