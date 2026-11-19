package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGallerySpaceFragments(t *testing.T) {
	in := newGrantedInstance(t)
	for _, gallery := range []galleryDefinition{starredGallery, searchGallery, sharedGallery, activityGallery} {
		for _, history := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/history=%t", gallery.ID, history), func(t *testing.T) {
				path := gallery.PagePath
				if !gallery.HideSearch {
					path += "?q=absent"
				}
				r := httptest.NewRequest("GET", path, nil)
				r.AddCookie(in.cookieTwo)
				r.Header.Set("HX-Request", "true")
				if history {
					r.Header.Set("HX-History-Restore-Request", "true")
				} else {
					r.Header.Set("HX-Target", "div#gallery-content")
				}
				w := httptest.NewRecorder()
				in.ro.ServeHTTP(w, r)
				require.Equal(t, http.StatusOK, w.Code)
				body := w.Body.String()
				for _, forbidden := range []string{"<html", "<header", "<script", "data-clerk-target", "const TOKEN", "const READ_ONLY"} {
					assert.NotContains(t, body, forbidden)
				}
				assert.Contains(t, body, `hx-history-elt data-gallery-target="space"`)
				assert.Contains(t, body, `hx-swap-oob="outerHTML:#gallery-search-region"`)
				if !gallery.Activity {
					assert.Contains(t, body, `hx-get="`+gallery.FragmentPath+`"`)
				}
				assert.Contains(t, body, `href="`+gallery.PagePath+`" aria-current="page"`)
				assert.Contains(t, body, fmt.Sprintf(`data-gallery-read-only="%t"`, gallery.ReadOnly))
				if !gallery.HideSearch {
					assert.Contains(t, body, `value="absent"`)
				}
				assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				if gallery.ReadOnly {
					assert.NotContains(t, body, "data-duplicate-dialog")
				} else {
					assert.Contains(t, body, "data-duplicate-dialog")
				}
				// Direct loads embed the exact same content, not a parallel page layout.
				content := body[strings.Index(body, `<div id="gallery-content"`):strings.Index(body, `<div hx-swap-oob=`)]
				full := in.get(t, path, in.cookieTwo).Body.String()
				assert.Contains(t, normalizePerRender(full), strings.TrimSpace(normalizePerRender(content)))
			})
		}
	}
}

func TestGallerySpaceFragmentExpiredSession(t *testing.T) {
	in := newGrantedInstance(t)
	for _, path := range []string{"/", "/search", "/shared", "/activity"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Target", "div#gallery-content")
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "true", w.Header().Get("HX-Refresh"))
		assert.NotContains(t, w.Body.String(), "gallery-content")
	}
}
