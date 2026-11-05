# Rebased visual follow-up: validation

Base: `origin/main` at `44def51afa43019771fd258bc61b28e3ebf4613b`.
Current application candidate: `01109602` (240px desktop rail). Review reports, screenshots and evidence
may be committed afterward without changing the reviewed application.

- `make build`: **passed** on `01109602`; includes editor assets and the Go
  server. [Raw output](visual-validation-rail-build.txt).
- `node --experimental-vm-modules --test web/gallery/*.test.mjs`: **468 passed**,
  none skipped. [Raw output](visual-validation-node.txt).
- `npm test --prefix e2e`: **14 passed**, 1.8m on `01109602`.
  [Raw output](visual-validation-rail-e2e.txt).
  Includes actual light/dark editor theme switching with draft preservation and
  readable placeholders, differentiated syntax and token contrast on field/active-line/selection
  surfaces with actual focused selections in Paste and the artifact editor at
  1280px/390px/320px through light/dark/light changes, plus rendered phone share-sheet geometry in both schemes
  at 320px and 390px. Existing pending creation, validation, duplicate, stored-state,
  enlarged text and narrow history/viewer behavior remain covered.
- Focused Chromium matrix on preceding candidate `4d725ba2`: **passed**, 59.216s. [Raw output](visual-validation-chromium.txt).
  Covers shared Share/Tag modal appearance and transactions, enlarged fields,
  readability, upstream fixed mobile tabs, scrolling phone chrome and global Search.
- Readability and enlarged-fields Chromium checks rerun on `1c86da10`: **passed**,
  13.815s. [Raw output](visual-validation-syntax-chromium.txt).
- `node e2e/capture-ui.mjs docs/screenshots/Exh-zrjo/visual-round-15`:
  **110 authenticated states**, explicit real light/dark media-query assertions,
  no captured uncaught page errors or document horizontal overflow.
  [Manifest](../../screenshots/Exh-zrjo/visual-round-15/manifest.json).
- `go test -short ./...` on preceding candidate `4d725ba2`: **passed**, exit 0;
  API package 520.334s and store 276.835s. [Raw output](visual-validation-4d-go.txt).
- `go test -short ./...` on candidate `1c86da10`: **passed**, exit 0, including
  cached API results and a fresh store run. [Raw output](visual-validation-1c-go.txt).
- `go test -short ./...` on preceding candidate `7e96d783`: **passed**, exit 0;
  API package 408.314s, store 174.280s. [Raw output](visual-validation-7e-go.txt).
- Full Go on preceding candidate `b1b43d96`: **passed**, exit 0; API 419.897s,
  store 176.467s. [Raw output](visual-validation-b1-go.txt).
  Full Go on `30bdaa5d`: **passed**, confirmed exit 0. The first run reported
  API 426.392s and store 190.014s, but its terminal session became unavailable
  before an exit status was captured. An explicit confirmation passed with cached
  API and a fresh store run (151.008s). [Confirmed output](visual-validation-30b-confirm-go.txt),
  [durable exit status](visual-validation-30b-confirm-go-status.json),
  [original package results](visual-validation-30b-initial-go.txt).
- Readability/enlarged-fields Chromium checks on `b1b43d96`: **passed**, 17.787s.
  [Raw output](visual-validation-active-line-chromium.txt).
- Readability/enlarged-fields Chromium checks on `30bdaa5d`: **passed**,
  15.380s. [Raw output](visual-validation-editor-chrome-chromium.txt).

- Current navigation/empty-sharing/phone-tab/enlarged-field Chromium matrix:
  **passed**, exit 0, 44.657s. Includes the strengthened actual label-line and
  stable-row check at 768px/1280px, two-digit counts, phone safe areas and enlarged
  text. [Raw output](visual-validation-rail-browser.txt).
- Current filters/phone-scroll-chrome/readability Chromium group: **passed**,
  exit 0, 21.234s. [Raw output](visual-validation-rail-context-browser.txt).
- Full Go short suite on `01109602`: **passed**, confirmed exit 0.
  [Raw output](visual-validation-go.txt),
  [durable exit status](visual-validation-go-status.json).

The first placeholder adjustment failed the existing hover-surface readability
check; the color was corrected and the test retained. A later shared-modal check
caught an unnecessary smaller helper-text override, which was removed. Only the
subsequent passing runs above are counted. The share helper's failed round-7 CSS
fix prompted an E2E check of rendered positions, rather than an assertion that a
flex-direction declaration exists.

The preview and capture fixtures use scratch data, separate from existing
instances. Test automation uses its own ephemeral server on ports 18080/18081.
The council uses the sample preview on ports 18280/18281 without persistent edits.

The round-9 syntax regression was reproduced by a failing browser test. The first
repair restored light colors; the same unchanged 4.5:1 floor caught a dark token
at 4.47:1. The palette was corrected, and the subsequent full 12-test E2E run
passed. Editor ESLint also passed ([output](visual-validation-editor-lint.txt)).

Round 10 exposed a gap in the first syntax regression: it resolved a CSS variable
instead of creating a real selection. The replacement test first reproduced the
stock selection-fill failure ([output](visual-validation-selection-red.txt)),
then passed after the selector matched CodeMirror's focused-selection hierarchy.
The focused-selection regression uses the painted layer's computed background, not
the root token. Parent live checks confirmed the same fill on existing source
in both actual schemes. The first full 1c86da10 driver was signalled after package
results; only the subsequent exit-0 confirmation is counted as a full pass.

Round 11 exposed opaque active-line paint hiding selections. The new real
word-selection test first failed with identical selected/unselected backgrounds
([output](visual-validation-active-line-red.txt)). It then passed with a
translucent active-line fill. The current 14-test pass includes actual double-click
selection and front-to-back background composition, with a visible paint
difference and at least 4.5:1 text contrast in both source surfaces at all three
widths through light/dark/light. The all-token and placeholder checks also
compose translucent backgrounds.

Round 12 exposed native repeated-word emphasis and undersized find controls.
Both new checks first failed on the old build ([output](visual-validation-editor-chrome-red.txt)).
The final 14-test pass includes quieter secondary occurrences, 32/44px flat
find controls with shared typography, no overlaps/overflow, retained source
viewport, working find/line navigation and live-theme query/draft preservation
in both source editors at all three widths. Current captures also include
find/replace, repeated identifiers, line navigation and completion lists.

Round 14 exposed a desktop primary-navigation wrap that existing width-only
checks missed: the text fits horizontally because it wraps. The strengthened
existing GallerySpaces browser test now checks actual one-line labels and stable
row heights/positions across destinations, with two-digit counts and the 768px
rail breakpoint. It first **failed on the unchanged 30bdaa5d UI**, reproducing the
reported issue ([output](visual-validation-rail-red.txt)). The 240px rail repair now passes the strengthened check and the browser groups
above. No wrapping suppression or clipping is applied; enlarged labels can still
wrap naturally. [Response and scope](visual-round-15-changes.md).
