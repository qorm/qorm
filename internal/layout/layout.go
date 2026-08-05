// Package layout is the CSS-flexbox-subset layout engine backing the QORM
// canvas renderer (internal/render/canvas). It is a pure function over plain
// values: no I/O, no clocks, no randomness — identical inputs always produce
// identical rectangles, in child declaration order.
package layout

// Direction is the container's main axis.
type Direction int

const (
	// Row lays children out horizontally, left to right. It is the zero
	// value so a zero Style is a valid CSS-initial flex container.
	Row Direction = iota
	// Column lays children out vertically, top to bottom.
	Column
)

// Align is a cross-axis alignment, shared by the container's align-items
// (Style.Items) and a child's align-self (Child.AlignSelf).
type Align int

const (
	// AlignStretch stretches a child with an auto (zero) cross size to the
	// line's cross size, margins deducted. It is the CSS initial value, so
	// it is the zero value here as well.
	AlignStretch Align = iota
	// AlignStart packs the child against the line's cross-start edge.
	AlignStart
	// AlignCenter centers the child's margin box in the line's cross axis.
	AlignCenter
	// AlignEnd packs the child against the line's cross-end edge.
	AlignEnd
	// AlignInherit is the align-self sentinel: follow the container's
	// align-items. It is only meaningful on Child.AlignSelf.
	AlignInherit Align = -1
)

// Justify distributes leftover main-axis space inside one line.
type Justify int

const (
	// JustifyStart packs the line against the main-start edge (CSS initial).
	JustifyStart Justify = iota
	// JustifyCenter centers the line.
	JustifyCenter
	// JustifyEnd packs the line against the main-end edge.
	JustifyEnd
	// JustifySpaceBetween splits the leftover evenly between items; a single
	// item falls back to start, per CSS.
	JustifySpaceBetween
	// JustifySpaceAround gives every item an equal half-space on both sides.
	JustifySpaceAround
	// JustifySpaceEvenly splits the leftover evenly before, between, and
	// after items.
	JustifySpaceEvenly
)

// Child is one flex item. W and H are its intrinsic border-box sizes,
// already min/max-clamped by the caller; a zero cross-axis size means "auto"
// and is what AlignStretch stretches. Margins participate on both axes and
// are never collapsed or negated.
type Child struct {
	W, H                               float64
	MarginT, MarginR, MarginB, MarginL float64
	// AlignSelf overrides the container's align-items for this child;
	// AlignInherit (the sentinel) follows Style.Items.
	AlignSelf Align
	// Grow is flex-grow: the child's share of the leftover main-axis space
	// (spacer widgets use it). Negative space never shrinks.
	Grow float64
	// BasisAuto reports whether the main size is auto: a Grow child's basis
	// is then its content size. false means flex-basis:0 — the HTML path's
	// flexGrow (render_style.go emits flex-grow + flex-basis:0).
	BasisAuto bool
}

// Style is the container's flex configuration. The zero value is the CSS
// initial state: row direction, no wrap, stretch items, start-justified.
type Style struct {
	Direction Direction
	Wrap      bool
	Gap       float64 // between items on the main axis; also the line spacing when Wrap is set
	Items     Align   // align-items; AlignInherit here behaves as AlignStretch
	Justify   Justify
}

// Rect is a child's final border box, relative to the content-box origin
// Flex was given.
type Rect struct{ X, Y, W, H float64 }

// Line is one flex line. Rects follow child declaration order. CrossSize is
// the line's cross-axis extent: a single nowrap line spans the container's
// whole cross axis (so stretch reaches the content-box edge, matching the
// canvas engine's established stretch), while each wrapped line shrinks to
// its largest item outer cross size, pre-stretch.
type Line struct {
	Rects     []Rect
	CrossSize float64
}

// Flex lays children out inside the container's content box (innerW ×
// innerH, padding already removed) and returns one Line per flex line, in
// child order. A nil children slice yields nil.
func Flex(innerW, innerH float64, st Style, children []Child) []Line {
	if len(children) == 0 {
		return nil
	}
	row := st.Direction != Column

	innerMain, innerCross := innerW, innerH
	if !row {
		innerMain, innerCross = innerH, innerW
	}

	// Hypothetical main sizes (flex-basis): content size, or 0 for a
	// flex-basis:0 grower.
	basis := make([]float64, len(children))
	for i, c := range children {
		basis[i] = mainOf(c, row)
		if c.Grow > 0 && !c.BasisAuto {
			basis[i] = 0
		}
	}

	// Break lines on the hypothetical main size, per CSS flexbox; the first
	// item always stays on its line even when it overflows alone.
	var lines [][]int
	var lineBase []float64 // per-line Σ(basis+main margins) + gaps, pre-grow
	var cur []int
	curMain := 0.0
	flush := func() {
		if len(cur) > 0 {
			lines = append(lines, cur)
			lineBase = append(lineBase, curMain)
			cur = nil
			curMain = 0
		}
	}
	for i, c := range children {
		itemMain := basis[i] + mainMargin(c, row)
		if st.Wrap && len(cur) > 0 && curMain+st.Gap+itemMain > innerMain {
			flush()
		}
		if len(cur) > 0 {
			curMain += st.Gap
		}
		cur = append(cur, i)
		curMain += itemMain
	}
	flush()

	out := make([]Line, len(lines))
	crossPos := 0.0
	for li, idxs := range lines {
		n := len(idxs)
		sizes := make([]float64, n)
		totalGrow := 0.0
		for k, i := range idxs {
			sizes[k] = basis[i]
			if children[i].Grow > 0 {
				totalGrow += children[i].Grow
			}
		}

		// Distribute leftover main space to growers, proportional to Grow;
		// negative space never shrinks (flex items keep their basis).
		lineMain := lineBase[li]
		if free := innerMain - lineMain; free > 0 && totalGrow > 0 {
			lineMain = 0
			for k, i := range idxs {
				if g := children[i].Grow; g > 0 {
					sizes[k] += free * g / totalGrow
				}
				if k > 0 {
					lineMain += st.Gap
				}
				lineMain += sizes[k] + mainMargin(children[i], row)
			}
		}

		lineCross := innerCross
		if st.Wrap {
			lineCross = 0
			for _, i := range idxs {
				if outer := crossOf(children[i], row) + crossMargin(children[i], row); outer > lineCross {
					lineCross = outer
				}
			}
		}

		offset, extra := justifyOffsets(st.Justify, innerMain-lineMain, n)

		line := Line{Rects: make([]Rect, n), CrossSize: lineCross}
		mainPos := offset
		for k, i := range idxs {
			c := children[i]
			if k > 0 {
				mainPos += st.Gap + extra
			}
			mainStart := mainPos + mainStartMargin(c, row)
			mainPos += mainMargin(c, row) + sizes[k]

			cross := crossOf(c, row)
			align := c.AlignSelf
			if align == AlignInherit {
				align = st.Items
			}
			if align == AlignInherit {
				align = AlignStretch
			}
			crossStart := crossStartMargin(c, row)
			if align == AlignStretch && cross == 0 {
				cross = lineCross - crossMargin(c, row)
				if cross < 0 {
					cross = 0
				}
			}
			var itemCrossPos float64
			switch align {
			case AlignCenter:
				itemCrossPos = (lineCross-cross-crossMargin(c, row))/2 + crossStart
			case AlignEnd:
				itemCrossPos = lineCross - cross - crossEndMargin(c, row)
			default: // AlignStart; AlignStretch with an explicit size keeps its start edge
				itemCrossPos = crossStart
			}

			if row {
				line.Rects[k] = Rect{X: mainStart, Y: crossPos + itemCrossPos, W: sizes[k], H: cross}
			} else {
				line.Rects[k] = Rect{X: crossPos + itemCrossPos, Y: mainStart, W: cross, H: sizes[k]}
			}
		}
		out[li] = line
		crossPos += lineCross + st.Gap
	}
	return out
}

// justifyOffsets turns leftover main-axis space into a leading offset plus
// an extra inter-item gap. Leftover <= 0 (overflow) behaves as start: items
// keep their size and spill toward the main-end edge, never shrink.
func justifyOffsets(j Justify, leftover float64, n int) (offset, extra float64) {
	if leftover <= 0 {
		return 0, 0
	}
	switch j {
	case JustifyCenter:
		return leftover / 2, 0
	case JustifyEnd:
		return leftover, 0
	case JustifySpaceBetween:
		if n < 2 {
			return 0, 0
		}
		return 0, leftover / float64(n-1)
	case JustifySpaceAround:
		extra = leftover / float64(n)
		return extra / 2, extra
	case JustifySpaceEvenly:
		extra = leftover / float64(n+1)
		return extra, extra
	default: // JustifyStart
		return 0, 0
	}
}

func mainOf(c Child, row bool) float64 {
	if row {
		return c.W
	}
	return c.H
}

func crossOf(c Child, row bool) float64 {
	if row {
		return c.H
	}
	return c.W
}

func mainMargin(c Child, row bool) float64 {
	if row {
		return c.MarginL + c.MarginR
	}
	return c.MarginT + c.MarginB
}

func crossMargin(c Child, row bool) float64 {
	if row {
		return c.MarginT + c.MarginB
	}
	return c.MarginL + c.MarginR
}

func mainStartMargin(c Child, row bool) float64 {
	if row {
		return c.MarginL
	}
	return c.MarginT
}

func crossStartMargin(c Child, row bool) float64 {
	if row {
		return c.MarginT
	}
	return c.MarginL
}

func crossEndMargin(c Child, row bool) float64 {
	if row {
		return c.MarginB
	}
	return c.MarginR
}
