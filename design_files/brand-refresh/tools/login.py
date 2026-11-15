"""login.py out.png scheme w h scale hero.jpg"""
import json, sys, os
from playwright.sync_api import sync_playwright
out, scheme, w, h, scale, hero = sys.argv[1:7]
src = open("/tmp/exhibit-mock/frame.py").read()
grain = src.split('grain = """')[1].split('"""')[0]
css = open("/tmp/exhibit-mock/pal.css").read() + open("/tmp/exhibit-mock/frame.css").read() + open("/tmp/exhibit-mock/login.css").read() + (open(os.environ["EXTRA"]).read() if os.environ.get("EXTRA") else "") + ":root{--grain-url:%s}" % grain
fonts = "https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght,SOFT,WONK@9..144,100..900,0..100,0..1&family=Schibsted+Grotesk:wght@400..900&display=swap"
js = """
const l=document.createElement('link');l.rel='stylesheet';l.href=%s;document.head.append(l);
const s=document.createElement('style');s.textContent=%s;document.head.append(s);
const r=document.documentElement; r.dataset.pal='1'; r.dataset.accent='ACCENT';
const old=document.querySelector('svg.logo');const t=document.createElement('div');t.innerHTML=%s;const n=t.firstElementChild;n.setAttribute('class','logo');old.replaceWith(n);
document.querySelector('.app-header-title').textContent='Exhibit';
document.querySelector('.login-sub').textContent='Sign in to your library';
const b=document.createElement('div');b.innerHTML='<h2 class="mock-head">A shelf for<br><i>small</i> tools.</h2><p class="mock-tag">Saved as files, rendered anywhere, yours to keep.</p>';
document.querySelector('.login-content').prepend(b);document.activeElement.blur();
""" % (json.dumps(fonts), json.dumps(css), json.dumps(open("/tmp/exhibit-mock/logo-paint.svg").read()))
js = js.replace("ACCENT", os.environ.get("ACCENT", "sky"))
with sync_playwright() as p:
    b = p.chromium.launch()
    c = b.new_context(viewport={"width": int(w), "height": int(h)}, device_scale_factor=float(scale), color_scheme=scheme)
    pg = c.new_page()
    pg.route("**/mock/hero.jpg", lambda r: r.fulfill(path=hero, content_type="image/jpeg"))
    pg.goto("http://localhost:18080/auth/login"); pg.wait_for_timeout(600); pg.evaluate(js); pg.wait_for_timeout(2500)
    pg.screenshot(path=out); b.close()
