package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	josejwt "github.com/go-jose/go-jose/v3/jwt"
)

// Clerk's session token lives about a minute, and ClerkJS in an open page is
// what renews it. A page opened after a pause — an installed app relaunched,
// a tab restored — therefore arrives with a lapsed token while the browser is
// still signed in to Clerk. Sending that request to the login page worked,
// but cost a redirect, two large scripts and three more round trips before
// the page the person asked for (Exh-8409).
//
// The handshake is Clerk's server-side answer, and what its own backend SDKs
// do (@clerk/backend, HandshakeService). The browser makes a round trip
// through the Frontend API, which knows its client, and comes back carrying a
// nonce. The server exchanges the nonce for fresh cookies over the Backend
// API and the page loads with no script involved. The Go SDK does not
// implement it, so this file does, with the same eligibility rules and loop
// guard.
const (
	handshakeNonceParam = "__clerk_handshake_nonce"
	clientUATCookie     = "__client_uat"
	devBrowserCookie    = "__clerk_db_jwt"
	// handshakePendingCookie marks a browser this server sent to the
	// handshake, and counts how many times in a row. Only such a browser may
	// redeem a nonce: one arriving on a link somebody else sent would
	// otherwise sign this browser in as whoever obtained it.
	handshakePendingCookie = "exhibit_clerk_handshake"
	// Long enough for a slow network's round trip through the Frontend API.
	handshakePendingSeconds = 60
	// The Backend API version whose handshake payload this file reads.
	handshakeAPIVersion = "2026-05-12"
	// A browser that keeps returning without a usable token is sent to the
	// login page instead, the same limit Clerk's SDK uses.
	maxHandshakeRedirects = 3
)

// The Frontend API may answer with the earlier form instead of a nonce: the
// directives themselves, in a JWT signed with the instance's session key.
// Development instances do, whatever format was asked for.
const handshakeTokenParam = "__clerk_handshake"

// Session tokens and handshake tokens share a signing key, so a token's
// category is what says it is not, say, a machine token (@clerk/backend's
// isNonSessionJwtCategory).
var handshakeTokenCategories = map[string]bool{"": true, "cl_B7d4PD111AAA": true, "cl_I7d4PD111III": true}

// handshakeURLParams are what the handshake appends to the page's URL on the
// way back; none belong in the address bar afterward.
var handshakeURLParams = []string{handshakeNonceParam, handshakeTokenParam, "__clerk_help", devBrowserCookie}

// HandshakeRedirect sends a page request whose session token is missing or
// lapsed to Clerk's handshake, when the browser is still signed in there, and
// reports whether it did. A request it declines is left to the caller, which
// sends it to the login page as before.
func (c *Clerk) HandshakeRedirect(w http.ResponseWriter, r *http.Request) bool {
	if !isDocumentRequest(r) || c.Subject(r) != "" {
		return false
	}
	// __client_uat is when the browser's Clerk client last changed, and "0"
	// once it has signed out. Without it there is no session to renew, and
	// the person needs the login page anyway.
	if uat, err := r.Cookie(clientUATCookie); err != nil || uat.Value == "" || uat.Value == "0" {
		return false
	}
	redirects := pendingHandshakes(r)
	if redirects >= maxHandshakeRedirects {
		return false
	}
	q := url.Values{}
	q.Set("redirect_url", c.appOrigin+r.URL.RequestURI())
	q.Set("__clerk_api_version", handshakeAPIVersion)
	q.Set("suffixed_cookies", "false")
	q.Set("__clerk_hs_reason", "session-token-expired")
	q.Set("format", "nonce")
	if c.development {
		// A development instance knows its browser by this token rather than
		// a cookie on its own domain.
		db, err := r.Cookie(devBrowserCookie)
		if err != nil || db.Value == "" {
			return false
		}
		q.Set(devBrowserCookie, db.Value)
	}
	c.setPendingHandshakes(w, redirects+1)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, c.FrontendAPI+"/v1/client/handshake?"+q.Encode(), http.StatusTemporaryRedirect)
	return true
}

// completeHandshake answers a browser returning from the handshake: it sets
// the cookies the nonce stands for and sends the browser on to the page it
// asked for, without the handshake's parameters. It reports whether the
// request was such a return.
func (c *Clerk) completeHandshake(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || pendingHandshakes(r) == 0 {
		return false
	}
	nonce, token := r.URL.Query().Get(handshakeNonceParam), r.URL.Query().Get(handshakeTokenParam)
	if nonce == "" && token == "" {
		// A production Frontend API may hand the nonce back as a cookie. One
		// left behind must not cost a Backend API call on every request, so
		// it is only spent when the token it would replace is unusable.
		if cookie, err := r.Cookie(handshakeNonceParam); err == nil && isDocumentRequest(r) && !c.hasUsableToken(r) {
			nonce = cookie.Value
		}
	}
	if nonce == "" && token == "" {
		return false
	}
	var directives []string
	var err error
	if nonce != "" {
		directives, err = c.handshakeCookies(r.Context(), nonce)
	} else {
		directives, err = c.handshakeTokenCookies(r.Context(), token)
	}
	if err == nil {
		err = checkDirectives(directives)
	}
	if err != nil {
		// The page continues without a session, and its gate sends it to the
		// login page, which renews the session the slower way. Retrying the
		// handshake would only fail the same way more slowly.
		slog.Warn("Clerk handshake could not be completed", "err", err)
		directives = nil
		c.setPendingHandshakes(w, maxHandshakeRedirects)
	}
	for _, d := range directives {
		w.Header().Add("Set-Cookie", d)
	}
	clean := *r.URL
	q := clean.Query()
	for _, p := range handshakeURLParams {
		q.Del(p)
	}
	clean.RawQuery = q.Encode()
	clean.Scheme, clean.Host = "", ""
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, clean.RequestURI(), http.StatusFound)
	return true
}

// pendingHandshakes is how many handshakes this browser has started in a row
// without coming back with a usable token; zero when it started none.
func pendingHandshakes(r *http.Request) int {
	cookie, err := r.Cookie(handshakePendingCookie)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(cookie.Value)
	return max(n, 0)
}

// setPendingHandshakes records the count; it is never cleared on success,
// because a browser that renews its token stops asking, and the count lapses
// with the cookie.
func (c *Clerk) setPendingHandshakes(w http.ResponseWriter, n int) {
	http.SetCookie(w, &http.Cookie{Name: handshakePendingCookie, Value: strconv.Itoa(n), Path: "/",
		MaxAge: handshakePendingSeconds, HttpOnly: true, Secure: c.secureCookies, SameSite: http.SameSiteLaxMode})
}

// hasUsableToken reports whether the request's session token verified, which
// the middleware has already decided by the time this runs.
func (c *Clerk) hasUsableToken(r *http.Request) bool {
	_, ok := clerk.SessionClaimsFromContext(r.Context())
	return ok
}

type handshakePayload struct {
	clerk.APIResource
	Directives []string `json:"directives"`
}

type nonceParams struct{ nonce string }

func (p nonceParams) ToQuery() url.Values { return url.Values{"nonce": {p.nonce}} }
func (p nonceParams) ToMultipart() ([]byte, string, error) {
	return nil, "", errors.New("not multipart")
}

// handshakeCookies exchanges a handshake nonce for the Set-Cookie directives
// it stands for. The nonce is single-use and only the instance's secret key
// can redeem it, so the directives are Clerk's, not the browser's.
func (c *Clerk) handshakeCookies(ctx context.Context, nonce string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req := clerk.NewAPIRequest(http.MethodGet, "/clients/handshake_payload")
	req.SetParams(nonceParams{nonce})
	payload := &handshakePayload{}
	if err := c.backend.Call(ctx, req, payload); err != nil {
		return nil, err
	}
	return payload.Directives, nil
}

// handshakeTokenCookies reads the directives out of a handshake JWT, which
// the signature makes Clerk's rather than the browser's.
func (c *Clerk) handshakeTokenCookies(ctx context.Context, token string) ([]string, error) {
	parsed, err := josejwt.ParseSigned(token)
	if err != nil {
		return nil, err
	}
	if len(parsed.Headers) != 1 || parsed.Headers[0].Algorithm != "RS256" {
		return nil, errors.New("Clerk handshake token is not RS256")
	}
	header := parsed.Headers[0]
	category, _ := header.ExtraHeaders["cat"].(string)
	if !handshakeTokenCategories[category] {
		return nil, errors.New("Clerk handshake token has the wrong category")
	}
	key, err := c.signingKey(ctx, header.KeyID)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Handshake []string `json:"handshake"`
	}
	if err := parsed.Claims(key, &payload); err != nil {
		return nil, err
	}
	return payload.Handshake, nil
}

// signingKey is the instance's public key with this id. Keys rotate rarely,
// and a handshake is on the path to a page, so they are kept.
func (c *Clerk) signingKey(ctx context.Context, kid string) (any, error) {
	c.keysMu.Lock()
	key, ok := c.keys[kid]
	c.keysMu.Unlock()
	if ok {
		return key, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	jwk, err := clerkjwt.GetJSONWebKey(ctx, &clerkjwt.GetJSONWebKeyParams{KeyID: kid, JWKSClient: c.jwks})
	if err != nil {
		return nil, err
	}
	c.keysMu.Lock()
	c.keys[kid] = jwk.Key
	c.keysMu.Unlock()
	return jwk.Key, nil
}

// checkDirectives refuses a directive that is not one header value. Net/http
// would not split it, but a cookie line that is not what Clerk meant is not
// one to set.
func checkDirectives(directives []string) error {
	for _, d := range directives {
		if strings.ContainsAny(d, "\r\n") {
			return errors.New("Clerk handshake directive is not a header value")
		}
	}
	return nil
}

// isDocumentRequest reports whether the browser is loading a page, the only
// kind of request a redirect through another site can satisfy. The rule is
// Clerk's: Sec-Fetch-Dest where the browser sends it, else an HTML Accept.
func isDocumentRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" {
		return dest == "document"
	}
	return strings.HasPrefix(r.Header.Get("Accept"), "text/html")
}
