import os
import re

# 1. Update internal/render/graph/shapes.go
shapes_path = "internal/render/graph/shapes.go"
with open(shapes_path, "r") as f:
    shapes = f.read()

rich_text_structs = """
// Span represents a formatted segment of text
type Span struct {
	Content       string
	FontSize      float64
	FontWeight    int
	LetterSpacing float64
	Italic        bool
	Fill          color.RGBA
	StrokeColor   color.RGBA
	StrokeWidth   float64
	ShadowColor   color.RGBA
	ShadowBlur    float64
	ShadowX       float64
	ShadowY       float64
	Underline     bool
	LineThrough   bool
	Overline      bool
}

// PositionedSpan represents a Span laid out at a specific (X, Y) relative to the RichText node.
type PositionedSpan struct {
	Span
	X, Y float64
}

// RichText represents a text shape capable of rendering multiple formatted spans.
type RichText struct {
	BaseNode
	Lines [][]PositionedSpan
}

func NewRichText() *RichText {
	rt := &RichText{}
	rt.Init(rt)
	return rt
}

func (rt *RichText) Base() *BaseNode { return &rt.BaseNode }

func (rt *RichText) Draw(ctx *Context) {
	rt.UpdateGlobalTransform()

	if len(rt.Lines) == 0 {
		return
	}

	ctx.Save()
	ctx.Transform(localTransform(&rt.BaseNode))
	ctx.Opacity(rt.Opacity)

	for _, line := range rt.Lines {
		for _, s := range line {
			if s.Content == "" {
				continue
			}
			ctx.Fill(s.Fill)
			ctx.AddText(op.TextOp{
				Text: s.Content, Pos: image.Point{int(s.X), int(s.Y)}, Scale: s.FontSize / 10.0,
				Weight: s.FontWeight, LetterSpacing: s.LetterSpacing, Italic: s.Italic,
				StrokeColor: s.StrokeColor, StrokeWidth: s.StrokeWidth,
				ShadowColor: s.ShadowColor, ShadowBlur: s.ShadowBlur, ShadowX: s.ShadowX, ShadowY: s.ShadowY,
				Underline: s.Underline, LineThrough: s.LineThrough, Overline: s.Overline,
			})
		}
	}
	ctx.Restore()
}
"""
if "type Span struct" not in shapes:
    shapes += rich_text_structs
    with open(shapes_path, "w") as f:
        f.write(shapes)
