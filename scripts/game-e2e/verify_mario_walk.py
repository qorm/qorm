"""Verify mario level stays visible after camera scroll."""
import sys
import json
from playwright.sync_api import sync_playwright
from PIL import Image

def main():
    with sync_playwright() as p:
        browser = p.chromium.launch()
        ctx = browser.new_context(viewport={"width": 1280, "height": 800})
        page = ctx.new_page()
        page.goto("https://qorm.com/games/", wait_until="networkidle", timeout=60000)
        page.wait_for_function("typeof window.qormGetState === 'function'", timeout=60000)
        page.wait_for_timeout(2500)
        page.click("#g-mario")
        page.wait_for_timeout(3000)
        cv = page.locator("#gamecanvas")
        page.keyboard.down("ArrowRight")
        for sec in [0, 1, 2, 3, 5, 7, 10, 15, 20]:
            if sec > 0:
                page.wait_for_timeout(1000)
            cv.screenshot(path=f"/tmp/mario_verify_{sec:02d}.png")
            s = page.evaluate("() => window.qormGetState('minimal')")
            j = json.loads(s)
            # pixel check
            im = Image.open(f"/tmp/mario_verify_{sec:02d}.png").convert("RGB")
            w, h = im.size
            px = im.load()
            brown = sum(1 for y in range(0,h,2) for x in range(0,300,2)
                        for r,g,b in [px[x,y]] if 100<r<200 and 30<g<100 and b<30)
            print(f"t={sec:2d}s: PanX={j['PanX']:.0f} imgs={j['graph_imgs']:3d} brown_px={brown}")
        page.keyboard.up("ArrowRight")
        browser.close()

if __name__ == "__main__":
    main()
