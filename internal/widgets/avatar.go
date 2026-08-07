package widgets

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("avatar", Avatar{})
}

// Avatar is the circular user thumbnail (HTML render_media.go avatar()): a
// size×size (default 40) circle showing the `src` image cropped to the
// circle, or the initials (first two runes of `initials`/`name`, uppercased)
// on a coloured disc. With neither it degrades to a grey disc with a "?"
// placeholder; a src that fails to load degrades to the grey disc the image
// pipeline paints for broken sources.
type Avatar struct{}

// Measure reports the square content size: the `size` prop (logical px,
// default 40 like the HTML propNum(n, "size", 40)) times the device scale.
func (Avatar) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	sz := int(propNumDefault(n, "size", 40)) * scale
	return sz, sz
}

// Record builds the avatar for the laid-out box.
func (Avatar) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	radius := float64(ln.Height) / 2 // CSS border-radius:50%

	// src form: the image pipeline's own RecordImage (canvas/image.go —
	// exported, unlike the loadImage the planning note assumed) already
	// resolves src against the app's BaseDir jail, decodes/caches and paints
	// a grey placeholder box on failure; borderRadius = half the box clips
	// the bitmap to the circle (cover fit, the HTML object-fit:cover).
	if avatarSrc(ln.Node, ln, rt) != "" {
		return canvas.RecordImage(ln.Node, rt, ln.Width, ln.Height, radius, ln.EvalVars)
	}

	disc := draw.NewRect()
	disc.Width = float64(ln.Width)
	disc.Height = float64(ln.Height)
	disc.BorderRadius = radius

	text := ""
	ink := color.RGBA{255, 255, 255, 255}
	if initials := avatarInitials(ln.Node, ln, rt); initials != "" {
		// HTML hardcodes #6366f1 behind initials; the canvas palette's nearest
		// token is secondary, with the HTML colour as the theme-less fallback.
		disc.Fill = themeColor(rt, "secondary", color.RGBA{0x63, 0x66, 0xf1, 255})
		text = initials
	} else {
		// No src and no name: the degraded form is a grey disc + placeholder
		// (the HTML path would paint an empty coloured div).
		disc.Fill = themeColor(rt, "separator", color.RGBA{198, 198, 200, 255})
		text = "?"
	}

	g := draw.NewGroup()
	g.AddChild(disc)
	if text != "" {
		// ~40% of the disc reads like the HTML default (14px on a 40px disc);
		// the canvas text stack has no weight axis, so the HTML's
		// font-weight:600 is approximated by size alone.
		fs := float64(ln.Height) * 0.4
		txt := draw.NewText()
		txt.Content = text
		txt.FontSize = fs
		txt.Fill = ink
		txtW := int(canvas.MeasureText(text, fs))
		txtH := int(fs * 1.2)
		txt.X = float64((ln.Width - txtW) / 2)
		txt.Y = float64((ln.Height - txtH) / 2)
		g.AddChild(txt)
	}
	return g
}

// avatarSrc evaluates the interpolated `src` prop ({{state.x}} resolves, like
// the HTML path's interp, render_media.go:89).
func avatarSrc(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	raw, ok := n.Prop("src")
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(runtime.EvalBinding(fmt.Sprint(raw), formCtxLn(rt, ln))))
}

// avatarInitials resolves the initials text: the `initials` prop, falling
// back to `name` (HTML propStrOr(n, "initials", propStr(n, "name"))), then
// the node text (the icon widget's third source — a plain-text avatar in a
// list template is written {"type":"avatar","text":"{{item.name}}"}),
// interpolated, rune-safely truncated to two glyphs and uppercased.
func avatarInitials(n *model.Node, ln *canvas.LayoutNode, rt *runtime.Runtime) string {
	raw := ""
	if v, ok := n.Prop("initials"); ok {
		raw = fmt.Sprint(v)
	} else if v, ok := n.Prop("name"); ok {
		raw = fmt.Sprint(v)
	} else if n.Text != "" {
		raw = n.Text
	}
	if raw == "" {
		return ""
	}
	s := fmt.Sprint(runtime.EvalBinding(raw, formCtxLn(rt, ln)))
	if rs := []rune(s); len(rs) > 2 {
		s = string(rs[:2]) // rune-safe: don't split a multibyte glyph
	}
	return strings.ToUpper(s)
}

// Inline marks Avatar as inline-level (canvas.InlineWidget): flex containers keep its content size.
func (Avatar) Inline() {}
