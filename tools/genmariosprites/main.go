// genmariosprites writes the 24×24 pixel-art PNGs that examples/mario renders
// inside its gridview. Each sprite is described as a 24-line ASCII map (one
// char per pixel); the tool runs once at edit time and the outputs are
// checked in. Run from the repo root:
//
//   go run ./tools/genmariosprites
//
// Pixel legend (single character → color, transparent otherwise):
//
//   R mario red      Z goomba brown   G coin gold       O brick orange
//   E mario skin     L goomba belly   N coin dark       X brick dark
//   B mario brown    W white          K black           Y ground brown
//                                              T ground topsoil
//                                              F flag green    D flag dark
//                                              P flag pole     Q pole dark
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

const size = 24

type pixel byte

func rgba(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

var palette = map[pixel]color.RGBA{
	'.': {0, 0, 0, 0},
	'R': rgba(229, 37, 33),    // mario red
	'E': rgba(255, 179, 128),  // mario skin
	'B': rgba(107, 51, 0),     // mario brown (shoes / moustache)
	'W': rgba(255, 255, 255),  // white
	'K': rgba(26, 26, 26),     // black
	'Z': rgba(124, 74, 0),     // goomba brown
	'L': rgba(179, 129, 74),   // goomba belly (lighter)
	'G': rgba(255, 215, 0),    // coin gold
	'N': rgba(184, 148, 0),    // coin dark
	'O': rgba(200, 76, 12),    // brick orange
	'X': rgba(107, 36, 0),     // brick dark (grout)
	'Y': rgba(139, 69, 19),    // ground brown
	'T': rgba(92, 51, 23),     // ground topsoil
	'F': rgba(30, 180, 30),    // flag green
	'D': rgba(10, 122, 10),    // flag dark
	'P': rgba(124, 74, 0),     // flag pole
	'Q': rgba(92, 51, 23),     // pole dark
}

type sprite = [size]string

// Each sprite is 24 lines of exactly 24 characters; anything shorter is
// padded with transparent pixels. The shapes are intentionally chunky so
// they read clearly at 24 px.

var sprites = map[string]sprite{
	// Mario: red cap, peach face (two black eyes, a moustache), red body,
	// two brown shoes.
	"mario": {
		`......RRRRRRRR......`,
		`....RRRRRRRRRRRR....`,
		`..RRRRRRRRRRRRRRRR..`,
		`.RRRRRRRRRRRRRRRRRR.`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`EEEEEEEEEEEEEEEEEEEE`,
		`EKEEEEEEEKEEEEEEEKEE`,
		`EKEEEEEEEKEEEEEEEKEE`,
		`EEEEBBBBBBBBBBBBEEEE`,
		`EEEEBBBBBBBBBBBBEEEE`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RBRRRRRRRRRRRRRRRBRR`,
		`RBRRRRRRRRRRRRRRRBRR`,
		`RBRRRRRRRRRRRRRRRBRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`BBBB..........BBBB.`,
		`BBBB..........BBBB.`,
		`BBBB..........BBBB.`,
		`.BBB............BBB.`,
		`..BBB..........BBB.`,
	},

	// Goomba: brown dome, two angry white-and-black eyes, lighter belly,
	// two feet.
	"goomba": {
		`........................`,
		`...ZZZZZZZZZZZZZZZ...`,
		`..ZZZZZZZZZZZZZZZZZZ.`,
		`.ZZLZZZZZZZZZZZZZZLZZ.`,
		`.ZZLZZZZZZZZZZZZZZLZZ.`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`.ZZWKKWWWWWWWWWWKKWZZ.`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZZZZZZZZZZZZZLZZL`,
		`.ZZLZZZZZZZZZZZZZZLZZ.`,
		`.ZLZZZZZZZZZZZZZZZLZZ.`,
		`..ZZZZZZZZZZZZZZZZZZ.`,
		`...ZZZZZZZZZZZZZZZZ.`,
		`...ZZ...ZZ...ZZ...ZZ.`,
		`...BB...BB...BB...BB.`,
		`..BBBB.BBBB.BBBB.BBB.`,
		`..BBBB.BBBB.BBBB.BBB.`,
		`..BBBB.BBBB.BBBB.BBB.`,
		`..BBB....BBBB....BB.`,
		`..BB......BB......BB.`,
		`..BB.......B......BB.`,
	},

	// Coin: gold disc with a darker inner ring.
	"coin": {
		`........GGGGGG......`,
		`.....GGGGGGGGGGGG...`,
		`....GGGGGGGGGGGGGG..`,
		`...GGGGGGGGGGGGGGGG.`,
		`..GGGGGGGGGGGGGGGGGG`,
		`..GGGGGGGGGGGGGGGGGG`,
		`.GGGGGGGGGGGGGGGGGGGG`,
		`.GGGGGGGGNNNGGGGGGGG`,
		`GGGGGGNNNNNNNNGGGGGG`,
		`GGGGGGNNNNNNNNGGGGGG`,
		`GGGGGGNNNNNNNNGGGGGG`,
		`GGGGGGGGNNNGGGGGGGGG`,
		`GGGGGGGGGGGGGGGGGGGG`,
		`GGGGGGGGGGGGGGGGGGGG`,
		`.GGGGGGGGGGGGGGGGGGGG`,
		`..GGGGGGGGGGGGGGGGGG`,
		`..GGGGGGGGGGGGGGGGGG`,
		`...GGGGGGGGGGGGGGGG.`,
		`....GGGGGGGGGGGGGG..`,
		`.....GGGGGGGGGGGG...`,
		`.....GGGGGGGGGGGG...`,
		`.....GGGGGGGGGGGG...`,
		`......GGGGGGGGGG...`,
		`........GGGGGG.....`,
	},

	// Brick: orange-red blocks with darker grout lines.
	"brick": {
		`XXXXXXXXXXXXXXXXXXXXXXXX`,
		`XOOOOOOOOOOOOOOOOOOOOX`,
		`XOXXOOOXOOOOOOXOXXOOX`,
		`XOXXOOOXOOOOOOXOXXOOX`,
		`XOXXOOOXOOOOOOXOXXOOX`,
		`XOXXOOOXOOOOOOXOXXOOX`,
		`XXXXXXXXXXXXXXXXXXXXXX`,
		`XXOXXXXOOOOOXOXXXXOOX`,
		`XXOXXXXOOOOOXOXXXXOOX`,
		`XXXXXXXXXXXXXXXXXXXXXX`,
		`XXOXXXXOOOOOOOXXOOOX`,
		`XXOXXXXOOOOOOOXXOOOX`,
		`XXOXXXXOOOOOOOXXOOOX`,
		`XXOXXXXOOOOOOOXXOOOX`,
		`XXXXXXXXXXXXXXXXXXXXXX`,
		`XOOOOOOOOOOOOOOOOOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOXXXXOXOOOOOOOXXOOX`,
		`XOOOOOOOOOOOOOOOOOOX`,
		`XXXXXXXXXXXXXXXXXXXXXXXX`,
	},

	// Ground: brown block with darker topsoil line and texture.
	"ground": {
		`TTTTTTTTTTTTTTTTTTTTTTTT`,
		`TYYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYYY.YYYYYYYYYYYYYYYT`,
		`TY.YYYY.YY.YYYYY.YYYY.T`,
		`TYYYY.YY.YY.YYYYY.YY.YT`,
		`TY.YYY.YYYYY.YYYY.YYY.T`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TY.YY.YYYYYYYYYY.YY.YYT`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYY.YYYYYYYYYY.YYYYT`,
		`TY.YYYYYYYYYYYYYYY.YY.T`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYYY.YYYYYYYYYYYYYYYT`,
		`TY.YYYYYYYYYYYYYYYYYY.T`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYY.YYYYY.YY.YYY.YY.T`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TY.YY.YYYYYYYYYYY.YY.T`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYYYY.YYYYYYYYYY.YYT`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TYYYYYYYYYYYYYYYYYYYYYT`,
		`TTTTTTTTTTTTTTTTTTTTTTTT`,
	},

	// Flag: brown pole + green triangular flag (right side).
	"flag": {
		`........................`,
		`....PPPPPPPPPPPP......`,
		`....FFFFFFFFFFFF......`,
		`....FDFFFFFFFFFFF.....`,
		`....FDDFFFFFFFFFF.....`,
		`....PFDDDFFFFFFFFFF...`,
		`....PFFFDDDFFFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFDDDFFFFFFFF.`,
		`....PFFFFFFFFFFFDDDFFF.`,
		`....PFFFFFFFFFFFFFDDDF`,
		`....PFFFFFFFFFFFFFDDD.`,
		`....PFFFFFFFFFFFFFDD..`,
		`....PFFFFFFFFFFFFFF...`,
		`....PFFFFFFFFFFFFF....`,
		`....PFFFFFFFFFFFF.....`,
		`....QQQQQQQQQQQQQQ....`,
		`....P..........P.....`,
		`....P..........P.....`,
	},
}

func main() {
	out := filepath.Join("examples", "mario", "assets")
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, px := range sprites {
		// Validate every row is exactly `size` chars (or shorter — padded).
		for i, row := range px {
			if len(row) > size {
				log.Fatalf("%s: row %d has %d cols, want <= %d", name, i, len(row), size)
			}
		}
		if err := writeSprite(filepath.Join(out, name+".png"), px); err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		log.Printf("wrote %s.png (%dx%d)", name, size, size)
	}
}

func writeSprite(path string, px sprite) error {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y, row := range px {
		for x := 0; x < len(row) && x < size; x++ {
			c, ok := palette[pixel(row[x])]
			if !ok {
				continue
			}
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}