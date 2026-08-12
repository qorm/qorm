// gen-mario-sprites generates pixel-art NES-style sprites for the mario example
// at 16x16 (then 2x scale to 32x32 for the cell). The output is real 8-bit
// pixel art: 16 colors palette, hand-drawn pixel grids, no anti-aliasing.
//
// Usage:  go run ./scripts/gen-mario-sprites
// Output: examples/mario/assets/*.png (overwrites AI-generated 16x16 PNGs)
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// NES 1-1 palette (curated subset, all opaque).
var (
	CTrans    = color.RGBA{0, 0, 0, 0}
	CSky      = color.RGBA{0x5C, 0x94, 0xFC, 0xFF}
	CBlack    = color.RGBA{0x00, 0x00, 0x00, 0xFF}
	CWhite    = color.RGBA{0xFC, 0xFC, 0xFC, 0xFF}
	CDark     = color.RGBA{0x40, 0x00, 0x00, 0xFF}
	CGray     = color.RGBA{0x84, 0x84, 0x84, 0xFF}
	CLight    = color.RGBA{0xFC, 0xB8, 0xB0, 0xFF}
	CRed      = color.RGBA{0xE4, 0x00, 0x58, 0xFF}
	CRedDk    = color.RGBA{0xA4, 0x00, 0x20, 0xFF}
	CBlue     = color.RGBA{0x00, 0x58, 0xF8, 0xFF}
	CBlueDk   = color.RGBA{0x00, 0x28, 0xA8, 0xFF}
	CBrown    = color.RGBA{0x90, 0x40, 0x00, 0xFF}
	CBrownDk  = color.RGBA{0x68, 0x28, 0x00, 0xFF}
	CBrownLt  = color.RGBA{0xC0, 0x70, 0x20, 0xFF}
	CYellow   = color.RGBA{0xF8, 0xB8, 0x00, 0xFF}
	CYellowDk = color.RGBA{0xB8, 0x70, 0x00, 0xFF}
	CGreenLt  = color.RGBA{0x80, 0xD0, 0x10, 0xFF}
	CGreen    = color.RGBA{0x00, 0xA8, 0x00, 0xFF}
	CGreenDk  = color.RGBA{0x00, 0x68, 0x00, 0xFF}
	CCoin     = color.RGBA{0xFC, 0xE0, 0x40, 0xFF}
	CCoinDk   = color.RGBA{0xB8, 0x80, 0x00, 0xFF}
	COrange   = color.RGBA{0xFC, 0x88, 0x00, 0xFF}
)

// setPx draws a pixel at (x,y) on a 16x16 grid; ignores out-of-range.
func setPx(g [][]color.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= 16 || y >= 16 {
		return
	}
	g[y][x] = c
}

// fillRect fills a 16x16 region [x0,x1)x[y0,y1).
func fillRect(g [][]color.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			setPx(g, x, y, c)
		}
	}
}

// newGrid allocates a fresh 16x16 grid filled with transparent.
func newGrid() [][]color.RGBA {
	g := make([][]color.RGBA, 16)
	for i := range g {
		g[i] = make([]color.RGBA, 16)
		for j := range g[i] {
			g[i][j] = CTrans
		}
	}
	return g
}

// writeSprite writes the 16x16 grid to PNG, scaled 2x to 32x32 (no AA, pixel-art).
func writeSprite(outDir, name string, g [][]color.RGBA) {
	up := image.NewRGBA(image.Rect(0, 0, 32, 32))
	// Clear to transparent
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			up.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
		}
	}
	// 2x nearest-neighbour upscale.
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := g[y][x]
			up.SetRGBA(x*2, y*2, c)
			up.SetRGBA(x*2+1, y*2, c)
			up.SetRGBA(x*2, y*2+1, c)
			up.SetRGBA(x*2+1, y*2+1, c)
		}
	}
	f, err := os.Create(filepath.Join(outDir, name+".png"))
	if err != nil {
		log.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := png.Encode(f, up); err != nil {
		log.Fatalf("encode %s: %v", name, err)
	}
}

// --- Sprite definitions ---

// ground: 1 row of green grass on top, brown earth body. NES 1-1 ground tile.
func makeGround() [][]color.RGBA {
	g := newGrid()
	// Top 2 rows: green grass with 2 tufts.
	fillRect(g, 0, 0, 16, 2, CGreen)
	setPx(g, 3, 1, CGreenLt)
	setPx(g, 4, 0, CGreenLt)
	setPx(g, 9, 1, CGreenLt)
	setPx(g, 10, 0, CGreenLt)
	// Transition row 2: dark green.
	fillRect(g, 0, 2, 16, 3, CGreenDk)
	// Earth body rows 3-15: brown with two-tone brick-like variation.
	for y := 3; y < 16; y++ {
		offset := 0
		if y%2 == 1 {
			offset = 8
		}
		for x := 0; x < 16; x++ {
			px := (x + offset) % 16
			switch {
			case px == 0 || px == 7:
				setPx(g, x, y, CBrownDk)
			case px == 1 || px == 6:
				setPx(g, x, y, CBrown)
			default:
				setPx(g, x, y, CBrownLt)
			}
		}
	}
	return g
}

// brick: classic Mario brick block, with horizontal mortar lines and 4 vertical
// brick rows offset on alternating courses.
func makeBrick() [][]color.RGBA {
	g := newGrid()
	// Dark border.
	fillRect(g, 0, 0, 16, 1, CBrownDk)
	fillRect(g, 0, 15, 16, 16, CBrownDk)
	fillRect(g, 0, 0, 1, 16, CBrownDk)
	fillRect(g, 15, 0, 16, 16, CBrownDk)
	// Mortar rows 4, 8, 12: black.
	for _, y := range []int{4, 8, 12} {
		fillRect(g, 1, y, 15, y+1, CBlack)
	}
	// Mortar vertical, offset per course.
	for _, y := range []int{0, 4} {
		setPx(g, 8, y, CBlack)
	}
	for _, y := range []int{4, 8, 12} {
		setPx(g, 4, y, CBlack)
		setPx(g, 12, y, CBlack)
	}
	for y := 8; y < 12; y++ {
		setPx(g, 4, y, CBlack)
		setPx(g, 12, y, CBlack)
	}
	// Body: brown with light highlight on top edge of each brick.
	for y := 0; y < 16; y++ {
		if y == 4 || y == 8 || y == 12 {
			continue
		}
		off := 0
		if y > 4 && y < 8 || y > 12 {
			off = 8
		}
		for x := 0; x < 16; x++ {
			px := (x + off) % 16
			switch px {
			case 0:
				setPx(g, x, y, CBlack)
			case 1:
				setPx(g, x, y, CBrownDk)
			case 2:
				setPx(g, x, y, CBrown)
			default:
				setPx(g, x, y, CBrownLt)
			}
		}
	}
	return g
}

// question: yellow ? block with rivets on the 4 corners.
func makeQuestion() [][]color.RGBA {
	g := newGrid()
	// Dark yellow border.
	fillRect(g, 0, 0, 16, 1, CYellowDk)
	fillRect(g, 0, 15, 16, 16, CYellowDk)
	fillRect(g, 0, 0, 1, 16, CYellowDk)
	fillRect(g, 15, 0, 16, 16, CYellowDk)
	// Body: bright yellow.
	fillRect(g, 1, 1, 15, 15, CYellow)
	// Rivets (dark dots) at corners.
	setPx(g, 2, 2, CBrownDk)
	setPx(g, 13, 2, CBrownDk)
	setPx(g, 2, 13, CBrownDk)
	setPx(g, 13, 13, CBrownDk)
	// "?" glyph in white with dark outline (mario-style).
	// Top curve: rows 3-4, cols 6-9
	for y := 3; y < 5; y++ {
		for x := 6; x < 10; x++ {
			setPx(g, x, y, CWhite)
		}
	}
	setPx(g, 5, 4, CWhite)
	setPx(g, 10, 4, CWhite)
	for y := 5; y < 7; y++ {
		setPx(g, 9, y, CWhite)
	}
	for x := 7; x < 10; x++ {
		setPx(g, x, 6, CWhite)
	}
	// Stem at col 7, rows 7-9
	for y := 7; y < 10; y++ {
		setPx(g, 7, y, CWhite)
	}
	// Dot at row 11.
	setPx(g, 7, 11, CWhite)
	// Dark outline pixels to give the ? definition.
	setPx(g, 5, 3, CBrownDk)
	setPx(g, 5, 5, CBrownDk)
	setPx(g, 10, 3, CBrownDk)
	setPx(g, 10, 5, CBrownDk)
	setPx(g, 10, 6, CBrownDk)
	setPx(g, 10, 7, CBrownDk)
	setPx(g, 6, 6, CBrownDk)
	setPx(g, 7, 10, CBrownDk)
	setPx(g, 7, 12, CBrownDk)
	return g
}

// used: gray/tan block after ? is hit.
func makeUsed() [][]color.RGBA {
	g := newGrid()
	fillRect(g, 0, 0, 16, 1, CBrownDk)
	fillRect(g, 0, 15, 16, 16, CBrownDk)
	fillRect(g, 0, 0, 1, 16, CBrownDk)
	fillRect(g, 15, 0, 16, 16, CBrownDk)
	fillRect(g, 1, 1, 15, 15, CBrown)
	setPx(g, 2, 2, CBrownDk)
	setPx(g, 13, 2, CBrownDk)
	setPx(g, 2, 13, CBrownDk)
	setPx(g, 13, 13, CBrownDk)
	return g
}

// coin: spinning gold coin (front face).
func makeCoin() [][]color.RGBA {
	g := newGrid()
	// Rounded rect: 4-12 wide, 2-14 tall.
	for y := 2; y < 14; y++ {
		for x := 4; x < 12; x++ {
			edge := y == 2 || y == 13 || x == 4 || x == 11
			if edge {
				setPx(g, x, y, CCoinDk)
			} else {
				setPx(g, x, y, CCoin)
			}
		}
	}
	// Top/bottom darken more.
	setPx(g, 5, 1, CCoinDk)
	setPx(g, 10, 1, CCoinDk)
	setPx(g, 5, 14, CCoinDk)
	setPx(g, 10, 14, CCoinDk)
	// Inner highlight (top-left).
	for _, p := range [][2]int{{5, 3}, {6, 3}, {5, 4}} {
		setPx(g, p[0], p[1], CWhite)
	}
	return g
}

// pipe_top: 1 cell of green pipe (the cap) — wide top lip.
func makePipeTop() [][]color.RGBA {
	g := newGrid()
	// Outer dark green border at the sides, top edge.
	fillRect(g, 0, 0, 16, 2, CGreenDk)
	fillRect(g, 0, 0, 1, 16, CGreenDk)
	fillRect(g, 15, 0, 16, 16, CGreenDk)
	// The "lip" overhang at top — rows 2-6, full width 16.
	fillRect(g, 0, 2, 16, 6, CGreen)
	// Light highlight on the left side of the lip.
	setPx(g, 1, 2, CGreenLt)
	setPx(g, 1, 3, CGreenLt)
	setPx(g, 1, 4, CGreenLt)
	setPx(g, 1, 5, CGreenLt)
	// Shadow band under the lip (dark green line).
	fillRect(g, 1, 6, 15, 7, CGreenDk)
	// The 1-cell body preview.
	fillRect(g, 1, 7, 4, 16, CGreen)     // main body
	fillRect(g, 5, 7, 13, 16, CGreenLt)  // bright mid
	fillRect(g, 14, 7, 15, 16, CGreenDk) // dark right edge
	return g
}

// pipe_body: 1 cell of straight pipe segment.
func makePipeBody() [][]color.RGBA {
	g := newGrid()
	fillRect(g, 0, 0, 1, 16, CGreenDk)
	fillRect(g, 15, 0, 16, 16, CGreenDk)
	// Main body: 3 vertical bands of color (light left, bright mid, dark right).
	fillRect(g, 1, 0, 3, 16, CGreen)
	fillRect(g, 4, 0, 13, 16, CGreenLt)
	fillRect(g, 13, 0, 15, 16, CGreen)
	return g
}

// flag: green flag at top, gray pole, small base.
func makeFlag() [][]color.RGBA {
	g := newGrid()
	// Ball at top (row 0).
	setPx(g, 7, 0, CGreenLt)
	setPx(g, 8, 0, CGreenLt)
	setPx(g, 7, 1, CGreen)
	setPx(g, 8, 1, CGreen)
	// Flag triangle (rows 2-5, cols 4-8).
	for y := 2; y < 6; y++ {
		for x := 4; x <= 8-(y-2); x++ {
			setPx(g, x, y, CGreen)
		}
	}
	// Pole: col 8, rows 6-15.
	for y := 6; y < 15; y++ {
		setPx(g, 8, y, CGray)
	}
	// Base block at row 15.
	fillRect(g, 7, 15, 11, 16, CGray)
	return g
}

// mario: small mario standing, classic red+blue.
func makeMario() [][]color.RGBA {
	g := newGrid()
	// Hat: row 0-3, red with brim.
	fillRect(g, 4, 0, 12, 1, CRed)
	fillRect(g, 3, 1, 13, 3, CRed)
	fillRect(g, 2, 3, 14, 4, CRed)
	// Hat brim: row 4.
	fillRect(g, 2, 4, 5, 5, CRed)
	fillRect(g, 5, 4, 6, 5, CLight) // face area
	fillRect(g, 6, 4, 12, 5, CRed)
	fillRect(g, 12, 4, 14, 5, CRed)
	// Face: rows 5-6.
	fillRect(g, 3, 5, 5, 7, CLight)
	setPx(g, 4, 5, CBlack)  // eye
	setPx(g, 10, 5, CBlack) // eye
	fillRect(g, 9, 5, 12, 6, CLight)
	setPx(g, 11, 5, CBlack)
	fillRect(g, 5, 6, 12, 7, CLight)
	// Sideburn (brown).
	setPx(g, 3, 6, CBrownDk)
	setPx(g, 12, 6, CBrownDk)
	// Body / overalls: rows 7-12, blue.
	fillRect(g, 4, 7, 12, 8, CRed)   // shirt under overall strap
	fillRect(g, 4, 8, 12, 13, CBlue) // overall
	setPx(g, 6, 8, CYellow)          // button
	setPx(g, 10, 8, CYellow)         // button
	fillRect(g, 4, 13, 12, 14, CBlue)
	// Arms: rows 9-11, red (sleeve).
	setPx(g, 3, 9, CRed)
	setPx(g, 12, 9, CRed)
	setPx(g, 3, 10, CRed)
	setPx(g, 12, 10, CRed)
	// Hands.
	setPx(g, 3, 11, CLight)
	setPx(g, 12, 11, CLight)
	// Legs: rows 14-15, brown shoes.
	fillRect(g, 5, 14, 7, 15, CBrownDk)
	fillRect(g, 7, 14, 9, 15, CBrown)
	fillRect(g, 9, 14, 11, 15, CBrownDk)
	fillRect(g, 4, 15, 8, 16, CBrownDk)
	fillRect(g, 8, 15, 12, 16, CBrown)
	return g
}

// mario_walk: mid-stride — same as mario but one leg forward.
func makeMarioWalk() [][]color.RGBA {
	g := makeMario()
	// Erase legs (rows 14-15) and redraw with stride.
	fillRect(g, 4, 14, 12, 16, CTrans)
	fillRect(g, 4, 14, 7, 15, CBrownDk)
	fillRect(g, 4, 15, 8, 16, CBrownDk)
	fillRect(g, 9, 14, 12, 15, CBrown)
	fillRect(g, 8, 15, 13, 16, CBrown)
	return g
}

// mario_jump: mario in mid-jump (one leg forward, arms up).
func makeMarioJump() [][]color.RGBA {
	g := makeMario()
	// Arms up: rows 4-7.
	setPx(g, 4, 5, CLight)
	setPx(g, 11, 5, CLight)
	setPx(g, 3, 6, CLight)
	setPx(g, 12, 6, CLight)
	setPx(g, 2, 7, CLight)
	setPx(g, 13, 7, CLight)
	// Hide side arms on the body.
	setPx(g, 3, 9, CTrans)
	setPx(g, 12, 9, CTrans)
	setPx(g, 3, 10, CTrans)
	setPx(g, 12, 10, CTrans)
	// Legs wider.
	fillRect(g, 4, 14, 12, 16, CTrans)
	fillRect(g, 3, 14, 8, 15, CBrownDk)
	fillRect(g, 3, 15, 8, 16, CBrownDk)
	fillRect(g, 8, 14, 13, 15, CBrown)
	fillRect(g, 8, 15, 13, 16, CBrown)
	return g
}

// goomba: brown mushroom enemy with eyes and feet.
func makeGoomba() [][]color.RGBA {
	g := newGrid()
	// Head/body: rows 2-12.
	fillRect(g, 3, 2, 13, 4, CBrown)
	fillRect(g, 2, 4, 14, 12, CBrown)
	// Top dark band.
	fillRect(g, 4, 2, 12, 3, CBrownDk)
	// Side dark edges.
	setPx(g, 2, 5, CBrownDk)
	setPx(g, 13, 5, CBrownDk)
	setPx(g, 2, 10, CBrownDk)
	setPx(g, 13, 10, CBrownDk)
	// Eyes (rows 5-7, cols 4-5 and 10-11).
	fillRect(g, 4, 5, 6, 7, CWhite)
	fillRect(g, 10, 5, 12, 7, CWhite)
	setPx(g, 5, 6, CBlack)
	setPx(g, 11, 6, CBlack)
	// Eyebrows angry.
	fillRect(g, 4, 4, 6, 5, CBlack)
	fillRect(g, 10, 4, 12, 5, CBlack)
	// Mouth/teeth area: rows 8-9.
	fillRect(g, 6, 9, 10, 11, CBlack)
	setPx(g, 7, 9, CWhite)
	setPx(g, 9, 9, CWhite)
	// Feet: rows 12-14.
	fillRect(g, 3, 12, 7, 14, CBrownDk)
	fillRect(g, 9, 12, 13, 14, CBrownDk)
	fillRect(g, 2, 14, 8, 16, CBlack)
	fillRect(g, 8, 14, 14, 16, CBlack)
	return g
}

// goomba_walk: goomba with squashed (1 frame) walk cycle.
func makeGoombaWalk() [][]color.RGBA {
	g := makeGoomba()
	// Shift feet inward — both feet close together.
	fillRect(g, 2, 12, 14, 16, CTrans)
	fillRect(g, 4, 12, 7, 14, CBrownDk)
	fillRect(g, 9, 12, 12, 14, CBrownDk)
	fillRect(g, 3, 14, 8, 16, CBlack)
	fillRect(g, 8, 14, 13, 16, CBlack)
	return g
}

// cloud: white puffy cloud (no gray background frame).
func makeCloud() [][]color.RGBA {
	g := newGrid()
	// Bottom flat: row 8.
	fillRect(g, 3, 8, 14, 10, CWhite)
	// Middle bulge.
	fillRect(g, 2, 6, 15, 8, CWhite)
	// Top puffs.
	fillRect(g, 4, 4, 13, 6, CWhite)
	setPx(g, 3, 6, CWhite)
	setPx(g, 13, 6, CWhite)
	setPx(g, 5, 3, CWhite)
	setPx(g, 9, 3, CWhite)
	setPx(g, 6, 2, CWhite)
	setPx(g, 10, 2, CWhite)
	// Subtle shadow on the bottom edge.
	for x := 3; x < 14; x++ {
		setPx(g, x, 9, CLight) // peach shadow (slight pink instead of full shadow)
	}
	// Outline pixels (light gray) for definition — never use sky color.
	for x := 3; x < 14; x++ {
		setPx(g, x, 10, CGray)
	}
	setPx(g, 2, 8, CGray)
	setPx(g, 14, 8, CGray)
	return g
}

// bush: low NES 1-1 hedge — flat top with 3 puffs, single thick green
// body sitting on the ground. No body highlights (those made it look
// like a planter pot).
func makeBush() [][]color.RGBA {
	g := newGrid()
	// 3 puffs on top (rows 5-7).
	fillRect(g, 2, 6, 5, 8, CGreen)
	fillRect(g, 4, 5, 6, 7, CGreen)
	fillRect(g, 6, 6, 10, 8, CGreen)
	fillRect(g, 7, 4, 9, 6, CGreen)
	fillRect(g, 10, 6, 14, 8, CGreen)
	fillRect(g, 10, 5, 12, 7, CGreen)
	// Flat top of the body (row 8, the line between puffs and body).
	fillRect(g, 1, 8, 15, 9, CGreen)
	// Body: rows 9-15 (the bulk of the bush, plain green).
	fillRect(g, 1, 9, 15, 16, CGreen)
	// Right edge dark line for definition.
	setPx(g, 14, 9, CGreenDk)
	setPx(g, 14, 10, CGreenDk)
	setPx(g, 14, 11, CGreenDk)
	return g
}

// hill: NES 1-1 big background hill. A wide half-dome with a flat base
// that sits on the ground line, two small bushes on the left, and a
// "step" cut into the right side. Drawn 16x16 with the dome filling the
// top 11 rows and the dark base filling rows 13-15.
func makeHill() [][]color.RGBA {
	g := newGrid()
	// Dome: rows 0-12, widening as we go down.
	for y := 0; y < 13; y++ {
		w := y + 1
		if w > 15 {
			w = 15
		}
		x0 := 8 - w/2
		x1 := 8 + w/2
		for x := x0; x < x1; x++ {
			setPx(g, x, y, CGreen)
		}
	}
	// Light highlights on the dome (left side).
	for y := 2; y < 10; y++ {
		w := y + 1
		if w > 15 {
			w = 15
		}
		x0 := 8 - w/2
		setPx(g, x0, y, CGreenLt)
		if w > 4 {
			setPx(g, x0+1, y, CGreenLt)
		}
	}
	// Dark shadow on the right side.
	for y := 4; y < 12; y++ {
		w := y + 1
		if w > 15 {
			w = 15
		}
		x1 := 8 + w/2
		setPx(g, x1-1, y, CGreenDk)
	}
	// Two small "bush" details on the left slope.
	setPx(g, 4, 7, CGreenLt)
	setPx(g, 5, 7, CGreenLt)
	setPx(g, 4, 8, CGreenLt)
	setPx(g, 5, 8, CGreenLt)
	// Step cut on the right slope (NES hill has a "lip" on right).
	for y := 7; y < 11; y++ {
		setPx(g, 12, y, CGreenDk)
		setPx(g, 13, y, CGreenDk)
	}
	// Base: dark green strip along the ground line.
	fillRect(g, 0, 13, 16, 16, CGreenDk)
	fillRect(g, 0, 12, 16, 13, CGreen)
	return g
}

// mushroom: classic power-up mushroom (red+white).
func makeMushroom() [][]color.RGBA {
	g := newGrid()
	// Cap: rows 1-9.
	fillRect(g, 3, 1, 13, 2, CRed)
	fillRect(g, 2, 2, 14, 3, CRed)
	fillRect(g, 1, 3, 15, 8, CRed)
	fillRect(g, 2, 8, 14, 9, CRed)
	// White spots on cap.
	setPx(g, 4, 3, CWhite)
	setPx(g, 11, 3, CWhite)
	setPx(g, 7, 5, CWhite)
	setPx(g, 9, 6, CWhite)
	setPx(g, 4, 7, CWhite)
	setPx(g, 12, 7, CWhite)
	// Cap dark outline.
	setPx(g, 1, 4, CRedDk)
	setPx(g, 14, 4, CRedDk)
	setPx(g, 1, 6, CRedDk)
	setPx(g, 14, 6, CRedDk)
	// Stem: rows 9-14, peach.
	fillRect(g, 4, 9, 12, 14, CLight)
	// Stem shadow on right.
	setPx(g, 11, 9, CRed)
	setPx(g, 12, 9, CRed)
	setPx(g, 11, 10, CRed)
	setPx(g, 11, 11, CRed)
	// Eyes.
	setPx(g, 6, 10, CBlack)
	setPx(g, 9, 10, CBlack)
	// Feet: row 14-15.
	fillRect(g, 3, 14, 13, 16, CLight)
	return g
}

// castle: NES end-of-level castle.
func makeCastle() [][]color.RGBA {
	g := newGrid()
	// Door at bottom center (rows 10-15, cols 6-9).
	fillRect(g, 6, 10, 10, 16, CBlack)
	// Body: rows 2-15, cols 1-14.
	fillRect(g, 1, 3, 15, 10, CGray)
	fillRect(g, 0, 3, 1, 10, CGray)
	// Battlements (top crenellations): rows 0-2, alternating.
	for x := 0; x < 16; x += 2 {
		fillRect(g, x, 0, x+1, 3, CGray)
	}
	// Door arch.
	setPx(g, 6, 10, CBlack)
	setPx(g, 9, 10, CBlack)
	// Window in door.
	fillRect(g, 7, 11, 9, 14, CDark)
	// Red roof strip just under battlements.
	fillRect(g, 0, 3, 16, 5, CGray)
	// A second-story window.
	fillRect(g, 3, 6, 6, 9, CDark)
	fillRect(g, 10, 6, 13, 9, CDark)
	// Lighter strip on the body.
	fillRect(g, 0, 9, 16, 10, CGray)
	// Base row 15 (ground).
	fillRect(g, 0, 15, 16, 16, CGreen)
	return g
}

// stair: 1 step of mario's pyramid staircase.
func makeStair() [][]color.RGBA {
	g := newGrid()
	// Step L-shape: rows 8-15 left-half, rows 12-15 right-half.
	// Actually stair tile in NES shows ONE step. We'll draw the top-left corner
	// of a step: a 1-cell rise.
	fillRect(g, 0, 8, 16, 16, CGray)
	// Top edge highlight (white).
	fillRect(g, 0, 8, 16, 9, CWhite)
	// Lighter top-step face.
	fillRect(g, 0, 9, 16, 10, CGray)
	// Darker bottom band.
	fillRect(g, 0, 14, 16, 16, CGray)
	setPx(g, 0, 14, CBlack)
	setPx(g, 0, 15, CBlack)
	return g
}

// used.bonus: 1-1 second ?-block produces a "mushroom" — same as question for now.
func makeSky() [][]color.RGBA {
	g := newGrid()
	fillRect(g, 0, 0, 16, 16, CSky)
	return g
}

func main() {
	outDir := "examples/mario/assets"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	sprites := map[string][][]color.RGBA{
		"ground":      makeGround(),
		"brick":       makeBrick(),
		"question":    makeQuestion(),
		"used":        makeUsed(),
		"coin":        makeCoin(),
		"pipe_top":    makePipeTop(),
		"pipe_body":   makePipeBody(),
		"flag":        makeFlag(),
		"mario":       makeMario(),
		"mario_walk":  makeMarioWalk(),
		"mario_jump":  makeMarioJump(),
		"goomba":      makeGoomba(),
		"goomba_walk": makeGoombaWalk(),
		"cloud":       makeCloud(),
		"bush":        makeBush(),
		"hill":        makeHill(),
		"mushroom":    makeMushroom(),
		"castle":      makeCastle(),
		"stair":       makeStair(),
		"sky":         makeSky(),
	}
	for name, g := range sprites {
		writeSprite(outDir, name, g)
	}
	log.Printf("wrote %d sprites to %s", len(sprites), outDir)
}
