package connectors

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
)

const (
	VendorInject         = "inject"
	VendorForward        = "forward"
	VendorNeedsReconnect = "needs_reconnect"
	VendorRateLimited    = "rate_limited"
	VendorRefused        = "refused"
	VendorUpstreamError  = "upstream_error"
)

// VendorError never incorporates vendor response bodies or request URLs. The
// same codes reach the relay, trusted UI and operator activity for each engine.
type VendorError struct {
	Engine     string
	Code       string
	RetryAfter string
	HTTPStatus int
}

func (e *VendorError) Error() string {
	switch e.Code {
	case VendorNeedsReconnect:
		return "managed connection needs to be reconnected"
	case VendorRateLimited:
		return "managed connector request limit reached; retry later"
	case VendorRefused:
		return "managed connector refused this request"
	default:
		return "managed connector is temporarily unavailable"
	}
}
func (e *VendorError) Unwrap() error {
	if e.Code == VendorRefused {
		return errors.Join(ErrEgress, ErrInvalid)
	}
	return nil
}
func vendorFailure(engine, code string) *VendorError { return &VendorError{Engine: engine, Code: code} }

// VendorProvider is operator-reviewed configuration. A copy is pinned to each
// flow and saved connection: changing a provider affects new connections only.
type VendorProvider struct {
	Name         string   `json:"name"`
	Origins      []string `json:"origins"`
	Engine       string   `json:"engine,omitempty"`
	AuthConfigID string   `json:"auth_config_id,omitempty"`
	Toolkit      string   `json:"toolkit,omitempty"`
	DeliveryMode string   `json:"delivery_mode,omitempty"`
	DailyLimit   int      `json:"daily_limit,omitempty"`
	CacheMinutes int      `json:"cache_minutes,omitempty"`
}

func (p VendorProvider) defaults(provider string) VendorProvider {
	if p.Engine == "" {
		p.Engine = "nango"
	}
	if p.DeliveryMode == "" {
		p.DeliveryMode = VendorInject
		if p.Engine == "composio" {
			p.DeliveryMode = VendorForward
		}
	}
	if p.Name == "" {
		p.Name = provider
	}
	if p.Toolkit == "" {
		p.Toolkit = provider
	}
	p.Origins = append([]string{}, p.Origins...)
	return p
}

type VendorConfig struct {
	BaseURL         string // Nango: retained for existing operator configuration.
	SecretKey       string
	ComposioBaseURL string
	ComposioAPIKey  string
	Providers       map[string]VendorProvider
	IdleDays        int
	UserNamespace   string
}

func (v VendorConfig) configured() bool {
	for id, raw := range v.Providers {
		p := raw.defaults(id)
		if v.engineConfigured(p.Engine) {
			return true
		}
	}
	return false
}
func (v VendorConfig) engineConfigured(engine string) bool {
	switch engine {
	case "nango":
		return v.SecretKey != ""
	case "composio":
		return v.ComposioAPIKey != ""
	}
	return false
}
func (v VendorConfig) base() string { return v.engineBase("nango") }
func (v VendorConfig) engineBase(engine string) string {
	switch engine {
	case "nango":
		if v.BaseURL != "" {
			return strings.TrimSuffix(v.BaseURL, "/")
		}
		return "https://api.nango.dev"
	case "composio":
		if v.ComposioBaseURL != "" {
			return strings.TrimSuffix(v.ComposioBaseURL, "/")
		}
		return "https://backend.composio.dev"
	}
	return ""
}
func (v VendorConfig) ownerTag(ownerID int64) string {
	return v.UserNamespace + ":exhibit:" + strconv.FormatInt(ownerID, 10)
}

type VendorSession struct {
	ID           string `json:"session_id"`
	URL          string `json:"url"`
	ExpiresAt    string `json:"expires_at"`
	Provider     string `json:"provider"`
	Engine       string `json:"engine"`
	DeliveryMode string `json:"delivery_mode"`
	ExternalRef  string `json:"-"`
}

type VendorFlowRequest struct {
	ID          string
	OwnerTag    string
	Provider    string
	Spec        VendorProvider
	ExternalRef string
}
type VendorConnectionSpec struct {
	OwnerTag    string
	Provider    string
	ExternalRef string
	Spec        VendorProvider
}

type vendorFlow struct {
	ownerID  int64
	provider string
	binding  [32]byte
	expires  time.Time
	resultID string
	request  VendorFlowRequest
}

type VendorProviderInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Origins      []string `json:"origins"`
	Engine       string   `json:"engine"`
	DeliveryMode string   `json:"delivery_mode"`
}

func (s *Service) VendorProviders() []VendorProviderInfo {
	out := []VendorProviderInfo{}
	if !s.cfg.VendorEnabled {
		return out
	}
	for id, raw := range s.cfg.Vendor.Providers {
		p := raw.defaults(id)
		if !s.cfg.Vendor.engineConfigured(p.Engine) {
			continue
		}
		out = append(out, VendorProviderInfo{ID: id, Name: p.Name, Origins: p.Origins, Engine: p.Engine, DeliveryMode: p.DeliveryMode})
	}
	slices.SortFunc(out, func(a, b VendorProviderInfo) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (s *Service) vendorEngine(id string) (VendorEngine, error) {
	if !s.cfg.VendorEnabled || !s.cfg.Vendor.engineConfigured(id) {
		return nil, ErrDisabled
	}
	switch id {
	case "nango":
		return nangoEngine{s: s}, nil
	case "composio":
		return composioEngine{s: s}, nil
	}
	return nil, ErrInvalid
}
func (s *Service) BeginVendor(ctx context.Context, ownerID int64, provider string, limits Limits) (*VendorSession, error) {
	return s.BeginVendorBound(ctx, ownerID, provider, limits, "direct-service")
}
func (s *Service) BeginVendorBound(ctx context.Context, ownerID int64, provider string, limits Limits, binding string) (*VendorSession, error) {
	if !s.cfg.VendorEnabled {
		return nil, ErrDisabled
	}
	raw, ok := s.cfg.Vendor.Providers[provider]
	if !ok || ownerID <= 0 || binding == "" {
		return nil, ErrInvalid
	}
	spec := raw.defaults(provider)
	engine, err := s.vendorEngine(spec.Engine)
	if err != nil {
		return nil, err
	}
	if spec.DeliveryMode != engine.DeliveryMode() {
		return nil, ErrInvalid
	}
	list, err := s.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, c := range list {
		if c.Kind == "vendor" && (c.Status == "active" || c.Status == VendorNeedsReconnect || c.Status == "reconnect") {
			count++
		}
	}
	if limits.Vendor >= 0 && count >= limits.Vendor {
		return nil, store.ErrConnectionLimit
	}
	id := randomToken()
	request := VendorFlowRequest{ID: id, OwnerTag: s.cfg.Vendor.ownerTag(ownerID), Provider: provider, Spec: spec}
	flow := vendorFlow{ownerID: ownerID, provider: provider, binding: sha256.Sum256([]byte(binding)), expires: s.cfg.Now().Add(30 * time.Minute), request: request}
	s.mu.Lock()
	for oldID, f := range s.vendorFlows {
		if !f.expires.After(s.cfg.Now()) {
			delete(s.vendorFlows, oldID)
			s.locks.Delete("vendor-flow:" + oldID)
		}
	}
	if len(s.vendorFlows) >= 512 {
		s.mu.Unlock()
		return nil, errors.New("too many pending managed connections; try again later")
	}
	s.vendorFlows[id] = flow
	s.mu.Unlock()
	result, err := engine.Start(ctx, request)
	if err != nil {
		s.mu.Lock()
		delete(s.vendorFlows, id)
		s.mu.Unlock()
		return nil, err
	}
	flow.request.ExternalRef = result.ExternalRef
	s.mu.Lock()
	s.vendorFlows[id] = flow
	s.mu.Unlock()
	result.ID = id
	result.Provider = provider
	result.Engine = engine.ID()
	result.DeliveryMode = engine.DeliveryMode()
	return result, nil
}

// CompleteVendor polls only the server-created engine flow. The caller cannot
// supply a remote connection identity, switch its engine, or alter its policy.
func (s *Service) CompleteVendor(ctx context.Context, ownerID int64, binding, id, name string, limits Limits) (*Connection, bool, error) {
	if !s.cfg.VendorEnabled {
		return nil, false, ErrDisabled
	}
	s.mu.Lock()
	_, exists := s.vendorFlows[id]
	s.mu.Unlock()
	if !exists {
		return nil, false, ErrInvalid
	}
	lock, _ := s.locks.LoadOrStore("vendor-flow:"+id, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	s.mu.Lock()
	flow, ok := s.vendorFlows[id]
	s.mu.Unlock()
	hash := sha256.Sum256([]byte(binding))
	if !ok || flow.ownerID != ownerID || !flow.expires.After(s.cfg.Now()) || subtle.ConstantTimeCompare(hash[:], flow.binding[:]) != 1 {
		return nil, false, fmt.Errorf("%w: managed OAuth session expired or did not match this browser", ErrInvalid)
	}
	if flow.resultID != "" {
		record, err := s.cfg.Store.GetConnection(ctx, ownerID, flow.resultID)
		if err != nil {
			return nil, false, err
		}
		c, err := s.metadata(ctx, record)
		return c, true, err
	}
	engine, err := s.vendorEngine(flow.request.Spec.Engine)
	if err != nil {
		return nil, false, err
	}
	ref, complete, err := engine.Complete(ctx, flow.request)
	if err != nil || !complete {
		return nil, false, err
	}
	if strings.TrimSpace(name) == "" {
		name = flow.request.Spec.Name
	}
	c, err := s.saveVendor(ctx, ownerID, name, flow.request.Provider, flow.request.OwnerTag, ref, flow.request.Spec, limits)
	if err != nil {
		return nil, false, err
	}
	flow.resultID = c.ID
	s.mu.Lock()
	s.vendorFlows[id] = flow
	s.mu.Unlock()
	return c, true, nil
}

func (s *Service) saveVendor(ctx context.Context, ownerID int64, name, provider, ownerTag, ref string, spec VendorProvider, limits Limits) (*Connection, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || !validVendorReference(ref) {
		return nil, ErrInvalid
	}
	if spec.DailyLimit < 0 || spec.CacheMinutes < 0 || spec.CacheMinutes > 525600 {
		return nil, ErrInvalid
	}
	spec = spec.defaults(provider)
	engine, err := s.vendorEngine(spec.Engine)
	if err != nil {
		return nil, err
	}
	if spec.DeliveryMode != engine.DeliveryMode() || len(spec.Origins) == 0 || len(spec.Origins) > 8 {
		return nil, ErrInvalid
	}
	for i, entry := range spec.Origins {
		normalized, err := normalizeHTTPSOrigin(entry)
		if err != nil {
			return nil, err
		}
		spec.Origins[i] = normalized
	}
	r := recipe{Origins: append([]string{}, spec.Origins...), Header: "Authorization", Scheme: "Bearer ", Sensitivity: "personal", DailyLimit: spec.DailyLimit, CacheSeconds: spec.CacheMinutes * 60, VendorProvider: provider, VendorEngine: spec.Engine, VendorSpec: &spec, VendorOwnerTag: ownerTag}
	return s.save(ctx, ownerID, name, provider, "vendor", r, credential{VendorConnectionID: ref}, limits)
}

func (s *Service) vendorConnectionSpec(ctx context.Context, rec *store.ConnectionRecord, r recipe, c credential) (VendorConnectionSpec, error) {
	provider := r.VendorProvider
	if provider == "" {
		provider = rec.Provider
	}
	spec := VendorProvider{Name: rec.Name, Origins: append([]string{}, r.Origins...), Engine: rec.Engine}
	if r.VendorSpec != nil {
		spec = *r.VendorSpec
		spec.Origins = append([]string{}, r.VendorSpec.Origins...)
	}
	if rec.Engine != "" {
		spec.Engine = rec.Engine
	} else if r.VendorEngine != "" {
		spec.Engine = r.VendorEngine
	}
	spec = spec.defaults(provider)
	ref := rec.ExternalRef
	if ref == "" {
		ref = c.VendorConnectionID
		if ref != "" {
			if err := s.cfg.Store.UpdateVendorConnectionRef(ctx, rec.ID, spec.Engine, ref); err != nil {
				return VendorConnectionSpec{}, err
			}
		}
	}
	tag := r.VendorOwnerTag
	if tag == "" {
		tag = s.cfg.Vendor.ownerTag(rec.OwnerID)
	}
	if ref == "" {
		return VendorConnectionSpec{}, vendorFailure(spec.Engine, VendorNeedsReconnect)
	}
	return VendorConnectionSpec{OwnerTag: tag, Provider: provider, ExternalRef: ref, Spec: spec}, nil
}

// VendorStatus is an explicit health check; listing metadata does not spend a
// vendor API call. Changing configured engines recommends reconnecting while
// the pinned engine remains usable until the user does so.
func (s *Service) VendorStatus(ctx context.Context, ownerID int64, id string) (string, error) {
	rec, err := s.cfg.Store.GetConnection(ctx, ownerID, id)
	if err != nil {
		return "", err
	}
	if rec.Kind != "vendor" {
		return "", ErrInvalid
	}
	r, c, err := s.readCredential(rec)
	if err != nil {
		return "", err
	}
	spec, err := s.vendorConnectionSpec(ctx, rec, r, c)
	if err != nil {
		return "", err
	}
	engine, err := s.vendorEngine(spec.Spec.Engine)
	if err != nil {
		return "", err
	}
	status, statusErr := engine.Status(ctx, spec)
	var failure *VendorError
	needsReconnect := status == VendorNeedsReconnect || status == "revoked" || errors.As(statusErr, &failure) && failure.Code == VendorNeedsReconnect
	if needsReconnect {
		if err := s.cfg.Store.MarkVendorConnectionReconnect(ctx, rec.OwnerID, rec.ID); err != nil {
			return "", err
		}
	}
	return status, statusErr
}

func (s *Service) vendorAPIRequest(ctx context.Context, engineID, provider, method, path string, body []byte, operation string) (*egress.Response, error) {
	if !s.cfg.VendorEnabled || !s.cfg.Vendor.engineConfigured(engineID) {
		return nil, ErrDisabled
	}
	baseString := s.cfg.Vendor.engineBase(engineID)
	base, err := url.Parse(baseString)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, vendorFailure(engineID, VendorRefused)
	}
	expected := requestOrigin(base)
	gate, err := s.cfg.GateFactory([]string{expected})
	if err != nil {
		return nil, err
	}
	defer gate.Close()
	req, err := http.NewRequestWithContext(ctx, method, baseString+path, bytes.NewReader(body))
	if err != nil {
		return nil, vendorFailure(engineID, VendorRefused)
	}
	response, err := gate.DoAuthorized(ctx, req, func(hop *http.Request) error {
		if requestOrigin(hop.URL) != expected {
			return ErrEgress
		}
		if err := s.cfg.Store.RecordVendorCall(ctx, engineID, provider, operation, s.cfg.Now()); err != nil {
			return err
		}
		if engineID == "nango" {
			hop.Header.Set("Authorization", "Bearer "+s.cfg.Vendor.SecretKey)
		} else {
			hop.Header.Set("x-api-key", s.cfg.Vendor.ComposioAPIKey)
		}
		hop.Header.Set("Content-Type", "application/json")
		hop.Header.Set("Accept", "application/json")
		return nil
	})
	if err != nil {
		var known *VendorError
		if errors.As(err, &known) {
			return nil, err
		}
		return nil, vendorFailure(engineID, VendorUpstreamError)
	}
	if response.Status < 200 || response.Status >= 300 {
		if method == http.MethodDelete && response.Status == http.StatusNotFound {
			return response, nil
		}
		return nil, vendorHTTPError(engineID, response.Status, response.Header)
	}
	return response, nil
}
func vendorHTTPError(engine string, status int, header http.Header) *VendorError {
	code := VendorUpstreamError
	switch {
	case status == 401:
		code = VendorNeedsReconnect
	case status == 429:
		code = VendorRateLimited
	case status == 403 || status == 400 || status == 404 || status == 409 || status == 422:
		code = VendorRefused
	}
	return &VendorError{Engine: engine, Code: code, RetryAfter: header.Get("Retry-After"), HTTPStatus: status}
}

func (s *Service) deleteVendor(ctx context.Context, rec *store.ConnectionRecord) error {
	r, c, err := s.readCredential(rec)
	if err != nil {
		return err
	}
	spec, err := s.vendorConnectionSpec(ctx, rec, r, c)
	if err != nil {
		return err
	}
	engine, err := s.vendorEngine(spec.Spec.Engine)
	if err != nil {
		return err
	}
	revokeErr := engine.Revoke(ctx, spec)
	var failure *VendorError
	if errors.As(revokeErr, &failure) && failure.Code == VendorNeedsReconnect {
		if err := s.cfg.Store.MarkVendorConnectionReconnect(ctx, rec.OwnerID, rec.ID); err != nil {
			return err
		}
	}
	return revokeErr
}

// PruneIdle retains local records on failed remote deletion so cost leakage
// remains visible and retryable. Revocation always uses the saved engine.
func (s *Service) PruneIdle(ctx context.Context) (int, error) {
	if !s.cfg.VendorEnabled {
		return 0, nil
	}
	days := s.cfg.Vendor.IdleDays
	if days <= 0 {
		days = 30
	}
	idle, err := s.cfg.Store.ListIdleVendorConnections(ctx, s.cfg.Now().Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		return 0, err
	}
	count := 0
	var joined error
	for _, rec := range idle {
		if err = s.Revoke(ctx, rec.OwnerID, rec.ID); err != nil {
			joined = errors.Join(joined, fmt.Errorf("prune connection %s: %w", rec.ID, err))
			continue
		}
		count++
	}
	return count, joined
}

func validVendorReference(ref string) bool {
	if ref == "" || len(ref) > 256 || ref == "." || ref == ".." {
		return false
	}
	for _, c := range ref {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_-.:@", c)) {
			return false
		}
	}
	return true
}

func (s *Service) vendorOperatorSecrets() []string {
	out := []string{}
	for _, secret := range []string{s.cfg.Vendor.SecretKey, s.cfg.Vendor.ComposioAPIKey} {
		if secret != "" && !slices.Contains(out, secret) {
			out = append(out, secret)
		}
	}
	return out
}
