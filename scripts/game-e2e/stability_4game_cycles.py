"""Stability test for qorm.com/games.

Cycles through all 4 games (tetris, g2048, raiden, mario) N times and
verifies each game renders SOMETHING non-trivial after each switch. The
regression we're guarding against: after multiple switches, only the
player sprite + sky background remain (every other element silently
dropped because the preloader marked their URLs failed and the engine
never got a chance to retry).

Detection: a 2-color test (BG color vs non-BG pixels). If the screenshot
has < some threshold of non-BG pixels, the game is degraded.
"""
import sys
import time
from pathlib import Path
from playwright.sync_api import sync_playwright

GAMES = ["tetris", "g2048", "raiden", "mario"]
# Render dimensions (different per game — see GAME_SIZES in index.html)
GAME_SIZES = {
    "tetris": (420, 680),
    "g2048": (420, 680),
    "raiden": (320, 560),
    "mario": (512, 480),
}
CYCLES = 6  # 6 full rotations = 24 game switches
SHOT_DIR = Path("/tmp/stability")
SHOT_DIR.mkdir(exist_ok=True)
MIN_NON_BG_FRAC = 0.01  # at least 1% of pixels should be non-background


def bg_color_of(png_path: Path) -> tuple[int, int, int]:
    """Return the dominant top-left 4x4 pixel color (the canvas BG)."""
    # Lazy import — Pillow may not be in default env.
    from PIL import Image
    im = Image.open(png_path).convert("RGB")
    w, h = im.size
    # Sample 4 corners + center.
    samples = [
        im.getpixel((2, 2)),
        im.getpixel((w - 3, 2)),
        im.getpixel((2, h - 3)),
        im.getpixel((w - 3, h - 3)),
    ]
    # Use top-left as canonical BG.
    return samples[0]


def non_bg_fraction(png_path: Path, bg: tuple[int, int, int], tol: int = 6) -> float:
    from PIL import Image
    im = Image.open(png_path).convert("RGB")
    px = im.load()
    w, h = im.size
    total = w * h
    nb = 0
    # Sample on a 4x4 grid for speed (still 65k samples for 1024x480).
    step = 4
    for y in range(0, h, step):
        for x in range(0, w, step):
            r, g, b = px[x, y]
            if abs(r - bg[0]) > tol or abs(g - bg[1]) > tol or abs(b - bg[2]) > tol:
                nb += 1
    sampled = (h // step) * (w // step)
    return nb / sampled if sampled else 0.0


def main() -> int:
    failures: list[str] = []
    cycle_report: list[dict] = []

    with sync_playwright() as p:
        browser = p.chromium.launch()
        ctx = browser.new_context(viewport={"width": 1280, "height": 800})
        page = ctx.new_page()

        page.goto("https://qorm.com/games/", wait_until="networkidle", timeout=60000)
        # Boot WASM
        page.wait_for_function("typeof window.qormCanvasFrame === 'function'", timeout=60000)
        # Let the initial tetris render
        page.wait_for_timeout(2500)
        # Get canvas bounding rect for screenshots
        cv = page.locator("#gamecanvas")

        for cycle in range(CYCLES):
            cycle_data = {"cycle": cycle, "games": {}}
            for game in GAMES:
                print(f"[cycle {cycle}] switching to {game} ...", flush=True)
                page.click(f"#g-{game}")
                # Let preloader + a few frames run.
                page.wait_for_timeout(1800)

                shot = SHOT_DIR / f"cycle{cycle:02d}_{game}.png"
                cv.screenshot(path=str(shot))

                bg = bg_color_of(shot)
                frac = non_bg_fraction(shot, bg)
                ok = frac >= MIN_NON_BG_FRAC
                status = "OK" if ok else "DEGRADED"
                print(f"  {game:8s} bg={bg} non_bg_frac={frac:.3f} -> {status}", flush=True)
                cycle_data["games"][game] = {"bg": bg, "frac": frac, "ok": ok}
                if not ok:
                    failures.append(f"cycle {cycle} {game}: non_bg_frac={frac:.3f} < {MIN_NON_BG_FRAC}")
            cycle_report.append(cycle_data)

        browser.close()

    # Summary
    print("\n=== STABILITY SUMMARY ===")
    for c in cycle_report:
        for g, info in c["games"].items():
            mark = "OK " if info["ok"] else "FAIL"
            print(f"  cycle {c['cycle']:2d}  {g:8s}  {mark}  frac={info['frac']:.3f}")

    if failures:
        print(f"\n{len(failures)} FAILURE(S):")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("\nALL CYCLES PASSED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
