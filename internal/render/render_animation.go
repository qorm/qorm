package render

import (
	"fmt"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// animationWidgets consume the `animation` prop themselves (via motion), so the
// universal wrap skips them to avoid double-animating.
var animationWidgets = map[string]bool{
	"motion": true, "animated": true, "transition": true, "animatedswitcher": true,
	"fadetransition": true, "slidetransition": true, "scaletransition": true,
	"rotationtransition": true, "sizetransition": true, "hero": true,
}

// wrapAnimation renders n inside a div playing the named entrance effect, so a
// component instance (`{"type":"Card","animation":"fadeup"}`) or any widget
// animates without a `motion` wrapper.
func (r *renderer) wrapAnimation(n *model.Node, effect string) {
	kf := motionKeyframe[strings.ToLower(effect)]
	if kf == "" {
		kf = "qa-fade"
	}
	dur := propNum(n, "duration", 450)
	delay := propNum(n, "delay", 0)
	curve := propStrOr(n, "curve", "cubic-bezier(.34,1.2,.64,1)")
	repeat := propStrOr(n, "repeat", "1")
	fmt.Fprintf(&r.sb, `<div style="animation:%s %gms %s %gms %s both;">`, kf, dur, curve, delay, repeat)
	r.renderInner(n)
	r.sb.WriteString(`</div>`)
}

// animatedContainer is Flutter's AnimatedContainer (and AnimatedPadding/Align/
// Positioned): a container whose style transitions smoothly whenever a bound
// style value changes — so an agent flipping state animates in the live session.
func (r *renderer) animatedContainer(n *model.Node) {
	dur := propNum(n, "duration", 300)
	curve := propStrOr(n, "curve", "cubic-bezier(.4,0,.2,1)")
	trans := fmt.Sprintf("transition:all %gms %s;", dur, curve)
	// containerCSS (not boxCSS) so an AnimatedContainer honours layout align/justify
	// like any other container — e.g. centring an icon inside an animated circle.
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.containerCSS(n)+trans, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// animatedOpacity is Flutter's AnimatedOpacity: fades children to the bound
// `opacity` (0..1) over `duration`.
func (r *renderer) animatedOpacity(n *model.Node) {
	dur := propNum(n, "duration", 300)
	op := 1.0
	if v := propStr(n, "opacity"); v != "" {
		op = asFloat(runtime.EvalBinding(v, r.ctx()))
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q>`, n.ID,
		r.boxCSS(n)+fmt.Sprintf("opacity:%g;transition:opacity %gms cubic-bezier(.4,0,.2,1);", op, dur))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// motionKeyframe maps a widget type / effect name to a keyframe in the catalog.
var motionKeyframe = map[string]string{
	"fade": "qa-fade", "fadeup": "qa-fadeup", "fadedown": "qa-fadedown",
	"slideup": "qa-slideup", "slidedown": "qa-slidedown", "slideleft": "qa-slideleft", "slideright": "qa-slideright",
	"scale": "qa-scale", "zoomout": "qa-zoomout", "rotate": "qa-rotate", "flip": "qa-flip",
	"pop": "qa-pop", "bounce": "qa-bounce", "shake": "qa-shake", "pulse": "qa-pulse", "spin": "qa-spin", "size": "qa-size",
	// Flutter transition widgets → sensible default effect
	"fadetransition": "qa-fade", "slidetransition": "qa-slideup", "scaletransition": "qa-scale",
	"rotationtransition": "qa-rotate", "sizetransition": "qa-size", "hero": "qa-scale",
	"animatedswitcher": "qa-fade", "transition": "qa-fade", "animated": "qa-fade", "motion": "qa-fade",
}

// motion plays a named entrance/attention animation on its children. The effect
// comes from the `animation` prop (bindable — so an agent switches the whole
// implementation by changing state) or is derived from the widget type. Entrance
// effects fire when the element mounts: the live update morphs the DOM in place
// and preserves existing nodes, so an effect replays when a node is newly created
// (e.g. an item appended to a bound `list`), not on every state change. For
// value-driven, in-place motion use `animatedcontainer` (a CSS transition).
// Covers Flutter's Fade/Slide/Scale/Rotation/Size transitions, Hero and
// AnimatedSwitcher, plus attention effects (bounce/shake/pulse).
func (r *renderer) motion(n *model.Node) {
	effect := r.interp(propStr(n, "animation"))
	if effect == "" {
		effect = n.Type
	}
	kf := motionKeyframe[strings.ToLower(effect)]
	if kf == "" {
		kf = "qa-fade"
	}
	dur := propNum(n, "duration", 450)
	delay := propNum(n, "delay", 0)
	curve := propStrOr(n, "curve", "cubic-bezier(.34,1.2,.64,1)")
	repeat := propStrOr(n, "repeat", "1")
	anim := fmt.Sprintf("animation:%s %gms %s %gms %s both;", kf, dur, curve, delay, repeat)
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+anim, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}

// transform is Flutter's Transform (and RotatedBox): applies rotate/scale/
// translate/skew to its children. Every value is bindable, so an agent can spin
// or scale a widget by setting state — smooth when paired with a transition.
func (r *renderer) transform(n *model.Node) {
	var parts []string
	if v := r.numProp(n, "rotate"); v != nil {
		parts = append(parts, fmt.Sprintf("rotate(%gdeg)", *v))
	}
	if v := r.numProp(n, "scale"); v != nil {
		parts = append(parts, fmt.Sprintf("scale(%g)", *v))
	}
	if v := r.numProp(n, "scaleX"); v != nil {
		parts = append(parts, fmt.Sprintf("scaleX(%g)", *v))
	}
	if v := r.numProp(n, "scaleY"); v != nil {
		parts = append(parts, fmt.Sprintf("scaleY(%g)", *v))
	}
	if x, y := r.numProp(n, "translateX"), r.numProp(n, "translateY"); x != nil || y != nil {
		xv, yv := 0.0, 0.0
		if x != nil {
			xv = *x
		}
		if y != nil {
			yv = *y
		}
		parts = append(parts, fmt.Sprintf("translate(%gpx,%gpx)", xv, yv))
	}
	if v := r.numProp(n, "skew"); v != nil {
		parts = append(parts, fmt.Sprintf("skew(%gdeg)", *v))
	}
	tf := ""
	if len(parts) > 0 {
		tf = "transform:" + strings.Join(parts, " ") + ";transform-origin:center;"
	}
	fmt.Fprintf(&r.sb, `<div id=%q style=%q%s>`, n.ID, r.boxCSS(n)+tf, a11y(n))
	for _, c := range n.Children {
		r.node(c)
	}
	r.sb.WriteString(`</div>`)
}
