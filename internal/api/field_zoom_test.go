package api

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-3qmf: iOS zooms in on focusing a field under 16px and never zooms back
// out. The fix is the two tokens and the coarse-pointer override that raises
// them, so that is what this asserts.
func TestFieldsAreSixteenPixelsOnTouch(t *testing.T) {
	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/tokens.css")

	assert.Contains(t, css, "--field-font-size:14px")
	assert.Contains(t, css, "--field-code-font-size:12px")

	coarse := strings.Index(css, "@media (pointer:coarse)")
	require.NotEqual(t, -1, coarse, "no coarse-pointer override — every field would zoom on focus")
	assert.Contains(t, css[coarse:], "--field-font-size:16px")
	assert.Contains(t, css[coarse:], "--field-code-font-size:16px")
}

// A px size hardcoded on a control opts it out of the touch bump silently:
// invisible on desktop, a zoom on a phone. Caught new.css's CodeMirror.
func TestNoGalleryFieldSetsItsOwnSizeBelowThreshold(t *testing.T) {
	r := newTestRouter(t)

	// A declaration block whose selector names a form control. .cm-editor is a
	// contenteditable, which iOS treats the same way.
	fieldRule := regexp.MustCompile(`(?m)^[^@{}]*\b(input|select|textarea|cm-editor)\b[^{}]*\{[^{}]*\}`)
	pxSize := regexp.MustCompile(`font-size:(\d+(?:\.\d+)?)px`)

	for _, sheet := range []string{
		"tokens.css", "components.css", "index.css",
		"detail.css", "edit.css", "new.css", "agent.css", "notfound.css",
	} {
		t.Run(sheet, func(t *testing.T) {
			css := galleryAsset(t, r, "/assets/gallery/"+sheet)
			for _, rule := range fieldRule.FindAllString(css, -1) {
				if pxSize.MatchString(rule) {
					assert.Fail(t, "form control sizes itself in px instead of var(--field-font-size)",
						"%s: %s", sheet, strings.TrimSpace(rule))
				}
			}
		})
	}
}
