package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The vendored Phosphor stylesheet is subset to exactly the icon classes
// listed in web/icons/icons.txt (av-diue) — an explicit registry, not a
// scrape, so adding an icon is one deliberate line and forgetting it fails
// here, before any build, with the fix named.
//
// Two halves, two failure modes:
//   - TestIconRegistryCoversUsage: a template/script/stylesheet, or a Go file
//     in this package, names an icon missing from the registry. Fix: add the class to web/icons/icons.txt.
//   - TestServedCSSCoversUsage: the embedded CSS predates the usage (assets
//     not rebuilt). Fix: rebuild assets (make assets).
// Weight-family classes (ph-bold etc.) live in weights we don't vendor, so
// both halves ignore them; page scripts' icons are covered because a script
// naming an icon the registry lacks fails the first half.

var (
	iconClassRe = regexp.MustCompile(`\bph-([a-z0-9-]+)`)

	// Font-weight families, not icons: no :before rule exists for these in
	// any weight's stylesheet, subset or full.
	iconFamily = map[string]bool{
		"thin": true, "light": true, "regular": true,
		"bold": true, "fill": true, "duotone": true,
	}

	// Source roots that may name an icon, relative to this package dir.
	// node_modules is skipped in the walk: upstream's own demo/docs classes
	// must not leak into the required set.
	iconUsageRoots = []string{"templates", "../../web/gallery", "../../web/editor"}
)

// iconUsage collects every ph-* icon class named by templates, scripts, and
// stylesheets under the usage roots, plus this package's own Go source: a
// view model can pick an icon too (shareBadgeView.Icon), and that one
// shipped blank once because nothing here read Go (av-uvc6).
func iconUsage(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	goFiles, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, p := range goFiles {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		body, err := os.ReadFile(p)
		require.NoError(t, err)
		for _, m := range iconClassRe.FindAllSubmatch(body, -1) {
			if !iconFamily[string(m[1])] {
				used[string(m[1])] = true
			}
		}
	}
	for _, root := range iconUsageRoots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".tmpl", ".js", ".css", ".html":
			default:
				return nil
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range iconClassRe.FindAllSubmatch(body, -1) {
				if !iconFamily[string(m[1])] {
					used[string(m[1])] = true
				}
			}
			return nil
		})
		require.NoError(t, err, "scanning %s for icon usage", root)
	}
	require.NotEmpty(t, used, "expected icon usage; scanner may have broken")
	return used
}

// iconRegistry reads web/icons/icons.txt: one `ph-*` class per line,
// `#` comments and blanks ignored.
func iconRegistry(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile("../../web/icons/icons.txt")
	require.NoError(t, err)
	set := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		require.Regexp(t, regexp.MustCompile(`^ph-[a-z0-9-]+$`), line,
			"icons.txt: bad line %q — want ph-<name>", line)
		set[line] = true
	}
	require.NotEmpty(t, set, "icons.txt holds no icons")
	return set
}

func TestIconRegistryCoversUsage(t *testing.T) {
	registry := iconRegistry(t)
	for cls := range iconUsage(t) {
		// Plain Errorf, not Contains: the failure is one missing name, and
		// dumping the whole registry buries it.
		if !registry["ph-"+cls] {
			t.Errorf("icon .ph-%s is used but not registered — add it to web/icons/icons.txt and rebuild assets", cls)
		}
	}
}

func TestServedCSSCoversUsage(t *testing.T) {
	req := httptest.NewRequest("GET", "/assets/phosphor/regular.css", nil)
	w := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	css := w.Body.String()
	for cls := range iconUsage(t) {
		if !strings.Contains(css, ".ph-"+cls+":before") {
			t.Errorf("icon .ph-%s missing from served CSS — rebuild assets (make assets)", cls)
		}
	}
}
