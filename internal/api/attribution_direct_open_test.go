package api

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This endpoint redirects into the same /a/:id document pinned by render's
// legacy fixture. Strip only unpredictable artifact IDs and signed credentials.
func TestAttributionDirectOpenLegacyBytes(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	for _, enabled := range []bool{false, true} {
		ro.cfg.ShareAttributionEnabled = enabled
		w := doJSON(t, ro, "GET", "/artifacts/"+id+"/open?direct=1&r=123", nil)
		require.Equal(t, 302, w.Code)
		location, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		token := location.Query().Get("t")
		require.NotEmpty(t, token)
		normalized := strings.ReplaceAll(strings.ReplaceAll(w.Body.String()+fmt.Sprint(w.Header()), id, "ARTIFACT"), token, "TOKEN")
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
		require.Equal(t, "0cd2827024473471a0d3220db06a359a9b7c9c91f5cfb828ce2cee6c998b6825", got)
	}
}
