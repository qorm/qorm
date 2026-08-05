//go:build !qorm_nocjk

package fonts

import "embed"

// FS holds the embedded font assets (default build; qorm_nocjk opts out).
// The assets directory always contains at least OFL.txt, so this package
// still compiles when the subset font itself
// has not been generated yet (e.g. a checkout before CI ran
// scripts/subset-font.sh) — ReadFile(FontFile) then misses and the canvas
// text stack falls back to the phase-1 bitmap font.
//
//go:embed assets
var FS embed.FS

// FontFile is the subset font inside FS: Source Han Sans SC Normal (the
// Adobe build of Noto Sans CJK SC), SIL OFL-1.1, subset to GB2312 hanzi +
// GB2312 fullwidth symbols + printable ASCII by scripts/subset-font.sh.
const FontFile = "assets/SourceHanSansSC-Normal.subset.otf"
