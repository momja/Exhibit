package connectors

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
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

type VendorProvider struct {
	Name        string   `json:"name"`
	Origins     []string `json:"origins"`
	AllowWrites bool     `json:"allow_writes"`
}
type VendorConfig struct {
	BaseURL       string
	SecretKey     string
	Providers     map[string]VendorProvider
	IdleDays      int
	UserNamespace string
}

func (v VendorConfig) configured() bool { return v.SecretKey != "" && len(v.Providers) > 0 }
func (v VendorConfig) base() string {
	if v.BaseURL == "" {
		return "https://api.nango.dev"
	}
	return strings.TrimSuffix(v.BaseURL, "/")
}
func (v VendorConfig) ownerTag(ownerID int64) string {
	return v.UserNamespace + ":exhibit:" + strconv.FormatInt(ownerID, 10)
}

type VendorSession struct {
	ID        string `json:"session_id"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
	Provider  string `json:"provider"`
}

type vendorFlow struct {
	ownerID  int64
	provider string
	binding  [32]byte
	expires  time.Time
	resultID string
}

type VendorProviderInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Origins []string `json:"origins"`
}

func (s *Service) VendorProviders() []VendorProviderInfo {
	out := []VendorProviderInfo{}
	if !s.cfg.VendorEnabled {
		return out
	}
	for id, p := range s.cfg.Vendor.Providers {
		out = append(out, VendorProviderInfo{ID: id, Name: p.Name, Origins: p.Origins})
	}
	slices.SortFunc(out, func(a, b VendorProviderInfo) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (s *Service) BeginVendor(ctx context.Context, ownerID int64, provider string, limits Limits) (*VendorSession, error) {
	return s.BeginVendorBound(ctx, ownerID, provider, limits, "direct-service")
}

func (s *Service) BeginVendorBound(ctx context.Context, ownerID int64, provider string, limits Limits, binding string) (*VendorSession, error) {
	if !s.cfg.VendorEnabled || !s.cfg.Vendor.configured() {
		return nil, ErrDisabled
	}
	if _, ok := s.cfg.Vendor.Providers[provider]; !ok {
		return nil, ErrInvalid
	}
	list, err := s.List(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, c := range list {
		if c.Kind == "vendor" && c.Status == "active" {
			n++
		}
	}
	if limits.Vendor >= 0 && n >= limits.Vendor {
		return nil, store.ErrConnectionLimit
	}
	if ownerID <= 0 || binding == "" {
		return nil, ErrInvalid
	}
	flowID := randomToken()
	s.mu.Lock()
	for id, f := range s.vendorFlows {
		if !f.expires.After(s.cfg.Now()) {
			delete(s.vendorFlows, id)
			s.locks.Delete("vendor-flow:" + id)
		}
	}
	full := len(s.vendorFlows) >= 512
	if !full {
		s.vendorFlows[flowID] = vendorFlow{ownerID: ownerID, provider: provider, binding: sha256.Sum256([]byte(binding)), expires: s.cfg.Now().Add(30 * time.Minute)}
	}
	s.mu.Unlock()
	if full {
		return nil, errors.New("too many pending managed connections; try again later")
	}
	keepFlow := false
	defer func() {
		if !keepFlow {
			s.mu.Lock()
			delete(s.vendorFlows, flowID)
			s.mu.Unlock()
		}
	}()
	tag := s.cfg.Vendor.ownerTag(ownerID)
	raw, _ := json.Marshal(map[string]any{"end_user": map[string]string{"id": tag}, "tags": map[string]string{"end_user_id": tag, "exhibit_owner": tag, "exhibit_flow": flowID}, "allowed_integrations": []string{provider}})
	response, err := s.vendorRequest(ctx, http.MethodPost, "/connect/sessions", raw)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data struct {
			Token   string `json:"token"`
			Link    string `json:"connect_link"`
			Expires string `json:"expires_at"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body, &result) != nil || result.Data.Token == "" {
		return nil, errors.New("managed OAuth did not return a connection session")
	}
	link := result.Data.Link
	if link == "" {
		link = "https://connect.nango.dev/?session_token=" + url.QueryEscape(result.Data.Token)
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != "connect.nango.dev" {
		return nil, errors.New("managed OAuth returned an invalid connect destination")
	}
	keepFlow = true
	return &VendorSession{ID: flowID, URL: link, ExpiresAt: result.Data.Expires, Provider: provider}, nil
}

// CompleteVendor uses Nango's supported tag-filtered connections listing. A
// fresh per-flow tag is copied by Nango to the newly authorized connection;
// arbitrary postMessages and caller-supplied vendor connection ids are not
// completion proof. Repeated polling is idempotent after the connection saves.
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
	tag := s.cfg.Vendor.ownerTag(ownerID)
	query := url.Values{"tags[exhibit_flow]": []string{id}, "tags[end_user_id]": []string{tag}}
	response, err := s.vendorRequest(ctx, http.MethodGet, "/connections?"+query.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	var listing struct {
		Connections []vendorConnection `json:"connections"`
	}
	if json.Unmarshal(response.Body, &listing) != nil {
		return nil, false, errors.New("managed OAuth completion could not be read")
	}
	for _, remote := range listing.Connections {
		if remote.Provider != flow.provider || remote.Tags["exhibit_flow"] != id || remote.Tags["end_user_id"] != tag || remote.Tags["exhibit_owner"] != tag || len(remote.Errors) > 0 {
			continue
		}
		verified, err := s.getVendorConnection(ctx, flow.provider, remote.ID)
		if err != nil {
			return nil, false, err
		}
		if verified.Tags["exhibit_flow"] != id || verified.EndUser.ID != tag || verified.Tags["exhibit_owner"] != tag {
			return nil, false, ErrEgress
		}
		if strings.TrimSpace(name) == "" {
			name = s.cfg.Vendor.Providers[flow.provider].Name
		}
		c, err := s.Create(ctx, ownerID, CreateRequest{Name: name, Recipe: "nango", VendorProvider: flow.provider, VendorConnectionID: remote.ID}, limits)
		if err != nil {
			return nil, false, err
		}
		flow.resultID = c.ID
		s.mu.Lock()
		s.vendorFlows[id] = flow
		s.mu.Unlock()
		return c, true, nil
	}
	return nil, false, nil
}

type vendorConnection struct {
	ID       string            `json:"connection_id"`
	Provider string            `json:"provider_config_key"`
	Tags     map[string]string `json:"tags"`
	Errors   []json.RawMessage `json:"errors"`
	EndUser  struct {
		ID string `json:"id"`
	} `json:"end_user"`
	Credentials struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Type         string `json:"type"`
	} `json:"credentials"`
}

func (s *Service) getVendorConnection(ctx context.Context, provider, id string) (*vendorConnection, error) {
	path := "/connections/" + url.PathEscape(id) + "?provider_config_key=" + url.QueryEscape(provider)
	response, err := s.vendorRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var result vendorConnection
	if json.Unmarshal(response.Body, &result) != nil || result.ID != id || result.Provider != provider {
		return nil, errors.New("managed OAuth connection could not be verified")
	}
	return &result, nil
}
func (s *Service) verifyVendorConnection(ctx context.Context, provider, id string, ownerID int64) error {
	c, err := s.getVendorConnection(ctx, provider, id)
	if err != nil {
		return err
	}
	tag := s.cfg.Vendor.ownerTag(ownerID)
	if c.Tags["exhibit_owner"] != tag || c.EndUser.ID != tag {
		return fmt.Errorf("%w: managed connection belongs to another account", ErrInvalid)
	}
	return nil
}

func (s *Service) vendorRequest(ctx context.Context, method, path string, body []byte) (*egress.Response, error) {
	if !s.cfg.VendorEnabled || !s.cfg.Vendor.configured() {
		return nil, ErrDisabled
	}
	base, err := url.Parse(s.cfg.Vendor.base())
	if err != nil || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("invalid managed OAuth configuration")
	}
	expected := requestOrigin(base)
	gate, err := s.cfg.GateFactory([]string{expected})
	if err != nil {
		return nil, err
	}
	defer gate.Close()
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.Vendor.base()+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("managed OAuth request could not be created")
	}
	response, err := gate.DoAuthorized(ctx, req, func(hop *http.Request) error {
		if requestOrigin(hop.URL) != expected {
			return ErrEgress
		}
		hop.Header.Set("Authorization", "Bearer "+s.cfg.Vendor.SecretKey)
		hop.Header.Set("Content-Type", "application/json")
		hop.Header.Set("Accept", "application/json")
		return nil
	})
	if err != nil {
		return nil, errors.New("managed OAuth provider is unavailable")
	}
	if response.Status < 200 || response.Status >= 300 {
		if method == http.MethodDelete && response.Status == 404 {
			return response, nil
		}
		return nil, errors.New("managed OAuth request was refused by the provider")
	}
	return response, nil
}

// vendorEngine retrieves fresh OAuth credentials server-side, then injects them
// through Exhibit's original gate. This deliberately avoids a vendor proxy
// following redirects or reflecting secrets outside Exhibit's visibility.
type vendorEngine struct{ s *Service }

func (e vendorEngine) Authorize(ctx context.Context, req *http.Request, rec *store.ConnectionRecord, r recipe, c *credential) error {
	if !e.s.cfg.VendorEnabled {
		return ErrDisabled
	}
	remote, err := e.s.getVendorConnection(ctx, r.VendorProvider, c.VendorConnectionID)
	if err != nil {
		return err
	}
	tag := e.s.cfg.Vendor.ownerTag(rec.OwnerID)
	if remote.Tags["exhibit_owner"] != tag || remote.EndUser.ID != tag {
		return ErrEgress
	}
	if remote.Credentials.AccessToken == "" {
		return errors.New("managed provider did not return an OAuth 2 access token")
	}
	c.Value = remote.Credentials.AccessToken
	c.Password = remote.Credentials.RefreshToken
	r.Header = "Authorization"
	r.Scheme = "Bearer "
	return (NativeEngine{}).Authorize(ctx, req, rec, r, c)
}
func (s *Service) deleteVendor(ctx context.Context, rec *store.ConnectionRecord) error {
	r, c, err := s.readCredential(rec)
	if err != nil {
		return err
	}
	_, err = s.vendorRequest(ctx, http.MethodDelete, "/connections/"+url.PathEscape(c.VendorConnectionID)+"?provider_config_key="+url.QueryEscape(r.VendorProvider), nil)
	return err
}

// PruneIdle is safe to run repeatedly. Failed vendor deletions retain their
// local record, keeping cost leakage visible and the next attempt retryable.
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
