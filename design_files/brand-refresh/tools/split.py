"""split.py src.jpg prefix mode(day|dusk) -> prefix-sky.webp (tall), prefix-clouds.webp, prefix-hill.webp, prefix-debug.png"""
import sys, numpy as np
from PIL import Image, ImageFilter
src, prefix, mode = sys.argv[1:4]
im = np.asarray(Image.open(src).convert("RGB").resize((1600, 900), Image.LANCZOS)).astype(np.float32)
H, W, _ = im.shape
r, g, b = im[..., 0], im[..., 1], im[..., 2]
lum = .3*r + .59*g + .11*b

# 1. hill boundary: scan up from the bottom while the pixel looks like grass
if mode == "day":
    grass = (g > b + 25) & (g > r - 25)
else:
    grass = (lum < 115) & (b < g + 20) & (r < 130)
gf = grass.astype(np.float32)
cs = np.vstack([np.zeros((1, W), np.float32), np.cumsum(gf, 0)])
RUN = 24
frac = (cs[RUN:] - cs[:-RUN]) / RUN            # share of grass in the RUN rows starting at y
top = np.full(W, H - 1, dtype=np.float32)
for x in range(W):
    hit = np.nonzero(frac[:, x] > .85)[0]
    if len(hit): top[x] = hit[0]
xs = np.arange(W)
keep = ~((xs > 1150) & (xs < 1460))           # ignore the figure when fitting the hill line
for _ in range(3):
    c = np.polyfit(xs[keep], top[keep], 3); fit = np.polyval(c, xs)
    keep &= np.abs(top - fit) < 25
line = np.polyval(c, xs)
Y = np.arange(H)[:, None]
d = Y - line[None, :]                          # >0 below the hill edge
hill = np.clip((d - 1) / 6, 0, 1)              # soft 8px edge

# 2. sky estimate by normalised blur over sky pixels
def box1(a, rad, axis):
    p = np.pad(a, [(rad+1, rad) if i == axis else (0, 0) for i in range(2)], mode="edge")
    c = np.cumsum(p, axis=axis, dtype=np.float64)
    n = a.shape[axis]
    hi = np.take(c, np.arange(2*rad+1, 2*rad+1+n), axis=axis); lo = np.take(c, np.arange(0, n), axis=axis)
    return ((hi - lo) / (2*rad+1)).astype(np.float32)
def blur(a, rad):
    a = a.astype(np.float32)
    for _ in range(3): a = box1(box1(a, rad, 0), rad, 1)
    return a
def est_from(mask, rad=40):
    w = blur(mask.astype(np.float32), rad)
    out = np.stack([blur(im[..., k]*mask, rad) for k in range(3)], -1) / np.maximum(w, 1e-4)[..., None]
    valid = w > 1e-3
    for y in range(1, H):                      # fill gaps downward
        bad = ~valid[y]
        out[y, bad] = out[y-1, bad]; valid[y, bad] = valid[y-1, bad]
    return out
# robust smooth fit of the sky: 2D polynomial per channel, clouds rejected as outliers
yy, xx = np.mgrid[0:H, 0:W]
u, v = xx / W * 2 - 1, yy / H * 2 - 1
DEG = 5 if mode == 'day' else 7
terms = [u**i * v**j for i in range(DEG) for j in range(DEG - i)]
M = np.stack([t.ravel() for t in terms], 1).astype(np.float64)
flat = im.reshape(-1, 3).astype(np.float64)
if mode == "day":
    use = ((b - r) > 80).ravel()
else:
    use = np.ones(H*W, bool)
use &= (d < -12).ravel()
rng = np.random.default_rng(0)
for it in range(6):
    idx = np.nonzero(use)[0]; idx = rng.choice(idx, min(len(idx), 60000), replace=False)
    coef, *_ = np.linalg.lstsq(M[idx], flat[idx], rcond=None)
    est = (M @ coef).reshape(H, W, 3).astype(np.float32)
    dist = np.sqrt(((im - est)**2).sum(-1))
    use = (d < -12).ravel() & (dist.ravel() < (40 if it < 2 else 26))
poly = est.copy()
# let the estimate follow slow painted blotches the polynomial misses
resid = (im - est) * (dist < 26)[..., None]
RB = 30 if mode == 'day' else 18
wgt = blur((dist < 26).astype(np.float32), RB)
est = est + np.stack([blur(resid[..., k], RB) for k in range(3)], -1) / np.maximum(wgt, 1e-3)[..., None] * (wgt > .05)[..., None]
dist = np.sqrt(((im - est)**2).sum(-1))
ref = np.clip((line - 14).astype(int), 0, H - 1)
edge = est[ref, np.arange(W)]                   # W x 3
below = d > -14
est = np.where(below[..., None], edge[None, :, :], est)
pedge = poly[ref, np.arange(W)]
poly = np.where(below[..., None], pedge[None, :, :], poly)
dist = np.sqrt(((im - est)**2).sum(-1))
t0, t1 = (22, 70) if mode == "day" else (16, 58)
a = np.clip((dist - t0) / (t1 - t0), 0, 1) * (d < 0)
A = Image.fromarray((a*255).astype(np.uint8)).filter(ImageFilter.MinFilter(5)).filter(ImageFilter.MaxFilter(5)).filter(ImageFilter.GaussianBlur(.8))
a = np.asarray(A).astype(np.float32) / 255     # opening removes stars and grain specks

# figure: keep everything that isn't sky near the desk
fig = np.zeros_like(a); fy0 = int(line[1300]) - 22
box = (slice(fy0, H), slice(1175, 1440))
fig[box] = np.clip((dist[box] - 30) / 30, 0, 1)
fig = np.asarray(Image.fromarray((fig*255).astype(np.uint8)).filter(ImageFilter.MedianFilter(3))).astype(np.float32)/255
hill = np.maximum(hill, fig)
a = a * (1 - hill)

# 3. layers
def save(rgb, alpha, path):
    out = np.dstack([np.clip(rgb, 0, 255), np.clip(alpha*255, 0, 255)]).astype(np.uint8)
    Image.fromarray(out, "RGBA").save(path, quality=82, method=6)
cloud_rgb = (im - (1 - a[..., None]) * est) / np.maximum(a[..., None], 1e-3)
cloud_rgb = np.where(a[..., None] > .02, cloud_rgb, est)
save(cloud_rgb, a, prefix + "-clouds.webp")
save(im, hill, prefix + "-hill.webp")

# sky plate: original sky where clean, estimate + borrowed grain elsewhere, extended upward
CMAX, CBL = (9, 3) if mode == 'day' else (31, 10)
cover = np.asarray(Image.fromarray((np.maximum(a, hill)*255).astype(np.uint8)).filter(ImageFilter.MaxFilter(CMAX)).filter(ImageFilter.GaussianBlur(CBL))).astype(np.float32)/255
patch = (im - est)[60:360, 60:660]             # clean sky grain
ph, pw = patch.shape[:2]
reps = np.tile(np.concatenate([patch, patch[:, ::-1]], 1), (H//ph + 2, W//(2*pw) + 2, 1))[:H, :W]
fill = est if mode == 'day' else poly
plate = im*(1 - cover[..., None]) + (fill + reps)*cover[..., None]
EXT = 520
topc = est[:40].mean((0, 1))
deep = np.array([28, 82, 176], np.float32) if mode == "day" else np.array([10, 14, 46], np.float32)
tt = (np.arange(EXT)[::-1] / EXT)[:, None, None] ** 1.3
ext = topc*(1 - tt) + deep*tt + np.tile(np.concatenate([patch, patch[:, ::-1]], 1), (EXT//ph + 2, W//(2*pw) + 2, 1))[:EXT, :W]
if mode == "dusk":
    rng = np.random.default_rng(3)
    for _ in range(90):
        y, x = rng.integers(2, EXT-2), rng.integers(2, W-2); v = rng.uniform(140, 255)
        ext[y, x] = v; ext[y-1:y+2, x] = np.maximum(ext[y-1:y+2, x], v*.45); ext[y, x-1:x+2] = np.maximum(ext[y, x-1:x+2], v*.45)
sky = np.concatenate([ext, plate], 0)
Image.fromarray(np.clip(sky, 0, 255).astype(np.uint8)).save(prefix + "-sky.webp", quality=82, method=6)

# debug sheet: sky plate / clouds on magenta / hill on magenta
mag = np.array([255, 0, 255], np.float32)
dbg = np.concatenate([plate, cloud_rgb*a[..., None] + mag*(1-a[..., None]), im*hill[..., None] + mag*(1-hill[..., None])], 0)
Image.fromarray(np.clip(dbg, 0, 255).astype(np.uint8)).resize((800, 1350)).save(prefix + "-debug.png")
print(prefix, "EXT", EXT, "line@0,800,1599:", [int(line[i]) for i in (0, 800, 1599)])
