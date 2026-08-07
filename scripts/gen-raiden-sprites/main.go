// gen-raiden-sprites regenerates the raiden example's pixel art at 32x32
// from hand-drawn 32x32 pixel grids. The AI-generated sprites shipped with
// the example are decent triangles but lack the visual punch the canvas
// engine can deliver — bigger player, recognizable terrain, and clearer
// silhouettes for the enemies make the game read as a shmup at a glance.
//
// Usage:  go run ./scripts/gen-raiden-sprites
// Output: examples/raiden/assets/*.png (overwrites the 32x32 AI ones)
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// Palette — shmup-friendly colors with proper light/mid/dark per hue so
// each sprite has visible depth without anti-aliasing.
var (
	CTrans = color.RGBA{0, 0, 0, 0}
	CBlack = color.RGBA{0x10, 0x10, 0x18, 0xFF}
	CWhite = color.RGBA{0xFC, 0xFC, 0xFC, 0xFF}
	CGray  = color.RGBA{0x88, 0x88, 0xA0, 0xFF}
	CGrayDk = color.RGBA{0x44, 0x48, 0x58, 0xFF}

	// Red (player ship, enemies)
	CRedLt = color.RGBA{0xFF, 0x88, 0x88, 0xFF}
	CRed   = color.RGBA{0xE4, 0x38, 0x38, 0xFF}
	CRedDk = color.RGBA{0xA0, 0x18, 0x18, 0xFF}

	// Blue (cockpit, lasers)
	CBlueLt = color.RGBA{0xA8, 0xE4, 0xFF, 0xFF}
	CBlue   = color.RGBA{0x40, 0x90, 0xE8, 0xFF}
	CBlueDk = color.RGBA{0x18, 0x40, 0xA0, 0xFF}

	// Yellow (bullets, accents)
	CYellow = color.RGBA{0xFC, 0xDC, 0x40, 0xFF}
	CYelDk  = color.RGBA{0xC0, 0x80, 0x10, 0xFF}

	// Orange (flames)
	COrLt = color.RGBA{0xFF, 0xE0, 0x60, 0xFF}
	COr   = color.RGBA{0xFF, 0x88, 0x10, 0xFF}
	COrDk = color.RGBA{0xC0, 0x40, 0x00, 0xFF}

	// Green (grass, powerups)
	CGrLt  = color.RGBA{0xA8, 0xE8, 0x60, 0xFF}
	CGreen = color.RGBA{0x48, 0xB0, 0x28, 0xFF}
	CGrDk  = color.RGBA{0x18, 0x60, 0x10, 0xFF}

	// Brown (terrain)
	CBrownLt = color.RGBA{0xC0, 0x80, 0x40, 0xFF}
	CBrown   = color.RGBA{0x80, 0x48, 0x20, 0xFF}
	CBrownDk = color.RGBA{0x40, 0x20, 0x10, 0xFF}

	// Purple (boss accents)
	CPurLt = color.RGBA{0xE0, 0xA0, 0xFF, 0xFF}
	CPur   = color.RGBA{0x80, 0x40, 0xC0, 0xFF}
	CPurDk = color.RGBA{0x40, 0x10, 0x80, 0xFF}
)

const size = 32

type grid [size][size]color.RGBA

func newGrid() grid {
	var g grid
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			g[y][x] = CTrans
		}
	}
	return g
}

func setPx(g *grid, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= size || y >= size {
		return
	}
	g[y][x] = c
}

func fillRect(g *grid, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			setPx(g, x, y, c)
		}
	}
}

// writePng writes the 32x32 grid directly to a 32x32 PNG (no upscale — the
// canvas engine can use intrinsic size + style scale for display).
func writePng(outDir, name string, g grid) {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, g[y][x])
		}
	}
	f, err := os.Create(filepath.Join(outDir, name+".png"))
	if err != nil {
		log.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatalf("encode %s: %v", name, err)
	}
	log.Printf("  %s.png", name)
}

// -----------------------------------------------------------------------------
// Player — red triangular fighter with blue cockpit, pointed nose up.
// Drawn as: nose (top) → wings (middle) → tail (bottom). Clean silhouette.
// -----------------------------------------------------------------------------
func makePlayer() grid {
	g := newGrid()
	// The plane points UP. Nose at y=2..6, wings at y=12..18, tail at y=22..28.

	// --- Outline (black) ---
	// Nose tip (narrow)
	for y := 2; y < 6; y++ {
		w := 2 + (y - 2) // 2, 3, 4, 5
		for x := 16 - w; x <= 15+w; x++ {
			setPx(&g, x, y, CBlack)
		}
	}
	// Fuselage widening
	for y := 6; y < 12; y++ {
		w := 5 + (y - 6)*2 // 5, 7, 9, 11, 13
		for x := 16 - w; x <= 15+w; x++ {
			setPx(&g, x, y, CBlack)
		}
	}
	// Wings (max width row at y=12)
	for x := 0; x < size; x++ {
		setPx(&g, x, 12, CBlack)
		setPx(&g, x, 13, CBlack)
	}
	// Wing leading edge
	setPx(&g, 1, 14, CBlack)
	setPx(&g, 30, 14, CBlack)
	// Wing tips slope down
	for y := 14; y < 20; y++ {
		setPx(&g, y-13, y, CBlack) // left
		setPx(&g, 31-(y-14), y, CBlack) // right
	}
	// Wing underside
	for y := 14; y < 20; y++ {
		for x := y - 12; x <= 31-(y-14); x++ {
			if y == 14 {
				continue // already set above
			}
		}
	}
	// Fuselage between wings
	for y := 14; y < 22; y++ {
		for x := 11; x < 21; x++ {
			setPx(&g, x, y, CBlack)
		}
	}
	// Tail (narrower, V-shape)
	for y := 22; y < 28; y++ {
		w := 9 - (y - 22) // 9, 7, 5, 3, 1
		if w < 1 {
			w = 1
		}
		for x := 16 - w; x <= 15+w; x++ {
			setPx(&g, x, y, CBlack)
		}
	}

	// --- Fill body (red) ---
	for y := 2; y < 28; y++ {
		for x := 0; x < size; x++ {
			if g[y][x] == CBlack {
				g[y][x] = CRed
			}
		}
	}

	// --- Shading ---
	// Left side highlight (lighter red)
	for y := 7; y < 22; y++ {
		for x := 0; x < 16; x++ {
			if g[y][x] == CRed {
				g[y][x] = CRedLt
			}
		}
	}
	// Right side shadow
	for y := 7; y < 22; y++ {
		for x := 17; x < size; x++ {
			if g[y][x] == CRed {
				g[y][x] = CRedDk
			}
		}
	}
	// Re-apply black outline (shading step may have leaked past)
	for y := 0; y < size; y++ {
		// Re-outline wings
		if y == 12 || y == 13 {
			for x := 0; x < size; x++ {
				if g[y][x] == CRed || g[y][x] == CRedLt || g[y][x] == CRedDk {
					g[y][x] = CRed
				}
			}
		}
	}
	// Wing edge silhouette (clear left/right of the main body)
	for y := 14; y < 20; y++ {
		// left tip
		x := y - 13
		if x >= 0 && x < size {
			g[y][x] = CBlack
		}
		// right tip
		x = 31 - (y - 14)
		if x >= 0 && x < size {
			g[y][x] = CBlack
		}
	}

	// --- Cockpit (blue glass, on the nose) ---
	// Position: roughly y=8..14, x=13..18
	for y := 8; y < 14; y++ {
		for x := 13; x < 19; x++ {
			g[y][x] = CBlueDk
		}
	}
	// Inner glass
	for y := 9; y < 12; y++ {
		for x := 14; x < 18; x++ {
			g[y][x] = CBlue
		}
	}
	// Specular highlight
	g[9][15] = CBlueLt
	g[10][15] = CBlueLt
	// Cockpit rim
	for y := 8; y < 14; y++ {
		g[y][12] = CBlack
		g[y][19] = CBlack
	}
	for x := 12; x < 20; x++ {
		g[7][x] = CBlack
		g[14][x] = CBlack
	}

	// --- Wing leading edge (white) ---
	for y := 12; y < 14; y++ {
		for x := 4; x < 28; x++ {
			if g[y][x] == CRed {
				g[y][x] = CWhite
			}
		}
	}

	// --- Tail engine glow (orange) ---
	for y := 24; y < 28; y++ {
		for x := 14; x < 18; x++ {
			g[y][x] = COrDk
		}
	}
	for y := 25; y < 27; y++ {
		for x := 15; x < 17; x++ {
			g[y][x] = COr
		}
	}
	g[26][15] = CYellow
	g[26][16] = CYellow

	// --- Engine flame trail (below tail) ---
	flameY := []int{28, 29, 30}
	for _, y := range flameY {
		if y < size {
			g[y][15] = COr
			g[y][16] = COr
		}
	}
	if 30 < size {
		g[30][15] = CYellow
		g[30][16] = CYellow
	}

	// --- Wing tip lights (red/green nav lights, classic aircraft) ---
	g[15][2] = CGreen
	g[16][2] = CRed

	// --- Cockpit canopy frame (dark line down the middle) ---
	for y := 8; y < 14; y++ {
		g[y][15] = CBlueDk
		g[y][16] = CBlueDk
	}
	// Restore highlight
	g[9][14] = CBlueLt

	return g
}

// -----------------------------------------------------------------------------
// Ground — proper terrain tile: green grass strip on top, brown stone below
// with darker speckles for texture. 32x32 tile.
// -----------------------------------------------------------------------------
func makeGround() grid {
	g := newGrid()
	// Grass strip (top 6 rows)
	for y := 0; y < 6; y++ {
		for x := 0; x < size; x++ {
			g[y][x] = CGreen
		}
	}
	// Grass highlights
	for x := 0; x < size; x++ {
		g[0][x] = CGrLt
	}
	// Grass darker tufts
	for x := 2; x < size; x += 5 {
		g[4][x] = CGrDk
		g[5][x] = CGrDk
	}
	for x := 4; x < size; x += 7 {
		g[2][x] = CGrDk
	}
	// Grass line (darker bottom of grass)
	for x := 0; x < size; x++ {
		g[5][x] = CGrDk
	}
	// Stone body (rows 6-31)
	for y := 6; y < size; y++ {
		for x := 0; x < size; x++ {
			g[y][x] = CBrown
		}
	}
	// Stone speckles (highlights + shadows for texture)
	// Pattern: irregular, 3-4 light spots and 2-3 dark spots
	lightSpots := [][2]int{
		{3, 9}, {11, 11}, {18, 14}, {24, 8}, {7, 18}, {27, 22}, {14, 24}, {5, 27}, {22, 28}, {9, 14}, {28, 18}, {16, 8},
	}
	for _, s := range lightSpots {
		g[s[1]][s[0]] = CBrownLt
	}
	darkSpots := [][2]int{
		{6, 10}, {15, 13}, {22, 11}, {3, 16}, {25, 16}, {12, 20}, {20, 22}, {8, 25}, {26, 27}, {1, 30}, {17, 30}, {28, 30},
	}
	for _, s := range darkSpots {
		g[s[1]][s[0]] = CBrownDk
	}
	// A few pebbles
	g[12][3], g[12][4] = CBrownLt, CBrownLt
	g[20][14], g[20][15] = CBrownDk, CBrownDk
	g[26][6] = CBrownLt
	g[28][19] = CBrownDk
	// Edge dark line below grass
	for x := 0; x < size; x++ {
		g[6][x] = CBrownDk
	}
	// A vertical crack
	for y := 8; y < 20; y++ {
		g[y][10] = CBrownDk
	}
	return g
}

// -----------------------------------------------------------------------------
// Boss — large red warship with purple accents, 4 turrets visible
// -----------------------------------------------------------------------------
func makeBoss() grid {
	g := newGrid()
	// Outline (triangular front)
	// y=0..4: nose
	for x := 14; x <= 17; x++ {
		setPx(&g, x, 0, CBlack)
	}
	for x := 12; x <= 19; x++ {
		setPx(&g, x, 1, CBlack)
	}
	for x := 10; x <= 21; x++ {
		setPx(&g, x, 2, CBlack)
	}
	for x := 8; x <= 23; x++ {
		setPx(&g, x, 3, CBlack)
	}
	for x := 6; x <= 25; x++ {
		setPx(&g, x, 4, CBlack)
	}
	// Wings (max width)
	for y := 4; y < 14; y++ {
		for x := 0; x < size; x++ {
			if x <= 3 || x >= 28 {
				setPx(&g, x, y, CBlack)
			}
		}
	}
	// Body fill
	for y := 0; y < 24; y++ {
		for x := 4; x < 28; x++ {
			if g[y][x] == CBlack {
				g[y][x] = CRed
			}
		}
	}
	// Wing tips
	for y := 4; y < 14; y++ {
		if g[y][0] == CBlack {
			g[y][0] = CRed
		}
		if g[y][31] == CBlack {
			g[y][31] = CRed
		}
	}
	// Lower body (rectangular)
	for y := 14; y < 28; y++ {
		for x := 6; x < 26; x++ {
			setPx(&g, x, y, CRed)
		}
	}
	// Outline lower body
	for y := 14; y < 28; y++ {
		setPx(&g, 5, y, CBlack)
		setPx(&g, 26, y, CBlack)
	}
	for x := 5; x < 27; x++ {
		setPx(&g, x, 27, CBlack)
	}
	for x := 5; x < 27; x++ {
		setPx(&g, x, 28, CBlack)
	}
	// Turrets (4 small bumps on top of body)
	for _, tx := range []int{9, 14, 17, 22} {
		setPx(&g, tx, 13, CBlack)
		setPx(&g, tx+1, 13, CBlack)
		setPx(&g, tx, 14, CGray)
		setPx(&g, tx+1, 14, CGrayDk)
	}
	// Cockpit (purple canopy at top)
	for y := 4; y < 11; y++ {
		for x := 13; x < 19; x++ {
			setPx(&g, x, y, CPur)
		}
	}
	for y := 5; y < 9; y++ {
		for x := 14; x < 18; x++ {
			setPx(&g, x, y, CPurLt)
		}
	}
	// Cockpit outline
	for x := 12; x < 20; x++ {
		setPx(&g, x, 4, CBlack)
	}
	for x := 12; x < 20; x++ {
		setPx(&g, x, 11, CBlack)
	}
	setPx(&g, 12, 4, CBlack)
	setPx(&g, 19, 4, CBlack)
	// Engine glow (red dot in middle)
	setPx(&g, 15, 20, CYellow)
	setPx(&g, 16, 20, CYellow)
	setPx(&g, 14, 21, COr)
	setPx(&g, 17, 21, COr)
	setPx(&g, 13, 22, COrDk)
	setPx(&g, 18, 22, COrDk)
	// Side highlights
	for y := 14; y < 27; y++ {
		setPx(&g, 6, y, CRedLt)
		setPx(&g, 25, y, CRedDk)
	}
	// White leading edge on wings
	for y := 4; y < 14; y++ {
		setPx(&g, 4, y, CWhite)
		setPx(&g, 27, y, CWhite)
	}
	// Bottom thrusters
	for x := 8; x < 24; x += 4 {
		setPx(&g, x, 29, CGrayDk)
		setPx(&g, x+1, 29, CGrayDk)
	}
	return g
}

// -----------------------------------------------------------------------------
// Enemy Bomber — green/gray bomber with wings
// -----------------------------------------------------------------------------
func makeEnemyBomber() grid {
	g := newGrid()
	// Outline
	for x := 12; x <= 19; x++ {
		setPx(&g, x, 4, CBlack)
	}
	for x := 10; x <= 21; x++ {
		setPx(&g, x, 5, CBlack)
	}
	for x := 8; x <= 23; x++ {
		setPx(&g, x, 6, CBlack)
	}
	for x := 6; x <= 25; x++ {
		setPx(&g, x, 7, CBlack)
	}
	for y := 7; y < 22; y++ {
		setPx(&g, 3, y, CBlack)
		setPx(&g, 28, y, CBlack)
	}
	for y := 8; y < 20; y++ {
		setPx(&g, 2, y, CBlack)
		setPx(&g, 29, y, CBlack)
	}
	// Fill body
	for y := 4; y < 25; y++ {
		for x := 4; x < 28; x++ {
			if g[y][x] == CBlack {
				g[y][x] = CGray
			}
		}
	}
	// Wing tip
	for y := 8; y < 20; y++ {
		if g[y][2] == CBlack {
			g[y][2] = CGray
		}
		if g[y][29] == CBlack {
			g[y][29] = CGray
		}
	}
	// Bomber body (darker fuselage stripe)
	for y := 8; y < 22; y++ {
		for x := 12; x < 20; x++ {
			setPx(&g, x, y, CGrayDk)
		}
	}
	// Cockpit (small green)
	for y := 9; y < 14; y++ {
		for x := 14; x < 18; x++ {
			setPx(&g, x, y, CGreen)
		}
	}
	for y := 10; y < 12; y++ {
		for x := 15; x < 17; x++ {
			setPx(&g, x, y, CGrLt)
		}
	}
	// Engine glow
	setPx(&g, 7, 14, CYellow)
	setPx(&g, 8, 14, CYellow)
	setPx(&g, 23, 14, CYellow)
	setPx(&g, 24, 14, CYellow)
	// White leading edge on wings
	setPx(&g, 3, 8, CWhite)
	setPx(&g, 28, 8, CWhite)
	setPx(&g, 2, 9, CWhite)
	setPx(&g, 29, 9, CWhite)
	// Bottom edge
	for x := 4; x < 28; x++ {
		setPx(&g, x, 24, CBlack)
	}
	return g
}

// -----------------------------------------------------------------------------
// Enemy Heli — small helicopter with rotor
// -----------------------------------------------------------------------------
func makeEnemyHeli() grid {
	g := newGrid()
	// Rotor (top horizontal bar)
	for x := 4; x < 28; x++ {
		setPx(&g, x, 5, CBlack)
	}
	for x := 6; x < 26; x++ {
		setPx(&g, x, 6, CGrayDk)
	}
	// Body (oval)
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			setPx(&g, x, y, CGray)
		}
	}
	// Body outline
	for y := 8; y < 24; y++ {
		setPx(&g, 7, y, CBlack)
		setPx(&g, 24, y, CBlack)
	}
	for x := 8; x < 24; x++ {
		setPx(&g, x, 7, CBlack)
		setPx(&g, x, 23, CBlack)
	}
	// Cockpit (cyan)
	for y := 10; y < 15; y++ {
		for x := 11; x < 21; x++ {
			setPx(&g, x, y, CBlue)
		}
	}
	for y := 11; y < 13; y++ {
		for x := 13; x < 19; x++ {
			setPx(&g, x, y, CBlueLt)
		}
	}
	// Tail
	setPx(&g, 24, 12, CBlack)
	setPx(&g, 25, 11, CBlack)
	setPx(&g, 26, 11, CBlack)
	setPx(&g, 27, 10, CBlack)
	// Landing skids
	for y := 21; y < 24; y++ {
		setPx(&g, 10, y, CBlack)
		setPx(&g, 22, y, CBlack)
	}
	for x := 10; x < 22; x++ {
		setPx(&g, x, 23, CBlack)
	}
	// Rotor center
	setPx(&g, 15, 4, CBlack)
	setPx(&g, 16, 4, CBlack)
	setPx(&g, 15, 5, CBlack)
	setPx(&g, 16, 5, CBlack)
	return g
}

// -----------------------------------------------------------------------------
// Enemy Turret — base with cannon barrel
// -----------------------------------------------------------------------------
func makeEnemyTurret() grid {
	g := newGrid()
	// Base (bottom)
	for y := 20; y < 28; y++ {
		for x := 4; x < 28; x++ {
			setPx(&g, x, y, CGrayDk)
		}
	}
	for y := 20; y < 28; y++ {
		setPx(&g, 3, y, CBlack)
		setPx(&g, 28, y, CBlack)
	}
	for x := 3; x < 29; x++ {
		setPx(&g, x, 19, CBlack)
		setPx(&g, x, 28, CBlack)
	}
	// Cannon barrel
	for y := 8; y < 22; y++ {
		for x := 14; x < 18; x++ {
			setPx(&g, x, y, CGray)
		}
	}
	for y := 8; y < 22; y++ {
		setPx(&g, 13, y, CBlack)
		setPx(&g, 18, y, CBlack)
	}
	for x := 13; x < 19; x++ {
		setPx(&g, x, 7, CBlack)
	}
	// Cannon tip
	setPx(&g, 14, 7, CGray)
	setPx(&g, 15, 6, CGrayDk)
	setPx(&g, 16, 6, CGrayDk)
	setPx(&g, 17, 7, CGray)
	// Base highlight
	for x := 4; x < 28; x++ {
		setPx(&g, x, 21, CGray)
	}
	return g
}

// -----------------------------------------------------------------------------
// Enemy Small — small red scout
// -----------------------------------------------------------------------------
func makeEnemySmall() grid {
	g := newGrid()
	// Diamond outline
	for x := 14; x <= 17; x++ {
		setPx(&g, x, 6, CBlack)
	}
	for x := 12; x <= 19; x++ {
		setPx(&g, x, 7, CBlack)
	}
	for x := 10; x <= 21; x++ {
		setPx(&g, x, 8, CBlack)
	}
	for x := 8; x <= 23; x++ {
		setPx(&g, x, 9, CBlack)
	}
	for y := 9; y < 22; y++ {
		setPx(&g, 6, y, CBlack)
		setPx(&g, 25, y, CBlack)
	}
	for x := 6; x < 26; x++ {
		setPx(&g, x, 22, CBlack)
	}
	// Fill
	for y := 7; y < 25; y++ {
		for x := 7; x < 25; x++ {
			if g[y][x] == CBlack {
				g[y][x] = CRed
			}
		}
	}
	// Center dark stripe
	for y := 10; y < 22; y++ {
		setPx(&g, 15, y, CRedDk)
		setPx(&g, 16, y, CRedDk)
	}
	// Cockpit (yellow eye)
	for y := 12; y < 17; y++ {
		for x := 13; x < 19; x++ {
			setPx(&g, x, y, CYellow)
		}
	}
	setPx(&g, 15, 14, CWhite)
	setPx(&g, 16, 14, CWhite)
	// Wing leading edge
	setPx(&g, 7, 10, CWhite)
	setPx(&g, 24, 10, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Player bullet — small yellow projectile with white core
// -----------------------------------------------------------------------------
func makeBullet() grid {
	g := newGrid()
	// Vertical bullet shape
	for y := 10; y < 22; y++ {
		setPx(&g, 15, y, CYellow)
		setPx(&g, 16, y, CYellow)
	}
	// Core
	for y := 12; y < 20; y++ {
		setPx(&g, 14, y, CWhite)
		setPx(&g, 17, y, CWhite)
	}
	// Tip
	setPx(&g, 15, 9, CYelDk)
	setPx(&g, 16, 9, CYelDk)
	setPx(&g, 15, 22, CYelDk)
	setPx(&g, 16, 22, CYelDk)
	// Halo
	for y := 11; y < 21; y++ {
		setPx(&g, 13, y, COrLt)
		setPx(&g, 18, y, COrLt)
	}
	return g
}

// -----------------------------------------------------------------------------
// Laser — wide red beam
// -----------------------------------------------------------------------------
func makeLaser() grid {
	g := newGrid()
	for y := 4; y < 28; y++ {
		for x := 12; x < 20; x++ {
			setPx(&g, x, y, CRed)
		}
	}
	// White core
	for y := 4; y < 28; y++ {
		for x := 14; x < 18; x++ {
			setPx(&g, x, y, CWhite)
		}
	}
	// Edges
	for y := 4; y < 28; y++ {
		setPx(&g, 12, y, CRedDk)
		setPx(&g, 19, y, CRedDk)
	}
	// Tip
	for y := 0; y < 4; y++ {
		setPx(&g, 15, y, CWhite)
		setPx(&g, 16, y, CWhite)
		setPx(&g, 14, y+1, CRed)
		setPx(&g, 17, y+1, CRed)
		setPx(&g, 13, y+2, CRedDk)
		setPx(&g, 18, y+2, CRedDk)
	}
	return g
}

// -----------------------------------------------------------------------------
// Plasma — purple energy ball
// -----------------------------------------------------------------------------
func makePlasma() grid {
	g := newGrid()
	// Outer ring
	for dy := -10; dy <= 10; dy++ {
		for dx := -10; dx <= 10; dx++ {
			d := dx*dx + dy*dy
			if d > 64 && d <= 100 {
				setPx(&g, 16+dx, 16+dy, CPurDk)
			}
		}
	}
	// Middle
	for dy := -8; dy <= 8; dy++ {
		for dx := -8; dx <= 8; dx++ {
			d := dx*dx + dy*dy
			if d > 25 && d <= 64 {
				setPx(&g, 16+dx, 16+dy, CPur)
			}
		}
	}
	// Inner
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
			d := dx*dx + dy*dy
			if d <= 25 {
				setPx(&g, 16+dx, 16+dy, CPurLt)
			}
		}
	}
	// White core
	setPx(&g, 16, 16, CWhite)
	setPx(&g, 15, 15, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Missile — red/white striped rocket
// -----------------------------------------------------------------------------
func makeMissile() grid {
	g := newGrid()
	// Body
	for y := 8; y < 24; y++ {
		setPx(&g, 14, y, CWhite)
		setPx(&g, 15, y, CRed)
		setPx(&g, 16, y, CRed)
		setPx(&g, 17, y, CWhite)
	}
	// Tip (cone)
	setPx(&g, 15, 6, CRedDk)
	setPx(&g, 16, 6, CRedDk)
	setPx(&g, 15, 7, CRed)
	setPx(&g, 16, 7, CRed)
	// Fins
	setPx(&g, 12, 22, CRedDk)
	setPx(&g, 13, 22, CRedDk)
	setPx(&g, 13, 23, CRedDk)
	setPx(&g, 19, 22, CRedDk)
	setPx(&g, 18, 22, CRedDk)
	setPx(&g, 18, 23, CRedDk)
	// Flame
	for y := 24; y < 28; y++ {
		setPx(&g, 15, y, COr)
		setPx(&g, 16, y, COr)
	}
	setPx(&g, 15, 28, CYellow)
	setPx(&g, 16, 28, CYellow)
	return g
}

// -----------------------------------------------------------------------------
// Fireball — orange fire puff
// -----------------------------------------------------------------------------
func makeFireball() grid {
	g := newGrid()
	for dy := -8; dy <= 8; dy++ {
		for dx := -8; dx <= 8; dx++ {
			d := dx*dx + dy*dy
			x, y := 16+dx, 16+dy
			if d <= 30 {
				setPx(&g, x, y, COrDk)
			} else if d <= 50 {
				setPx(&g, x, y, COr)
			} else if d <= 64 {
				setPx(&g, x, y, CYellow)
			}
		}
	}
	return g
}

// -----------------------------------------------------------------------------
// Explosion frames — three progressive sizes (small / med / large)
// -----------------------------------------------------------------------------
func makeExplode(stage int) grid {
	g := newGrid()
	var rOuter, rMid, rInner int
	switch stage {
	case 0:
		rOuter, rMid, rInner = 7, 4, 2
	case 1:
		rOuter, rMid, rInner = 11, 8, 5
	case 2:
		rOuter, rMid, rInner = 14, 11, 7
	}
	for dy := -rOuter; dy <= rOuter; dy++ {
		for dx := -rOuter; dx <= rOuter; dx++ {
			d := dx*dx + dy*dy
			x, y := 16+dx, 16+dy
			if d <= rOuter*rOuter {
				if d > rMid*rMid {
					setPx(&g, x, y, COrDk)
				} else if d > rInner*rInner {
					setPx(&g, x, y, COr)
				} else {
					setPx(&g, x, y, CYellow)
				}
			}
		}
	}
	// Bright core
	if stage >= 1 {
		setPx(&g, 16, 16, CWhite)
	}
	if stage >= 2 {
		setPx(&g, 15, 16, CWhite)
		setPx(&g, 17, 16, CWhite)
		setPx(&g, 16, 15, CWhite)
		setPx(&g, 16, 17, CWhite)
	}
	return g
}

// -----------------------------------------------------------------------------
// Powerup B (bomb) — blue capsule with B
// -----------------------------------------------------------------------------
func makePowerupB() grid {
	g := newGrid()
	// Capsule shape
	for y := 6; y < 26; y++ {
		for x := 8; x < 24; x++ {
			dx := x - 16
			dy := y - 16
			// Round capsule
			if dy*dy < 100 || (y >= 6 && y < 9) || (y >= 23 && y < 26) {
				if dx*dx+(dy*4)*(dy*4) < 64 {
					g[y][x] = CBlue
				}
			}
		}
	}
	// Outline
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g[y][x] == CBlue {
				// Is neighbor empty? Then it's an edge
				edge := false
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := x+dx, y+dy
						if nx < 0 || ny < 0 || nx >= size || ny >= size {
							continue
						}
						if g[ny][nx] == CTrans {
							edge = true
						}
					}
				}
				if edge {
					g[y][x] = CBlack
				}
			}
		}
	}
	// Highlight
	setPx(&g, 12, 10, CBlueLt)
	setPx(&g, 13, 10, CBlueLt)
	// Letter B
	fillRect(&g, 12, 13, 14, 20, CWhite)
	fillRect(&g, 12, 16, 14, 17, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Powerup P (power) — red capsule with P
// -----------------------------------------------------------------------------
func makePowerupP() grid {
	g := newGrid()
	for y := 6; y < 26; y++ {
		for x := 8; x < 24; x++ {
			dx := x - 16
			dy := y - 16
			if dy*dy < 100 || (y >= 6 && y < 9) || (y >= 23 && y < 26) {
				if dx*dx+(dy*4)*(dy*4) < 64 {
					g[y][x] = CRed
				}
			}
		}
	}
	// Outline
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g[y][x] == CRed {
				edge := false
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := x+dx, y+dy
						if nx < 0 || ny < 0 || nx >= size || ny >= size {
							continue
						}
						if g[ny][nx] == CTrans {
							edge = true
						}
					}
				}
				if edge {
					g[y][x] = CBlack
				}
			}
		}
	}
	// Highlight
	setPx(&g, 12, 10, CRedLt)
	setPx(&g, 13, 10, CRedLt)
	// Letter P
	fillRect(&g, 12, 13, 14, 21, CWhite)
	fillRect(&g, 12, 13, 16, 14, CWhite)
	fillRect(&g, 15, 14, 16, 17, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Medal — golden star
// -----------------------------------------------------------------------------
func makeMedal() grid {
	g := newGrid()
	// Star shape (5-pointed)
	// Outer points
	star := [][2]int{
		{16, 4}, {18, 12}, {26, 12}, {20, 17}, {22, 26},
		{16, 21}, {10, 26}, {12, 17}, {6, 12}, {14, 12},
	}
	for _, p := range star {
		setPx(&g, p[0], p[1], CYellow)
		setPx(&g, p[0]+1, p[1], CYellow)
		setPx(&g, p[0]-1, p[1], CYellow)
	}
	// Fill center
	for y := 12; y < 22; y++ {
		for x := 12; x < 21; x++ {
			dx := x - 16
			dy := y - 16
			if dx*dx+dy*dy < 25 {
				setPx(&g, x, y, CYellow)
			}
		}
	}
	// Outline
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g[y][x] == CYellow {
				edge := false
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := x+dx, y+dy
						if nx < 0 || ny < 0 || nx >= size || ny >= size {
							continue
						}
						if g[ny][nx] == CTrans {
							edge = true
						}
					}
				}
				if edge {
					g[y][x] = CYelDk
				}
			}
		}
	}
	// Inner highlight
	setPx(&g, 15, 16, CWhite)
	setPx(&g, 16, 15, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Enemy bullet — small purple orb
// -----------------------------------------------------------------------------
func makeEnemyBullet() grid {
	g := newGrid()
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
			if dx*dx+dy*dy <= 16 {
				setPx(&g, 16+dx, 16+dy, CPur)
			} else if dx*dx+dy*dy <= 25 {
				setPx(&g, 16+dx, 16+dy, CPurDk)
			}
		}
	}
	setPx(&g, 16, 16, CWhite)
	return g
}

// -----------------------------------------------------------------------------
// Player icon — small player for HUD lives
// -----------------------------------------------------------------------------
func makePlayerIcon() grid {
	g := makePlayer()
	// Scale down by skipping rows/cols — simpler: just use the same sprite
	// but it's a 32x32, the HUD displays at 12x12. The canvas will downscale.
	return g
}

// -----------------------------------------------------------------------------
// Water — alternate terrain (unused for now but generated for completeness)
// -----------------------------------------------------------------------------
func makeWater() grid {
	g := newGrid()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			g[y][x] = CBlue
		}
	}
	// Wave highlights
	for x := 0; x < size; x += 4 {
		g[8][x] = CBlueLt
		g[8][x+1] = CBlueLt
	}
	for x := 2; x < size; x += 5 {
		g[16][x] = CBlueLt
		g[16][x+1] = CBlueLt
	}
	for x := 0; x < size; x += 6 {
		g[24][x] = CBlueLt
	}
	return g
}

// -----------------------------------------------------------------------------
// Upgrade icons (kept simple, may not all be used by current scene)
// -----------------------------------------------------------------------------
func makeUpgrade(color color.RGBA) grid {
	g := newGrid()
	for dy := -10; dy <= 10; dy++ {
		for dx := -10; dx <= 10; dx++ {
			if dx*dx+dy*dy <= 100 {
				setPx(&g, 16+dx, 16+dy, color)
			} else if dx*dx+dy*dy <= 121 {
				setPx(&g, 16+dx, 16+dy, CBlack)
			}
		}
	}
	// Inner highlight
	for dy := -5; dy <= 0; dy++ {
		for dx := -5; dx <= 0; dx++ {
			if dx*dx+dy*dy <= 16 {
				setPx(&g, 14+dx, 14+dy, CWhite)
			}
		}
	}
	return g
}

func main() {
	outDir := "examples/raiden/assets"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}
	log.Printf("writing sprites to %s:", outDir)
	writePng(outDir, "player", makePlayer())
	writePng(outDir, "ground", makeGround())
	writePng(outDir, "water", makeWater())
	writePng(outDir, "boss", makeBoss())
	writePng(outDir, "enemy_bomber", makeEnemyBomber())
	writePng(outDir, "enemy_heli", makeEnemyHeli())
	writePng(outDir, "enemy_turret", makeEnemyTurret())
	writePng(outDir, "enemy_small", makeEnemySmall())
	writePng(outDir, "bullet", makeBullet())
	writePng(outDir, "laser", makeLaser())
	writePng(outDir, "plasma", makePlasma())
	writePng(outDir, "missile", makeMissile())
	writePng(outDir, "fireball", makeFireball())
	writePng(outDir, "explode_1", makeExplode(0))
	writePng(outDir, "explode_2", makeExplode(1))
	writePng(outDir, "explode_3", makeExplode(2))
	writePng(outDir, "powerup_b", makePowerupB())
	writePng(outDir, "powerup_p", makePowerupP())
	writePng(outDir, "medal", makeMedal())
	writePng(outDir, "enemy_bullet", makeEnemyBullet())
	writePng(outDir, "player_icon", makePlayerIcon())
	writePng(outDir, "upgrade_blue", makeUpgrade(CBlue))
	writePng(outDir, "upgrade_purple", makeUpgrade(CPur))
	log.Printf("done.")
}
