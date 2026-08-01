package layout

import (
	"reflect"
	"testing"
)

// inherit models how the canvas integrator maps an unset align (style key
// absent) onto the AlignInherit sentinel; explicit alignments map to the
// other Align constants.
func TestFlex(t *testing.T) {
	tests := []struct {
		name     string
		innerW   float64
		innerH   float64
		style    Style
		children []Child
		want     []Line
	}{
		{
			name:     "zero children yields nil",
			innerW:   200,
			innerH:   100,
			children: nil,
			want:     nil,
		},
		{
			name:     "single child",
			innerW:   200,
			innerH:   100,
			children: []Child{{W: 50, H: 30, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 50, H: 30}}, CrossSize: 100},
			},
		},
		{
			name:     "row stretch fills the cross axis",
			innerW:   200,
			innerH:   100,
			children: []Child{{W: 50, AlignSelf: AlignInherit}}, // auto cross size (H == 0)
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 50, H: 100}}, CrossSize: 100},
			},
		},
		{
			name:     "row align-items center",
			innerW:   200,
			innerH:   100,
			style:    Style{Items: AlignCenter},
			children: []Child{{W: 50, H: 40, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 30, W: 50, H: 40}}, CrossSize: 100},
			},
		},
		{
			name:     "row align-items end",
			innerW:   200,
			innerH:   100,
			style:    Style{Items: AlignEnd},
			children: []Child{{W: 50, H: 40, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 60, W: 50, H: 40}}, CrossSize: 100},
			},
		},
		{
			name:     "align-self overrides align-items",
			innerW:   200,
			innerH:   100,
			style:    Style{Items: AlignStart},
			children: []Child{{W: 50, H: 20, AlignSelf: AlignEnd}, {W: 50, H: 20, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 80, W: 50, H: 20},
					{X: 50, Y: 0, W: 50, H: 20},
				}, CrossSize: 100},
			},
		},
		{
			name:     "align-self start keeps an auto cross size",
			innerW:   200,
			innerH:   100,
			children: []Child{{W: 50, AlignSelf: AlignStart}}, // no stretch
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 50, H: 0}}, CrossSize: 100},
			},
		},
		{
			name:     "column stretch fills the cross axis",
			innerW:   200,
			innerH:   100,
			style:    Style{Direction: Column},
			children: []Child{{H: 30, AlignSelf: AlignInherit}}, // auto cross size (W == 0)
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 200, H: 30}}, CrossSize: 200},
			},
		},
		{
			name:     "gap between items",
			innerW:   200,
			innerH:   100,
			style:    Style{Gap: 10},
			children: []Child{{W: 50, H: 20, AlignSelf: AlignInherit}, {W: 50, H: 20, AlignSelf: AlignInherit}, {W: 50, H: 20, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 50, H: 20},
					{X: 60, Y: 0, W: 50, H: 20},
					{X: 120, Y: 0, W: 50, H: 20},
				}, CrossSize: 100},
			},
		},
		{
			name:     "justify center",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifyCenter},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 75, Y: 0, W: 50, H: 10},
					{X: 125, Y: 0, W: 50, H: 10},
					{X: 175, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "justify end",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifyEnd},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 150, Y: 0, W: 50, H: 10},
					{X: 200, Y: 0, W: 50, H: 10},
					{X: 250, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "justify space-between exact coordinates",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifySpaceBetween},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 50, H: 10},
					{X: 125, Y: 0, W: 50, H: 10},
					{X: 250, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "justify space-around exact coordinates",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifySpaceAround},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 25, Y: 0, W: 50, H: 10},
					{X: 125, Y: 0, W: 50, H: 10},
					{X: 225, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "justify space-evenly exact coordinates",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifySpaceEvenly},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}, {W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 37.5, Y: 0, W: 50, H: 10},
					{X: 125, Y: 0, W: 50, H: 10},
					{X: 212.5, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "space-between with a single item falls back to start",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifySpaceBetween},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 50, H: 10}}, CrossSize: 100},
			},
		},
		{
			name:     "space-around with a single item centers it",
			innerW:   300,
			innerH:   100,
			style:    Style{Justify: JustifySpaceAround},
			children: []Child{{W: 50, H: 10, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 125, Y: 0, W: 50, H: 10}}, CrossSize: 100},
			},
		},
		{
			name:     "wrap breaks into two lines",
			innerW:   200,
			innerH:   300,
			style:    Style{Wrap: true, Gap: 10},
			children: []Child{{W: 90, H: 40, AlignSelf: AlignInherit}, {W: 90, H: 40, AlignSelf: AlignInherit}, {W: 90, H: 40, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 90, H: 40},
					{X: 100, Y: 0, W: 90, H: 40},
				}, CrossSize: 40},
				{Rects: []Rect{
					{X: 0, Y: 50, W: 90, H: 40},
				}, CrossSize: 40},
			},
		},
		{
			name:     "wrapped line cross size stretches auto items to the line max",
			innerW:   200,
			innerH:   300,
			style:    Style{Wrap: true},
			children: []Child{{W: 90, H: 40, AlignSelf: AlignInherit}, {W: 90, AlignSelf: AlignInherit}, {W: 90, H: 25, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 90, H: 40},
					{X: 90, Y: 0, W: 90, H: 40}, // auto H stretched to the line's 40
				}, CrossSize: 40},
				{Rects: []Rect{
					{X: 0, Y: 40, W: 90, H: 25},
				}, CrossSize: 25},
			},
		},
		{
			name:     "justify applies per line when wrapped",
			innerW:   200,
			innerH:   300,
			style:    Style{Wrap: true, Justify: JustifyEnd},
			children: []Child{{W: 90, H: 40, AlignSelf: AlignInherit}, {W: 90, H: 40, AlignSelf: AlignInherit}, {W: 90, H: 40, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{
					{X: 20, Y: 0, W: 90, H: 40},
					{X: 110, Y: 0, W: 90, H: 40},
				}, CrossSize: 40},
				{Rects: []Rect{
					{X: 110, Y: 40, W: 90, H: 40},
				}, CrossSize: 40},
			},
		},
		{
			name:   "grow distributes free space, grow zero stays put",
			innerW: 400,
			innerH: 100,
			children: []Child{
				{W: 50, H: 10, AlignSelf: AlignInherit, Grow: 1, BasisAuto: true},
				{W: 50, H: 10, AlignSelf: AlignInherit}, // Grow == 0 control: untouched by the 250 free
				{W: 50, H: 10, AlignSelf: AlignInherit, Grow: 1, BasisAuto: true},
			},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 175, H: 10},
					{X: 175, Y: 0, W: 50, H: 10},
					{X: 225, Y: 0, W: 175, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:   "grow with flex-basis zero ignores content size",
			innerW: 400,
			innerH: 100,
			children: []Child{
				{W: 50, H: 10, AlignSelf: AlignInherit, Grow: 1}, // BasisAuto false: basis 0
				{W: 100, H: 10, AlignSelf: AlignInherit},
				{W: 50, H: 10, AlignSelf: AlignInherit, Grow: 1},
			},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 0, W: 150, H: 10},
					{X: 150, Y: 0, W: 100, H: 10},
					{X: 250, Y: 0, W: 150, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "negative space never shrinks",
			innerW:   100,
			innerH:   100,
			style:    Style{Justify: JustifyCenter}, // no effect on overflow
			children: []Child{{W: 150, H: 10, AlignSelf: AlignInherit, Grow: 1, BasisAuto: true}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 0, W: 150, H: 10}}, CrossSize: 100},
			},
		},
		{
			name:   "margins count on the main axis",
			innerW: 200,
			innerH: 100,
			children: []Child{
				{W: 50, H: 10, MarginL: 10, MarginR: 20, AlignSelf: AlignInherit},
				{W: 50, H: 10, AlignSelf: AlignInherit},
			},
			want: []Line{
				{Rects: []Rect{
					{X: 10, Y: 0, W: 50, H: 10},
					{X: 80, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:   "margins feed space-between",
			innerW: 200,
			innerH: 100,
			style:  Style{Justify: JustifySpaceBetween},
			children: []Child{
				{W: 50, H: 10, MarginL: 10, MarginR: 20, AlignSelf: AlignInherit},
				{W: 50, H: 10, AlignSelf: AlignInherit},
			},
			want: []Line{
				{Rects: []Rect{
					{X: 10, Y: 0, W: 50, H: 10},
					{X: 150, Y: 0, W: 50, H: 10},
				}, CrossSize: 100},
			},
		},
		{
			name:     "stretch deducts cross margins",
			innerW:   200,
			innerH:   100,
			children: []Child{{W: 50, MarginT: 10, MarginB: 5, AlignSelf: AlignInherit}},
			want: []Line{
				{Rects: []Rect{{X: 0, Y: 10, W: 50, H: 85}}, CrossSize: 100},
			},
		},
		{
			name:   "column justify center with margins",
			innerW: 200,
			innerH: 300,
			style:  Style{Direction: Column, Justify: JustifyCenter},
			children: []Child{
				{W: 40, H: 50, MarginT: 10, AlignSelf: AlignInherit},
				{W: 40, H: 50, AlignSelf: AlignInherit},
			},
			want: []Line{
				{Rects: []Rect{
					{X: 0, Y: 105, W: 40, H: 50},
					{X: 0, Y: 155, W: 40, H: 50},
				}, CrossSize: 200},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Flex(tt.innerW, tt.innerH, tt.style, tt.children)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Flex(%g, %g, %+v, %+v)\n got: %+v\nwant: %+v",
					tt.innerW, tt.innerH, tt.style, tt.children, got, tt.want)
			}
		})
	}
}
