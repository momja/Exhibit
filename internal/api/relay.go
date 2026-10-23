package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/store"
)

// RelayConfig is opt-in even on private instances. An operator flag enables
// the feature; viewer consent and the artifact's origin decisions enable use.
type RelayConfig struct {
	Enabled              bool
	SchedulesEnabled     bool
	OperatorAllowOrigins []string
	OperatorDenyOrigins  []string
	// A dependency seam for fixture servers; the executable always uses New.
	GateFactory func([]string, egress.Limits) (*egress.Gate, error)
}

func experimentalSwitch(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return v, nil
}

func RelayFromEnv() (RelayConfig, error) {
	var c RelayConfig
	var err error
	if c.Enabled, err = experimentalSwitch("RELAY_ENABLED"); err != nil {
		return c, err
	}
	if c.SchedulesEnabled, err = experimentalSwitch("RELAY_SCHEDULES_ENABLED"); err != nil {
		return c, err
	}
	for _, item := range []struct {
		name string
		dest *[]string
	}{{"RELAY_ALLOW_ORIGINS", &c.OperatorAllowOrigins}, {"RELAY_DENY_ORIGINS", &c.OperatorDenyOrigins}} {
		entries := strings.FieldsFunc(os.Getenv(item.name), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' })
		*item.dest, err = origin.NormalizeOrigins(entries)
		if err != nil {
			return c, fmt.Errorf("%s: %w", item.name, err)
		}
	}
	if c.SchedulesEnabled && !c.Enabled {
		return c, errors.New("RELAY_SCHEDULES_ENABLED requires RELAY_ENABLED")
	}
	return c, nil
}

func (c RelayConfig) Gate(allowed []string, appOrigin string, before ...func(*url.URL) error) (*egress.Gate, error) {
	l := egress.Limits{UserAgent: "Exhibit-Relay/1 (+" + appOrigin + ")", OperatorAllowOrigins: c.OperatorAllowOrigins, OperatorDenyOrigins: c.OperatorDenyOrigins}
	if len(before) > 0 {
		l.BeforeRequest = before[0]
	}
	if c.GateFactory != nil {
		return c.GateFactory(allowed, l)
	}
	return egress.New(allowed, l)
}

// These endpoints spend a person's authority. Agent credentials, anonymous
// visitors and an unauthenticated pass-through instance cannot use a relay.
func (ro *Router) relayHumanRequest(w http.ResponseWriter, r *http.Request) bool {
	p := principalFromCtx(r.Context())
	if p.Kind != PrincipalSession && p.Kind != PrincipalServiceToken {
		writeError(w, 401, "sign in to use connections and the experimental relay")
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != ro.cfg.AppOrigin {
		writeError(w, 403, "use Exhibit to configure connections and relay access")
		return false
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, 403, "cross-site relay requests are refused")
		return false
	}
	return true
}

func (ro *Router) relayStore() (store.RelayStore, bool) {
	s, ok := ro.cfg.Store.(store.RelayStore)
	return s, ok
}

func (ro *Router) relayArtifact(w http.ResponseWriter, r *http.Request) (*store.Artifact, store.RelayStore) {
	if !ro.cfg.Relay.Enabled {
		writeError(w, 404, "experimental relay is disabled")
		return nil, nil
	}
	if !ro.relayHumanRequest(w, r) {
		return nil, nil
	}
	s, ok := ro.relayStore()
	if !ok {
		writeError(w, 503, "relay storage is unavailable")
		return nil, nil
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(ownerIDFromCtx(r.Context())), urlParamID(r, "artifactID"))
	if err != nil {
		serverError(w, r, "relay artifact", err)
		return nil, nil
	}
	if a == nil {
		writeError(w, 404, "artifact not found")
		return nil, nil
	}
	return a, s
}

func (ro *Router) getRelayGrant(w http.ResponseWriter, r *http.Request) {
	a, s := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	g, err := s.GetRelayGrant(r.Context(), ownerIDFromCtx(r.Context()), a.ID)
	if err != nil {
		serverError(w, r, "relay grant", err)
		return
	}
	writeJSON(w, 200, g)
}

func (ro *Router) setRelayGrant(w http.ResponseWriter, r *http.Request) {
	a, s := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	var g store.RelayGrant
	if !decodeJSON(w, r, &g) {
		return
	}
	if g.Writes && !g.Reads {
		writeError(w, 400, "enable reads before allowing writes")
		return
	}
	if err := s.SetRelayGrant(r.Context(), ownerIDFromCtx(r.Context()), a.ID, g); err != nil {
		serverError(w, r, "save relay grant", err)
		return
	}
	writeJSON(w, 200, g)
}

type relayRequest struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64"`
	Cache      string            `json:"cache,omitempty"`
}

type relayResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64"`
	URL        string            `json:"url"`
}

type relayRefusal struct {
	status                int
	code, message, origin string
}

func (e *relayRefusal) Error() string { return e.message }

func relayWrite(method string) bool { return method != "GET" && method != "HEAD" }

func (ro *Router) performRelay(w http.ResponseWriter, r *http.Request) {
	a, _ := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	if !ro.requireRelayPolicy(w, r, a) {
		return
	}
	var input relayRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.BodyBase64) > base64.StdEncoding.EncodedLen(1<<20) {
		writeError(w, 413, "relay request exceeds 1 MiB")
		return
	}
	body, err := base64.StdEncoding.DecodeString(input.BodyBase64)
	if err != nil {
		writeError(w, 400, "invalid relay body encoding")
		return
	}
	method := strings.ToUpper(input.Method)
	if method == "" {
		method = "GET"
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		writeError(w, 400, "unsupported relay method")
		return
	}
	if (method == "GET" || method == "HEAD") && len(body) > 0 {
		writeError(w, 400, "GET and HEAD requests cannot carry a body")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), method, input.URL, bytes.NewReader(body))
	if err != nil {
		writeError(w, 400, "invalid relay URL")
		return
	}
	if len(input.Headers) > 64 {
		writeError(w, 400, "too many relay headers")
		return
	}
	for k, v := range input.Headers {
		if len(k)+len(v) > 8192 {
			writeError(w, 400, "relay header is too large")
			return
		}
		req.Header.Set(k, v)
	}
	if input.Cache == "no-store" || input.Cache == "no-cache" || input.Cache == "reload" {
		req.Header.Set("Cache-Control", "no-cache")
	}
	response, err := ro.executeRelay(r.Context(), ownerIDFromCtx(r.Context()), a, req)
	if err != nil {
		var refusal *relayRefusal
		status, code, message, denied := http.StatusBadGateway, "relay_failed", "The API request failed. Try again later.", ""
		if errors.As(err, &refusal) {
			status, code, message, denied = refusal.status, refusal.code, refusal.message, refusal.origin
		} else {
			var gate *egress.DeniedError
			if errors.As(err, &gate) {
				status, code, message, denied = 403, "relay_denied", gate.Reason, gate.Origin
			}
			if errors.Is(err, store.ErrConnectionQuota) {
				status, code, message = 429, "connection_quota", "This connection has reached its daily request limit."
			}
			if errors.Is(err, connectors.ErrEgress) {
				status, code, message = 403, "connection_egress", "The connection grant does not permit this data destination."
			}
			if errors.Is(err, connectors.ErrStandin) {
				status, code, message = 403, "credential_misuse", "A credential stand-in was used outside its approved origin or position. The request was blocked."
			}
			if vendorCode := managedErrorCode(err); vendorCode != "" {
				status, code, message, denied = connectors.ErrorStatus(err), vendorCode, vendorErrorMessage(vendorCode), ""
			}
		}
		writeJSON(w, status, map[string]any{"error": message, "code": code, "origin": denied})
		return
	}
	headers := map[string]string{}
	for k, v := range response.Header {
		headers[k] = strings.Join(v, ", ")
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, relayResponse{Status: response.Status, StatusText: response.StatusText, Headers: headers, BodyBase64: base64.StdEncoding.EncodeToString(response.Body), URL: response.URL})
}

// All callers, including schedules, pass through the same consent, rate,
// egress and credential checks. Response bodies remain bytes until the opaque
// frame constructs a Response, or a schedule stores the data as a string.
func (ro *Router) executeRelay(ctx context.Context, viewer int64, a *store.Artifact, req *http.Request) (response *egress.Response, err error) {
	s, ok := ro.relayStore()
	if !ok || !ro.cfg.Relay.Enabled {
		return nil, &relayRefusal{503, "relay_disabled", "Experimental relay is disabled.", ""}
	}
	g, err := s.GetRelayGrant(ctx, viewer, a.ID)
	if err != nil {
		return nil, err
	}
	if !g.Reads || relayWrite(req.Method) && !g.Writes {
		return nil, &relayRefusal{403, "relay_consent_required", "This artifact needs your permission to use the relay.", ""}
	}
	if ro.cfg.Connections == nil && ro.cfg.ConnectionPolicy != nil {
		policies, policyErr := ro.cfg.ConnectionPolicy.DataPolicies(ctx, viewer, a.ID)
		if policyErr != nil {
			return nil, policyErr
		}
		if len(policies) > 0 {
			return nil, &relayRefusal{403, "connections_disabled", "Connections are disabled for this instance. Ask the operator to re-enable them before using this connected artifact.", ""}
		}
	}
	o, err := origin.NormalizeOrigin(req.URL.Scheme + "://" + req.URL.Host)
	if err != nil {
		return nil, &relayRefusal{400, "invalid_url", "Use an HTTPS API URL.", ""}
	}
	if !ro.relayRuntime.allow(viewer, a.ID, o) {
		return nil, &relayRefusal{429, "relay_rate_limited", "Relay request limit reached. Wait a minute before trying again.", ""}
	}
	allowed, err := ro.connectionOrigins(ctx, viewer, a.ID, a.NetworkAllowlist)
	if err != nil {
		return nil, err
	}
	firstHop := true
	gate, err := ro.cfg.Relay.Gate(allowed, ro.cfg.AppOrigin, func(target *url.URL) error {
		if firstHop {
			firstHop = false
			return nil
		}
		if !ro.relayRuntime.allow(viewer, a.ID, relayOrigin(target.String())) {
			return &relayRefusal{429, "relay_rate_limited", "The API is backing off or the relay request limit has been reached.", ""}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer gate.Close()
	started := time.Now()
	cached := false
	defer func() {
		status, size := 0, 0
		if response != nil {
			status, size = response.Status, len(response.Body)
		}
		duration := time.Since(started).Milliseconds()
		code := ""
		if errors.Is(err, connectors.ErrStandin) {
			code = "credential_misuse_blocked"
		}
		if vendorCode := managedErrorCode(err); vendorCode != "" {
			code, status = vendorCode, connectors.ErrorStatus(err)
		}
		_ = s.RecordRelayActivity(ctx, viewer, a.ID, store.RelayActivity{Origin: o, Method: req.Method, Status: status, Bytes: size, DurationMS: duration, Cached: cached, ErrorCode: code})
		slog.InfoContext(ctx, "artifact relay", slog.Int64("viewer_id", viewer), slog.String("artifact_id", a.ID), slog.String("origin", o), slog.String("method", req.Method), slog.Int("status", status), slog.Int("bytes", size), slog.Int64("duration_ms", duration))
	}()
	canCache := req.Method == "GET" && req.Header.Get("Authorization") == "" && req.Header.Get("Range") == "" && req.Header.Get("Cache-Control") == "" && gate.Allows(req.URL)
	if canCache && ro.cfg.Connections != nil {
		grants, readErr := ro.cfg.Connections.Grants(ctx, viewer, a.ID)
		if readErr != nil {
			return nil, readErr
		}
		canCache = len(grants) == 0
	}
	var cacheKey [32]byte
	if canCache {
		policy, policyErr := ro.relayPolicy(ctx, viewer, a)
		if policyErr != nil {
			return nil, policyErr
		}
		digest := requestDigest(req)
		cacheKey = sha256.Sum256(append([]byte(policy), digest[:]...))
		if hit := ro.relayRuntime.cached(cacheKey); hit != nil {
			cached = true
			return hit, nil
		}
	}
	if ro.cfg.Connections != nil {
		response, err = ro.cfg.Connections.Do(ctx, viewer, a.ID, req, gate)
	} else {
		response, err = gate.Do(ctx, req)
	}
	if response != nil && response.Status == 429 {
		ro.relayRuntime.backoff(o, response.Header.Get("Retry-After"))
		if target := relayOrigin(response.URL); target != "" && target != o {
			ro.relayRuntime.backoff(target, response.Header.Get("Retry-After"))
		}
	}
	var vendorError *connectors.VendorError
	if errors.As(err, &vendorError) && vendorError.Code == connectors.VendorRateLimited {
		ro.relayRuntime.backoff(o, vendorError.RetryAfter)
	}
	if err == nil && canCache && response != nil {
		ro.relayRuntime.cacheResponse(cacheKey, response)
	}
	return response, err
}

type relayBucket struct {
	tokens  float64
	updated time.Time
}
type relayRuntime struct {
	mu        sync.Mutex
	buckets   map[string]relayBucket
	blocked   map[string]time.Time
	policyKey [32]byte
	cache     map[[32]byte]relayCacheEntry
}

func newRelayRuntime() *relayRuntime {
	rt := &relayRuntime{buckets: map[string]relayBucket{}, blocked: map[string]time.Time{}, cache: map[[32]byte]relayCacheEntry{}}
	if _, err := rand.Read(rt.policyKey[:]); err != nil {
		panic("cannot generate relay policy key")
	}
	return rt
}

// The app owns this token. A frame never supplies or refreshes it. A grant
// change invalidates older host pages before a narrower CSP can be bypassed by
// an already-open document. Source changes also require a new host render.
func (ro *Router) relayPolicy(ctx context.Context, viewer int64, a *store.Artifact) (string, error) {
	allowed, err := ro.connectionOrigins(ctx, viewer, a.ID, a.NetworkAllowlist)
	if err != nil {
		return "", err
	}
	allowed = append([]string{}, allowed...)
	slices.Sort(allowed)
	type binding struct {
		Connection   string
		Destinations []string
		Sharing      bool
	}
	bindings := []binding{}
	policy := ro.cfg.Connections
	if policy == nil {
		policy = ro.cfg.ConnectionPolicy
	}
	if policy != nil {
		grants, err := policy.Grants(ctx, viewer, a.ID)
		if err != nil {
			return "", err
		}
		for _, g := range grants {
			dest := append([]string{}, g.PermittedEgress...)
			slices.Sort(dest)
			bindings = append(bindings, binding{g.Connection.ID, dest, g.ShareWithRecipients})
		}
		retained, err := policy.DataPolicies(ctx, viewer, a.ID)
		if err != nil {
			return "", err
		}
		// A recipient records custody when receiving an owner's shared data.
		// That copy adds no authority and must not invalidate the very response
		// it protects. Fingerprint the intersection for each connection, so
		// duplicate custody rows and later narrowing copies are equivalent to
		// the same effective restriction. Sharing affects row eligibility in
		// DataPolicies, rather than the authority of an already selected row.
		dataDestinations := map[string][]string{}
		for _, p := range retained {
			if current, exists := dataDestinations[p.ConnectionID]; exists {
				dataDestinations[p.ConnectionID] = slices.DeleteFunc(current, func(o string) bool { return !slices.Contains(p.Origins, o) })
			} else {
				dataDestinations[p.ConnectionID] = append([]string{}, p.Origins...)
			}
		}
		for id, dest := range dataDestinations {
			slices.Sort(dest)
			bindings = append(bindings, binding{"data:" + id, slices.Compact(dest), false})
		}
	}
	slices.SortFunc(bindings, func(a, b binding) int { return strings.Compare(a.Connection, b.Connection) })
	raw, _ := json.Marshal(struct {
		Viewer           int64
		Artifact, Source string
		Allowed          []string
		Bindings         []binding
	}{viewer, a.ID, a.SourceBlobID, allowed, bindings})
	mac := hmac.New(sha256.New, ro.relayRuntime.policyKey[:])
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (ro *Router) requireRelayPolicy(w http.ResponseWriter, r *http.Request, a *store.Artifact) bool {
	policy, err := ro.relayPolicy(r.Context(), ownerIDFromCtx(r.Context()), a)
	if err != nil {
		writeError(w, 503, "Connection policy is unavailable.")
		return false
	}
	if !hmac.Equal([]byte(policy), []byte(r.Header.Get("X-Exhibit-Relay-Policy"))) {
		writeJSON(w, 409, map[string]string{"code": "relay_policy_changed", "error": "Artifact access changed. Reopen the artifact before making API requests."})
		return false
	}
	return true
}
func (rt *relayRuntime) allow(viewer int64, artifact, target string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	now := time.Now()
	if until := rt.blocked[target]; until.After(now) {
		return false
	}
	keys := []string{fmt.Sprintf("owner:%d", viewer), fmt.Sprintf("artifact:%d:%s", viewer, artifact), "target:" + target}
	rates := []float64{120, 60, 300}
	// Pruning bounds memory even when an owner continually requests new hosts.
	if len(rt.buckets) > 4096 {
		for key, b := range rt.buckets {
			if now.Sub(b.updated) > 5*time.Minute {
				delete(rt.buckets, key)
			}
		}
		if len(rt.buckets) > 4096 {
			return false
		}
	}
	for i, key := range keys {
		b, ok := rt.buckets[key]
		if !ok {
			b = relayBucket{tokens: rates[i], updated: now}
		}
		b.tokens = min(rates[i], b.tokens+now.Sub(b.updated).Minutes()*rates[i])
		b.updated = now
		rt.buckets[key] = b
		if b.tokens < 1 {
			return false
		}
	}
	for _, key := range keys {
		b := rt.buckets[key]
		b.tokens--
		rt.buckets[key] = b
	}
	return true
}
func (rt *relayRuntime) backoff(target, value string) {
	un := time.Now().Add(time.Minute)
	if n, err := strconv.Atoi(value); err == nil && n > 0 {
		un = time.Now().Add(time.Duration(min(n, 86400)) * time.Second)
	} else if parsed, err := http.ParseTime(value); err == nil && parsed.After(time.Now()) {
		un = minTime(parsed, time.Now().Add(24*time.Hour))
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.blocked) > 4096 {
		for k, v := range rt.blocked {
			if time.Now().After(v) {
				delete(rt.blocked, k)
			}
		}
	}
	rt.blocked[target] = un
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// requestDigest is safe for private cache keys: queries never appear in logs or
// activity metadata. Kept here so every relay cache can use the same identity.
func requestDigest(req *http.Request) [32]byte {
	headers, _ := json.Marshal(req.Header)
	return sha256.Sum256([]byte(req.Method + "\n" + req.URL.String() + "\n" + string(headers)))
}

type relayCacheEntry struct {
	response *egress.Response
	expires  time.Time
}

func copyRelayResponse(response *egress.Response) *egress.Response {
	out := *response
	out.Body = append([]byte{}, response.Body...)
	out.Header = response.Header.Clone()
	return &out
}
func (rt *relayRuntime) cached(key [32]byte) *egress.Response {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	entry, ok := rt.cache[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expires) {
		delete(rt.cache, key)
		return nil
	}
	return copyRelayResponse(entry.response)
}
func (rt *relayRuntime) cacheResponse(key [32]byte, response *egress.Response) {
	if response.Status != 200 || len(response.Body) > 1<<20 || strings.TrimSpace(response.Header.Get("Vary")) == "*" {
		return
	}
	ttl := 0
	for _, directive := range strings.Split(strings.ToLower(response.Header.Get("Cache-Control")), ",") {
		directive = strings.TrimSpace(directive)
		if directive == "no-store" || directive == "no-cache" || directive == "private" {
			return
		}
		if value, ok := strings.CutPrefix(directive, "max-age="); ok {
			ttl, _ = strconv.Atoi(strings.Trim(value, `"`))
		}
	}
	if ttl <= 0 {
		return
	}
	ttl = min(ttl, 300)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.cache) >= 16 {
		var oldest [32]byte
		var expiry time.Time
		for k, v := range rt.cache {
			if expiry.IsZero() || v.expires.Before(expiry) {
				oldest, expiry = k, v.expires
			}
		}
		delete(rt.cache, oldest)
	}
	rt.cache[key] = relayCacheEntry{copyRelayResponse(response), time.Now().Add(time.Duration(ttl) * time.Second)}
}

func relayOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	o, _ := origin.NormalizeOrigin(u.Scheme + "://" + u.Host)
	return o
}
