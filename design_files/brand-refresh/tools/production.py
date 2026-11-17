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
from skimage.color import rgb2lab, lab2rgb, deltaE_ciede2000
from pymatting import estimate_alpha_cf, estimate_foreground_ml

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'production'
SIZE = (2560, 1440)
cv2.setNumThreads(2)
GRAIN_SEED = 6101
GRAIN_SIGMA = .7 / 255

EXT = 832  # 520/900 of the final height, matching descend.html


def rgb(path, size=None):
    image = Image.open(path).convert('RGB')
    if size:
        image = image.resize(size, Image.Resampling.LANCZOS)
    return np.asarray(image).astype(np.float64) / 255


def image(array):
    return Image.fromarray(np.uint8(np.clip(np.rint(array * 255), 0, 255)))


def matte(source, mode, kind):
    seed = Image.open(OUT / f'{mode}-{kind}-native.webp').convert('RGBA').resize(
        (source.shape[1], source.shape[0]), Image.Resampling.LANCZOS)
    alpha = np.asarray(seed)[..., 3] / 255
    support = alpha > .5
    radius = 12 if kind == 'hill' else 24
    inside = binary_erosion(support, iterations=radius, border_value=1)
    outside = ~binary_dilation(support, iterations=radius)
    if kind == 'clouds':
        # Extraction occasionally joins wisps with opaque source-blue pixels.
        # Color only supplies sure trimap samples; CF still solves soft alpha.
        red_minus_blue = source[..., 0] - source[..., 2]
        inside &= red_minus_blue > (-.15 if mode == 'day' else -.22)
        inside |= binary_erosion(support, iterations=1) & (red_minus_blue > (.03 if mode == 'day' else .08))
        outside |= red_minus_blue < (-.35 if mode == 'day' else -.32)
    trimap = np.full(alpha.shape, .5)
    trimap[inside] = 1
    trimap[outside] = 0
    result = estimate_alpha_cf(source, trimap)
    # Foreground estimation removes the old blue/pink sky from partial pixels.
    foreground = estimate_foreground_ml(source, result)
    return foreground, result


def tone_layers(mode, plate, layers):
    """Match robust Lab moments by semantic region; never filter alpha.

    Work at delivery resolution. Sky row offsets retain its vertical gradient
    and horizontal sunset variation. Shared scene-coordinate grain means a
    feathered overlap has the same noise as either opaque layer.
    """
    mock = rgb(ROOT / 'paintings' / f'hero-{mode}.png', SIZE)
    mock_lab = rgb2lab(mock)
    masks = {}
    for kind in ['clouds', 'hill']:
        ref = Image.open(ROOT / 'layers' / f'{mode}-{kind}.webp').resize(SIZE, Image.Resampling.LANCZOS)
        masks[kind] = np.asarray(ref)[..., 3] / 255
    masks['sky'] = 1 - np.maximum(masks['clouds'], masks['hill'])
    scene_mask = 1 - np.maximum(np.asarray(layers[0])[..., 3], np.asarray(layers[1])[..., 3]) / 255
    rng = np.random.default_rng(GRAIN_SEED)
    grain = rng.normal(0, GRAIN_SIGMA, (SIZE[1] + EXT, SIZE[0], 1))
    adjusted = []
    for kind, original in [('sky', plate), *zip(['clouds', 'hill'], layers)]:
        pixels = np.asarray(original).copy()
        colors = pixels[..., :3].astype(np.float32) / 255
        # Bilateral range threshold rejects figure/CRT/poppy boundaries while
        # averaging the lower-contrast brush ridges inside painted regions.
        smooth = cv2.bilateralFilter(colors, 21, .10 if kind != 'hill' else .055, 8)
        lab = rgb2lab(smooth)
        support = scene_mask > .98 if kind == 'sky' else pixels[..., 3] > 250
        reference = masks[kind] > .98
        if kind == 'sky':
            def row_fit(values, mask):
                rows = np.full((SIZE[1], 3), np.nan)
                for y in range(SIZE[1]):
                    if mask[y].sum() > 20:
                        rows[y] = np.median(values[y, mask[y]], axis=0)
                valid = np.isfinite(rows[:, 0])
                for c in range(3):
                    rows[:, c] = np.interp(np.arange(SIZE[1]), np.flatnonzero(valid), rows[valid, c])
                return gaussian_filter1d(rows, 40, axis=0)
            source_rows = row_fit(lab, support)
            target_rows = row_fit(mock_lab, reference)
            lab += (target_rows - source_rows)[:, None, :]
        else:
            # Winsorization prevents tiny bright CRT cells and transparent
            # fringes from driving an entire meadow/cloud colour transfer.
            for c in range(3):
                src = lab[..., c][support]
                ref = mock_lab[..., c][reference]
                src = np.clip(src, *np.percentile(src, [2, 98]))
                ref = np.clip(ref, *np.percentile(ref, [2, 98]))
                scale = np.clip(ref.std() / max(src.std(), .01), .65, 1.35)
                lab[..., c] = (lab[..., c] - src.mean()) * scale + ref.mean()
        colors = lab2rgb(lab)
        pixels[..., :3] = np.asarray(image(colors + grain[EXT:]))
        adjusted.append(Image.fromarray(pixels))
    return adjusted, grain


def tone_evidence(review, mode, before, after):
    folder = review / 'tone'
    folder.mkdir(exist_ok=True)
    mock = Image.open(ROOT / 'paintings' / f'hero-{mode}.png').convert('RGB').resize((1600, 900), Image.Resampling.LANCZOS)
    versions = [mock, before.convert('RGB').resize((1600, 900), Image.Resampling.LANCZOS),
                after.convert('RGB').resize((1600, 900), Image.Resampling.LANCZOS)]
    scores = [float(deltaE_ciede2000(rgb2lab(np.asarray(mock) / 255),
              rgb2lab(np.asarray(v) / 255)).mean()) for v in versions[1:]]
    # Full frames use the metric size; detail crops retain delivery pixels 1:1.
    details = [v.convert('RGB').resize(SIZE, Image.Resampling.LANCZOS)
               for v in [Image.open(ROOT / 'paintings' / f'hero-{mode}.png'), before, after]]
    for label, box in [('full', (0, 0, 1600, 900)), ('cloud', (2048, 160, 2560, 672)),
                       ('figure', (1856, 816, 2368, 1328))]:
        width, height = box[2] - box[0], box[3] - box[1]
        sheet = Image.new('RGB', (width * 3, height))
        for i, version in enumerate(versions if label == 'full' else details):
            sheet.paste(version.crop(box), (i * width, 0))
        sheet.save(folder / f'{mode}-{label}.png')
    return dict(before=scores[0], after=scores[1])


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


def matte_and_tone(mode):
    """Native inputs to toned sky, clouds and hill; returns the pre-tone composite too."""
    print(f'{mode}: matting', flush=True)
    source = rgb(OUT / f'{mode}-native.webp')
    h, w = source.shape[:2]
    foreground, hill = matte(source, mode, 'hill')
    cloud_rgb, clouds = matte(source, mode, 'clouds')
    clouds *= 1 - hill
    # Generated painted-through plate, color-matched using uncovered source sky.
    plate = rgb(OUT / f'{mode}-sky-native.webp', (w, h))
    cover = np.maximum(hill, clouds)
    # Clear the complete generated cloud support even where a faint source
    # wisp has near-zero solved opacity, so no clouds remain in the sky plate.
    cloud_seed = Image.open(OUT / f'{mode}-clouds-native.webp').convert('RGBA')
    cloud_support = np.asarray(cloud_seed)[..., 3] > 2
    cover = np.maximum(cover, cloud_support)
    clean = ~binary_dilation(cover > .01, iterations=24)
    # Fit a smooth per-row correction. Nearest-pixel extrapolation produces
    # visible Voronoi facets behind large occlusions when the layers move.
    row_weight = clean.sum(axis=1)
    valid_rows = np.flatnonzero(row_weight > 20)
    for channel in range(3):
        delta = ((source[..., channel] - plate[..., channel]) * clean).sum(axis=1)
        row_delta = delta / np.maximum(row_weight, 1)
        correction = np.interp(np.arange(h), valid_rows, row_delta[valid_rows])
        plate[..., channel] += gaussian_filter1d(correction, 35)[:, None]
    plate = np.clip(plate, 0, 1)
    # Keep unoccluded native sky, feather into the painted-through regions.
    blend = gaussian_filter(binary_dilation(cover > .01, iterations=18).astype(float), 10)
    plate = source * (1 - blend[..., None]) + plate * blend[..., None]
    plate_image = image(plate).resize(SIZE, Image.Resampling.LANCZOS)
    layers_before = [image(np.dstack([colors, alpha])).resize(SIZE, Image.Resampling.LANCZOS)
                     for colors, alpha in [(cloud_rgb, clouds), (foreground, hill)]]
    before = plate_image.convert('RGBA')
    for layer in layers_before:
        before = Image.alpha_composite(before, layer)
    (plate_image, *layers), grain = tone_layers(mode, plate_image, layers_before)
    alpha_error = max(int(np.abs(np.asarray(a)[..., 3].astype(int) -
                                np.asarray(b)[..., 3].astype(int)).max())
                      for a, b in zip(layers_before, layers))
    assert alpha_error == 0
    # Extend with reflected painted sky texture, progressively deepened. The
    # mirrored join retains continuous underlying tone across the grain field.
    base = np.asarray(plate_image).astype(float) / 255
    # Strip reflected grain before deepening, then add equal-strength grain.
    extension = (base[:EXT] - grain[EXT:EXT*2])[::-1].copy()
    t = np.linspace(1, 0, EXT)[:, None, None] ** 1.5
    deep = np.array([.07, .20, .46] if mode == 'day' else [.025, .035, .10])
    extension = extension * (1 - .65*t) + deep * .65*t
    extension += grain[:EXT]
    sky = image(np.concatenate([extension, base])).convert('RGBA')
    sky.convert('RGB').save(OUT / f'{mode}-sky.webp', **MASTER_KW)
    for kind, layer in zip(LAYERS, layers):
        layer.save(OUT / f'{mode}-{kind}.webp', **MASTER_KW)
    return before, alpha_error, (w, h)


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
        files += [OUT / f'{mode}-{kind}.webp' for kind in ['native', 'sky-native', 'clouds-native', 'hill-native']]
        files += [ROOT / 'paintings' / f'hero-{mode}.png']
        files += [ROOT / 'layers' / f'{mode}-{kind}.webp' for kind in LAYERS]
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mode', choices=['day', 'dusk', 'all'], default='all')
    parser.add_argument('--review-dir', type=Path, default=Path('/tmp/exhibit-review/Exh-qhvv'),
                        help='Where review images are written (never inside the checkout)')
    parser.add_argument('--assemble-only', action='store_true',
                        help='Skip matting and toning: verify and hash the saved masters, '
                             'refresh review images, keep the other recorded figures')
    args = parser.parse_args()
    review = args.review_dir
    review.mkdir(parents=True, exist_ok=True)
    verification_path = OUT / 'verification.json'
    report = json.loads(verification_path.read_text()) if verification_path.exists() else {}
    modes = ['day', 'dusk'] if args.mode == 'all' else [args.mode]
    for mode in modes:
        entry = report.setdefault(mode, {})
        if args.assemble_only:
            before = None
            w, h = entry['native']
        else:
            before, alpha_error, (w, h) = matte_and_tone(mode)
            entry['alpha_max_error'] = alpha_error
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
            entry['delta_e_2000'] = tone_evidence(review, mode, before, composite)
            native_scaled = Image.open(OUT / f'{mode}-native.webp').convert('RGB').resize(SIZE, Image.Resampling.LANCZOS)
            restoration_diff = np.abs(np.asarray(composite.convert('RGB')).astype(int) - np.asarray(native_scaled).astype(int))
            entry['restoration_max_error'] = int(restoration_diff.max())
            entry['restoration_mean_error'] = float(restoration_diff.mean())
        review_images(review, mode, sky, layers, composite, phone, recombined)
        print(entry, flush=True)
    # Every input read and every master written, so a rerun can show what changed.
    outputs = [OUT / f'{mode}-{kind}.webp' for mode in modes for kind in ['sky', *LAYERS]]
    hashes = report.setdefault('sha256', {'inputs': {}, 'outputs': {}})
    for group, files in [('inputs', input_files(modes)), ('outputs', outputs)]:
        hashes[group].update({str(f.relative_to(ROOT)): sha256(f) for f in files})
    verification_path.write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
