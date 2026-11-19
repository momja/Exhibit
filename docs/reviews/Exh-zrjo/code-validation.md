# Code-quality validation

The final application is `742301c5`, assets `72ef2efad788`, stacked on PR273
`ab0135c9`. [Runtime identity](code-runtime-identity.json) records the complete
source scope, its unchanged fingerprint, matching build/main/sample executable
hashes, and every delta from the broad-suite baselines. No exclusions hide
runtime or validation changes.

| Check | Actual revision | Result |
| --- | --- | --- |
| Standard `go test -short ./...` | `907cb944` | Exit 0, 421.99s |
| Full browser suite | `8a4c0fe1` | 29 passed, 185.72s |
| Full Node suite with `--experimental-vm-modules` | `8a4c0fe1` | 479 passed, 16.60s |
| Editor lint | Unchanged editor source | Passed |
| Final gallery asset copy and Go build | `742301c5` | Exit 0, 8.07s |
| Final template and native Tag modal checks | `742301c5` | Exit 0, 17.66s |
| Final four Tag browser checks | `742301c5` | 4 passed, 28.32s |

Only the refresh explanation's class and its helper/error CSS composition
changed after the full Go pass. The complete two-file delta is retained in the
runtime receipt. JavaScript is unchanged since the full Node/browser pass.
The final style change received focused checks, rather than a claim that broad
suites were rerun on that revision. Current checks confirm their source stayed
unchanged while running. No assets were rebuilt during Go test compilation.

Browser coverage includes a held tag GET followed by network failure, a 503
fragment, and successful GET retry without duplicate POST; actual session cookie
expiry and htmx login recovery; a deleted artifact's real 404 fragment without a
script error; native color input events at four widths in both schemes;
CodeMirror Replace all during a locked import; ordinary Back; and an explicit
persisted-page event distinguishing completed import from unfinished approval.

The main preview and isolated Tag playground run the same final executable.
Main at 1280px/light and 390px/dark retained healthy layout with no console errors
or overflow on the earlier behavior candidate. The final 390px/dark error view
used a local simulated offline GET: computed text was 14px, the existing Tags
error color was `rgb(255, 150, 144)`, and the focused Retry control had a 12px
paragraph gap. Fields stayed locked until an actual partial GET restored them.
The fetch shim was removed; no persisted fixture writes occurred. The comparison
was previewed at 728px and 360px with no console messages and rendered in-thread.

Source reviews were independent of execution. Three review axes found no
remaining functional issues; later component-class and specificity findings
were preserved and corrected. The final composition review reports zero
remaining findings. See [the review](code-review.md).

The failed broad Go run on `8a4c0fe1` had one stale exact trigger expectation.
The update preserves the mutation trigger, checks the new declarative GET retry,
and adds a native-fieldset assertion. Its focused test then passed. Failed and
interrupted receipts remain historical, including the runs deliberately stopped
after new review findings; none is represented as a pass. The initial Node
invocation omitted its VM-module prerequisite and failed the harness. Actual
BFCache restoration was declined by headless Chromium; only ordinary Back and
the explicit persisted event are claimed.
