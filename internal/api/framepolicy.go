package api

import (
	"net/url"
	"strings"
)

// relayFramePolicy constrains all later navigations of app-owned artifact
// frames, not only their initial src. This closes the navigation channel around
// a connected artifact's own CSP. A blank id is the agent surface, which must
// be able to create its first artifact and inspect its version/widget history;
// its document-bound relay channel supplies the narrower authority there.
func relayFramePolicy(renderOrigin, artifactID string, appOrigin ...string) string {
	return relayDocumentFramePolicy(renderOrigin, artifactID, "", appOrigin...)
}

func (u renderURLs) framePolicy(artifactID string, appOrigin ...string) string {
	policy, err := u.policyPath(artifactID)
	if err != nil {
		return "frame-src 'none'"
	}
	return relayDocumentFramePolicy(u.origin, artifactID, policy, appOrigin...)
}

func relayDocumentFramePolicy(renderOrigin, artifactID, fingerprint string, appOrigin ...string) string {
	base := framePolicyOrigin(renderOrigin)
	if base == "" {
		return "frame-src 'none'"
	}
	if artifactID == "" {
		return "frame-src " + base + "/a/ " + base + "/w/"
	}
	if artifactID == "." || artifactID == ".." {
		return "frame-src 'none'"
	}
	artifactPath := url.PathEscape(artifactID)
	suffix := policySuffix(fingerprint)
	policy := "frame-src " + base + "/a/" + artifactPath + suffix + " " + base + "/w/" + artifactPath + suffix + " " + base + "/a/" + artifactPath + suffix + "/versions/"
	// The detail frame refreshes through this exact app route to mint a fresh
	// render token. The agent refreshes its server-rendered preview instead,
	// and must never allow a broad app-origin prefix while still unbound.
	if len(appOrigin) > 0 {
		app := framePolicyOrigin(appOrigin[0])
		if app == "" {
			return "frame-src 'none'"
		}
		policy += " " + app + "/artifacts/" + artifactPath + "/open"
	}
	return policy
}

func framePolicyOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || u.Opaque != "" ||
		(u.Scheme != "https" && u.Scheme != "http") ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	base := canonicalOrigin(raw)
	if base == "" || strings.ContainsAny(base, ";\r\n\t ") {
		return ""
	}
	return base
}
