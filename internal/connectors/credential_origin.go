package connectors

// A custom connection's origin is chosen by the person setting it up, and the
// credential is then sent there. Nothing in that flow stopped a tool or a
// message from talking someone into pasting an OpenAI key with the origin
// https://api-openai.co: the relay would faithfully deliver it.
//
// This file recognizes the handful of credential formats whose issuer is
// public and known, so a mismatch between the credential and the destination
// can be surfaced before the connection is saved. It is a warning, not a
// wall — the person can still confirm an intentional pairing — and it never
// runs on the credential's behalf anywhere else: the check is on the create
// request only, in the trusted app, and the secret is never stored by it.

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// CredentialOriginError reports a credential whose well-known format belongs to
// a different service than the origin it would be sent to. It carries only what
// the person needs to make the decision; the credential itself is never part of
// the error text.
type CredentialOriginError struct {
	// Label names the kind of credential that was recognized, e.g.
	// "Anthropic API key".
	Label string
	// Expected is the origin that credential is normally used with.
	Expected string
	// Origin is the destination the person chose.
	Origin string
}

func (e *CredentialOriginError) Error() string {
	return fmt.Sprintf("this looks like a %s, which normally goes to %s, but it would be sent to %s", e.Label, e.Expected, e.Origin)
}

// credentialIssuer is one recognizable credential format and the service that
// issues it. Domain is the registrable domain; every host under it counts as
// the same service, which is what keeps https://eu.api.openai.com valid.
type credentialIssuer struct {
	prefixes []string
	domain   string
	expected string
	label    string
}

// The order matters: sk-ant- is also an sk- prefix, so the more specific
// families are listed first.
var credentialIssuers = []credentialIssuer{
	{[]string{"sk-ant-"}, "anthropic.com", "https://api.anthropic.com", "Anthropic API key"},
	{[]string{"sk-proj-", "sk-svcacct-", "sk-admin-", "sk-"}, "openai.com", "https://api.openai.com", "OpenAI API key"},
	{[]string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_"}, "github.com", "https://api.github.com", "GitHub token"},
	{[]string{"sk_live_", "rk_live_"}, "stripe.com", "https://api.stripe.com", "Stripe secret key"},
	{[]string{"xoxb-", "xoxp-", "xoxa-", "xoxr-", "xoxs-"}, "slack.com", "https://slack.com/api", "Slack token"},
	{[]string{"glpat-"}, "gitlab.com", "https://gitlab.com", "GitLab token"},
	{[]string{"hf_"}, "huggingface.co", "https://huggingface.co", "Hugging Face token"},
	{[]string{"npm_"}, "npmjs.org", "https://registry.npmjs.org", "npm token"},
	{[]string{"AIza"}, "googleapis.com", "https://www.googleapis.com", "Google API key"},
}

// RecognizedCredential names the issuer of secret when its format is one this
// package knows. The second return is the issuer's registrable domain, the
// third the origin that credential normally accompanies.
func RecognizedCredential(secret string) (label, domain, expected string, ok bool) {
	for _, issuer := range credentialIssuers {
		for _, prefix := range issuer.prefixes {
			// Prefix matching is deliberately exact and case-sensitive: these
			// formats are, and a case-insensitive match would flag a credential
			// whose case is part of its value.
			if strings.HasPrefix(secret, prefix) {
				return issuer.label, issuer.domain, issuer.expected, true
			}
		}
	}
	return "", "", "", false
}

// credentialDestinationMismatch returns the error to show when secret is a
// recognized credential that does not belong to any of origins. It returns nil
// when the credential is unrecognized, when one of the origins is the issuer's
// service, or when there are no origins to compare against.
func credentialDestinationMismatch(secret string, origins []string) *CredentialOriginError {
	label, domain, expected, ok := RecognizedCredential(secret)
	if !ok || len(origins) == 0 {
		return nil
	}
	for _, o := range origins {
		if registrableDomain(o) == domain {
			return nil
		}
	}
	return &CredentialOriginError{Label: label, Expected: expected, Origin: origins[0]}
}

// registrableDomain reduces an origin (or any URL) to the eTLD+1 the browser's
// own protections reason about, so api.openai.com and eu.api.openai.com both
// answer openai.com while api-openai.co and openai.com.evil.test do not.
//
// The public suffix list is a proxy for "the same service", not a security
// boundary: this is a warning the person can override, never an authorization
// decision.
func registrableDomain(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return ""
	}
	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return domain
	}
	// An IP literal, a single-label host or a suffix the list does not cover
	// still compares usefully against itself.
	return host
}

// credentialDestinationCheck refuses to save a recognized credential aimed at a
// service that does not issue that kind of credential, unless the person
// confirmed the destination after seeing the warning.
func credentialDestinationCheck(secret string, origins []string, confirmed bool) error {
	if confirmed {
		return nil
	}
	if mismatch := credentialDestinationMismatch(secret, origins); mismatch != nil {
		return mismatch
	}
	return nil
}
