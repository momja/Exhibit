package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig is what an operator supplies to point Exhibit at their identity
// provider. Issuer is the only endpoint they name: everything else —
// authorization URL, token URL, signing keys — comes from that issuer's
// /.well-known/openid-configuration document. Discovery is what makes "any
// OIDC provider" true in practice rather than in principle; hand-configured
// endpoints would make every provider its own support question.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is this instance's /auth/callback on the app origin. It
	// must match what is registered at the provider.
	RedirectURL string
	// Scopes defaults to openid+email+profile. Overridable because some
	// providers name their email scope differently.
	Scopes []string
}

// OIDCProvider is the generic provider: Authorization Code + PKCE against any
// spec-compliant OIDC issuer. There is deliberately no vendor SDK behind it —
// Authentik, Keycloak, Zitadel, Dex, Ory and the hosted providers that expose
// a standard OIDC surface are all configuration of this one type.
type OIDCProvider struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// Compile-time proof that the generic provider satisfies the seam. A second
// provider needs nothing but this same assertion.
var _ IdentityProvider = (*OIDCProvider)(nil)

// NewOIDCProvider performs discovery against the issuer and returns a provider
// ready to serve logins. It reaches the network once, at construction: an
// unreachable or misconfigured issuer is a startup failure the operator can
// see, not a surprise at the first login attempt.
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("oidc: no issuer")
	}
	if cfg.ClientID == "" {
		return nil, errors.New("oidc: no client id")
	}
	if cfg.RedirectURL == "" {
		return nil, errors.New("oidc: no redirect url")
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", cfg.Issuer, err)
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return &OIDCProvider{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// AuthURL is where the browser is sent to log in. The PKCE challenge is
// derived from the verifier here so callers never handle both halves.
func (p *OIDCProvider) AuthURL(state, verifier string) string {
	return p.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// Exchange redeems the callback's code for an identity. This is the only
// place in the system that talks to the provider, and it runs once per login.
func (p *OIDCProvider) Exchange(ctx context.Context, code, verifier string) (*Identity, error) {
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("oidc token exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc: token response carried no id_token")
	}
	// Signature, issuer, audience and expiry, checked against the keys
	// discovery pointed at. An unverified id_token is just a claim the
	// browser handed us.
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify id_token: %w", err)
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: decode claims: %w", err)
	}
	if idToken.Subject == "" {
		return nil, errors.New("oidc: id_token carried no subject")
	}
	return &Identity{ExternalID: idToken.Subject, Email: claims.Email}, nil
}
