// genmariosprites writes the pixel-art PNGs that examples/mario renders
// inside its gridview. Each sprite is described as ASCII maps (one char per
// pixel); the tool runs at edit time and the outputs are checked in. Run:
//
//	go run ./tools/genmariosprites
//
// Pixel legend (single character → color, transparent otherwise):
//
//	R mario red      E mario skin     B mario brown     W white
//	K black          Y yellow buttons Z goomba brown    L goomba belly
//	G coin gold      N coin dark      O brick orange    X brick dark
//	V ground brown  T topsoil        F flag green      D flag dark
//	P flag pole      Q pole dark      U sky blue        C cloud
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
	'R': rgba(229, 37, 33),   // mario red
	'E': rgba(255, 179, 128), // mario skin
	'B': rgba(107, 51, 0),    // mario brown (shoes / moustache)
	'W': rgba(255, 255, 255), // white
	'K': rgba(20, 20, 20),    // black
	'Y': rgba(255, 215, 0),   // yellow buttons / belt
	'Z': rgba(124, 74, 0),    // goomba brown
	'L': rgba(179, 129, 74),  // goomba belly (lighter)
	'G': rgba(255, 215, 0),   // coin gold
	'N': rgba(184, 148, 0),   // coin dark
	'O': rgba(200, 76, 12),   // brick orange
	'X': rgba(107, 36, 0),    // brick dark (grout)
	'V': rgba(139, 69, 19),   // ground brown
	'T': rgba(92, 51, 23),    // ground topsoil
	'F': rgba(30, 180, 30),   // flag green
	'D': rgba(10, 122, 10),   // flag dark
	'P': rgba(124, 74, 0),    // flag pole
	'Q': rgba(92, 51, 23),    // pole dark
	'U': rgba(92, 148, 252),  // sky blue (cloud bg)
	'C': rgba(255, 255, 255), // cloud white
}

type sprite = [size]string

// Each sprite is 24 lines of exactly 24 characters; anything shorter is
// padded with transparent pixels. The shapes are intentionally chunky so
// they read clearly at 24 px.

var sprites = map[string]sprite{
	// Mario STANDING: red cap with brim, peach face with eyes + moustache,
	// blue (red here) body with yellow buttons, brown shoes.
	"mario": {
		`......RRRRRRRR......`,
		`....RRRRRRRRRRRR....`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`....RRRRRRRRRRRR....`,
		`.....EEEEEEEEEEE....`,
		`....EKEEEEEKEEEEEK..`,
		`....EKEEEEEKEEEEEK..`,
		`....EEEEBBBBBBBBEE..`,
		`....EEEEBBBBBBBBEE..`,
		`....RRRRRRRRRRRRRR..`,
		`....RRRRRRRRRRRRRR..`,
		`...RRRRYRRRRRRRRRRR.`,
		`...RRRRYRRRRRRRRRRR.`,
		`...RRRRRRRRRRRRRRRR.`,
		`...RRRRRRRRRRRRRRRR.`,
		`...RRRRRRRRRRRRRRRR.`,
		`...RRRRRRRRRRRRRRRR.`,
		`....BBBBBBBBBBBBBB..`,
		`....BBBBBBBBBBBBBB..`,
		`....BBBB......BBBB.`,
		`....BBB........BBB.`,
		`....BBB........BBB.`,
	},

	// Mario WALKING (frame 2): legs swapped, arm forward — a stride pose.
	"mario_walk": {
		`......RRRRRRRR......`,
		`....RRRRRRRRRRRR....`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`....RRRRRRRRRRRR....`,
		`.....EEEEEEEEEEE....`,
		`....EKEEEEEKEEEEEK..`,
		`....EKEEEEEKEEEEEK..`,
		`....EEEEBBBBBBBBEE..`,
		`....EEEEBBBBBBBBEE..`,
		`...RRRRRRRRRRRRRR...`,
		`..RRRRRRRRRRRRRRRR..`,
		`.RRRRRYRRRRRRRRRRRRR`,
		`.RRRRRYRRRRRRRRRRRRR`,
		`..RRRRRRRRRRRRRRRR..`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`...BBBBBBBBBBBBBB...`,
		`..BBBBBBBBBBBBBBBB..`,
		`..BBB.........BBB...`,
		`..BBB.........BB....`,
		`..BBB..........B....`,
	},

	// Mario JUMPING: legs tucked under, arms out — a leap pose.
	"mario_jump": {
		`......RRRRRRRR......`,
		`....RRRRRRRRRRRR....`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`...RRRRRRRRRRRRRR...`,
		`....RRRRRRRRRRRR....`,
		`.....EEEEEEEEEEE....`,
		`....EKEEEEEKEEEEEK..`,
		`....EKEEEEEKEEEEEK..`,
		`....EEEEBBBBBBBBEE..`,
		`....EEEEBBBBBBBBEE..`,
		`.RRRRRRRRRRRRRRRRRR.`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`RRRYRRRRRRRRRRRRRYRR`,
		`RRRRRRRRRRRRRRRRRRRR`,
		`.RRRRRRRRRRRRRRRRRR.`,
		`..RRRRRRRRRRRRRRRR..`,
		`...RRRRRRRRRRRRRR...`,
		`....BBBBBBBBBBBB...`,
		`.....BBBBBBBBBB....`,
		`.....BBB....BBB....`,
		`......BB....BB.....`,
		`......BB....BB.....`,
		`......B......B.....`,
	},

	// Goomba STANDING: angry brown dome with eyes + feet.
	"goomba": {
		`........................`,
		`....ZZZZZZZZZZZZZZ....`,
		`...ZZZZZZZZZZZZZZZZ...`,
		`..ZZZLZZZZZZZZZZLZZZ..`,
		`..ZZZLZZZZZZZZZZLZZZ..`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`.ZZWKKWWWWWWWWWWKKWZZ.`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`.ZZLZZLZZZZZZZZLZZLZZ.`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZZZZZZZZZZZZZLZZL`,
		`.ZZLZZZZZZZZZZZZZZLZZ.`,
		`.ZLZZZZZZZZZZZZZZZLZZ.`,
		`..ZZZZZZZZZZZZZZZZZZ.`,
		`...ZZZZZZZZZZZZZZZZ..`,
		`....ZZ...ZZ...ZZ...Z.`,
		`....BB...BB...BB...B.`,
		`...BBBB.BBBB.BBBB.BB.`,
		`...BBBB.BBBB.BBBB.BB.`,
		`...BBBB.BBBB.BBBB.BB.`,
		`...BBB....BBBB....B..`,
		`...BB......BB......B.`,
	},

	// Goomba WALKING (frame 2): feet shifted one pixel — alternating stride.
	"goomba_walk": {
		`........................`,
		`....ZZZZZZZZZZZZZZ....`,
		`...ZZZZZZZZZZZZZZZZ...`,
		`..ZZZLZZZZZZZZZZLZZZ..`,
		`..ZZZLZZZZZZZZZZLZZZ..`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`.ZZWKKWWWWWWWWWWKKWZZ.`,
		`.ZZWWKKWWWWWWWWKKWWZZ.`,
		`.ZZLZZLZZZZZZZZLZZLZZ.`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZLZZZZZZZZZZLZZLZ`,
		`ZZLZZZZZZZZZZZZZZLZZL`,
		`.ZZLZZZZZZZZZZZZZZLZZ.`,
		`.ZLZZZZZZZZZZZZZZZLZZ.`,
		`..ZZZZZZZZZZZZZZZZZZ.`,
		`...ZZZZZZZZZZZZZZZZ..`,
		`...ZZ....ZZ....ZZ....`,
		`...BB....BB....BB....`,
		`..BBB....BBB....BB...`,
		`..BBBB..BBBB..BBBB...`,
		`..BBBB..BBBB..BBBB...`,
		`..BBB....BBB....BB...`,
		`..BB......BB......B..`,
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
		`TVVVVVVVVVVTg`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYY.YYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
		`TVYYYYYYYYYYYYYYYYYVT`,
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
		`....PFFFFFFFFFFFFFFDD.`,
		`....PFFFFFFFFFFFFFFFDD`,
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
