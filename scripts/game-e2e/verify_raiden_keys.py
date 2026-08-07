"""Test raiden button press for 20s — verify FIRE button keeps working."""
import sys
import json
from playwright.sync_api import sync_playwright

def main():
    with sync_playwright() as p:
        browser = p.chromium.launch()
        ctx = browser.new_context(viewport={"width": 1280, "height": 800})
        page = ctx.new_page()
        page.goto("https://qorm.com/games/", wait_until="networkidle", timeout=60000)
        page.wait_for_function("typeof window.qormGetState === 'function'", timeout=60000)
        page.wait_for_timeout(2500)
        page.click("#g-raiden")
        page.wait_for_timeout(3000)
        cv = page.locator("#gamecanvas")
        box = cv.bounding_box()
        # FIRE button is at scene's "fireControls" around (180, 510) on 320x560 canvas
        # css scale depends on screen — use board-relative coords
        fire_x = box["x"] + box["width"] * 0.3
        fire_y = box["y"] + box["height"] * 0.85
        for sec in [0, 3, 5, 8, 12, 15, 20]:
            if sec > 0:
                page.wait_for_timeout(1000)
            # Click FIRE 3 times
            for _ in range(3):
                page.mouse.click(fire_x, fire_y)
                page.wait_for_timeout(80)
            page.wait_for_timeout(200)
            cv.screenshot(path=f"/tmp/raiden_keys_{sec:02d}.png")
            s = page.evaluate("() => window.qormGetState('minimal')")
            try:
                j = json.loads(s)
                print(f"t={sec:2d}s: imgs={j.get('graph_imgs','?')} PanX={j.get('PanX',0):.0f} surf={j.get('viewportW','?')}x{j.get('viewportH','?')}", flush=True)
            except Exception as e:
                print(f"t={sec:2d}s: {e}", flush=True)
        browser.close()

if __name__ == "__main__":
    main()
