# Planned response after both33 reviews (not yet applied)

Both33 tasks terminal. Sol all14=10; Opus6values10/8values9.5. Joint FAIL, streak0.
Code72be0ccc/assetsbc262e992d4d remains current until the next source change.
Capture current invalid-hex BEFORE photos first; valid-state before photos are
already in tags-ten-round-33/. Do not lose current evidence/actual score tables.

Required fixes from Opus33:
- Raise specificity of hex-invalid border above later modal input rule.
- Replace the color hint with the error, not two lines37px below the field.
- Hide the selected color caption and show an empty/neutral native swatch when
  the hex is incomplete. Native picker remains usable to choose a valid color.
- Gate Create on valid Name/color consistently with Save tag; update on input,
  ColorPicker changes, swaps and after pending/409. No Enter bypass of known409.
- Give desktop label/action rows subtle separators (and optional focus/hover
  treatment), preserving compact pills/aligned columns and32/44px targets.
- Outline unselected palette cells subtly to make the spacious4x2grid intentional.
- Keep validation geometry steady. Reuse a reserved field-feedback slot with
  a short Name hint (up to60characters) or its error, same helper font/lineheight;
  Color hint and error share a slot. This resolves the local-error recenter jump.
  Do not use arbitrary global modal changes or weaken shared modal assertions.

Useful polish while changing this exact feedback path: stable footer/creation
button footprints during pending labels; preserve native modal focus/locking.
Field errors can use form-help +tag-field-error rather than global modal-error,
so field hint/error use one consistent13px helper role. Generic server errors
retain modal-error. Keep clear hints only while valid; CSS invalid input rule
must use input[type=text][aria-invalid=true] (stronger than modal default).

Additional meaningful proof needed: actual partial create success then attach503
must keep its error next to the Add row THROUGH htmx, with the new tag available
for retry and focus on the select. Source currently refresh(null) drops focus in
this path; use createdID for restoration, preselect matching available option,
then enable Add. This gives a direct retry, not a duplicate create. Unit currently
simulates error retention; add real disposable E2E coverage rather than infer it.

Provide a separate controlled Tag playground (same built binary, fresh scratch
DB/ports e.g18480/18481 if free) with a long attached name and an unattached tag.
Do NOT modify MainRunLog or its library to expose Add. Main/default and playground/
incognito profiles avoid host cookie collision. New reviewers remain readonly;
main16matrix plus limited playground Add/long-label views are sufficient, no broad
pages. Parent persistence tests remain disposable18080fixture. All probes restore.

Strengthen existing actual E2E: computed invalid border color, hint replaced by
near error, neutral swatch/captionhidden, Create disabled for invalid hex/name,
modal top/field geometry steady across local validation, row/palette structure,
real partial-create attach error +retry. Preserve old assertions and intent.
No production code changes until current BEFORE probes collected. No full Go
suite is active. After fix build/focused checks/full23(+newtestifneeded), current
captures and new actual comparison, freeze candidate and fresh BOTH34 tasks.
Prior BOTH32+33 and all responses must be supplied; same34 peer withheld. Target
still all14=10 BOTH in TWO consecutive joint rounds. Earlier single-model10 is
not credit on changed source. Full standard Go short remains required at end.
