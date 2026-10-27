package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/session"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

// Clerk verifies hosted browser sessions. Self-hosted OIDC and password logins
// continue to use Exhibit's opaque sessions; Clerk never issues one of those.
type Clerk struct {
	PublishableKey string
	FrontendAPI    string
	middleware     func(http.Handler) http.Handler
	users          *user.Client
	sessions       *session.Client
	cacheMu        sync.Mutex
	identities     map[string]cachedHostedIdentity
	appOrigin      string
}

type clerkSessionState struct {
	Status string `json:"sts"`
}

func NewClerk(publishableKey, secretKey, appOrigin, renderOrigin string) (*Clerk, error) {
	appURL, appErr := url.Parse(appOrigin)
	renderURL, renderErr := url.Parse(renderOrigin)
	if appErr != nil || renderErr != nil || appURL.Hostname() == "" || renderURL.Hostname() == "" || strings.EqualFold(appURL.Hostname(), renderURL.Hostname()) {
		return nil, errors.New("Clerk requires different APP_ORIGIN and RENDER_ORIGIN hostnames: cookies are not isolated by port")
	}
	config := &clerk.ClientConfig{}
	config.Key = clerk.String(secretKey)
	return newClerk(publishableKey, appOrigin, config)
}

func newClerk(publishableKey, appOrigin string, config *clerk.ClientConfig) (*Clerk, error) {
	if publishableKey == "" || config.Key == nil || *config.Key == "" {
		return nil, errors.New("Clerk requires CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY")
	}
	if !strings.HasPrefix(publishableKey, "pk_test_") && !strings.HasPrefix(publishableKey, "pk_live_") {
		return nil, errors.New("invalid Clerk publishable key")
	}
	encoded := strings.TrimPrefix(strings.TrimPrefix(publishableKey, "pk_test_"), "pk_live_")
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil || !strings.HasSuffix(string(decoded), "$") {
		return nil, errors.New("invalid Clerk publishable key domain")
	}
	domain := strings.TrimSuffix(string(decoded), "$")
	u, err := url.Parse("https://" + domain)
	if err != nil || u.Hostname() != domain || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.Contains(domain, ".") {
		return nil, errors.New("invalid Clerk frontend domain")
	}
	origin, err := url.Parse(appOrigin)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" {
		return nil, errors.New("Clerk requires an absolute APP_ORIGIN without a path")
	}
	c := &Clerk{appOrigin: appOrigin, PublishableKey: publishableKey, FrontendAPI: u.String(), users: user.NewClient(config), sessions: session.NewClient(config), identities: make(map[string]cachedHostedIdentity)}
	c.middleware = func(next http.Handler) http.Handler {
		return clerkhttp.WithHeaderAuthorization(
			clerkhttp.AuthorizationFailureHandler(next),
			clerkhttp.JWKSClient(jwks.NewClient(config)),
			clerkhttp.AuthorizationJWTExtractor(func(r *http.Request) string {
				cookie, err := r.Cookie("__session")
				if err != nil {
					return ""
				}
				return cookie.Value
			}),
			clerkhttp.AuthorizedParty(func(party string) bool { return party == appOrigin }),
			clerkhttp.CustomClaimsConstructor(func(context.Context) any { return &clerkSessionState{} }),
		)(next)
	}
	return c, nil
}

// Middleware leaves unauthenticated requests to Exhibit's existing route gates.
// Invalid/expired cookies follow the same path, allowing the login page to load
// ClerkJS and renew a session instead of trapping the browser behind a 401.
func (c *Clerk) Middleware(next http.Handler) http.Handler {
	verified := c.middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("__session"); err == nil && cookie.Value != "" && r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if origin := r.Header.Get("Origin"); origin != "" && origin != c.appOrigin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		verified.ServeHTTP(w, r)
	})
}

// SessionExpiresIn is how long the request's verified session token has left,
// or zero without one. It is measured on the server's clock, the one the
// token is checked against, so a page can schedule its renewal without
// trusting the device's.
func (c *Clerk) SessionExpiresIn(r *http.Request) time.Duration {
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok || claims.Expiry == nil {
		return 0
	}
	return max(time.Until(time.Unix(*claims.Expiry, 0)), 0)
}

func (c *Clerk) Subject(r *http.Request) string {
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok || claims.Issuer != c.FrontendAPI || claims.Subject == "" || claims.SessionID == "" || claims.Expiry == nil {
		return ""
	}
	state, ok := claims.Custom.(*clerkSessionState)
	if !ok || (state.Status != "" && state.Status != "active") {
		return ""
	}
	return "clerk:" + claims.Subject
}

// HostedIdentity binds the immutable provider session creation time to its user.
// JWT iat is refreshed frequently and cannot be used as a deletion fence.
type HostedIdentity struct {
	Subject          string
	Email            string
	SessionCreatedAt int64
}

type cachedHostedIdentity struct {
	identity HostedIdentity
	expires  time.Time
}

// Identity periodically refreshes provider status and profile data. The local
// deletion fence is checked on every request, independently of this small cache.
func (c *Clerk) Identity(r *http.Request) (HostedIdentity, error) {
	subject := c.Subject(r)
	if subject == "" {
		return HostedIdentity{}, errors.New("invalid Clerk session")
	}
	claims, _ := clerk.SessionClaimsFromContext(r.Context())
	c.cacheMu.Lock()
	cached, ok := c.identities[claims.SessionID]
	c.cacheMu.Unlock()
	if ok && time.Now().Before(cached.expires) && cached.identity.Subject == subject {
		return cached.identity, nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sess, err := c.sessions.Get(ctx, claims.SessionID)
	if err != nil {
		return HostedIdentity{}, err
	}
	if sess.Status != "active" || "clerk:"+sess.UserID != subject || sess.CreatedAt <= 0 {
		return HostedIdentity{}, errors.New("Clerk session is unavailable")
	}
	u, err := c.users.Get(ctx, sess.UserID)
	if err != nil {
		return HostedIdentity{}, err
	}
	if u.Banned || u.Locked {
		return HostedIdentity{}, errors.New("Clerk account is unavailable")
	}
	// Provider-side enforcement, not CSS hiding: users must erase their library
	// through Exhibit so cleanup and the last-administrator guard cannot be skipped.
	if u.DeleteSelfEnabled {
		u, err = c.users.Update(ctx, u.ID, &user.UpdateParams{DeleteSelfEnabled: clerk.Bool(false)})
		if err != nil {
			return HostedIdentity{}, err
		}
		if u.DeleteSelfEnabled {
			return HostedIdentity{}, errors.New("Clerk self-deletion must be disabled")
		}
	}
	identity := HostedIdentity{Subject: subject, Email: primaryVerifiedEmail(u), SessionCreatedAt: sess.CreatedAt}
	c.cacheMu.Lock()
	if len(c.identities) >= 1024 {
		clear(c.identities)
	}
	c.identities[claims.SessionID] = cachedHostedIdentity{identity, time.Now().Add(30 * time.Second)}
	c.cacheMu.Unlock()
	return identity, nil
}

func primaryVerifiedEmail(u *clerk.User) string {
	for _, address := range u.EmailAddresses {
		if u.PrimaryEmailAddressID != nil && address.ID == *u.PrimaryEmailAddressID && address.Verification != nil && address.Verification.Status == "verified" {
			return address.EmailAddress
		}
	}
	return ""
}

// ResolveEmail consults current verified primary addresses at grant time rather
// than trusting a historical display address in Exhibit's users table.
func (c *Clerk) ResolveEmail(ctx context.Context, email string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	users, err := c.users.List(ctx, &user.ListParams{EmailAddresses: []string{email}, ListParams: clerk.ListParams{Limit: clerk.Int64(2)}})
	if err != nil {
		return "", err
	}
	if users.TotalCount != 1 || len(users.Users) != 1 {
		return "", nil
	}
	u := users.Users[0]
	if u.Banned || u.Locked || !strings.EqualFold(primaryVerifiedEmail(u), email) {
		return "", nil
	}
	return "clerk:" + u.ID, nil
}

// RevokeSessions ends provider sessions and returns their latest creation time.
// Include inactive sessions too: a recently ended session can still have a
// cached JWT. The returned provider timestamp keeps erasure safe across clock skew.
func (c *Clerk) RevokeSessions(ctx context.Context, externalID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	id := strings.TrimPrefix(externalID, "clerk:")
	var cutoff, offset int64
	for {
		list, err := c.sessions.List(ctx, &session.ListParams{UserID: &id, ListParams: clerk.ListParams{Limit: clerk.Int64(100), Offset: &offset}})
		if err != nil {
			return 0, err
		}
		if len(list.Sessions) == 0 {
			break
		}
		for _, sess := range list.Sessions {
			cutoff = max(cutoff, sess.CreatedAt)
			if sess.Status == "active" {
				if _, err := c.sessions.Revoke(ctx, &session.RevokeParams{ID: sess.ID}); err != nil {
					return 0, err
				}
			}
		}
		offset += int64(len(list.Sessions))
		if offset >= list.TotalCount {
			break
		}
	}
	c.cacheMu.Lock()
	for sid, cached := range c.identities {
		if cached.identity.Subject == externalID {
			delete(c.identities, sid)
		}
	}
	c.cacheMu.Unlock()
	return cutoff, nil
}
