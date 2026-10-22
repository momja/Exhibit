package connectors

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"golang.org/x/oauth2"
)

// OAuthProvider is operator configuration, never an artifact or agent recipe.
// GitHub is the first built-in provider; endpoint overrides enable enterprise
// instances and mock-provider verification without changing authorization code.
type OAuthProvider struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	APIOrigin    string
	RevokeURL    string
	Scopes       []string
	RedirectURL  string
}

func (p OAuthProvider) defaults() OAuthProvider {
	if p.AuthURL == "" {
		p.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if p.TokenURL == "" {
		p.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if p.APIOrigin == "" {
		p.APIOrigin = "https://api.github.com"
	}
	if p.RevokeURL == "" {
		p.RevokeURL = "https://api.github.com/applications/" + url.PathEscape(p.ClientID) + "/token"
	}
	if len(p.Scopes) == 0 {
		p.Scopes = []string{"read:user"}
	}
	return p
}
func (p OAuthProvider) oauthConfig(redirect string) *oauth2.Config {
	if p.RedirectURL != "" {
		redirect = p.RedirectURL
	}
	return &oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, Endpoint: oauth2.Endpoint{AuthURL: p.AuthURL, TokenURL: p.TokenURL, AuthStyle: oauth2.AuthStyleInParams}, RedirectURL: redirect, Scopes: p.Scopes}
}

type oauthFlow struct {
	ownerID                  int64
	binding                  [32]byte
	verifier, name, redirect string
	expires                  time.Time
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("operating system random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (s *Service) BeginOAuth(_ context.Context, ownerID int64, binding, name, redirect string) (string, error) {
	if !s.cfg.OAuthEnabled || s.cfg.GitHub.ClientID == "" || s.cfg.GitHub.ClientSecret == "" {
		return "", ErrDisabled
	}
	if binding == "" || ownerID <= 0 {
		return "", ErrInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "GitHub"
	}
	if len(name) > 100 {
		return "", ErrInvalid
	}
	flow := oauthFlow{ownerID: ownerID, binding: sha256.Sum256([]byte(binding)), verifier: oauth2.GenerateVerifier(), name: name, redirect: redirect, expires: s.cfg.Now().Add(10 * time.Minute)}
	state := randomToken()
	s.mu.Lock()
	for key, f := range s.flows {
		if !f.expires.After(s.cfg.Now()) {
			delete(s.flows, key)
		}
	}
	if len(s.flows) >= 1024 {
		s.mu.Unlock()
		return "", errors.New("too many pending OAuth connections; try again later")
	}
	s.flows[state] = flow
	s.mu.Unlock()
	return s.cfg.GitHub.oauthConfig(redirect).AuthCodeURL(state, oauth2.S256ChallengeOption(flow.verifier)), nil
}

func (s *Service) CompleteOAuth(ctx context.Context, ownerID int64, binding, state, code string, limits Limits) (*Connection, error) {
	if !s.cfg.OAuthEnabled {
		return nil, ErrDisabled
	}
	s.mu.Lock()
	flow, ok := s.flows[state]
	if ok {
		delete(s.flows, state)
	}
	s.mu.Unlock()
	hash := sha256.Sum256([]byte(binding))
	if !ok || state == "" || code == "" || flow.ownerID != ownerID || !flow.expires.After(s.cfg.Now()) || subtle.ConstantTimeCompare(hash[:], flow.binding[:]) != 1 {
		return nil, fmtOAuthState()
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.oauthClient())
	token, err := s.cfg.GitHub.oauthConfig(flow.redirect).Exchange(ctx, code, oauth2.VerifierOption(flow.verifier))
	if err != nil {
		return nil, errors.New("GitHub authorization could not be completed; reconnect from Exhibit")
	}
	raw, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}
	return s.save(ctx, ownerID, flow.name, "github", "oauth", recipe{Origins: []string{s.cfg.GitHub.APIOrigin}, Header: "Authorization", Scheme: "Bearer ", Sensitivity: "personal"}, credential{OAuthToken: raw}, limits)
}
func fmtOAuthState() error {
	return fmt.Errorf("%w: OAuth state expired or did not match this browser session", ErrInvalid)
}

// oauthTransport sends acquisition, refresh and revocation through the same
// dial-time gate used by relay requests. Its trusted headers come from oauth2,
// never from the artifact's postMessage payload.
type oauthTransport struct{ s *Service }

func (t oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	expected := requestOrigin(req.URL)
	gate, err := t.s.cfg.GateFactory([]string{expected})
	if err != nil {
		return nil, err
	}
	defer gate.Close()
	headers := req.Header.Clone()
	response, err := gate.DoAuthorized(req.Context(), req, func(hop *http.Request) error {
		if requestOrigin(hop.URL) != expected {
			return ErrEgress
		}
		for key, values := range headers {
			hop.Header[key] = append([]string{}, values...)
		}
		return nil
	})
	if err != nil {
		return nil, errors.New("OAuth provider request failed")
	}
	return &http.Response{StatusCode: response.Status, Status: response.StatusText, Header: response.Header, Body: io.NopCloser(bytes.NewReader(response.Body)), ContentLength: int64(len(response.Body)), Request: req}, nil
}
func (s *Service) oauthClient() *http.Client {
	return &http.Client{Transport: oauthTransport{s: s}, Timeout: 20 * time.Second}
}

type oauthEngine struct{ s *Service }

func (e oauthEngine) Authorize(ctx context.Context, req *http.Request, rec *store.ConnectionRecord, r recipe, c *credential) error {
	if !e.s.cfg.OAuthEnabled {
		return ErrDisabled
	}
	if requestOrigin(req.URL) != e.s.cfg.GitHub.APIOrigin {
		return ErrEgress
	}
	var current oauth2.Token
	if err := json.Unmarshal(c.OAuthToken, &current); err != nil {
		return errors.New("invalid stored OAuth token")
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, e.s.oauthClient())
	token, err := e.s.cfg.GitHub.oauthConfig("").TokenSource(ctx, &current).Token()
	if err != nil {
		return errors.New("GitHub connection needs to be reconnected")
	}
	if token.AccessToken != current.AccessToken || token.RefreshToken != current.RefreshToken || !token.Expiry.Equal(current.Expiry) {
		raw, err := json.Marshal(token)
		if err != nil {
			return err
		}
		c.OAuthToken = raw
		all, err := json.Marshal(c)
		if err != nil {
			return err
		}
		sealed, err := e.s.cfg.Box.Encrypt(string(all))
		if err != nil {
			return err
		}
		if err = e.s.cfg.Store.UpdateConnectionSecret(ctx, rec.OwnerID, rec.ID, sealed); err != nil {
			return err
		}
	}
	c.Value = token.AccessToken
	return (NativeEngine{}).Authorize(ctx, req, rec, r, c)
}

func (s *Service) Revoke(ctx context.Context, ownerID int64, id string) error {
	rec, err := s.cfg.Store.GetConnection(ctx, ownerID, id)
	if err != nil {
		return err
	}
	lock, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	rec, err = s.cfg.Store.GetConnection(ctx, ownerID, id)
	if err != nil {
		return err
	}
	// Revoke locally only once the provider has accepted revocation. A network
	// failure remains visible and retryable instead of claiming remote removal.
	switch rec.Kind {
	case "oauth":
		if !s.cfg.OAuthEnabled {
			return ErrDisabled
		}
		_, c, err := s.readCredential(rec)
		if err != nil {
			return err
		}
		var token oauth2.Token
		if err = json.Unmarshal(c.OAuthToken, &token); err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]string{"access_token": token.AccessToken})
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.cfg.GitHub.RevokeURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/vnd.github+json")
		req.SetBasicAuth(s.cfg.GitHub.ClientID, s.cfg.GitHub.ClientSecret)
		resp, err := s.oauthClient().Do(req)
		if err != nil {
			return errors.New("GitHub token could not be revoked; retry or revoke it in GitHub settings")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 204 && resp.StatusCode != 404 {
			return errors.New("GitHub token could not be revoked; retry or revoke it in GitHub settings")
		}
	case "vendor":
		if err = s.deleteVendor(ctx, rec); err != nil {
			return err
		}
	}
	if err = s.cfg.Store.DeleteConnection(ctx, ownerID, id); err != nil {
		return err
	}
	s.mu.Lock()
	for token, b := range s.standins {
		if b.connectionID == id {
			delete(s.standins, token)
		}
	}
	s.mu.Unlock()
	s.locks.Delete(id)
	return nil
}

// These helpers keep callers' error mapping independent of provider messages.
func ErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrDisabled):
		return 404
	case errors.Is(err, store.ErrNotFound):
		return 404
	case errors.Is(err, store.ErrConnectionLimit), errors.Is(err, store.ErrConnectionQuota):
		return 429
	case errors.Is(err, ErrEgress), errors.Is(err, ErrStandin):
		return 403
	case errors.Is(err, store.ErrConnectionSharing), errors.Is(err, store.ErrConnectionOverlap):
		return 409
	case errors.Is(err, ErrInvalid):
		return 400
	default:
		return 502
	}
}
