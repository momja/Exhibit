// Stylelint config for the hand-written gallery stylesheets (tokens/components
// + one sheet per page). Extends the community-standard ruleset and turns off
// only the rules that fight conventions this project chose deliberately —
// each one is explained below rather than silently suppressed. Genuine
// findings (deprecated values, real specificity risk) stay on or are called
// out as follow-up rather than disabled quietly.
export default {
  extends: 'stylelint-config-standard',
  rules: {
    // One selector + declaration block per line is the deliberate authoring
    // style for every gallery stylesheet (see build.mjs): it keeps the
    // served bytes greppable for the Go tests that assert on exact CSS
    // substrings (exhibit-<ticket for de-brittling those assertions>).
    // Reformatting to one-declaration-per-line would rewrite every rule in
    // every file and break those tests; decoupled from this linter rollout.
    'declaration-block-single-line-max-declarations': null,

    // Blank-line-before-comment/declaration/rule/at-rule are pure vertical
    // spacing preferences that don't match the dense single-line style above.
    'comment-empty-line-before': null,
    'declaration-empty-line-before': null,
    'rule-empty-line-before': null,
    'at-rule-empty-line-before': null,

    // The stylesheets consistently use legacy rgba()/decimal-alpha syntax
    // (`rgba(0,0,0,.4)`) rather than the modern rgb()/percentage-alpha form;
    // both are valid CSS. Enforcing the modern form here would rewrite every
    // color value across every file for a cosmetic change.
    'color-function-notation': null,
    'color-function-alias-notation': null,
    'alpha-value-notation': null,

    // Same story for media query range syntax: every @media in this project
    // uses `(max-width:640px)`, not the modern `(width <= 640px)` range form.
    'media-feature-range-notation': null,

    // Attribute selectors are written unquoted (`input[type=text]`), which is
    // valid CSS; the standard config wants them quoted.
    'selector-attribute-quotes': null,

    // notfound.css deliberately uses BEM (`__`/`--`) class naming for the
    // 404 page's illustration; the standard config's default pattern only
    // allows kebab-case.
    'selector-class-pattern': null,

    // Real finding, left as a known follow-up rather than silently disabled:
    // several stylesheets declare same-specificity selectors targeting the
    // same element out of cascade order (e.g. a `:hover` rule before the
    // `:disabled` rule it should lose to). Reordering them is a behavior
    // change, not a formatting one, so it needs case-by-case visual
    // verification rather than a blanket lint-driven reorder. Tracked
    // alongside the brittle-CSS-test ticket rather than fixed here.
    'no-descending-specificity': null,
  },
};
