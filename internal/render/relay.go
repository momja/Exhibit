package render

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed relay.js
var relayJS string

func injectRelay(body, appOrigin, artifactID string, origins []string) string {
	config, _ := json.Marshal(struct {
		AppOrigin  string   `json:"app_origin"`
		ArtifactID string   `json:"artifact_id"`
		Origins    []string `json:"origins"`
	}{appOrigin, artifactID, origins})
	script := strings.Replace(relayJS, "__EXHIBIT_RELAY_CONFIG__", string(config), 1)
	return prependToHead(body, "<script>"+script+"</script>")
}
