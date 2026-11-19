# Production hero art

The visible-brushwork daytime painting and its night companion are the sources
for the sign-in scene. The exported assets are listed in
[`web/hero-art/images/manifest.json`](../../../web/hero-art/images/manifest.json).

## Files and coordinates

For each of `day` and `dusk`:

| File | Pixels | Purpose |
| --- | --- | --- |
| `*-brushwork.webp` | 1672×941 | Source painting, retained as lossless WebP |
| `*-clouds-native.webp`, `*-hill-native.webp` | 1672×941 | Silhouettes from the earlier paintings, used as matting guides |
| `*-sky.webp` | 2560×2272 | Master with clean, extended sky |
| `*-clouds.webp`, `*-hill.webp` | 2560×1440 | Straight-alpha layer masters |

The composite is assembled from the three masters with the sky at `(0,-832)`,
then the cloud and hill layers at `(0,0)`. The phone composition is assembled
from the same layers, using the existing `(1540,192,960,2080)` crop. The sky
extension and layer positions retain the existing descent geometry.

The lower sky plate reflects texture from the unobstructed part of each source
across the areas hidden by clouds and landscape. Matting separates the silhouettes
at source resolution. Foreground colors are solved against the clean sky so the
resting desktop composite closely matches the source painting. Renders and
`verification.json` record the color differences after resampling.

## Regenerate

From the repository root (Python dependencies stay outside the checkout):

```sh
python3 -m venv /tmp/exhibit-art-venv
/tmp/exhibit-art-venv/bin/pip install -r design_files/brand-refresh/tools/production-requirements.txt
# Rebuild the three masters from the retained source paintings and silhouette guides:
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

Use the sharp version pinned in `hero-export/package-lock.json`. These derivatives
were encoded on Linux arm64. Use the same Linux architecture for a byte-for-byte
check, since encoder output can differ across architectures.

Generation is separate from the build. The source paintings and guides are
retained, so matting and export run without a generation service.

`production.py` writes review images only to its `--review-dir` (default `/tmp/exhibit-review/Exh-qhvv/`):
composites, phone crops, saved-layer recombinations, clouds/hill on checkerboard,
white and near-black, and three descent frames plus a contact sheet per mode.
These files are deliberately untracked.

## Web delivery

`tools/hero-export/export.mjs` encodes desktop images/layers at 960, 1600 and 2560px,
and portrait images at 750 and 1170px, in WebP and AVIF. Alpha is retained.
The committed `web/hero-art/images/manifest.json` lists every filename, pixel
size and byte count (56 files). Export enforces a small budget
table per kind and format at each kind's largest width: composite, phone, and the
sky+clouds+hill layer total. The more textured brushwork needs larger ceilings
than the earlier paintings. Each encode uses one libvips worker with its
operation cache disabled.

During a descent visit the page loads both the final composite and one set of
layers for the active color scheme. Repeat visits load only the composite.

Why derivatives are committed, and how drift is guarded, is in `docs/build_assets.md`
("Committed derivatives"). The `web/hero-art` workspace has no dependencies; the asset
build discovers it, copies the manifest's files verbatim into ignored
`internal/api/assets/hero-art/` and fails if any size differs from the manifest. The
existing Go embed serves them at `/assets/hero-art/<filename>`, or through the
versioned asset helper. Builds do not re-encode art or need access to design masters.

## Brushwork sources

The daytime source is the visible-brushwork study, which retained the original
composition and added dry bristle edges, scumbled sky, and more individually
painted grass and poppies. The nighttime image was generated from that study
with a lighting-only brief: indigo sky, faint stars, muted mauve clouds, a cool
meadow, and the monitor lighting the figure. Earlier native sources document the
previous paintings.

The following notes record the previous generation and tone pass.

## Earlier paintings and layer guides

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
