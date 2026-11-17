# Production hero art

Assets for [Exh-qhvv](../../../.tickets/Exh-qhvv.md). No sign-in implementation
is included. The approved paintings remain unchanged in `../paintings/`.
The delivered web files, with exact dimensions and byte counts, are listed in
[`web/hero-art/images/manifest.json`](../../../web/hero-art/images/manifest.json).
This directory's inputs and masters are lossless WebP, checked against the PNGs they
replaced pixel for pixel (decoded RGBA identical), which keeps the repository small.

## Files and coordinates

For each of `day` and `dusk`:

| File | Pixels | Purpose |
| --- | --- | --- |
| `*-native.webp` | 1672×941 | Native image-tool restoration; retained as source |
| `*-sky-native.webp` | 1672×941 | Image-tool painted-through sky |
| `*-clouds-native.webp`, `*-hill-native.webp` | 1672×941 | Image-tool RGBA extraction inputs, refined before shipping |
| `*-sky.webp` | 2560×2272 | Master: full sky, extended 832px upward |
| `*-clouds.webp`, `*-hill.webp` | 2560×1440 | Masters: straight-alpha, matted layers |

These three layers per mode are the only masters. The 2560×1440 composite
(sky cropped at y=832, then clouds, then hill, source-over) and the 1170×2535 phone
crop are *assembled* from them by `tools/hero-export/assemble.mjs`, which reproduces
Pillow's compositing and Lanczos resize exactly (libvips differs by up to 70/255 on
the phone upscale). Neither is stored; `production.py` rebuilds both for review and
records their pixel hashes in `verification.json`.

The built-in image generator was asked for its highest available resolution,
preferably 3840×2160. It returned **1672×941 for every input**; these are not native
2560px paintings. Production masters use Pillow Lanczos resampling to 2560×1440
(a negligible aspect correction from the tool's almost-16:9 output).

At rest, place the sky at `(0,-832)` and the other layers at `(0,0)`. Composite
clouds over sky, then hill over both, with standard source-over alpha. The full
sky extends behind all objects. Scale all positions together for other widths.
The prototype's 520/760/1000px starting offsets become **832/1216/1600px** at
production width. `verification.json` records comparison against the saved masters.
The canonical composite is assembled from the refined layers; zero error means
those delivered masters agree, not that generation preserved the mock pixelwise.

The phone image crops the extended assembled scene at `(1540,192,960,2080)`
before resizing. Its top half is sky for the sign-in content; the complete
figure, CRT, desk and chair remain in its lower portion. This is an art-directed
crop from the extended scene, rather than a center crop of the desktop image.
The figure sits nearer the horizontal center than in the early phone mock,
which keeps the complete desk and CRT inside the narrower portrait frame.

## Regenerate

From the repository root (Python dependencies stay outside the checkout):

```sh
python3 -m venv /tmp/exhibit-art-venv
/tmp/exhibit-art-venv/bin/pip install -r design_files/brand-refresh/tools/production-requirements.txt
# Full matting and toning from the retained native inputs, writing the three masters per mode:
NUMBA_NUM_THREADS=2 OPENBLAS_NUM_THREADS=2 /tmp/exhibit-art-venv/bin/python design_files/brand-refresh/tools/production.py
# Or, without redoing the matting, only verify, hash and review the saved masters:
/tmp/exhibit-art-venv/bin/python design_files/brand-refresh/tools/production.py --assemble-only  # refuses if the masters differ from the hashes in verification.json
cd design_files/brand-refresh/tools/hero-export
npm ci
npm run export      # encodes web/hero-art/images; `npm run check` fails on any byte drift
cd ../../../..
make assets
go test ./internal/api/...
```

`production.py` takes `--review-dir` (default `/tmp/exhibit-review/Exh-qhvv`) for its
review images, and records the SHA-256 of every input and master in
`verification.json` under `sha256`.

Rerunning the matting recomputes the masters from the native inputs and the mock
references; it was last run before the masters moved to WebP and the history was
squashed, so a matting rerun will not necessarily reproduce today's bytes. The
committed masters are the reference: `--assemble-only` and `npm run check` must
pass against them. An earlier `--tone-from-ref` option that replayed the tone pass from
a Git revision of the pre-tone PNG layers has been removed; those PNGs are not in
this repository's history.

**Regenerate the derivatives only on Linux x64 with the sharp version pinned in
`hero-export/package-lock.json`.** libvips encoder output is not portable across
platforms or versions, so any other machine will report drift in `npm run check`
even though the images look the same.

Generation is intentionally not part of the build. The retained native WebPs
make the matting/export reproducible without a service call. Closed-form
matting uses generated silhouettes to define a foreground/background/unknown
trimap; multilevel foreground estimation decontaminates partially covered
pixels. There is no hard-threshold final alpha. Sky holes use the generated
painted-through plate, matched to uncovered source sky and feathered into it.
The upper extension reflects existing painted texture and deepens gradually;
it is not an independently generated new scene.

`production.py` writes review images only to its `--review-dir` (default `/tmp/exhibit-review/Exh-qhvv/`):
composites, phone crops, saved-layer recombinations, clouds/hill on checkerboard,
white and near-black, and three descent frames plus a contact sheet per mode.
These files are deliberately untracked.

## Web delivery

`tools/hero-export/export.mjs` encodes desktop images/layers at 960, 1600 and 2560px,
and portrait images at 750 and 1170px, in WebP and AVIF. Alpha is retained.
The committed `web/hero-art/images/manifest.json` lists every filename, pixel
size and byte count (56 files, 2,737,171 bytes in all). Export enforces a small budget
table per kind and format at each kind's largest width: composite, phone, and the
sky+clouds+hill layer total. It uses one libvips worker with its operation cache
disabled to bound memory on the shared server.

Load only the active colour mode and one appropriate srcset width, but note that a
page needs **both** the static composite and the animated layers: the composite
paints first, and the layers take over for the descent animation. They are not
alternatives. At 2560px the layer set is about 1.5-1.7x the composite in AVIF and
2.0-2.9x in WebP (1.6x and 2.4x summed over both modes), so budget for both.

Why derivatives are committed, and how drift is guarded, is in `docs/build_assets.md`
("Committed derivatives"). The `web/hero-art` workspace has no dependencies; the asset
build discovers it, copies the manifest's files verbatim into ignored
`internal/api/assets/hero-art/` and fails if any size differs from the manifest. The
existing Go embed serves them at `/assets/hero-art/<filename>`, or through the
versioned asset helper. Builds do not re-encode art or need access to design masters.

## Image generation prompts and fidelity

Built-in `image_gen.imagegen` was used, with the original paintings as edit
targets for restoration and each restored native painting as the edit target
for its three layer inputs. These were the production prompt specifications:

1. **Day restoration (style-transfer):** faithful high-resolution restoration,
   highest available size preferably 3840×2160, 16:9; exact approved composition;
   vast blue sky, cream clouds on right, green hill ascending right, red-orange
   poppies, long brown-haired woman in cream sweater and jeans, wooden desk and
   chair, beige CRT with red/cream/blue/yellow Mondrian grid. Preserve palette,
   framing, fine painted texture; gently refine details; no redesign or text.
2. **Dusk restoration (style-transfer):** same composition constraints, native
   dusk reference; navy subtly starred sky, lavender-to-orange sunset low left,
   salmon/peach clouds right, dark teal meadow, warm CRT illumination. Preserve
   muted colors, lighting, figure scale and framing; no new objects or text.
3. **Day sky (precise-object-edit):** remove all clouds, hill, flowers, figure,
   desk and CRT, painting sky through former positions. Only blue sky matching
   existing hue and fine oil texture; gently deepen upward; full rectangle,
   highest resolution; no dramatic gradient, banding or objects.
4. **Dusk sky (precise-object-edit):** remove the same objects; retain navy top,
   lavender middle and orange/salmon lower-left sunset, sparse tiny stars and
   original painted texture. Only sky; deepen upward; no stripes or new objects.
5. **Day hill (background-extraction, transparent=true):** only complete meadow,
   poppies, grass, figure, furniture and Mondrian CRT. Original full 16:9 canvas,
   exact positions and size, hill from 88% height left to 56% right. Remove sky
   and clouds; fine antialiased silhouette, no blue fringe; no moving/cropping.
6. **Dusk hill (background-extraction, transparent=true):** same extraction,
   dark teal meadow and warm CRT; preserve exact source positions, grass and
   flowers; no blue/pink fringe, redesign, moving, resizing or added shadows.
7. **Day clouds (background-extraction, transparent=true):** only cream clouds,
   exact full-canvas positions and sizes including tiny wisps; remove sky and
   all foreground; preserve cream hue and fine painted soft edges; no halos.
8. **Dusk clouds (background-extraction, transparent=true):** only peach/salmon/
   lavender clouds in exact source positions; remove sky, stars, sunset and
   foreground; fine soft painted alpha, no dark fringes, movement or redesign.

The native restorations strengthened brush texture and saturation. The
deterministic tone pass described below corrects that drift without generating
new imagery. Small grass, poppy, hair and cloud-stroke details still differ.
The broad composition, object placement,
palette families and Mondrian grid remain faithful. Extraction inputs also
redrew some details; only their silhouette guidance is used for final matting,
with foreground color estimated from the restored painting.

## Deterministic tone and paper texture

`production.py` applies the pass after matting/resampling and before assembly.
Only RGB changes: each final cloud/hill alpha is preserved byte-for-byte.
Approved painting pixels supply colour; the corresponding mock cloud/hill
WebP alphas select opaque reference regions. Sky reference samples exclude both.
Cloud and hill Lab means/standard deviations use 2nd–98th percentile winsorized
samples, with standard-deviation ratios limited to 0.65–1.35 so small accents
remain legible. Sky matches the mock's median Lab value per row, interpolated
through occlusions and smoothed with a 40px Gaussian; the original horizontal
sunset variation remains. No flat sky fill is used.

A bilateral filter at delivery resolution (21px diameter, spatial sigma 8px,
RGB range sigma 0.10 for sky/clouds and 0.055 for hill) smooths brush ridges while
preserving high-contrast edges. Every layer receives the same monochrome grain
field (NumPy seed 6101, standard deviation 0.7/255 RGB), aligned to scene
coordinates. This makes grain agree across feathered overlaps. The upward sky
extension receives independent samples from that same field at equal strength.
All processing uses retained native inputs and local mock references.

`verification.json` records mean CIEDE2000 against the approved 1600×900 painting,
using Lanczos-downscaled production RGB, before and after the pass. This metric
also includes the native restoration's unchanged differences in shapes/placement;
it is not a pure palette error. It also records alpha and recombination errors.
Review comparisons are in `<review-dir>/tone/`: each
`{day,dusk}-{full,cloud,figure}.png` is **mock | before | after**. Crops are
512×512 pixels at 1:1 at the 2560×1440 delivery resolution; only the mock is
resampled to delivery size for these detail comparisons. Full frames and the
metric normalize both inputs to 1600×900.
Descent contact sheets remain in the parent review directory.

Measured mean ΔE00 (lower is closer): day **3.328042 → 2.252750**;
dusk **3.574025 → 2.544426**. Saved-layer recombination and alpha maximum
errors are both **0** for both modes. Native inputs and approved references
are unchanged.

The pass softens the heavy ridges but leaves some broader inherited cloud
strokes visible at delivery scale. The dusk figure's warm rim light is slightly
more subdued than the mock after the hill colour transfer; the face, hair,
CRT grid and poppy boundaries remain clear. This is a deterministic correction
of the existing restoration, so small painted detail differences remain.

Validation for this pass: all 56 exports passed and every manifest dimension/byte
count and retained layer alpha was checked. Both descent contact sheets, phone
previews and decoded AVIF/WebP static-versus-layer comparisons were viewed. No
application page or Go code changed in this pass.
