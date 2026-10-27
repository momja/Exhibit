package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
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
// guard, and with its return bound to the browser that started it.
const (
	handshakeNonceParam = "__clerk_handshake_nonce"
	clientUATCookie     = "__client_uat"
	devBrowserCookie    = "__clerk_db_jwt"
	// The Backend API version whose handshake payload this file reads.
	handshakeAPIVersion = "2026-05-12"
	// A browser that keeps returning without a usable token is sent to the
	// login page instead, the same limit Clerk's SDK uses.
	maxHandshakeRedirects = 3
	// Long enough for a slow network's round trip through the Frontend API.
	handshakeCookieSeconds = 60
)

// The Frontend API may answer with the earlier form instead of a nonce: the
// directives themselves, in a JWT signed with the instance's session key.
// Development instances do, whatever format was asked for.
const handshakeTokenParam = "__clerk_handshake"

// A handshake's return is bound to the browser that started it, the way an
// OAuth state is. Starting one puts a random value in an HttpOnly cookie and
// in the URL Clerk sends the browser back to; only a return carrying both,
// matching, is completed, and completing one spends the value. Without that,
// a link carrying a nonce somebody else obtained would sign this browser in
// as them.
//
// Over HTTPS the cookie takes the __Host- prefix, which a sibling domain cannot
// set, so the value cannot be planted from one either. Plain HTTP (a local
// development setup) has no such prefix.
const handshakeStateParam = "__exhibit_hs"

// The count of handshakes started in a row is a cookie of its own, outliving
// each state, so a browser whose renewed cookies still do not verify reaches
// the login page after three tries rather than going round forever.
const handshakeCountCookie = "exhibit_clerk_handshakes"

// Session tokens and handshake tokens share a signing key, so a token's
// category is what says it is not, say, a machine token (@clerk/backend's
// isNonSessionJwtCategory).
var handshakeTokenCategories = map[string]bool{"": true, "cl_B7d4PD111AAA": true, "cl_I7d4PD111III": true}

// handshakeURLParams are what the handshake appends to the page's URL on the
// way back; none belong in the address bar afterward.
var handshakeURLParams = []string{handshakeStateParam, handshakeNonceParam, handshakeTokenParam, "__clerk_help", devBrowserCookie}

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
	count := handshakeCount(r)
	if count >= maxHandshakeRedirects {
		return false
	}
	var devBrowser string
	if c.development {
		// A development instance knows its browser by this token rather than
		// a cookie on its own domain.
		db, err := r.Cookie(devBrowserCookie)
		if err != nil || db.Value == "" {
			return false
		}
		devBrowser = db.Value
	}
	state := make([]byte, 32)
	if _, err := rand.Read(state); err != nil {
		return false
	}
	stateValue := base64.RawURLEncoding.EncodeToString(state)
	back := *r.URL
	bq := back.Query()
	bq.Set(handshakeStateParam, stateValue)
	back.RawQuery = bq.Encode()

	q := url.Values{}
	q.Set("redirect_url", c.appOrigin+back.RequestURI())
	q.Set("__clerk_api_version", handshakeAPIVersion)
	q.Set("suffixed_cookies", "false")
	q.Set("__clerk_hs_reason", "session-token-expired")
	q.Set("format", "nonce")
	if devBrowser != "" {
		q.Set(devBrowserCookie, devBrowser)
	}
	c.setHandshakeCookie(w, c.stateCookieName(), stateValue, handshakeCookieSeconds)
	c.setHandshakeCookie(w, handshakeCountCookie, strconv.Itoa(count+1), handshakeCookieSeconds)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, c.FrontendAPI+"/v1/client/handshake?"+q.Encode(), http.StatusTemporaryRedirect)
	return true
}

// completeHandshake answers a browser returning from the handshake it started:
// it sets the cookies Clerk sent back and sends the browser on to the page it
// asked for, without the handshake's parameters. It reports whether the
// request was such a return.
func (c *Clerk) completeHandshake(w http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	returned := query.Get(handshakeStateParam)
	if r.Method != http.MethodGet || returned == "" {
		return false
	}
	// Spent whatever happens next, so a return is completed at most once.
	c.setHandshakeCookie(w, c.stateCookieName(), "", -1)
	w.Header().Set("Cache-Control", "no-store")

	var directives []string
	err := errors.New("the handshake was not started by this browser")
	if started, cerr := r.Cookie(c.stateCookieName()); cerr == nil && subtle.ConstantTimeCompare([]byte(started.Value), []byte(returned)) == 1 {
		directives, err = c.handshakeDirectives(r)
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
		c.setHandshakeCookie(w, handshakeCountCookie, strconv.Itoa(maxHandshakeRedirects), handshakeCookieSeconds)
	}
	for _, d := range directives {
		w.Header().Add("Set-Cookie", d)
	}
	clean := *r.URL
	for _, p := range handshakeURLParams {
		query.Del(p)
	}
	clean.RawQuery = query.Encode()
	// Absolute, on the app's own origin: a path like //elsewhere must stay a
	// path on this site rather than become a link to another.
	http.Redirect(w, r, c.appOrigin+clean.RequestURI(), http.StatusFound)
	return true
}

// handshakeDirectives reads what Clerk sent back, from the URL or, as a
// production Frontend API may deliver it, a cookie: a nonce to redeem or a
// signed token to verify.
func (c *Clerk) handshakeDirectives(r *http.Request) ([]string, error) {
	value := func(name string) string {
		if v := r.URL.Query().Get(name); v != "" {
			return v
		}
		if cookie, err := r.Cookie(name); err == nil {
			return cookie.Value
		}
		return ""
	}
	if nonce := value(handshakeNonceParam); nonce != "" {
		return c.handshakeCookies(r.Context(), nonce)
	}
	if token := value(handshakeTokenParam); token != "" {
		return c.handshakeTokenCookies(r.Context(), token)
	}
	return nil, errors.New("the handshake returned neither a nonce nor a token")
}

func (c *Clerk) stateCookieName() string {
	if c.secureCookies {
		return "__Host-exhibit_clerk_hs"
	}
	return "exhibit_clerk_hs"
}

func (c *Clerk) setHandshakeCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: c.secureCookies, SameSite: http.SameSiteLaxMode})
}

// handshakeCount is how many handshakes this browser has started in the last
// minute without settling.
func handshakeCount(r *http.Request) int {
	cookie, err := r.Cookie(handshakeCountCookie)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(cookie.Value)
	return max(n, 0)
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

// signingKey is the instance's public key with this id. The set is kept for
// an hour, as @clerk/backend keeps it, so a rotated-out key stops being
// trusted. An id it does not hold refetches at most every five minutes, and
// one fetch at a time, so tokens naming made-up ids cannot turn into a stream
// of Backend API calls.
func (c *Clerk) signingKey(ctx context.Context, kid string) (any, error) {
	c.keysMu.Lock()
	defer c.keysMu.Unlock()
	age := time.Since(c.keysFetched)
	if key, ok := c.keys[kid]; ok && age < time.Hour {
		return key, nil
	}
	if age < 5*time.Minute {
		return nil, errors.New("Clerk handshake token names an unknown signing key")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	set, err := c.jwks.Get(ctx, &jwks.GetParams{})
	c.keysFetched = time.Now()
	if err != nil {
		return nil, err
	}
	c.keys = map[string]any{}
	for _, k := range set.Keys {
		if k != nil {
			c.keys[k.KeyID] = k.Key
		}
	}
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	return nil, errors.New("Clerk handshake token names an unknown signing key")
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
