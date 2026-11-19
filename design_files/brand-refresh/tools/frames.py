import sys
from playwright.sync_api import sync_playwright
mode = sys.argv[1]
with sync_playwright() as p:
    b = p.chromium.launch(); pg = b.new_page(viewport={"width": 1440, "height": 1000})
    pg.goto("file:///tmp/exhibit-mock/descend.html"); pg.wait_for_timeout(1500)
    if mode == "dusk": pg.click('[data-m=dusk]'); pg.wait_for_timeout(800)
    for t in (0, 1200, 2000, 2800, 4200):
        pg.evaluate(f"window.__seek({t})"); pg.wait_for_timeout(150)
        pg.locator('#wrap').screenshot(path=f"/tmp/exhibit-mock/fr-{mode}-{t}.png")
    b.close()
