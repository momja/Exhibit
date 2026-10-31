//go:build screenshot

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// Capture the destination after a real htmx click. The wait expression also
// verifies that the screenshot still belongs to the original app header.
func TestCaptureGallerySpaceNavigation(t *testing.T) {
	in := newGrantedInstance(t)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.granted, in.ownerTwo))
	seedSharedGallery(t, in, 12)
	_, err := in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerTwo, in.artifactTwo, in.ownerOne)
	require.NoError(t, err)
	for i, title := range []string{"Reading list", "Weekend in Copenhagen", "Project timeline", "Garden planner", "Recipe collection", "Training log", "Team directory", "Budget tracker", "Book notes", "Workshop agenda", "Color palette", "Weekly meal plan"} {
		require.NoError(t, in.ro.cfg.Store.UpdateArtifact(context.Background(), in.ownerOne, fmt.Sprintf("shared-%03d", i), map[string]any{"title": title}))
	}
	app := httptest.NewServer(in.ro)
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	parsed, err := url.Parse(app.URL)
	require.NoError(t, err)
	state, err := json.Marshal(map[string]any{"cookies": []map[string]any{{"name": in.cookieTwo.Name, "value": in.cookieTwo.Value, "domain": parsed.Hostname(), "path": "/", "httpOnly": true, "secure": false, "sameSite": "Lax", "expires": -1}}, "origins": []any{}})
	require.NoError(t, err)
	auth := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(auth, state, 0600))
	out := "../../docs/screenshots/Exh-xw54"
	require.NoError(t, os.MkdirAll(out, 0755))
	for _, view := range []struct {
		name          string
		width, height int
	}{{"01-space-switch-desktop.png", 1280, 900}, {"02-space-switch-phone.png", 390, 844}} {
		cmd := exec.Command("shot-scraper", app.URL, "--auth", auth, "--width", strconv.Itoa(view.width), "--height", strconv.Itoa(view.height), "--javascript", `window.originalHeader=document.querySelector('header');document.querySelector('.gallery-spaces a[href="/shared"]').click()`, "--wait-for", `location.pathname === '/shared' && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '/shared' && document.querySelector('header') === originalHeader`, "--wait", "500", "--output", filepath.Join(out, view.name))
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		t.Log(string(output))
	}
}
