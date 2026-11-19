# Round 23 capture recovery

The unified exec wrapper exited143 (SIGTERM), cause unknown. At first recovery
177 PNGs existed with no manifest, so no pass was claimed. Its orphaned child
later finished a228-entry manifest at2026-11-04T21:14:38.057Z, errors[], all
c948e619826d. There is no reliable child exit receipt for that original wrapper.

The independent detached retry confirmed exit0 in276.1s/applicationUnchanged=true.
Its228-entry manifest at21:18:41.183Z independently passes actual media/assets/
errors/horizontal-overflow checks. Final evidence is nine-round-23-confirm/,
status nine-validation-capture-status.json. Only after verifying both directories
contained precisely their own generated PNGs/manifest was the redundant original
228-image directory removed. It was not177 images at cleanup time. Original
wrapper/late-child facts are retained in round-23-original-capture-status.json.

No app assertion or production-source change occurred in this recovery.
