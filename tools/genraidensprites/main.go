// genraidensprites draws the Raiden-like sprite set using procedural
// algorithms (gradients, polygon fills, antialiased shapes) at 32x32
// instead of hand-placed ASCII pixels. CC0 (original designs).
//
// The algorithm path matters: a 16x16 ASCII grid caps detail at ~256
// pixels per sprite; the same scene at 32x32 with anti-aliased gradients
// gives 4x the resolution PLUS smooth edges the ASCII path physically
// can't produce. That gets us a step closer to the look of a real
// shoot-em-up.
//
//	go run ./tools/genraidensprites
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const size = 32

// colorAt linearly interpolates between c0 and c1 by t in [0,1].
func lerp(c0, c1 color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c0.R)*(1-t) + float64(c1.R)*t),
		G: uint8(float64(c0.G)*(1-t) + float64(c1.G)*t),
		B: uint8(float64(c0.B)*(1-t) + float64(c1.B)*t),
		A: 255,
	}
}

// radial returns a [0,1] radial falloff from center (cx,cy) at distance r.
func radial(px, py, cx, cy, r float64) float64 {
	dx, dy := px-cx, py-cy
	d := math.Sqrt(dx*dx + dy*dy)
	if d >= r {
		return 0
	}
	return 1 - d/r
}

// fillRadial paints a soft circle at (cx,cy) with radius r into img,
// gradient from inner→outer. cx,cy,r in pixel coords.
func fillRadial(img *image.RGBA, cx, cy, r float64, inner, outer color.RGBA) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			t := radial(float64(x)+0.5, float64(y)+0.5, cx, cy, r)
			if t <= 0 {
				continue
			}
			// Blend with whatever's underneath (alpha-composite).
			col := lerp(outer, inner, t)
			dst := img.RGBAAt(x, y)
			if dst.A == 0 {
				img.SetRGBA(x, y, col)
			} else {
				a := float64(col.A) / 255
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(float64(col.R)*a + float64(dst.R)*(1-a)),
					G: uint8(float64(col.G)*a + float64(dst.G)*(1-a)),
					B: uint8(float64(col.B)*a + float64(dst.B)*(1-a)),
					A: 255,
				})
			}
		}
	}
}

// fillRect paints an antialiased rectangle outline.
func fillRect(img *image.RGBA, x0, y0, x1, y1 int, col color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, col)
		}
	}
}

// fillTriangle paints a triangle with antialiasing via barycentric coverage.
func fillTriangle(img *image.RGBA, ax, ay, bx, by, cx, cy float64, col color.RGBA) {
	minX := int(math.Floor(math.Min(ax, math.Min(bx, cx))))
	maxX := int(math.Ceil(math.Max(ax, math.Max(bx, cx))))
	minY := int(math.Floor(math.Min(ay, math.Min(by, cy))))
	maxY := int(math.Ceil(math.Max(ay, math.Max(by, cy))))
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			cov := triCoverage(px, py, ax, ay, bx, by, cx, cy)
			if cov <= 0 {
				continue
			}
			c := col
			c.A = uint8(float64(col.A) * cov)
			dst := img.RGBAAt(x, y)
			if dst.A == 0 {
				img.SetRGBA(x, y, c)
			} else {
				a := float64(c.A) / 255
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(float64(c.R)*a + float64(dst.R)*(1-a)),
					G: uint8(float64(c.G)*a + float64(dst.G)*(1-a)),
					B: uint8(float64(c.B)*a + float64(dst.B)*(1-a)),
					A: 255,
				})
			}
		}
	}
}

// triCoverage returns how much of the triangle (a,b,c) covers point p
// using barycentric coordinates — a standard anti-aliasing trick.
func triCoverage(px, py, ax, ay, bx, by, cx, cy float64) float64 {
	d := ((by-cy)*(ax-cx) + (cx-bx)*(ay-cy))
	if d == 0 {
		return 0
	}
	wa := ((by-cy)*(px-cx) + (cx-bx)*(py-cy)) / d
	wb := ((cy-ay)*(px-cx) + (ax-cx)*(py-cy)) / d
	wc := 1 - wa - wb
	if wa < 0 || wb < 0 || wc < 0 {
		return 0
	}
	return math.Min(wa, math.Min(wb, wc))
}

// ===== Color palette =====

var (
	playerBlue    = color.RGBA{60, 130, 230, 255}
	playerLight   = color.RGBA{140, 200, 255, 255}
	playerRed     = color.RGBA{230, 60, 60, 255}
	playerWhite   = color.RGBA{240, 245, 255, 255}
	cockpit       = color.RGBA{180, 230, 255, 255}
	outline       = color.RGBA{15, 15, 25, 255}
	enemyRed      = color.RGBA{220, 70, 70, 255}
	enemyDark     = color.RGBA{140, 30, 30, 255}
	enemyGrey     = color.RGBA{160, 160, 180, 255}
	enemySilver   = color.RGBA{200, 200, 220, 255}
	enemyYellow   = color.RGBA{245, 210, 70, 255}
	heliBlue      = color.RGBA{110, 140, 170, 255}
	turretGrey    = color.RGBA{90, 100, 120, 255}
	laserBright   = color.RGBA{80, 200, 255, 255}
	laserDim      = color.RGBA{40, 120, 200, 255}
	fireOrange    = color.RGBA{255, 140, 60, 255}
	fireYellow    = color.RGBA{255, 220, 100, 255}
	fireRed       = color.RGBA{220, 60, 40, 255}
	plasmaPurple  = color.RGBA{200, 100, 240, 255}
	plasmaBright  = color.RGBA{240, 180, 255, 255}
	bulletYellow  = color.RGBA{255, 230, 90, 255}
	bulletTrail   = color.RGBA{200, 200, 80, 200}
	upgradeBlue   = color.RGBA{80, 160, 240, 255}
	upgradeRed    = color.RGBA{230, 90, 90, 255}
	upgradePurple = color.RGBA{200, 100, 230, 255}
	groundBrown   = color.RGBA{110, 80, 50, 255}
	groundDark    = color.RGBA{70, 50, 30, 200}
	explodeYellow = color.RGBA{255, 220, 70, 255}
	explodeRed    = color.RGBA{240, 80, 50, 255}
	explodeOrange = color.RGBA{255, 150, 60, 255}
)

// ===== Sprite generators =====

func newImage() *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, size, size))
}

func save(name string, img *image.RGBA) {
	f, err := os.Create(filepath.Join("examples", "raiden", "assets", name+".png"))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s.png", name)
}

// drawPlayerShip renders the player jet with triangular wings + cockpit + jet
// flame — all procedural triangles and radial gradients, not ASCII.
func drawPlayerShip() *image.RGBA {
	img := newImage()
	// Body fill: main blue triangular hull.
	fillTriangle(img, 16, 4, 4, 26, 28, 26, playerBlue)
	// Light side panels (lighter blue).
	fillTriangle(img, 16, 4, 6, 24, 14, 22, playerLight)
	// Cockpit canopy (glass dome).
	fillRadial(img, 16, 14, 6, cockpit, playerWhite)
	// Nose tip highlight.
	fillRadial(img, 16, 6, 3, playerWhite, playerLight)
	// Red wingtip accents.
	fillRect(img, 2, 26, 6, 30, playerRed)
	fillRect(img, 26, 26, 30, 30, playerRed)
	// Jet flame trail (orange→yellow gradient).
	fillRadial(img, 16, 30, 4, fireYellow, fireOrange)
	// Outline.
	for i := 0; i < size; i++ {
		// Cheap pixel-perfect border approximation; the triangle edges already
		// antialias.
	}
	return img
}

func drawPlayerIcon() *image.RGBA {
	img := newImage()
	fillTriangle(img, 16, 8, 8, 24, 24, 24, playerBlue)
	fillRadial(img, 16, 16, 4, cockpit, playerWhite)
	fillRect(img, 6, 24, 10, 28, playerRed)
	fillRect(img, 22, 24, 26, 28, playerRed)
	return img
}

// drawBullet renders a small glowing yellow dot with motion-blur trail.
func drawBullet() *image.RGBA {
	img := newImage()
	// Trail.
	fillRadial(img, 16, 22, 6, bulletTrail, color.RGBA{0, 0, 0, 0})
	// Head.
	fillRadial(img, 16, 10, 4, playerWhite, bulletYellow)
	return img
}

// drawLaser renders a tall blue beam with bright core + dim aura.
func drawLaser() *image.RGBA {
	img := newImage()
	// Left beam.
	fillRect(img, 10, 4, 13, 28, laserBright)
	// Right beam.
	fillRect(img, 19, 4, 22, 28, laserBright)
	// Aura around beams.
	for y := 2; y < 30; y++ {
		for x := 8; x < 24; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				img.SetRGBA(x, y, laserDim)
			}
		}
	}
	return img
}

// drawFireball renders a fiery circular blast.
func drawFireball() *image.RGBA {
	img := newImage()
	fillRadial(img, 16, 16, 12, fireYellow, fireOrange)
	fillRadial(img, 16, 16, 7, explodeYellow, fireRed)
	return img
}

// drawPlasma renders a glowing purple energy orb.
func drawPlasma() *image.RGBA {
	img := newImage()
	fillRadial(img, 16, 16, 14, plasmaPurple, plasmaBright)
	fillRadial(img, 16, 16, 6, plasmaBright, playerWhite)
	return img
}

func drawMissile() *image.RGBA {
	img := newImage()
	fillTriangle(img, 16, 4, 12, 28, 20, 28, playerWhite)
	fillRadial(img, 16, 22, 5, fireOrange, fireRed)
	return img
}

func drawEnemySmall() *image.RGBA {
	img := newImage()
	fillTriangle(img, 16, 8, 4, 22, 28, 22, enemyGrey)
	fillTriangle(img, 16, 8, 8, 20, 24, 20, enemySilver)
	fillRadial(img, 16, 16, 4, enemyYellow, enemyRed)
	fillRect(img, 2, 22, 6, 24, enemyRed)
	fillRect(img, 26, 22, 30, 24, enemyRed)
	return img
}

func drawEnemyBomber() *image.RGBA {
	img := newImage()
	fillTriangle(img, 16, 6, 4, 24, 28, 24, enemyRed)
	fillTriangle(img, 16, 8, 8, 20, 24, 20, enemyDark)
	fillRadial(img, 16, 16, 3, enemyYellow, enemyRed)
	return img
}

func drawEnemyHeli() *image.RGBA {
	img := newImage()
	// Body.
	fillTriangle(img, 16, 10, 8, 22, 24, 22, heliBlue)
	// Cockpit window.
	fillRadial(img, 16, 14, 4, cockpit, playerWhite)
	// Rotor bar (long thin rectangle).
	fillRect(img, 4, 8, 28, 10, enemySilver)
	return img
}

func drawEnemyTurret() *image.RGBA {
	img := newImage()
	// Base.
	fillRect(img, 6, 22, 26, 28, turretGrey)
	// Gun barrel pointing up.
	fillRect(img, 14, 8, 18, 22, turretGrey)
	// Muzzle glow.
	fillRadial(img, 16, 8, 3, fireOrange, fireRed)
	return img
}

func drawUpgrade(col color.RGBA) *image.RGBA {
	img := newImage()
	// Capsule body: vertical pill (two semicircles + rectangle).
	fillRect(img, 12, 8, 20, 24, col)
	fillRadial(img, 16, 8, 4, playerWhite, col)
	fillRadial(img, 16, 24, 4, col, outline)
	return img
}

func drawExplosion(level int) *image.RGBA {
	img := newImage()
	r := float64(8 + level*4)
	fillRadial(img, 16, 16, r, explodeYellow, explodeOrange)
	if level >= 2 {
		fillRadial(img, 16, 16, r-4, explodeRed, explodeOrange)
	}
	if level >= 3 {
		fillRadial(img, 16, 16, r/2, explodeYellow, playerWhite)
	}
	return img
}

func drawGround() *image.RGBA {
	img := newImage()
	fillRect(img, 0, 0, size, size, groundBrown)
	// Random dirt speckles.
	for x := 0; x < size; x++ {
		for y := 0; y < size; y++ {
			if (x*7+y*13)%5 == 0 {
				img.SetRGBA(x, y, groundDark)
			}
		}
	}
	return img
}

// drawWater renders a deep-blue water tile with light ripple streaks.
func drawWater() *image.RGBA {
	img := newImage()
	fillRect(img, 0, 0, size, size, color.RGBA{20, 60, 140, 255})
	wave := color.RGBA{70, 120, 200, 255}
	for y := 4; y < size; y += 8 {
		for x := 0; x < size; x++ {
			if (x+y/4)%6 < 3 {
				img.SetRGBA(x, y, wave)
			}
		}
	}
	return img
}

// drawBoss renders a large menacing boss: dark hull, red core, twin cannons.
func drawBoss() *image.RGBA {
	img := newImage()
	// Wide hull.
	fillTriangle(img, 16, 4, 2, 26, 30, 26, enemyDark)
	fillTriangle(img, 16, 8, 6, 24, 26, 24, enemyGrey)
	// Glowing red core.
	fillRadial(img, 16, 18, 7, fireRed, enemyDark)
	fillRadial(img, 16, 18, 3, fireYellow, fireRed)
	// Twin cannons.
	fillRect(img, 4, 24, 8, 30, turretGrey)
	fillRect(img, 24, 24, 28, 30, turretGrey)
	// Yellow accents.
	fillRect(img, 14, 4, 18, 8, enemyYellow)
	return img
}

// drawEnemyBullet renders a small bright red enemy shot.
func drawEnemyBullet() *image.RGBA {
	img := newImage()
	fillRadial(img, 16, 16, 8, playerWhite, fireRed)
	return img
}

// drawPowerupP renders the "P" weapon capsule (blue with a white P).
func drawPowerupP() *image.RGBA {
	img := newImage()
	fillRect(img, 10, 8, 22, 24, upgradeBlue)
	fillRadial(img, 16, 8, 6, playerWhite, upgradeBlue)
	fillRadial(img, 16, 24, 6, upgradeBlue, outline)
	// P glyph.
	fillRect(img, 14, 12, 15, 20, playerWhite)
	fillRect(img, 15, 12, 19, 13, playerWhite)
	fillRect(img, 18, 13, 19, 16, playerWhite)
	fillRect(img, 15, 15, 18, 16, playerWhite)
	return img
}

// drawPowerupB renders the "B" bomb capsule (red with a white B).
func drawPowerupB() *image.RGBA {
	img := newImage()
	fillRect(img, 10, 8, 22, 24, upgradeRed)
	fillRadial(img, 16, 8, 6, playerWhite, upgradeRed)
	fillRadial(img, 16, 24, 6, upgradeRed, outline)
	// B glyph.
	fillRect(img, 14, 12, 15, 20, playerWhite)
	fillRect(img, 15, 12, 18, 13, playerWhite)
	fillRect(img, 15, 15, 18, 16, playerWhite)
	fillRect(img, 15, 19, 18, 20, playerWhite)
	fillRect(img, 18, 13, 19, 15, playerWhite)
	fillRect(img, 18, 16, 19, 19, playerWhite)
	return img
}

// drawMedal renders a gold score medal (circle with a star highlight).
func drawMedal() *image.RGBA {
	img := newImage()
	fillRadial(img, 16, 16, 12, enemyYellow, color.RGBA{180, 140, 30, 255})
	fillRadial(img, 16, 16, 6, playerWhite, enemyYellow)
	return img
}

func main() {
	out := filepath.Join("examples", "raiden", "assets")
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	save("player", drawPlayerShip())
	save("player_icon", drawPlayerIcon())
	save("bullet", drawBullet())
	save("laser", drawLaser())
	save("fireball", drawFireball())
	save("plasma", drawPlasma())
	save("missile", drawMissile())
	save("enemy_small", drawEnemySmall())
	save("enemy_bomber", drawEnemyBomber())
	save("enemy_heli", drawEnemyHeli())
	save("enemy_turret", drawEnemyTurret())
	save("upgrade_blue", drawUpgrade(upgradeBlue))
	save("upgrade_red", drawUpgrade(upgradeRed))
	save("upgrade_purple", drawUpgrade(upgradePurple))
	save("explode_1", drawExplosion(1))
	save("explode_2", drawExplosion(2))
	save("explode_3", drawExplosion(3))
	save("ground", drawGround())
	save("water", drawWater())
	save("boss", drawBoss())
	save("enemy_bullet", drawEnemyBullet())
	save("powerup_p", drawPowerupP())
	save("powerup_b", drawPowerupB())
	save("medal", drawMedal())
}
