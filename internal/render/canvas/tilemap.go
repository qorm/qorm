package canvas

import (
	"fmt"
	"hash/fnv"
	"image"
	"sort"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
)

// tilemap is a baked world bitmap: one Image for the whole level so a
// side-scroller pans a single blit instead of measuring hundreds of tiles
// every frame. Rebakes when rows / bump / scale / atlas change.
func init() {
	RegisterWidget("tilemap", tilemapWidget{})
}

type tilemapWidget struct{}

type tilemapBake struct {
	fp  uint64
	img *image.RGBA
}

var (
	tilemapMu    sync.Mutex
	tilemapCache = map[*model.Node]*tilemapBake{}
	tilemapBakes int
)

// ResetTilemapCache drops baked worlds (tests / scene switch).
func ResetTilemapCache() {
	tilemapMu.Lock()
	tilemapCache = map[*model.Node]*tilemapBake{}
	tilemapBakes = 0
	tilemapMu.Unlock()
}

func tilemapBakeCount() int {
	tilemapMu.Lock()
	defer tilemapMu.Unlock()
	return tilemapBakes
}

func (tilemapWidget) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	rows, ts := tilemapGrid(n, rt, scale)
	if len(rows) == 0 || ts < 1 {
		return 0, 0
	}
	cols := 0
	for _, row := range rows {
		if n := len(row); n > cols {
			cols = n
		}
	}
	return cols * ts, len(rows) * ts
}

func (tilemapWidget) Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	if ln == nil || ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	img := bakeTilemap(ln.Node, rt, scale)
	if img == nil {
		return nil
	}
	node := graph.NewImage()
	node.Width = float64(ln.Width)
	node.Height = float64(ln.Height)
	node.Bitmap = img
	node.Fit = "fill"
	node.Pixelated = true
	return node
}

func tilemapGrid(n *model.Node, rt *runtime.Runtime, scale int) (rows []string, tile int) {
	if scale < 1 {
		scale = 1
	}
	cell := 32.0
	if raw, ok := n.Prop("cell"); ok {
		if f, ok := toFloatAny(evalStyleProp(raw, rt)); ok && f > 0 {
			cell = f
		}
	} else if raw, ok := n.Prop("tileSize"); ok {
		if f, ok := toFloatAny(evalStyleProp(raw, rt)); ok && f > 0 {
			cell = f
		}
	}
	return evalTilemapRows(n, rt), int(cell) * scale
}

func evalTilemapRows(n *model.Node, rt *runtime.Runtime) []string {
	raw, ok := n.Prop("rows")
	if !ok {
		return nil
	}
	v := evalStyleProp(raw, rt)
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func tilemapAtlas(n *model.Node) map[rune]string {
	raw, ok := n.Prop("atlas")
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[rune]string, len(m))
	for k, v := range m {
		if k == "" {
			continue
		}
		s, _ := v.(string)
		if s == "" {
			continue
		}
		out[[]rune(k)[0]] = s
	}
	return out
}

func tilemapBump(n *model.Node, rt *runtime.Runtime) (cx, cy, t int) {
	if raw, ok := n.Prop("bumpX"); ok {
		if f, ok := toFloatAny(evalStyleProp(raw, rt)); ok {
			cx = int(f)
		}
	}
	if raw, ok := n.Prop("bumpY"); ok {
		if f, ok := toFloatAny(evalStyleProp(raw, rt)); ok {
			cy = int(f)
		}
	}
	if raw, ok := n.Prop("bumpT"); ok {
		if f, ok := toFloatAny(evalStyleProp(raw, rt)); ok {
			t = int(f)
		}
	}
	return
}

func tilemapFP(rows []string, atlas map[rune]string, ts, bx, by, bt, scale int) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d|%d|%d|%d|%d|", ts, scale, bx, by, bt)
	if len(atlas) > 0 {
		keys := make([]int, 0, len(atlas))
		for r := range atlas {
			keys = append(keys, int(r))
		}
		sort.Ints(keys)
		for _, k := range keys {
			r := rune(k)
			_, _ = fmt.Fprintf(h, "%c=%s;", r, atlas[r])
		}
	}
	for _, row := range rows {
		_, _ = h.Write([]byte(row))
		_, _ = h.Write([]byte{'\n'})
	}
	return h.Sum64()
}

func bakeTilemap(n *model.Node, rt *runtime.Runtime, scale int) *image.RGBA {
	if n == nil {
		return nil
	}
	rows, ts := tilemapGrid(n, rt, scale)
	if len(rows) == 0 || ts < 1 {
		return nil
	}
	atlas := tilemapAtlas(n)
	bx, by, bt := tilemapBump(n, rt)
	if bt <= 0 {
		bx, by = -1, -1
	}
	fp := tilemapFP(rows, atlas, ts, bx, by, bt, scale)

	tilemapMu.Lock()
	if ent := tilemapCache[n]; ent != nil && ent.fp == fp && ent.img != nil {
		img := ent.img
		tilemapMu.Unlock()
		return img
	}
	tilemapMu.Unlock()

	cols := 0
	for _, row := range rows {
		if n := len(row); n > cols {
			cols = n
		}
	}
	w, h := cols*ts, len(rows)*ts
	if w < 1 || h < 1 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	lift := 0
	if scale > 0 {
		lift = 6 * scale
	} else {
		lift = 6
	}
	for y, row := range rows {
		for x, ch := range row {
			if ch == '.' || ch == ' ' {
				continue
			}
			srcPath, ok := atlas[ch]
			if !ok {
				continue
			}
			src := loadImage(srcPath, rt)
			if src == nil {
				continue
			}
			dx, dy := x*ts, y*ts
			if bt > 0 && x == bx && y == by {
				dy -= lift
				if dy < 0 {
					dy = 0
				}
			}
			dest := image.Rect(dx, dy, dx+ts, dy+ts)
			blitImageNearest(dst, src, dest, dest, 1)
		}
	}

	tilemapMu.Lock()
	tilemapCache[n] = &tilemapBake{fp: fp, img: dst}
	tilemapBakes++
	tilemapMu.Unlock()
	return dst
}
