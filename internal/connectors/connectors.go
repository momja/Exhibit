// Package connectors keeps credential custody and reviewed acquisition and
// injection policy outside both artifact code and model-authored instructions.
package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

var (
	ErrDisabled = errors.New("this experimental connector phase is disabled")
	ErrInvalid  = errors.New("invalid connection configuration")
	ErrEgress   = errors.New("connection grant does not permit this data destination")
	ErrStandin  = errors.New("stand-in credential used outside its authorized position or origin")
)

type Limits struct {
	Native int `json:"native"`
	OAuth  int `json:"oauth"`
	Vendor int `json:"vendor"`
}

func UnlimitedLimits() Limits { return Limits{-1, -1, -1} }
func (l Limits) For(kind string) int {
	switch kind {
	case "native":
		return l.Native
	case "oauth":
		return l.OAuth
	case "vendor":
		return l.Vendor
	}
	return 0
}

type Config struct {
	Store           *store.SQLiteStore
	Box             *secrets.Box
	OAuthEnabled    bool
	VendorEnabled   bool
	StandinsEnabled bool
	GitHub          OAuthProvider
	Vendor          VendorConfig
	GateFactory     func([]string) (*egress.Gate, error)
	Now             func() time.Time
}

type Service struct {
	cfg         Config
	mu          sync.Mutex
	flows       map[string]oauthFlow
	locks       sync.Map
	standins    map[string]standinBinding
	vendorFlows map[string]vendorFlow
}

func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.GateFactory == nil {
		cfg.GateFactory = func(origins []string) (*egress.Gate, error) { return egress.New(origins, egress.Limits{}) }
	}
	cfg.GitHub = cfg.GitHub.defaults()
	return &Service{cfg: cfg, flows: map[string]oauthFlow{}, standins: map[string]standinBinding{}, vendorFlows: map[string]vendorFlow{}}
}

func (s *Service) Usage(ctx context.Context, ownerID int64, id string) ([]store.ConnectionUsageArtifact, error) {
	return s.cfg.Store.ConnectionArtifacts(ctx, ownerID, id)
}

func (s *Service) ValidateArtifactSharing(ctx context.Context, ownerID int64, id string) error {
	return s.cfg.Store.ValidateConnectionSharing(ctx, ownerID, id)
}

// DataPolicies remains available with all acquisition engines disabled. Saved
// data retains its restrictions after disconnection or credential revocation.
func (s *Service) DataPolicies(ctx context.Context, viewerID int64, artifactID string) ([]store.ConnectionDataPolicy, error) {
	return s.cfg.Store.ListConnectionDataPolicies(ctx, viewerID, artifactID)
}

func (s *Service) ClearData(ctx context.Context, viewerID int64, artifactID string) error {
	return s.cfg.Store.ClearConnectionData(ctx, viewerID, artifactID)
}

type Connection struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Provider             string     `json:"provider"`
	Kind                 string     `json:"kind"`
	Origins              []string   `json:"origins"`
	Capabilities         []string   `json:"capabilities"`
	Sensitivity          string     `json:"sensitivity"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at"`
	LastUsedAt           *time.Time `json:"last_used_at,omitempty"`
	DailyLimit           int        `json:"daily_limit"`
	RequestsToday        int        `json:"requests_today"`
	CacheMinutes         int        `json:"cache_minutes"`
	Endpoint             string     `json:"endpoint,omitempty"`
	Header               string     `json:"header,omitempty"`
	Engine               string     `json:"engine"`
	DeliveryMode         string     `json:"delivery_mode"`
	ReconnectRecommended bool       `json:"reconnect_recommended"`
}

type Provider struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Sensitivity  string   `json:"sensitivity"`
	Enabled      bool     `json:"enabled"`
}

func (s *Service) Providers() []Provider {
	return []Provider{
		{"api-key", "API key", "native", "A key injected into a reviewed header at the origin you choose.", []string{"read"}, "personal", true},
		{"bearer", "Bearer token", "native", "A token sent in the Authorization header to one origin.", []string{"read"}, "personal", true},
		{"basic", "HTTP Basic", "native", "A username and password, or a pasted HTTPS URL containing both. Data stays at this origin unless you explicitly approve another.", []string{"read"}, "personal", true},
		{"github", "GitHub", "oauth", "Connect GitHub using the operator's registered OAuth application.", []string{"read profile", "read repositories"}, "personal", s.cfg.OAuthEnabled && s.cfg.GitHub.ClientID != "" && s.cfg.GitHub.ClientSecret != ""},
		{"vendor", "Managed OAuth", "vendor", "Connect a provider approved by this instance's operator.", []string{"read"}, "personal", s.cfg.VendorEnabled && s.cfg.Vendor.configured()},
	}
}

// CreateRequest is accepted only by the trusted Exhibit app. There is no
// arbitrary recipe field, no agent-authored injection position and no secret
// acquisition through an artifact.
type CreateRequest struct {
	Name               string `json:"name"`
	Recipe             string `json:"recipe"`
	Provider           string `json:"provider,omitempty"`
	Origin             string `json:"origin,omitempty"`
	Header             string `json:"header,omitempty"`
	Secret             string `json:"secret,omitempty"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
	AccessURL          string `json:"access_url,omitempty"`
	DailyLimit         int    `json:"daily_limit,omitempty"`
	CacheMinutes       int    `json:"cache_minutes,omitempty"`
	AllowWrites        bool   `json:"allow_writes,omitempty"`
	VendorProvider     string `json:"vendor_provider,omitempty"`
	VendorConnectionID string `json:"vendor_connection_id,omitempty"`
}

type recipe struct {
	Origins        []string        `json:"origins"`
	Header         string          `json:"header"`
	Scheme         string          `json:"scheme"`
	AllowWrites    bool            `json:"allow_writes"`
	Sensitivity    string          `json:"sensitivity"`
	DailyLimit     int             `json:"daily_limit"`
	CacheSeconds   int             `json:"cache_seconds"`
	Endpoint       string          `json:"endpoint,omitempty"`
	VendorProvider string          `json:"vendor_provider,omitempty"`
	VendorEngine   string          `json:"vendor_engine,omitempty"`
	VendorSpec     *VendorProvider `json:"vendor_spec,omitempty"`
	VendorOwnerTag string          `json:"vendor_owner_tag,omitempty"`
}

type credential struct {
	Value              string          `json:"value,omitempty"`
	Username           string          `json:"username,omitempty"`
	Password           string          `json:"password,omitempty"`
	OAuthToken         json.RawMessage `json:"oauth_token,omitempty"`
	VendorConnectionID string          `json:"vendor_connection_id,omitempty"`
	Endpoint           string          `json:"endpoint,omitempty"` // Earlier native credentials stored a private endpoint alias.
	BasePath           string          `json:"base_path,omitempty"`
	ExtraSecrets       []string        `json:"extra_secrets,omitempty"`
}

func (s *Service) Create(ctx context.Context, ownerID int64, in CreateRequest, limits Limits) (*Connection, error) {
	if s.cfg.Store == nil || s.cfg.Box == nil {
		return nil, ErrDisabled
	}
	if ownerID <= 0 {
		return nil, store.ErrNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return nil, fmt.Errorf("%w: give the connection a name of at most 100 characters", ErrInvalid)
	}
	provider := in.Recipe
	if provider == "" {
		provider = in.Provider
	}
	kind := "native"
	if in.DailyLimit < 0 || in.CacheMinutes < 0 || in.CacheMinutes > 525600 {
		return nil, fmt.Errorf("%w: daily requests must be nonnegative and response reuse must be between 0 and 525600 minutes", ErrInvalid)
	}
	if in.AccessURL != "" && provider != "basic" {
		return nil, fmt.Errorf("%w: a credential URL is supported only for Basic authentication", ErrInvalid)
	}
	r := recipe{Sensitivity: "personal", AllowWrites: in.AllowWrites, DailyLimit: in.DailyLimit, CacheSeconds: in.CacheMinutes * 60}
	c := credential{}
	switch provider {
	case "api-key", "bearer", "basic":
		if provider == "basic" && in.AccessURL != "" {
			if in.Origin != "" || in.Username != "" || in.Password != "" {
				return nil, fmt.Errorf("%w: paste a credential URL or enter separate Basic fields", ErrInvalid)
			}
			o, parsed, err := parseBasicAccessURL(in.AccessURL)
			if err != nil {
				return nil, err
			}
			r.Origins, r.Endpoint, c = []string{o}, "/", parsed
		} else {
			o, err := normalizeHTTPSOrigin(in.Origin)
			if err != nil {
				return nil, err
			}
			r.Origins = []string{o}
		}
		switch provider {
		case "api-key":
			r.Header = http.CanonicalHeaderKey(strings.TrimSpace(in.Header))
			if r.Header == "" {
				r.Header = "X-Api-Key"
			}
			if !safeCredentialHeader(r.Header) {
				return nil, fmt.Errorf("%w: choose an API credential header such as X-API-Key", ErrInvalid)
			}
			c.Value = in.Secret
		case "bearer":
			r.Header = "Authorization"
			r.Scheme = "Bearer "
			c.Value = in.Secret
		case "basic":
			r.Header = "Authorization"
			r.Scheme = "Basic "
			if in.AccessURL == "" {
				c.Username = in.Username
				c.Password = in.Password
			}
			if c.Username == "" || strings.Contains(c.Username, ":") {
				return nil, fmt.Errorf("%w: Basic authentication needs a username without a colon", ErrInvalid)
			}
		}
		if (provider != "basic" && c.Value == "") || (provider == "basic" && c.Password == "") {
			return nil, fmt.Errorf("%w: credential is required", ErrInvalid)
		}
	case "vendor", "nango", "composio":
		if !s.cfg.VendorEnabled {
			return nil, ErrDisabled
		}
		raw, ok := s.cfg.Vendor.Providers[in.VendorProvider]
		if !ok {
			return nil, fmt.Errorf("%w: provider is not approved by the operator", ErrInvalid)
		}
		spec := raw.defaults(in.VendorProvider)
		engine, err := s.vendorEngine(spec.Engine)
		if err != nil {
			return nil, err
		}
		if in.VendorConnectionID == "" || len(in.VendorConnectionID) > 256 || strings.ContainsAny(in.VendorConnectionID, "\r\n") {
			return nil, ErrInvalid
		}
		connection := VendorConnectionSpec{OwnerTag: s.cfg.Vendor.ownerTag(ownerID), Provider: in.VendorProvider, ExternalRef: in.VendorConnectionID, Spec: spec}
		status, err := engine.Status(ctx, connection)
		if err != nil {
			return nil, err
		}
		if status != "ok" {
			return nil, vendorFailure(spec.Engine, VendorNeedsReconnect)
		}
		if in.DailyLimit > 0 {
			spec.DailyLimit = in.DailyLimit
		}
		if in.CacheMinutes > 0 {
			spec.CacheMinutes = in.CacheMinutes
		}
		return s.saveVendor(ctx, ownerID, in.Name, in.VendorProvider, connection.OwnerTag, in.VendorConnectionID, spec, limits)
	default:
		return nil, fmt.Errorf("%w: choose a supported provider", ErrInvalid)
	}
	if strings.ContainsAny(c.Value+c.Username+c.Password, "\r\n") || len(c.Value)+len(c.Username)+len(c.Password) > 16384 {
		return nil, fmt.Errorf("%w: credential has an invalid format", ErrInvalid)
	}
	return s.save(ctx, ownerID, in.Name, provider, kind, r, c, limits)
}

func (s *Service) save(ctx context.Context, ownerID int64, name, provider, kind string, r recipe, c credential, limits Limits) (*Connection, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	sealed, err := s.cfg.Box.Encrypt(string(raw))
	if err != nil {
		return nil, errors.New("encrypt connection credential")
	}
	policy, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	rec := &store.ConnectionRecord{ID: uuid.NewString(), OwnerID: ownerID, Name: name, Provider: provider, Kind: kind, RecipeJSON: string(policy), EncryptedSecret: sealed}
	if kind == "vendor" {
		rec.Engine = r.VendorEngine
		rec.ExternalRef = c.VendorConnectionID
	}
	if err = s.cfg.Store.CreateConnection(ctx, rec, limits.For(kind)); err != nil {
		if errors.Is(err, store.ErrConnectionLimit) {
			return nil, fmt.Errorf("%w: this account allows %d %s connections", err, limits.For(kind), kind)
		}
		return nil, err
	}
	rec, err = s.cfg.Store.GetConnection(ctx, ownerID, rec.ID)
	if err != nil {
		return nil, err
	}
	return s.metadata(ctx, rec)
}

func (s *Service) List(ctx context.Context, ownerID int64) ([]Connection, error) {
	rows, err := s.cfg.Store.ListConnections(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := []Connection{}
	for _, rec := range rows {
		c, err := s.metadata(ctx, rec)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}
func (s *Service) metadata(ctx context.Context, rec *store.ConnectionRecord) (*Connection, error) {
	var r recipe
	if err := json.Unmarshal([]byte(rec.RecipeJSON), &r); err != nil {
		return nil, errors.New("invalid stored connection policy")
	}
	caps := []string{"read"}
	if r.AllowWrites {
		caps = append(caps, "write")
	}
	used, err := s.cfg.Store.ConnectionRequestsToday(ctx, rec.ID, s.cfg.Now())
	if err != nil {
		return nil, err
	}
	endpoint := ""
	if r.Endpoint != "" {
		endpoint = "/" // The private base path is never metadata.
	}
	connection := &Connection{ID: rec.ID, Name: rec.Name, Provider: rec.Provider, Kind: rec.Kind, Origins: r.Origins, Capabilities: caps, Sensitivity: r.Sensitivity, Status: rec.Status, CreatedAt: rec.CreatedAt, LastUsedAt: rec.LastUsedAt, DailyLimit: r.DailyLimit, RequestsToday: used, CacheMinutes: r.CacheSeconds / 60, Endpoint: endpoint, Header: r.Header, Engine: "native", DeliveryMode: VendorInject}
	if rec.Status == "reconnect" {
		connection.Status = VendorNeedsReconnect
		connection.ReconnectRecommended = true
	}
	if rec.Kind == "vendor" {
		connection.Engine = rec.Engine
		if connection.Engine == "" {
			connection.Engine = r.VendorEngine
		}
		if connection.Engine == "" {
			connection.Engine = "nango"
		}
		if connection.Engine == "composio" {
			connection.DeliveryMode = VendorForward
		}
		if r.VendorSpec != nil && r.VendorSpec.DeliveryMode != "" {
			connection.DeliveryMode = r.VendorSpec.DeliveryMode
		}
		provider := r.VendorProvider
		if provider == "" {
			provider = rec.Provider
		}
		configured, ok := s.cfg.Vendor.Providers[provider]
		connection.ReconnectRecommended = connection.ReconnectRecommended || !ok || configured.defaults(provider).Engine != connection.Engine
	}
	return connection, nil
}

func (s *Service) readCredential(rec *store.ConnectionRecord) (recipe, credential, error) {
	var r recipe
	var c credential
	if err := json.Unmarshal([]byte(rec.RecipeJSON), &r); err != nil {
		return r, c, errors.New("invalid stored connection policy")
	}
	raw, err := s.cfg.Box.Decrypt(rec.EncryptedSecret)
	if err != nil {
		return r, c, errors.New("connection credential could not be decrypted")
	}
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return r, c, errors.New("invalid stored connection credential")
	}
	if c.BasePath == "" && c.Endpoint != "" && r.Endpoint != "" {
		// Shape-based migration retains the encrypted private endpoint. Its
		// public alias identifies the suffix to remove when recovering a base.
		c.BasePath = (&url.URL{Path: strings.TrimSuffix(c.Endpoint, r.Endpoint)}).EscapedPath()
		if c.BasePath == "" {
			c.BasePath = "/"
		}
	}
	return r, c, nil
}

func normalizeHTTPSOrigin(raw string) (string, error) {
	o, err := origin.NormalizeOrigin(raw)
	if err != nil || !strings.HasPrefix(o, "https://") {
		return "", fmt.Errorf("%w: enter an HTTPS origin without a path, query or credentials", ErrInvalid)
	}
	return o, nil
}
func requestOrigin(u *url.URL) string {
	if u == nil {
		return ""
	}
	o, err := origin.NormalizeOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return ""
	}
	return o
}
func safeCredentialHeader(h string) bool {
	lower := strings.ToLower(h)
	if h == "" || strings.ContainsAny(h, "\r\n :\t") {
		return false
	}
	for _, r := range h {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	if strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "x-forwarded-") {
		return false
	}
	return !slices.Contains([]string{"cookie", "set-cookie", "host", "origin", "referer", "content-length", "transfer-encoding", "connection", "accept-encoding", "user-agent", "forwarded", "upgrade", "trailer", "te"}, lower)
}

// parseBasicAccessURL splits a user-pasted URL without making a network
// request. The username, password and base path all stay in the encrypted
// credential; only the normalized origin and a virtual root are published.
func parseBasicAccessURL(raw string) (string, credential, error) {
	fail := func() (string, credential, error) {
		return "", credential{}, fmt.Errorf("%w: paste an HTTPS URL with a username and password, without a query or fragment", ErrInvalid)
	}
	if len(raw) > 16384 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n") {
		return fail()
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User == nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return fail()
	}
	username := u.User.Username()
	password, ok := u.User.Password()
	if !ok || username == "" || password == "" || strings.Contains(username, ":") || strings.ContainsAny(username+password, "\r\n") {
		return fail()
	}
	o, err := normalizeHTTPSOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return fail()
	}
	base := strings.TrimRight(u.EscapedPath(), "/")
	if base == "" {
		base = "/"
	}
	if !safeRelativePath(base) {
		return fail()
	}
	return o, credential{Username: username, Password: password, BasePath: base}, nil
}

// safeRelativePath refuses path traversal, including nested percent encoding,
// before a frame-controlled suffix can be appended to a private base path.
func safeRelativePath(escaped string) bool {
	for i := 0; i < 8; i++ {
		if strings.ContainsAny(escaped, "\\\r\n") || strings.HasPrefix(escaped, "//") {
			return false
		}
		for _, segment := range strings.Split(escaped, "/") {
			if segment == "." || segment == ".." {
				return false
			}
		}
		decoded, err := url.PathUnescape(escaped)
		if err != nil {
			return false
		}
		if decoded == escaped {
			return true
		}
		escaped = decoded
	}
	return false
}

func withPrivateBase(ctx context.Context, req *http.Request, base string) (*http.Request, error) {
	if base == "" || base == "/" {
		return req, nil
	}
	if !safeRelativePath(base) || !safeRelativePath(req.URL.EscapedPath()) {
		return nil, fmt.Errorf("%w: request path must stay within the connection's base URL", ErrEgress)
	}
	copy := req.Clone(ctx)
	u := *req.URL
	copy.URL = &u
	u.RawPath = strings.TrimRight(base, "/") + "/" + strings.TrimLeft(req.URL.EscapedPath(), "/")
	path, err := url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, ErrEgress
	}
	u.Path = path
	return copy, nil
}

func withinPrivateBase(u *url.URL, base string) bool {
	if base == "" || base == "/" {
		return true
	}
	path := u.EscapedPath()
	return safeRelativePath(path) && (path == base || strings.HasPrefix(path, strings.TrimRight(base, "/")+"/"))
}

type GrantRequest struct {
	ConnectionID        string   `json:"connection_id"`
	PermittedEgress     []string `json:"permitted_egress"`
	ShareWithRecipients bool     `json:"share_with_recipients"`
}
type Grant struct {
	Connection          Connection `json:"connection"`
	ArtifactID          string     `json:"artifact_id"`
	ViewerID            int64      `json:"viewer_id"`
	PermittedEgress     []string   `json:"permitted_egress"`
	ShareWithRecipients bool       `json:"share_with_recipients"`
	Consent             string     `json:"consent"`
}

func (s *Service) Grant(ctx context.Context, viewerID int64, artifactID string, in GrantRequest) (*Grant, error) {
	rec, err := s.cfg.Store.GetConnection(ctx, viewerID, in.ConnectionID)
	if err != nil {
		return nil, err
	}
	artifact, err := s.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(viewerID), artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, store.ErrNotFound
	}
	if (artifact.ShareStateMode == store.ShareStateShared || artifact.SharePublicLink) && (!in.ShareWithRecipients || artifact.OwnerID != viewerID) {
		return nil, fmt.Errorf("%w: shared-state or public-link artifacts require the owner's explicit connection sharing consent", ErrInvalid)
	}
	permitted, err := origin.NormalizeOrigins(in.PermittedEgress)
	if err != nil {
		return nil, fmt.Errorf("%w: permitted destinations must be origins", ErrInvalid)
	}
	for _, o := range permitted {
		if !strings.HasPrefix(o, "https://") {
			return nil, fmt.Errorf("%w: destinations must use HTTPS", ErrInvalid)
		}
	}
	policy, _, err := s.readCredential(rec)
	if err != nil {
		return nil, err
	}
	existing, err := s.Grants(ctx, viewerID, artifactID)
	if err != nil {
		return nil, err
	}
	for _, g := range existing {
		if g.Connection.ID == in.ConnectionID {
			continue
		}
		for _, o := range g.Connection.Origins {
			if slices.Contains(policy.Origins, o) {
				return nil, store.ErrConnectionOverlap
			}
		}
	}
	g := store.ConnectionGrantRecord{ArtifactID: artifactID, ConnectionID: in.ConnectionID, ViewerID: viewerID, PermittedEgress: permitted, ShareWithRecipients: in.ShareWithRecipients}
	if err = s.cfg.Store.PutConnectionGrant(ctx, g); err != nil {
		return nil, err
	}
	return s.grantMetadata(ctx, g)
}
func (s *Service) grantMetadata(ctx context.Context, g store.ConnectionGrantRecord) (*Grant, error) {
	rec, err := s.cfg.Store.GetGrantedConnection(ctx, g.ViewerID, g.ArtifactID, g.ConnectionID)
	if err != nil {
		return nil, err
	}
	meta, err := s.metadata(ctx, rec)
	if err != nil {
		return nil, err
	}
	dest := "nowhere else"
	if len(g.PermittedEgress) > 0 {
		dest = strings.Join(g.PermittedEgress, ", ")
	}
	consent := "Can " + strings.Join(meta.Capabilities, ", ") + " using " + meta.Name + "; can send data to: " + dest + "."
	if meta.DeliveryMode == VendorForward {
		consent += " Requests and responses pass through " + meta.Engine + " in forward mode."
	}
	return &Grant{Connection: *meta, ArtifactID: g.ArtifactID, ViewerID: g.ViewerID, PermittedEgress: g.PermittedEgress, ShareWithRecipients: g.ShareWithRecipients, Consent: consent}, nil
}
func (s *Service) Grants(ctx context.Context, viewerID int64, artifactID string) ([]Grant, error) {
	rows, err := s.cfg.Store.ListConnectionGrants(ctx, viewerID, artifactID)
	if err != nil {
		return nil, err
	}
	out := []Grant{}
	for _, g := range rows {
		meta, err := s.grantMetadata(ctx, g)
		if err != nil {
			return nil, err
		}
		out = append(out, *meta)
	}
	return out, nil
}
func (s *Service) Ungrant(ctx context.Context, viewerID int64, artifactID, id string) error {
	return s.cfg.Store.DeleteConnectionGrant(ctx, viewerID, artifactID, id)
}

// AllowedEgress is the intersection of every retained data policy's destinations.
// It is used for browser CSP as well as relay enforcement: credential custody
// alone cannot stop an artifact from sending the data it just read elsewhere.
func (s *Service) AllowedEgress(ctx context.Context, viewerID int64, artifactID string, base []string) ([]string, error) {
	policies, err := s.DataPolicies(ctx, viewerID, artifactID)
	if err != nil {
		return nil, err
	}
	out := append([]string{}, base...)
	for _, p := range policies {
		out = slices.DeleteFunc(out, func(o string) bool {
			return !slices.Contains(p.Origins, o)
		})
	}
	return out, nil
}

// Engine authorizes a request only after the common egress gate has validated
// its destination and removed frame-supplied headers. Engines never return a
// credential to a caller or execute artifact code.
type Engine interface {
	Authorize(context.Context, *http.Request, *store.ConnectionRecord, recipe, *credential) error
}

// VendorEngine owns only managed authentication and delivery. Exhibit checks
// all grants, destinations, request limits and cache custody before calling it.
// Inject engines add credentials after each checked hop; forward engines send
// only a sanitized request whose target the common gate preflighted.
type VendorEngine interface {
	ID() string
	DeliveryMode() string
	Start(context.Context, VendorFlowRequest) (*VendorSession, error)
	Complete(context.Context, VendorFlowRequest) (string, bool, error)
	Status(context.Context, VendorConnectionSpec) (string, error)
	Authorize(context.Context, *http.Request, VendorConnectionSpec, *credential) error
	Forward(context.Context, *http.Request, VendorConnectionSpec) (*egress.Response, error)
	Revoke(context.Context, VendorConnectionSpec) error
}

type NativeEngine struct{}

func (NativeEngine) Authorize(_ context.Context, req *http.Request, _ *store.ConnectionRecord, r recipe, c *credential) error {
	if !slices.Contains(r.Origins, requestOrigin(req.URL)) {
		return ErrEgress
	}
	value := c.Value
	if r.Scheme == "Basic " {
		value = base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
	}
	req.Header.Set(r.Header, r.Scheme+value)
	return nil
}

func (s *Service) Do(ctx context.Context, viewerID int64, artifactID string, req *http.Request, gate *egress.Gate) (*egress.Response, error) {
	policies, err := s.DataPolicies(ctx, viewerID, artifactID)
	if err != nil {
		return nil, err
	}
	grants, err := s.Grants(ctx, viewerID, artifactID)
	if err != nil {
		return nil, err
	}
	target := requestOrigin(req.URL)
	if err = s.checkStandinMisuse(viewerID, artifactID, req, target); err != nil {
		return nil, err
	}
	for _, p := range policies {
		if !slices.Contains(p.Origins, target) {
			return nil, ErrEgress
		}
	}
	var chosen *Grant
	for i := range grants {
		if slices.Contains(grants[i].Connection.Origins, target) {
			if chosen != nil && chosen.Connection.ID != grants[i].Connection.ID {
				return nil, store.ErrConnectionOverlap
			}
			chosen = &grants[i]
		}
	}
	if chosen == nil {
		// A data destination can redirect too. Apply the data policy intersection
		// at every validated hop even when this request injects no credential.
		return gate.DoAuthorized(ctx, req, func(hop *http.Request) error {
			for _, p := range policies {
				next := requestOrigin(hop.URL)
				if !slices.Contains(p.Origins, next) {
					return ErrEgress
				}
			}
			return nil
		})
	}
	rec, err := s.cfg.Store.GetGrantedConnection(ctx, viewerID, artifactID, chosen.Connection.ID)
	if err != nil {
		return nil, err
	}
	artifact, err := s.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(viewerID), artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, store.ErrNotFound
	}
	if (artifact.ShareStateMode == store.ShareStateShared || artifact.SharePublicLink) && (!chosen.ShareWithRecipients || chosen.ViewerID != artifact.OwnerID) {
		return nil, fmt.Errorf("%w: shared-state or public-link artifact needs the owner's explicit sharing consent", ErrEgress)
	}
	if chosen.ViewerID != viewerID {
		// Persist the recipient's custody before either a live or cached
		// provider response can be saved in their own state. Owner reset or
		// deletion must not release copies held by another viewer.
		if err = s.cfg.Store.RetainSharedConnectionDataPolicies(ctx, viewerID, artifactID); err != nil {
			return nil, err
		}
	}
	lock, _ := s.locks.LoadOrStore(rec.ID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	// Recheck revocation after waiting for another request's quota/cache work.
	rec, err = s.cfg.Store.GetGrantedConnection(ctx, viewerID, artifactID, rec.ID)
	if err != nil {
		return nil, err
	}
	if rec.Kind == "vendor" && rec.Status == "reconnect" {
		return nil, vendorFailure(rec.Engine, VendorNeedsReconnect)
	}
	r, c, err := s.readCredential(rec)
	if err != nil {
		return nil, err
	}
	if rec.Kind == "oauth" && !s.cfg.OAuthEnabled || rec.Kind == "vendor" && !s.cfg.VendorEnabled {
		return nil, ErrDisabled
	}
	var managed VendorEngine
	var managedSpec VendorConnectionSpec
	if rec.Kind == "vendor" {
		managedSpec, err = s.vendorConnectionSpec(ctx, rec, r, c)
		if err != nil {
			return nil, err
		}
		managed, err = s.vendorEngine(managedSpec.Spec.Engine)
		if err != nil {
			return nil, err
		}
		if managedSpec.Spec.DeliveryMode != managed.DeliveryMode() {
			return nil, vendorFailure(managed.ID(), VendorRefused)
		}
		// Operator API keys are also credentials. A vendor may reflect them in
		// a proxy payload; unlike opaque provider tokens, these are known here.
		c.ExtraSecrets = append(c.ExtraSecrets, s.vendorOperatorSecrets()...)
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead && !r.AllowWrites {
		return nil, fmt.Errorf("%w: connection grants read access only", ErrEgress)
	}
	// A generic virtual root hides every pasted URL's private base path.
	// Redirects are checked against the same base before reinjecting a secret.
	req, err = withPrivateBase(ctx, req, c.BasePath)
	if err != nil {
		return nil, err
	}
	if !gate.Allows(req.URL) {
		return nil, ErrEgress
	}
	hash := requestCacheKey(req)
	cacheable := r.CacheSeconds > 0 && req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody)
	if cacheable {
		sealed, e := s.cfg.Store.GetConnectionCache(ctx, rec.ID, hash, s.cfg.Now())
		if e == nil {
			plain, e := s.cfg.Box.Decrypt(sealed)
			if e != nil {
				return nil, errors.New("connection cache could not be decrypted")
			}
			cached, snapshot, usable, e := decodeConnectionCache(plain)
			if e != nil {
				return nil, e
			}
			if usable {
				snapshot.ExtraSecrets = append(snapshot.ExtraSecrets, c.ExtraSecrets...)
				// The cache is connection-wide; stand-ins are viewer-local and
				// expire. Scrub the encrypted original for this delivery only.
				if e = s.scrubResponse(rec, viewerID, r, snapshot, cached); e != nil {
					return nil, e
				}
				if e = s.cfg.Store.TouchConnection(ctx, rec.ID); e != nil {
					return nil, e
				}
				return cached, nil
			}
		} else if !errors.Is(e, store.ErrNotFound) {
			return nil, e
		}
	}
	var response *egress.Response
	if managed != nil && managed.DeliveryMode() == VendorForward {
		response, err = gate.DoForwarded(ctx, req, func(forwardCtx context.Context, checked *http.Request) (*egress.Response, error) {
			if requestOrigin(checked.URL) != target || !slices.Contains(managedSpec.Spec.Origins, target) {
				return nil, ErrEgress
			}
			if err := s.cfg.Store.ReserveConnectionRequest(forwardCtx, rec.ID, r.DailyLimit, s.cfg.Now()); err != nil {
				return nil, err
			}
			return managed.Forward(forwardCtx, checked, managedSpec)
		})
	} else {
		var engine Engine = NativeEngine{}
		if rec.Kind == "oauth" {
			engine = oauthEngine{s: s}
		}
		if managed != nil {
			engine = vendorInjectionEngine{engine: managed, spec: managedSpec}
		}
		response, err = gate.DoAuthorized(ctx, req, func(hop *http.Request) error {
			if requestOrigin(hop.URL) != target || !withinPrivateBase(hop.URL, c.BasePath) {
				return ErrEgress
			}
			if err := s.cfg.Store.ReserveConnectionRequest(ctx, rec.ID, r.DailyLimit, s.cfg.Now()); err != nil {
				return err
			}
			if err := engine.Authorize(ctx, hop, rec, r, &c); err != nil {
				return err
			}
			if managed != nil {
				return s.cfg.Store.RecordVendorCall(ctx, managed.ID(), managedSpec.Provider, "inject", s.cfg.Now())
			}
			return nil
		})
	}
	if err == nil && managed != nil && response.Status >= 400 {
		err = vendorHTTPError(managed.ID(), response.Status, response.Header)
	}
	if err != nil {
		var vendorErr *VendorError
		if errors.As(err, &vendorErr) {
			if vendorErr.Code == VendorNeedsReconnect {
				if saveErr := s.cfg.Store.MarkVendorConnectionReconnect(ctx, rec.OwnerID, rec.ID); saveErr != nil {
					return nil, saveErr
				}
			}
			return nil, err
		}
		var denied *egress.DeniedError
		if errors.As(err, &denied) {
			if managed != nil {
				return nil, vendorFailure(managed.ID(), VendorRefused)
			}
			return nil, ErrEgress
		}
		if errors.Is(err, ErrEgress) || errors.Is(err, ErrStandin) || errors.Is(err, ErrDisabled) || errors.Is(err, store.ErrConnectionQuota) {
			return nil, err
		}
		if managed != nil {
			return nil, vendorFailure(managed.ID(), VendorUpstreamError)
		}
		return nil, errors.New("connection provider request failed")
	}
	var cacheEntry []byte
	if cacheable && response.Status >= 200 && response.Status < 300 && response.Status != http.StatusPartialContent && !varyWildcard(response.Header) {
		if err = responseSizeAllowed(response); err != nil {
			return nil, err
		}
		// Seal the original bytes and only the secrets needed to scrub them.
		// A refreshed access token may no longer be the stored credential by
		// the next cache hit. Never persist a viewer's inert handle as data.
		cacheEntry, err = encodeConnectionCache(response, c)
		if err != nil {
			return nil, err
		}
	}
	if err = s.scrubResponse(rec, viewerID, r, c, response); err != nil {
		return nil, err
	}
	if err = s.cfg.Store.TouchConnection(ctx, rec.ID); err != nil {
		return nil, err
	}
	if cacheEntry != nil {
		sealed, e := s.cfg.Box.Encrypt(string(cacheEntry))
		if e != nil {
			return nil, e
		}
		if e = s.cfg.Store.PutConnectionCache(ctx, rec.ID, hash, sealed, s.cfg.Now().Add(time.Duration(r.CacheSeconds)*time.Second)); e != nil {
			return nil, e
		}
	}
	return response, nil
}

func requestCacheKey(req *http.Request) string {
	// Include every frame-controlled representation header the common gate
	// forwards. Stand-ins and discarded headers never split a shared cache,
	// while ranges, language and conditional requests cannot reuse each other.
	headers := make(http.Header)
	for key, values := range req.Header {
		key = http.CanonicalHeaderKey(key)
		switch key {
		case "Accept", "Accept-Language", "Content-Type", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since", "Range", "If-Range", "Cache-Control":
			headers[key] = append(headers[key], values...)
		}
	}
	raw, _ := json.Marshal(headers) // Header contains only JSON-safe string slices.
	sum := sha256.Sum256([]byte(req.Method + "\n" + req.URL.String() + "\n" + string(raw)))
	return hex.EncodeToString(sum[:])
}

func varyWildcard(header http.Header) bool {
	for _, value := range header.Values("Vary") {
		for _, field := range strings.Split(value, ",") {
			if strings.TrimSpace(field) == "*" {
				return true
			}
		}
	}
	return false
}

// cachedCredential deliberately excludes OAuth payloads and vendor metadata.
// It travels only inside the encrypted, bounded cache envelope.
type cachedCredential struct {
	Value        string   `json:"value,omitempty"`
	Username     string   `json:"username,omitempty"`
	Password     string   `json:"password,omitempty"`
	BasePath     string   `json:"base_path,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ExtraSecrets []string `json:"extra_secrets,omitempty"`
}

func encodeConnectionCache(response *egress.Response, c credential) ([]byte, error) {
	snapshot := cachedCredential{Value: c.Value, Username: c.Username, Password: c.Password, BasePath: c.BasePath, Endpoint: c.Endpoint, ExtraSecrets: append([]string{}, c.ExtraSecrets...)}
	if len(snapshot.ExtraSecrets) > 8 {
		return nil, ErrResponseSize
	}
	if len(c.OAuthToken) > 0 {
		// Preserve just the refresh secret for redaction, rather than arbitrary
		// token JSON. The current access token is already in c.Value.
		var token struct {
			RefreshToken string `json:"refresh_token"`
		}
		if json.Unmarshal(c.OAuthToken, &token) != nil {
			return nil, errors.New("invalid connection cache credential")
		}
		snapshot.Password = token.RefreshToken
	}
	secret, err := json.Marshal(snapshot)
	if err != nil || len(secret) > maxConnectionResponseMetadataBytes {
		return nil, ErrResponseSize
	}
	entry := struct {
		*egress.Response
		Version    int             `json:"version"`
		Credential json.RawMessage `json:"credential"`
	}{response, 1, secret}
	raw, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxConnectionCacheJSONBytes {
		return nil, ErrResponseSize
	}
	return raw, nil
}

func decodeConnectionCache(plain string) (*egress.Response, credential, bool, error) {
	// The shared decoder verifies the encoded body length before allocating
	// it, including for entries from an earlier version.
	response, err := decodeCachedResponse(plain)
	if err != nil {
		return nil, credential{}, false, err
	}
	var entry struct {
		Version    int             `json:"version"`
		Credential json.RawMessage `json:"credential"`
	}
	if json.Unmarshal([]byte(plain), &entry) != nil {
		return nil, credential{}, false, errors.New("connection cache contains an invalid response")
	}
	if entry.Version != 1 || len(entry.Credential) == 0 {
		return nil, credential{}, false, nil // Old scrubbed responses are a miss.
	}
	if len(entry.Credential) > maxConnectionResponseMetadataBytes {
		return nil, credential{}, false, ErrResponseSize
	}
	var snapshot cachedCredential
	if json.Unmarshal(entry.Credential, &snapshot) != nil {
		return nil, credential{}, false, errors.New("connection cache contains an invalid credential")
	}
	if len(snapshot.ExtraSecrets) > 8 {
		return nil, credential{}, false, ErrResponseSize
	}
	c := credential{Value: snapshot.Value, Username: snapshot.Username, Password: snapshot.Password, BasePath: snapshot.BasePath, Endpoint: snapshot.Endpoint, ExtraSecrets: snapshot.ExtraSecrets}
	return response, c, true, nil
}

type vendorInjectionEngine struct {
	engine VendorEngine
	spec   VendorConnectionSpec
}

func (e vendorInjectionEngine) Authorize(ctx context.Context, req *http.Request, _ *store.ConnectionRecord, _ recipe, c *credential) error {
	return e.engine.Authorize(ctx, req, e.spec, c)
}
