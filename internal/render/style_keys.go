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
	"flexGrow": true, "aspectRatio": true,
	"background": true, "gradient": true,
	"borderRadius": true, "borderWidth": true, "borderColor": true,
	"gap": true, "opacity": true, "shadow": true,
	"position": true, "top": true, "right": true, "bottom": true, "left": true,
	"cursor": true, "transition": true,
	"padding": true, "margin": true,
	// Text (textCSS).
	"color": true, "fontSize": true, "fontWeight": true, "fontFamily": true,
	"lineHeight": true, "letterSpacing": true, "fontStyle": true,
	"textDecoration": true, "textTransform": true, "lineClamp": true,
	"textAlign": true,
	// Widget-specific.
	"size": true, // spacer
}
