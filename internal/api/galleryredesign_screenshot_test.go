//go:build screenshot

package api

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCaptureGalleryRedesign(t *testing.T) {
	app, in := galleryNavigationApp(t)
	ctx := t.Context()
	titles := []string{"Reading list", "Budget tracker", "Garden notes", "Recipe collection", "Training log", "Project timeline"}
	for i, title := range titles {
		require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, fmt.Sprintf("shared-%03d", i), map[string]any{"title": title}))
	}
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerTwo, in.artifactTwo, map[string]any{"title": "Weekly planner"}))
	for _, id := range []string{in.artifactTwo, "shared-000", "shared-001"} {
		require.NoError(t, in.ro.cfg.Store.SetArtifactStarred(ctx, store.ViewerID(in.ownerTwo), id, true))
	}
	source := "activity-screenshot-body"
	emptyWidget := ""
	require.NoError(t, in.ro.cfg.Blob.Put(ctx, source, strings.NewReader("<h1>Weekly planner</h1>")))
	_, err := in.ro.cfg.Store.CommitVersion(ctx, in.ownerTwo, in.artifactTwo, store.VersionChange{Provenance: store.Provenance{Origin: store.VersionAgent, Message: "Added a weekly overview and a place for daily priorities.", SessionID: "weekly-planner-chat"}, BodyBlobID: &source, WidgetBlobID: &emptyWidget})
	require.NoError(t, err)
	require.NoError(t, in.ro.cfg.Store.SaveTranscript(ctx, in.ownerTwo, store.Transcript{ArtifactID: in.artifactTwo, SessionID: "weekly-planner-chat", Title: "Add a weekly overview", SessionFile: piFile}))
	out := "../../docs/screenshots/Exh-77m9"
	require.NoError(t, os.MkdirAll(out, 0755))
	for _, viewport := range []struct {
		name          string
		width, height int
	}{{"desktop", 1280, 900}, {"phone", 390, 844}} {
		for _, destination := range []struct{ name, path string }{{"starred", "/"}, {"search", "/search"}, {"shared", "/shared"}, {"activity", "/activity"}} {
			cmd := exec.Command("shot-scraper", app.URL+destination.path, "--width", fmt.Sprint(viewport.width), "--height", fmt.Sprint(viewport.height), "--wait", "800", "--javascript", `const account=document.querySelector('.cl-userButtonTrigger');if(account){account.textContent='';account.classList.add('ph','ph-user-circle');account.setAttribute('aria-label','Account');Object.assign(account.style,{width:'36px',height:'36px',border:'0',background:'transparent',fontSize:'20px',color:'var(--color-accent)'})}`, "--output", filepath.Join(out, destination.name+"-"+viewport.name+".png"))
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		}
	}
}
