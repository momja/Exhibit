package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

// Clerk verifies hosted browser sessions. Self-hosted OIDC and password logins
// continue to use Exhibit's opaque sessions; Clerk never issues one of those.
type Clerk struct {
	PublishableKey string
	FrontendAPI    string
	middleware     func(http.Handler) http.Handler
	users          *user.Client
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
	c := &Clerk{appOrigin: appOrigin, PublishableKey: publishableKey, FrontendAPI: u.String(), users: user.NewClient(config)}
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

func (c *Clerk) Email(ctx context.Context, externalID string) (string, error) {
	u, err := c.users.Get(ctx, strings.TrimPrefix(externalID, "clerk:"))
	if err != nil {
		return "", err
	}
	if u.Banned || u.Locked {
		return "", errors.New("Clerk account is unavailable")
	}
	for _, address := range u.EmailAddresses {
		if u.PrimaryEmailAddressID != nil && address.ID == *u.PrimaryEmailAddressID {
			return address.EmailAddress, nil
		}
	}
	return "", nil
}
