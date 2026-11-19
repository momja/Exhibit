"""pal.py out.png accent scheme paths(0|1) w h scale [selector]"""
import json, sys
from playwright.sync_api import sync_playwright
sys.argv += [None]
out, accent, scheme, paths, w, h, scale, sel = sys.argv[1:9]
w, h, scale = int(w), int(h), float(scale)
grain = """url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='.6 .6 .6 0 -.4 .6 .6 .6 0 -.4 .6 .6 .6 0 -.4 0 0 0 0 1'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E")"""
css = open("/tmp/exhibit-mock/pal.css").read() + ":root{--grain-url:%s}" % grain
fonts = "https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght,SOFT,WONK@9..144,100..900,0..100,0..1&family=Schibsted+Grotesk:wght@400..900&display=swap"
# title: (palette hue, worn level)
DATA = {"Guitar tuner": (136, 1), "Colour contrast checker": (8, 0), "Budget tracker": (35, 3), "Sourdough timer": (231, 0),
        "Reading list": (214, 2), "Trip packing list": (136, 0), "Weekly planner": (214, 3)}
js = """
const l=document.createElement('link');l.rel='stylesheet';l.href=%s;document.head.append(l);
const s=document.createElement('style');s.textContent=%s;document.head.append(s);
const D=%s, paths=%s; const r=document.documentElement; r.dataset.pal='1'; r.dataset.accent=%s;
document.querySelectorAll('.card').forEach(c=>{const a=c.querySelector('.card-title');const d=D[a.textContent.trim()];if(!d)return;
 if(a.textContent.trim().startsWith('Colour'))a.textContent='Contrast checker';
 const t=c.querySelector('.card-widget-default');t.style.setProperty('--tile-hue',d[0]);if(paths)t.dataset.worn=d[1]});
if(paths){const rail=document.querySelector('.gallery-rail');const n=document.createElement('div');n.className='trodden';
 n.innerHTML='<h2>Well trodden</h2><a href="#"><i style="--o:.9"></i>Budget tracker</a><a href="#"><i style="--o:.65"></i>Weekly planner</a><a href="#"><i style="--o:.4"></i>Run tracker</a>';
 rail.querySelector('nav').after(n)}
""" % (json.dumps(fonts), json.dumps(css), json.dumps(DATA), paths, json.dumps(accent))
with sync_playwright() as p:
    b = p.chromium.launch()
    c = b.new_context(storage_state="/tmp/exhibit-mock/auth.json", viewport={"width": w, "height": h},
                      device_scale_factor=scale, color_scheme=scheme)
    pg = c.new_page(); pg.goto("http://localhost:18080/"); pg.wait_for_timeout(800); pg.evaluate(js); pg.wait_for_timeout(2500)
    (pg.locator(sel).first if sel else pg).screenshot(path=out)
    b.close()
