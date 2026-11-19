"""Refine generated cutouts, write the lossless layer masters and render review evidence.

Run with the pinned requirements in production-requirements.txt. Generated native
inputs are retained; no image generation or network access is needed on rerun.

Only the sky, clouds and hill layers are masters (lossless WebP). The composite and
the phone crop are assembled from them, here for review and verification and in
hero-export/assemble.mjs for delivery; neither is stored.
"""
from pathlib import Path
import argparse
import hashlib
import json
import numpy as np
from PIL import Image
from scipy.ndimage import binary_dilation, binary_erosion, gaussian_filter, gaussian_filter1d
import cv2
from pymatting import estimate_alpha_cf

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'production'
SIZE = (2560, 1440)
cv2.setNumThreads(2)

EXT = 832  # 520/900 of the final height, matching descend.html


def rgb(path, size=None):
    image = Image.open(path).convert('RGB')
    if size:
        image = image.resize(size, Image.Resampling.LANCZOS)
    return np.asarray(image).astype(np.float64) / 255


def image(array):
    return Image.fromarray(np.uint8(np.clip(np.rint(array * 255), 0, 255)))


LAYERS = ['clouds', 'hill']
MASTER_KW = dict(lossless=True, exact=True, method=6)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def pixel_sha256(img):
    return hashlib.sha256(np.asarray(img.convert('RGB')).tobytes()).hexdigest()


def master(mode, kind):
    return Image.open(OUT / f'{mode}-{kind}.webp').convert('RGBA')


def assemble(mode):
    """Composite and phone crop from the saved masters (Pillow, the reference)."""
    sky = master(mode, 'sky')
    layers = [master(mode, kind) for kind in LAYERS]
    composite = sky.crop((0, EXT, SIZE[0], EXT + SIZE[1]))
    for layer in layers:
        composite = Image.alpha_composite(composite, layer)
    # Portrait crop from the extended scene: 1170x2535 is exactly 9:19.5.
    # Scale the 960x2080 art-directed window; bottom aligns with the meadow.
    extended = sky.copy()
    for layer in layers:
        extended.alpha_composite(layer, (0, EXT))
    phone = extended.crop((1540, 192, 2500, 2272)).resize((1170, 2535), Image.Resampling.LANCZOS)
    return sky, layers, composite, phone


def brushwork_layers(mode):
    """Separate the approved repaint while preserving its original colors and coordinates."""
    source = rgb(OUT / f'{mode}-brushwork.webp', (1600, 900))
    height, width = source.shape[:2]

    def seed(kind):
        path = OUT / f'{mode}-{kind}-native.webp'
        return np.asarray(Image.open(path).convert('RGBA').resize((width, height)))[..., 3] > 127

    def solve(foreground, background):
        trimap = np.full((height, width), .5)
        trimap[foreground] = 1
        trimap[background] = 0
        return estimate_alpha_cf(source, trimap)

    hill_seed = seed('hill')
    hill_extent = binary_dilation(hill_seed, iterations=28)
    hill = solve(binary_erosion(hill_seed, iterations=25, border_value=1), ~hill_extent)
    cloud_seed = seed('clouds')
    cloud_extent = binary_dilation(cloud_seed, iterations=40) & (hill < .98)

    # Sample the unobstructed left sky by row. Repeat reflected brush texture
    # in the occluded areas so the sky still reads as paint during the descent.
    sample_width = width * 2 // 5
    sky_sample = source[:, :sample_width]
    clean = ~binary_dilation(hill_seed, iterations=37)[:, :sample_width]
    rows = np.flatnonzero(clean.sum(axis=1) > 35)
    medians = np.stack([np.median(sky_sample[y, clean[y]], axis=0) for y in rows])
    sky_rows = gaussian_filter1d(np.column_stack([
        np.interp(np.arange(height), rows, c) for c in medians.T
    ]), 10, axis=0)
    sky_delta = source - sky_rows[:, None]
    chroma_delta = sky_delta[..., 0] - sky_delta[..., 2]
    cloud_foreground = cloud_extent & (
        (chroma_delta > (.22 if mode == 'day' else .052))
        | (binary_erosion(cloud_seed, iterations=10) & (chroma_delta > .025))
    )
    cloud_background = ~cloud_extent | (chroma_delta < (.012 if mode == 'day' else .009))
    clouds = solve(cloud_foreground, cloud_background) * (1 - hill)

    repeat_x = np.arange(width) % (2 * (sample_width - 1))
    repeat_x = np.minimum(repeat_x, 2 * (sample_width - 1) - repeat_x)
    bottom = np.argmax(~clean[:, repeat_x], axis=0) - 1
    y = np.arange(height)[:, None]
    reflected_y = np.minimum(y, 2 * bottom - y)
    texture = sky_delta[reflected_y, repeat_x] * .8
    hidden_sky = np.clip(sky_rows[:, None] + texture, 0, 1)
    cover = np.maximum(hill, clouds)
    # Some shaded wisps have sky-like chroma. Include them if they are visibly
    # brighter than the adjacent sky, without copying the surrounding sky.
    wisps = cloud_extent & (sky_delta[..., 0] > (.030 if mode == 'day' else .018))
    fill = gaussian_filter(binary_dilation((cover > .025) | wisps, iterations=6).astype(float), 3)
    sky = source * (1 - fill[..., None]) + hidden_sky * fill[..., None]

    # Solve the source-over equation against the clean plate. Increase opacity
    # where a low estimated alpha would require an out-of-gamut layer color.
    alpha = 1 - (1 - clouds) * (1 - hill)
    difference = source - sky
    needed = np.where(difference > 0, difference / np.maximum(1 - sky, 1e-6),
                      -difference / np.maximum(sky, 1e-6)).max(axis=2)
    alpha = np.maximum(alpha, needed)
    color = sky + difference / np.maximum(alpha[..., None], 1e-6)
    hill_share = hill / np.maximum(cover, 1e-6)
    hill_alpha = alpha * hill_share
    cloud_alpha = (alpha - hill_alpha) / np.maximum(1 - hill_alpha, 1e-6)
    for kind, opacity in [('clouds', cloud_alpha), ('hill', hill_alpha)]:
        rgba = image(np.dstack([color, opacity])).resize(SIZE, Image.Resampling.LANCZOS)
        pixels = np.asarray(rgba).copy()
        pixels[pixels[..., 3] < 3] = 0
        Image.fromarray(pixels).save(OUT / f'{mode}-{kind}.webp', **MASTER_KW)

    sky = np.asarray(image(sky).resize(SIZE, Image.Resampling.LANCZOS)) / 255
    fade = np.linspace(1, 0, EXT)[:, None, None] ** 1.5 * .60
    deep = np.array([.07, .20, .46] if mode == 'day' else [.025, .035, .10])
    extension = sky[:EXT][::-1] * (1 - fade) + deep * fade
    image(np.concatenate([extension, sky])).save(OUT / f'{mode}-sky.webp', **MASTER_KW)
    return Image.open(OUT / f'{mode}-brushwork.webp')


def review_images(review, mode, sky, layers, composite, phone, recombined):
    composite.convert('RGB').save(review / f'{mode}-composite.png')
    phone.save(review / f'{mode}-phone.png')
    sky.convert('RGB').save(review / f'{mode}-sky.png')
    recombined.save(review / f'{mode}-recombined.png')
    yy, xx = np.mgrid[:1440, :2560]
    check = np.where(((xx // 24 + yy // 24) % 2)[..., None], 190, 235) * np.ones((1, 1, 3))
    for kind, layer in zip(LAYERS, layers):
        Image.alpha_composite(Image.fromarray(check.astype('uint8')).convert('RGBA'), layer).save(review / f'{mode}-{kind}-checker.png')
        for label, color in [('dark', '#151515'), ('light', '#ffffff')]:
            Image.alpha_composite(Image.new('RGBA', SIZE, color), layer).save(review / f'{mode}-{kind}-{label}.png')
    frames = []
    # Progress is eased animation progress. Same offsets and 1440x900 viewport
    # as prototype, scaled 1.6 from its 1600x900 source painting.
    for label, progress in [('start', 0), ('middle', .5), ('end', 1)]:
        canvas = Image.new('RGBA', (2560, 1440))
        canvas.alpha_composite(sky, (0, -EXT + round(EXT*(1-progress))))
        for layer, offset in zip(layers, [1216, 1600]):
            canvas.alpha_composite(layer, (0, round(offset*(1-progress))))
        canvas = canvas.crop((200, 0, 2504, 1440))
        canvas.save(review / f'{mode}-descent-{label}.png')
        frames.append(canvas.resize((768, 480)))
    sheet = Image.new('RGB', (2304, 480))
    for index, frame in enumerate(frames):
        sheet.paste(frame, (768*index, 0))
    sheet.save(review / f'{mode}-descent.png')


def input_files(modes):
    files = []
    for mode in modes:
        files += [OUT / f'{mode}-{kind}.webp' for kind in ['brushwork', 'clouds-native', 'hill-native']]
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mode', choices=['day', 'dusk', 'all'], default='all')
    parser.add_argument('--review-dir', type=Path, default=Path('/tmp/exhibit-review/Exh-qhvv'),
                        help='Where review images are written (never inside the checkout)')
    parser.add_argument('--assemble-only', action='store_true',
                        help='Skip matting and toning: verify and hash the saved masters, '
                             'refresh review images, keep the recorded restoration measurements; refuses '
                             'if the masters differ from the recorded hashes')
    args = parser.parse_args()
    review = args.review_dir
    review.mkdir(parents=True, exist_ok=True)
    verification_path = OUT / 'verification.json'
    report = json.loads(verification_path.read_text()) if verification_path.exists() else {}
    modes = ['day', 'dusk'] if args.mode == 'all' else [args.mode]
    for mode in modes:
        entry = report.setdefault(mode, {})
        if args.assemble_only:
            # The restoration figures describe the masters
            # they were measured on. Refuse unless those are the masters on disk.
            recorded = report.get('sha256', {})
            stale = [str(f.relative_to(ROOT))
                     for group, files in [('inputs', input_files([mode])),
                                          ('outputs', [OUT / f'{mode}-{k}.webp' for k in ['sky', *LAYERS]])]
                     for f in files
                     if recorded.get(group, {}).get(str(f.relative_to(ROOT))) != sha256(f)]
            if stale:
                raise SystemExit(f'--assemble-only refused for {mode}: masters or inputs differ from the '
                                 f'hashes in verification.json, so its kept measurements are stale '
                                 f'(rerun without --assemble-only): {", ".join(stale)}')
            before = None
            w, h = entry['native']
        else:
            before = brushwork_layers(mode)
            w, h = before.size
            for field in ['alpha_max_error', 'delta_e_2000']:
                entry.pop(field, None)
        # Measure the delivered masters, re-opened from disk, not work arrays.
        sky, layers, composite, phone = assemble(mode)
        recombined = sky.crop((0, EXT, 2560, 2272))
        for layer in layers:
            recombined = Image.alpha_composite(recombined, layer)
        diff = np.abs(np.asarray(recombined.convert('RGB')).astype(int) - np.asarray(composite.convert('RGB')).astype(int))
        entry.update({'native': [w, h], 'composite': list(SIZE), 'sky': list(sky.size), 'phone': list(phone.size),
                      'max_error': int(diff.max()), 'mean_error': float(diff.mean()),
                      'composite_rgb_sha256': pixel_sha256(composite), 'phone_rgb_sha256': pixel_sha256(phone)})
        assert diff.max() == 0, 'Saved layers must recombine exactly'
        if before is not None:
            native_scaled = before.convert('RGB').resize(SIZE, Image.Resampling.LANCZOS)
            restoration_diff = np.abs(np.asarray(composite.convert('RGB')).astype(int) - np.asarray(native_scaled).astype(int))
            entry['restoration_max_error'] = int(restoration_diff.max())
            entry['restoration_mean_error'] = float(restoration_diff.mean())
        review_images(review, mode, sky, layers, composite, phone, recombined)
        print(entry, flush=True)
    # Every input read and every master written, so a rerun can show what changed.
    outputs = [OUT / f'{mode}-{kind}.webp' for mode in modes for kind in ['sky', *LAYERS]]
    hashes = report.setdefault('sha256', {'inputs': {}, 'outputs': {}})
    inputs = hashes.setdefault('inputs', {})
    valid_inputs = {str(path.relative_to(ROOT)) for path in input_files(['day', 'dusk'])}
    for stale in inputs.keys() - valid_inputs:
        del inputs[stale]
    for group, files in [('inputs', input_files(modes)), ('outputs', outputs)]:
        hashes[group].update({str(f.relative_to(ROOT)): sha256(f) for f in files})
    verification_path.write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
