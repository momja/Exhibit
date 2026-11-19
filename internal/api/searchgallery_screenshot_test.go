//go:build screenshot

package api

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureSearchTab(t *testing.T) {
	app, in := galleryNavigationApp(t)
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(t.Context(), in.ownerTwo, in.artifactTwo, map[string]any{"title": "Chart builder"}))
	for i, title := range []string{"Weekly activity chart", "Household budget chart", "Garden growth chart", "Training progress chart"} {
		require.NoError(t, in.ro.cfg.Store.UpdateArtifact(t.Context(), in.ownerOne, fmt.Sprintf("shared-%03d", i), map[string]any{"title": title}))
	}
	out := "../../docs/screenshots/Exh-im5m"
	require.NoError(t, os.MkdirAll(out, 0755))
	for _, shot := range []struct {
		name, path, js, wait string
		width                int
	}{
		{"01-library-phone.png", "/", "", `document.querySelector('.gallery-spaces a[href="/search"]').getClientRects().length > 0`, 390},
		{"02-search-phone.png", "/", `window.originalHeader=document.querySelector('header');document.querySelector('.gallery-spaces a[href="/search"]').click()`, `location.pathname === '/search' && document.querySelector('header') === originalHeader && document.querySelector('.gallery-page-search')`, 390},
		{"03-search-results-phone.png", "/search?q=chart", "", `document.querySelector('.card-by') && document.querySelector('.card-actions')`, 390},
		{"04-search-results-narrow.png", "/search?q=chart", "", `document.querySelector('.card-by') && document.querySelector('.card-actions')`, 320},
	} {
		args := []string{app.URL + shot.path, "--width", strconv.Itoa(shot.width), "--height", "844", "--wait-for", shot.wait, "--wait", "500", "--output", filepath.Join(out, shot.name)}
		if shot.js != "" {
			args = append(args, "--javascript", shot.js)
		}
		output, err := exec.Command("shot-scraper", args...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
}
