---
id: Exh-qhvv
status: closed
deps: []
links: []
created: 2026-11-15T02:03:46Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, art]
---
# Production hero paintings: day/dusk art and separate layers

Produce final day and dusk paintings (poppies, Mondrian grid on the CRT) at production resolution, delivered as: a full composite, separate sky / clouds / hill layers with clean alpha (sky extended upward for the descent), and a phone crop.

The mock's layers were cut by script from flat images (/tmp/exhibit-mock/layers, throwaway) and show halos; real layers should come from the source art. Deliver web-ready formats (WebP/AVIF) with sizes for responsive srcset.

## Acceptance Criteria

Day and dusk each have composite, three layers and phone crop; layers recombine with no seams or halos; file sizes reasonable for a login page.


## Notes

**2026-11-16T16:57:24Z**

Production art delivered in design_files/brand-refresh/production/; the delivered files and exact byte counts are in web/hero-art/images/manifest.json. Built-in image_gen.imagegen restored each approved painting and generated painted-through sky and transparent cutout inputs. Final prompts, provenance, drift and rerun instructions are in production/README.md.

Native resolution returned: 1672x941 for all eight generated inputs (highest resolution requested, preferably 3840x2160). Lanczos production composites/clouds/hill: 2560x1440. Sky: 2560x2272, including an 832px upward extension (57.78% of composite height). Phone: 1170x2535, exact 9:19.5, art-directed from the extended scene; full figure/CRT/furniture retained beneath open sky.

Eight retained native PNGs: 12,024,257 bytes. Ten production PNG masters: 33,291,382 bytes. Fifty-six responsive WebP/AVIF derivatives: 4,339,266 bytes total; desktop/layers 960/1600/2560px, phone 750/1170px. Largest 2560px AVIF composites: day 196,439 bytes, dusk 112,389 bytes. Largest three-layer AVIF sets: day 301,561 bytes, dusk 183,796 bytes. Phone AVIF at 1170px: day 131,806 bytes, dusk 82,945 bytes. Load only the active mode and one srcset candidate, using either the static composite or the animated layers.

Saved PNG layer recombination (source-over, sky top=-832, clouds then hill) versus canonical delivered composite: DAY max 0, mean 0.0; DUSK max 0, mean 0.0, in 8-bit RGB. These numbers describe agreement of the assembled production masters, not agreement with the original mock. Relative to upscaled native restorations, assembly/matting differences: day max 163, mean 1.0056578233506945; dusk max 74, mean 0.682322681568287. Closed-form matting and multilevel foreground estimation remove source-sky contamination; final alpha has 71,751/58,306 partial pixels for day clouds/hill and 178,605/61,707 for dusk clouds/hill.

Visually inspected both skies, composites, phone crops, cutouts on checkerboard/white/near-black, and start/middle/end descent frames at prototype-scaled offsets 832/1216/1600. No visible seam or fringe in final frames. Also decoded and inspected the compressed AVIF layers/composites. Review evidence is under /tmp/exhibit-review/Exh-qhvv/: {day,dusk}-composite.png, -sky.png, -{clouds,hill}-checker.png, -recombined.png, -descent-{start,middle,end}.png, -descent.png, -phone.png, and -web-{avif,webp}-recombined.png. Review images are not committed.

Drift from approved mocks: slightly stronger brush texture and saturation, especially day meadow and dusk warm illumination; small grass/poppy/hair/cloud-stroke details differ. Broad composition, subject positions, poppies and CRT Mondrian grid remain faithful. Upper sky extension reuses mirrored painted texture with smooth deepening; occluded areas use the generated sky plate with a smooth color match.

New web/hero-art workspace copies committed derivatives listed in its manifest into ignored internal/api/assets/hero-art/. Existing workspace discovery and Go embed serve /assets/hero-art/<filename>; no Go handler, gallery file-list, Docker or page changes. npm run export regenerates derivatives from masters; production.py regenerates masters/review evidence from retained native inputs. All 56 copied derivative files verified byte-for-byte after make assets.

Phone framing choice: the figure sits nearer the horizontal center than in the early phone mock, preserving the complete desk and CRT in the narrower crop.

Validation: make assets PASSED. All 56 manifest derivatives were verified byte-for-byte against internal/api/assets/hero-art/. Node syntax checks and git diff --check PASSED. Eight focused asset-serving/version/cache tests PASSED (go test -p 1 ./internal/api/... with an explicit asset-test selection, 4.097s).

Full API validation limitation: the first go test ./internal/api/... process terminated with exit 143 before output. The bounded-parallelism retry (GOMAXPROCS=2 go test -p 1 ./internal/api/...) FAILED at its default 10-minute timeout (600.205s). It reported TestGallerySpaceNavigationBrowser/phone=true: Cannot find context with specified id (-32000), and TestGalleryPhoneChromeBrowser: context deadline exceeded; TestMobileTargetsBrowser/320 was active at timeout. These tests exercise unchanged gallery/mobile code; no Go or gallery page code is modified by this delivery. Full log: /tmp/exhibit-review/Exh-qhvv/go-test-api.log; focused asset checks: go-test-assets.log; asset build: make-assets.log. The full API suite is not claimed to pass.

**2026-11-16T23:32:38Z**

Deterministic tone/texture follow-up completed without image regeneration. production.py matches robust Lab moments against approved painting regions selected with mock-layer alphas, fits the sky's vertical Lab gradient, applies edge-preserving bilateral smoothing and a shared scene-aligned monochrome grain field (seed 6101, sigma 0.7/255). Original layer alphas remain byte-identical. All ten delivery masters/phone crops and 56 WebP/AVIF derivatives regenerated; README.md and verification.json updated.

Mean CIEDE2000 at 1600x900 (both approved painting and delivered composite normalized with Lanczos): DAY 3.3280423968698596 before -> 2.2527499203116386 after; DUSK 3.5740254102773963 before -> 2.5444258171341847 after. Saved PNG layer recombination max/mean errors remain 0/0.0 for both modes; alpha max error 0 against the original committed layers. Independent phone recomposition error also 0. Native generation inputs, mock paintings/layers and sign-in references unchanged.

Largest 2560px AVIF composites: day 99,112 bytes; dusk 68,438 bytes. All 56 derivative dimensions/byte sizes/layer alpha checked against the manifest, and all build copies verified byte-for-byte. Python/Node syntax and git diff --check passed. Both descent sequences, phone crops and decoded AVIF/WebP static-versus-layer comparisons visually inspected: no visible seams. Some broad inherited cloud strokes remain at 1:1; dusk figure warm rim light is slightly subdued relative to the mock, with face/hair/CRT grid/poppies still clear.

Comparisons (mock | before | after): /tmp/exhibit-review/Exh-qhvv/tone/{day,dusk}-{full,cloud,figure}.png. Full frames are 1600x900 per panel; cloud/figure crops retain 512x512 delivery pixels per panel, with the mock resampled to delivery resolution. Descent contact sheets remain /tmp/exhibit-review/Exh-qhvv/{day,dusk}-descent.png. Validation/export logs and compressed comparison previews are in the tone directory; no review images committed.

The shared server terminated the unchanged full matting path and the first export attempt (exit 143). Successful production replay read the exact pre-tone masters from local Git commit 8cfd57abdbd40afddcd64e532a26aaa77cdee290 using the documented --tone-from-ref option. The default native matting path remains available; the fixed-revision option reproduces this pass while preserving exact original alphas. Export now uses one worker with the operation cache disabled; the complete retry passed. Ticket remains closed. No push or PR.

**2026-11-17T02:38:13Z**

Review-fix revision (supersedes earlier byte counts and the PNG-master and git-replay statements above). Repository weight: the branch was squashed to one commit so no PNG enters history. Only the sky, clouds and hill layers are masters, as lossless WebP (day 3,250,478 + 2,213,594 + 2,578,634; dusk 3,182,874 + 2,141,996 + 1,993,998 bytes; 15,361,574 total); the eight native inputs are lossless WebP too (8,750,872 bytes). Each WebP decodes to RGBA identical to the PNG it replaced. The four assembled masters (composite, phone) are no longer stored: hero-export/assemble.mjs rebuilds them with Pillow-exact source-over and Lanczos, and every one of the 56 derivatives re-encodes byte-identical to what was committed (npm run check). production/ is 24,128,897 bytes (was 48 MB of PNG). The 56 derivatives total 2,737,171 bytes (the 4,339,266 figure above predates the tone pass). Largest 2560px AVIF composites: day 99,112, dusk 68,438; AVIF layer sets (sky+clouds+hill at 2560): day 147,692, dusk 116,431; phone AVIF 1170: day 71,206, dusk 51,911. Composite and layers are both needed (first paint, then descent); the layer set is 1.5-1.7x the composite in AVIF and 2.0-2.9x in WebP. Build: sharp moved out of web/ to design_files/brand-refresh/tools/hero-export (pinned 0.35.3, same as web/pwa-icons, verified to give identical bytes to 0.35.5); web/hero-art is dependency-free, build.mjs clears its target, no longer copies manifest.json and fails on any size mismatch. production.py has main(), --review-dir, --assemble-only (replaces the removed --tone-from-ref) and sha256 of every input and master in verification.json. FILES.md deleted; manifest.json is the inventory. The full matting path was not rerun after the refactor (it was killed on the shared server before).
