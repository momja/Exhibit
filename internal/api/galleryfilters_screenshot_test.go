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

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCaptureGalleryFilters(t *testing.T) {
	in := newGrantedInstance(t)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.granted, in.ownerTwo))
	seedSharedGallery(t, in, 12)
	_, err := in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerTwo, in.artifactTwo, in.ownerOne)
	require.NoError(t, err)
	titles := []string{"Reading list", "Weekend in Copenhagen", "Project timeline", "Garden planner", "Recipe collection", "Training log", "Team directory", "Budget tracker", "Book notes", "Workshop agenda", "Color palette", "Weekly meal plan"}
	for i, title := range titles {
		require.NoError(t, in.ro.cfg.Store.UpdateArtifact(context.Background(), in.ownerOne, fmt.Sprintf("shared-%03d", i), map[string]any{"title": title}))
	}
	ctx := context.Background()
	for i, name := range []string{"Reading", "Projects"} {
		require.NoError(t, in.ro.cfg.Store.CreateCollection(ctx, &store.Collection{ID: fmt.Sprintf("collection-%d-%d", in.ownerOne, i), OwnerID: in.ownerOne, Name: name}))
	}
	for i, name := range []string{"Work", "Read later"} {
		require.NoError(t, in.ro.cfg.Store.CreateTag(ctx, &store.Tag{ID: fmt.Sprintf("tag-%d-%d", in.ownerOne, i), OwnerID: in.ownerOne, Name: name, Color: []string{"#2563eb", "#d97706"}[i]}))
	}
	for i := range 12 {
		id := fmt.Sprintf("shared-%03d", i)
		require.NoError(t, in.ro.cfg.Store.AddArtifactTag(ctx, in.ownerOne, id, fmt.Sprintf("tag-%d-0", in.ownerOne)))
		require.NoError(t, in.ro.cfg.Store.AddArtifactToCollection(ctx, in.ownerOne, id, fmt.Sprintf("collection-%d-0", in.ownerOne)))
	}
	app := httptest.NewServer(in.ro)
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	parsed, err := url.Parse(app.URL)
	require.NoError(t, err)
	state, err := json.Marshal(map[string]any{"cookies": []map[string]any{{"name": in.cookieOne.Name, "value": in.cookieOne.Value, "domain": parsed.Hostname(), "path": "/", "httpOnly": true, "secure": false, "sameSite": "Lax", "expires": -1}}, "origins": []any{}})
	require.NoError(t, err)
	auth := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(auth, state, 0600))
	out := "../../docs/screenshots/Exh-gx18"
	require.NoError(t, os.MkdirAll(out, 0755))
	for _, v := range []struct {
		name          string
		width, height int
	}{{"01-filters-desktop.png", 1280, 900}, {"02-filters-phone.png", 390, 844}} {
		cmd := exec.Command("shot-scraper", app.URL+"/?collection="+fmt.Sprintf("collection-%d-0", in.ownerOne)+"&tag=Work", "--auth", auth, "--width", strconv.Itoa(v.width), "--height", strconv.Itoa(v.height), "--wait", "500", "--output", filepath.Join(out, v.name))
		if v.width < 768 {
			cmd.Args = append(cmd.Args, "--javascript", `document.querySelector(".gallery-filter-trigger").click()`)
		}
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		t.Log(string(output))
	}
}
