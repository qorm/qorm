package canvas

// Text wrapping (the "5x7 bitmap font has no wrapping" milestone, now also
// the sfnt path's). The engine measures bottom-up without a width constraint,
// so wrapping happens BETWEEN measure and layout: once the auto-width clamp
// (performLayout) makes every ancestor's width definite, wrapTree walks the
// measured tree with definite available widths and folds any text node whose
// single-line measure exceeds its column. Heights then propagate back up
// (heightsFromChildren), so the layout pass sees consistent boxes.
//
// v1 scope: text nodes in COLUMN flow only (rows share their axis with
// siblings; grid/stack cells need the track geometry — both later). Breaking
// is CSS-greedy: words on spaces; an over-long word — and CJK text, which
// has no spaces — hard-breaks between runes.

import "strings"

// wrapText folds text into lines that each fit availW (px). It returns nil
// when the text already fits on one line (callers treat nil as unwrapped).
func wrapText(text string, fontSize float64, availW int) []string {
	if availW <= 0 || int(MeasureText(text, fontSize)) <= availW {
		return nil
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		if cur.Len() > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
	}
	pushRune := func(r rune, w int) {
		cur.WriteRune(r)
		curW += w
	}
	for _, word := range splitWords(text) {
		ww := int(MeasureText(word, fontSize))
		if ww > availW {
			// Over-long word (or unspaced CJK run): hard-break between runes.
			for _, r := range word {
				rw := int(MeasureText(string(r), fontSize))
				if curW+rw > availW {
					flush()
				}
				pushRune(r, rw)
			}
			continue
		}
		if curW+ww > availW {
			flush()
			word = strings.TrimLeft(word, " ")
			ww = int(MeasureText(word, fontSize))
		}
		cur.WriteString(word)
		curW += ww
	}
	flush()
	if len(lines) <= 1 {
		return nil
	}
	return lines
}

// splitWords splits on spaces, keeping each word's trailing space so line
// widths measure exactly like the source text (CSS collapses the break
// space; the trailing space on a wrapped line is invisible either way).
func splitWords(text string) []string {
	var out []string
	start := 0
	for i, r := range text {
		if r == ' ' {
			out = append(out, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

// wrapTree re-folds text nodes under ln, given ln's definite inner width.
// It runs after measure (sizes known) and before layout (origins assigned),
// and repairs ancestor heights on the way back up.
func wrapTree(ln *LayoutNode, availW int) {
	if ln == nil {
		return
	}
	inner := availW - ln.Style.Padding*2
	if inner < 0 {
		inner = 0
	}
	isRow := ln.Node.Type == "row"
	isGrid := ln.Node.Type == "grid" || ln.Node.Type == "gridview"
	for _, c := range ln.Children {
		cAvail := inner - c.Style.MarginLeft - c.Style.MarginRight
		if cAvail < 0 {
			cAvail = 0
		}
		// Only column-flow text wraps (v1): a row child shares its axis with
		// siblings; grid/stack cells need the track geometry.
		if c.Node.Type == "text" && !isRow && !isGrid && c.Style.Width <= 0 && c.Style.WidthRaw != "fill" {
			fs := c.Style.FontSize
			if fs == 0 {
				fs = 14
			}
			if lines := wrapText(c.Text, float64(fs), cAvail); lines != nil {
				c.Wrapped = lines
				// Block text takes the column width (CSS); the height is the
				// folded line count (measure had it at one line).
				c.Width = cAvail
				c.Height = len(lines) * textLineH(fs)
			}
		}
		wrapTree(c, cAvail)
	}
	heightsFromChildren(ln)
}

// heightsFromChildren recomputes a container's height from its (possibly
// re-folded) children, mirroring the measure pass's column/row sums. Leaf
// heights are left alone. Grid/stack keep their measured height (v1).
func heightsFromChildren(ln *LayoutNode) {
	if ln == nil || len(ln.Children) == 0 {
		return
	}
	switch ln.Node.Type {
	case "grid", "gridview", "stack":
		return
	}
	isRow := ln.Node.Type == "row"
	contentW, contentH := 0, 0
	for i, c := range ln.Children {
		cw := c.Width + c.Style.MarginLeft + c.Style.MarginRight
		ch := c.Height + c.Style.MarginTop + c.Style.MarginBot
		if isRow {
			contentW += cw
			if i > 0 {
				contentW += ln.Style.Gap
			}
			if ch > contentH {
				contentH = ch
			}
		} else {
			contentH += ch
			if i > 0 {
				contentH += ln.Style.Gap
			}
			if cw > contentW {
				contentW = cw
			}
		}
	}
	// A scroll viewport's box stays as measured (explicit/fill height is the
	// whole point); only its content height moves with the folded children.
	if isScrollType(ln.Node.Type) {
		ln.ContentH = contentH + ln.Style.Padding*2
		return
	}
	// Only AUTO heights track the (re-folded) children: an explicit or fill
	// height is the author's box and must survive the wrap pass (a fill-height
	// root shrunk to content height breaks justify:center — the form example
	// glued to the window bottom).
	autoH := ln.Style.Height <= 0 && ln.Style.HeightRaw != "fill"
	if autoH {
		ln.Height = contentH + ln.Style.Padding*2
	}
	if isRow && ln.Style.WidthRaw == "" && ln.Style.Width <= 0 {
		ln.Width = contentW + ln.Style.Padding*2
	}
}

// textLineH is the text block's per-line height (measure.go uses the same
// 1.2 factor for single-line text).
func textLineH(fs int) int { return int(float64(fs) * 1.2) }
