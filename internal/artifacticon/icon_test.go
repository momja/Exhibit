package artifacticon

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestTagDots(t *testing.T) {
	tags := []*store.Tag{{Color: "#f00"}, {Color: "#00ff00"}, {Color: `"/><script/>`}, {Color: "#123456"}}
	for count := 0; count <= len(tags); count++ {
		uri := DataURI(tags[:count])
		svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
		require.NoError(t, err)
		require.Equal(t, min(count, 3), strings.Count(string(svg), "<circle"))
		require.Contains(t, string(svg), `fill="#de281d"`)
		require.NotContains(t, string(svg), "<script")
		if count >= 3 {
			require.Contains(t, string(svg), `cy="11" r="3.5" fill="#ff0000"`)
			require.Contains(t, string(svg), `cy="16" r="3.5" fill="#00ff00"`)
			require.Contains(t, string(svg), `cy="21" r="3.5" fill="#6b7280"`)
		}
	}
}
