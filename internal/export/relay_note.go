package export

import (
	"encoding/json"
	stdhtml "html"
	"sort"

	"github.com/momja/Exhibit/internal/origin"
)

// RelayNote records the API origins an exported artifact uses through a relay.
// It is inert metadata for the person opening or adapting the portable file;
// no server address, credential, connection id or grant crosses this boundary.
// The caller supplies runtime-approved origins too, because a static scan
// cannot find API URLs composed while the artifact runs.
func RelayNote(body string, origins []string) string {
	seen := make(map[string]bool)
	for _, raw := range origins {
		if normalized, err := origin.NormalizeOrigin(raw); err == nil && normalized != "" {
			seen[normalized] = true
		}
	}
	if len(seen) == 0 {
		return body
	}
	origins = make([]string, 0, len(seen))
	for origin := range seen {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	payload, _ := json.Marshal(origins)
	note := `<!-- Exhibit portability: this file calls APIs at the origins listed below.
Outside Exhibit, APIs without CORS support need a compatible relay. Configure
credentials separately in your local runner; connection secrets are not exported.
-->` + "\n" + `<meta name="exhibit-relay-origins" content="` + stdhtml.EscapeString(string(payload)) + `">`
	at, before := headInsertion(body)
	if at < 0 {
		return note + "\n" + body
	}
	if before {
		return body[:at] + note + "\n" + body[at:]
	}
	return body[:at] + "\n" + note + body[at:]
}
