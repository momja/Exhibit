package api

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldsAreSixteenPixelsOnTouch is av-3qmf: iOS Safari zooms the page in
// when it focuses a field whose text is under 16px, and never zooms back out,
// which pushes the button beside that field off screen. The fix is entirely in
// the type scale — these assert the two tokens and the coarse-pointer override
// that raises them, since that is the whole mechanism.
func TestFieldsAreSixteenPixelsOnTouch(t *testing.T) {
	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/tokens.css")

	assert.Contains(t, css, "--field-font-size:14px", "default field size missing")
	assert.Contains(t, css, "--field-code-font-size:12px", "default code-field size missing")

	coarse := strings.Index(css, "@media (pointer:coarse)")
	require.NotEqual(t, -1, coarse, "no coarse-pointer override — every field would zoom on focus")
	override := css[coarse:]
	assert.Contains(t, override, "--field-font-size:16px", "fields stay under the iOS zoom threshold on touch")
	assert.Contains(t, override, "--field-code-font-size:16px", "code fields stay under the iOS zoom threshold on touch")
}

// TestNoGalleryFieldSetsItsOwnSizeBelowThreshold is the rule that keeps the fix
// working: a stylesheet that hardcodes a px size on a form control opts that
// control out of the token, and out of the coarse-pointer bump with it. The
// failure is invisible on desktop and only shows up as a zoom on a phone, so it
// is worth a test rather than a comment.
func TestNoGalleryFieldSetsItsOwnSizeBelowThreshold(t *testing.T) {
	r := newTestRouter(t)

	// Declarations for a selector naming a form control (input/select/textarea,
	// or the CodeMirror editor, which is a contenteditable iOS treats the same).
	fieldRule := regexp.MustCompile(`(?m)^[^@{}]*\b(input|select|textarea|cm-editor)\b[^{}]*\{[^{}]*\}`)
	pxSize := regexp.MustCompile(`font-size:(\d+(?:\.\d+)?)px`)

	for _, sheet := range []string{
		"tokens.css", "components.css", "index.css",
		"detail.css", "edit.css", "new.css", "agent.css", "notfound.css",
	} {
		t.Run(sheet, func(t *testing.T) {
			css := galleryAsset(t, r, "/assets/gallery/"+sheet)
			for _, rule := range fieldRule.FindAllString(css, -1) {
				if m := pxSize.FindStringSubmatch(rule); m != nil {
					assert.Fail(t,
						"form control sizes itself in px instead of var(--field-font-size)",
						"%s: %s\nA literal size here opts the control out of the 16px touch bump, so iOS zooms when it is focused.",
						sheet, strings.TrimSpace(rule))
				}
			}
		})
	}
}
