//go:build screenshot

package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// The real gallery and shared control, using the same local SDK double as the
// geometry tests. These images do not validate the hosted Clerk service.
func TestCaptureClerkPlaceholder(t *testing.T) {
	app := clerkPlaceholderApp(t)
	out := "../../docs/screenshots/Exh-j6wa"
	require.NoError(t, os.MkdirAll(out, 0755))
	for _, view := range []struct {
		name          string
		width, height int
	}{{"desktop", 1280, 900}, {"phone", 390, 844}} {
		for _, state := range []struct{ name, scenario, wait string }{
			{"loading", "", `window.finishReload && document.querySelector('.account-control').dataset.clerkState === 'loading'`},
			{"ready", "ready", `document.querySelector('.account-control').dataset.clerkState === 'ready'`},
			{"error", "load-failure", `document.querySelector('.account-control').dataset.clerkState === 'error'`},
		} {
			cmd := exec.Command("shot-scraper", app.URL+"/?account-test="+state.scenario,
				"--width", strconv.Itoa(view.width), "--height", strconv.Itoa(view.height),
				"--wait-for", state.wait, "--wait", "100", "--timeout", "15000",
				"--output", filepath.Join(out, state.name+"-"+view.name+".png"))
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			t.Log(string(output))
		}
	}
}
