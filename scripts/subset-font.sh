#!/bin/bash
# subset-font.sh — regenerate the embedded CJK subset font for the qorm_cjk
# build tag (text stack phase 2, plan B: embedded OFL subset + x/image/sfnt).
#
# Source font: Noto Sans SC == Source Han Sans SC (same design, co-published
# by Google and Adobe under the SIL Open Font License 1.1 — embedding and
# redistribution with the license text is explicitly allowed).
#
# Download URL (pick any one, or pass a local file as $1):
#   https://github.com/notofonts/noto-cjk (Sans/SubsetOTF/SC/SourceHanSansSC-Regular.otf)
#   https://github.com/adobe-fonts/source-han-sans (release OTFs)
#   https://fonts.google.com/noto/specimen/Noto+Sans+SC
#
# Charset: GB2312 hanzi (6763) + GB2312 symbol rows 0xA1-0xA3 (fullwidth
# ASCII/punctuation) + printable ASCII. Generated into fonts/subset-chars.txt
# (checked in for review); pyftsubset then produces
# fonts/assets/SourceHanSansSC-Normal.subset.otf.
#
# Requires: python3 + fonttools (pip install fonttools).
#
# Usage:
#   scripts/subset-font.sh [path/to/SourceHanSansSC.otf]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHARSET="$ROOT/fonts/subset-chars.txt"
OUT="$ROOT/fonts/assets/SourceHanSansSC-Normal.subset.otf"

SRC="${1:-}"
if [ -z "$SRC" ]; then
    echo "usage: $0 <path to Noto Sans SC / Source Han Sans SC OTF or TTF>" >&2
    echo "download one from https://github.com/notofonts/noto-cjk first" >&2
    exit 2
fi

# 1. Emit the charset file (one hex code point per line): printable ASCII,
#    GB2312 symbol rows 0xA1-0xA3 (fullwidth ASCII, CJK punctuation, numeric
#    forms) and the 6763 GB2312 hanzi (rows 0xB0-0xF7).
python3 - "$CHARSET" <<'PY'
import sys

chars = set(range(0x20, 0x7F))  # printable ASCII
for hi in list(range(0xA1, 0xA4)) + list(range(0xB0, 0xF8)):
    for lo in range(0xA1, 0xFF):
        try:
            s = bytes([hi, lo]).decode("gb2312")
        except UnicodeDecodeError:
            continue
        chars.update(ord(c) for c in s)

# Hand-picked extras: symbols the widgets/examples actually use — none of
# them are in GB2312. (Missing from the source font they subset to nothing,
# so keep this list to what Noto Sans SC really carries.)
chars.update([
    0x00B7,  # · middle dot
    0x2013, 0x2014,  # – —
    0x2022,  # • bullet (secure-input mask)
    0x2026,  # … ellipsis
    0x2190, 0x2191, 0x2192, 0x2193,  # ← ↑ → ↓
    0x25B2, 0x25BC, 0x25CB, 0x25CF,  # ▲ ▼ ○ ●
])

with open(sys.argv[1], "w") as f:
    for cp in sorted(chars):
        f.write(f"U+{cp:04X}\n")
print(f"{len(chars)} code points -> {sys.argv[1]}")
PY

# 2. Subset. Keep all name records (license text must survive subsetting,
#    OFL-1.1 §2) and all layout features; keep the .notdef outline so missing
#    input still boxes visibly in other renderers.
mkdir -p "$(dirname "$OUT")"
pyftsubset "$SRC" \
    --output-file="$OUT" \
    --unicodes-file="$CHARSET" \
    --layout-features='*' \
    --glyph-names \
    --symbol-cmap --legacy-cmap \
    --name-IDs='*' --name-legacy --name-languages='*' \
    --notdef-outline \
    --recalc-bounds --recalc-average-width --recalc-max-context \
    --prune-unicode-ranges

ls -l "$OUT"
echo "done. Make sure fonts/assets/OFL.txt ships alongside (OFL-1.1)."
