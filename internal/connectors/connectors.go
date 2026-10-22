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
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Provider      string     `json:"provider"`
	Kind          string     `json:"kind"`
	Origins       []string   `json:"origins"`
	Capabilities  []string   `json:"capabilities"`
	Sensitivity   string     `json:"sensitivity"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	DailyLimit    int        `json:"daily_limit"`
	RequestsToday int        `json:"requests_today"`
	Endpoint      string     `json:"endpoint,omitempty"`
	Header        string     `json:"header,omitempty"`
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
		{"basic", "HTTP Basic", "native", "A username and password sent to one origin over HTTPS.", []string{"read"}, "personal", true},
		{"simplefin", "SimpleFIN", "native", "Read accounts and transactions. Cached for one hour, at most 24 provider requests per UTC day. Financial data stays at this origin unless you explicitly approve another.", []string{"read accounts", "read transactions"}, "financial", true},
		{"github", "GitHub", "oauth", "Connect GitHub using the operator's registered OAuth application.", []string{"read profile", "read repositories"}, "personal", s.cfg.OAuthEnabled && s.cfg.GitHub.ClientID != "" && s.cfg.GitHub.ClientSecret != ""},
		{"nango", "Managed OAuth", "vendor", "Connect a provider approved by this instance's operator.", []string{"read"}, "personal", s.cfg.VendorEnabled && s.cfg.Vendor.configured()},
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
	SetupToken         string `json:"setup_token,omitempty"`
	AllowWrites        bool   `json:"allow_writes,omitempty"`
	VendorProvider     string `json:"vendor_provider,omitempty"`
	VendorConnectionID string `json:"vendor_connection_id,omitempty"`
}

type recipe struct {
	Origins        []string `json:"origins"`
	Header         string   `json:"header"`
	Scheme         string   `json:"scheme"`
	AllowWrites    bool     `json:"allow_writes"`
	Sensitivity    string   `json:"sensitivity"`
	DailyLimit     int      `json:"daily_limit"`
	CacheSeconds   int      `json:"cache_seconds"`
	Endpoint       string   `json:"endpoint,omitempty"`
	VendorProvider string   `json:"vendor_provider,omitempty"`
}

type credential struct {
	Value              string          `json:"value,omitempty"`
	Username           string          `json:"username,omitempty"`
	Password           string          `json:"password,omitempty"`
	OAuthToken         json.RawMessage `json:"oauth_token,omitempty"`
	VendorConnectionID string          `json:"vendor_connection_id,omitempty"`
	Endpoint           string          `json:"endpoint,omitempty"`
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
	r := recipe{Sensitivity: "personal", AllowWrites: in.AllowWrites}
	c := credential{}
	switch provider {
	case "api-key", "bearer", "basic":
		o, err := normalizeHTTPSOrigin(in.Origin)
		if err != nil {
			return nil, err
		}
		r.Origins = []string{o}
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
			c.Username = in.Username
			c.Password = in.Password
			if c.Username == "" || strings.Contains(c.Username, ":") {
				return nil, fmt.Errorf("%w: Basic authentication needs a username without a colon", ErrInvalid)
			}
		}
		if (provider != "basic" && c.Value == "") || (provider == "basic" && c.Password == "") {
			return nil, fmt.Errorf("%w: credential is required", ErrInvalid)
		}
	case "simplefin":
		if strings.TrimSpace(in.SetupToken) == "" {
			return nil, fmt.Errorf("%w: SimpleFIN setup token is required", ErrInvalid)
		}
		var err error
		r, c, err = s.claimSimpleFIN(ctx, in.SetupToken)
		if err != nil {
			return nil, err
		}
	case "nango":
		if !s.cfg.VendorEnabled || !s.cfg.Vendor.configured() {
			return nil, ErrDisabled
		}
		kind = "vendor"
		spec, ok := s.cfg.Vendor.Providers[in.VendorProvider]
		if !ok {
			return nil, fmt.Errorf("%w: provider is not approved by the operator", ErrInvalid)
		}
		if in.VendorConnectionID == "" || len(in.VendorConnectionID) > 200 || strings.ContainsAny(in.VendorConnectionID, "\r\n") {
			return nil, fmt.Errorf("%w: a completed managed OAuth connection is required", ErrInvalid)
		}
		r.Origins = append([]string{}, spec.Origins...)
		r.Header = "Authorization"
		r.VendorProvider = in.VendorProvider
		r.AllowWrites = spec.AllowWrites
		c.VendorConnectionID = in.VendorConnectionID
		if err := s.verifyVendorConnection(ctx, in.VendorProvider, in.VendorConnectionID, ownerID); err != nil {
			return nil, err
		}
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
	if rec.Provider == "simplefin" {
		caps = []string{"read accounts", "read transactions"}
	}
	used, err := s.cfg.Store.ConnectionRequestsToday(ctx, rec.ID, s.cfg.Now())
	if err != nil {
		return nil, err
	}
	return &Connection{ID: rec.ID, Name: rec.Name, Provider: rec.Provider, Kind: rec.Kind, Origins: r.Origins, Capabilities: caps, Sensitivity: r.Sensitivity, Status: rec.Status, CreatedAt: rec.CreatedAt, LastUsedAt: rec.LastUsedAt, DailyLimit: r.DailyLimit, RequestsToday: used, Endpoint: r.Endpoint, Header: r.Header}, nil
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

func (s *Service) claimSimpleFIN(ctx context.Context, setup string) (recipe, credential, error) {
	fail := func() (recipe, credential, error) {
		return recipe{}, credential{}, fmt.Errorf("%w: SimpleFIN setup token could not be claimed; obtain a new token from your bridge", ErrInvalid)
	}
	if len(setup) > 16384 {
		return fail()
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(setup))
	if err != nil {
		return fail()
	}
	u, err := url.Parse(string(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Host == "" {
		return fail()
	}
	o := requestOrigin(u)
	gate, err := s.cfg.GateFactory([]string{o})
	if err != nil {
		return fail()
	}
	defer gate.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return fail()
	}
	resp, err := gate.Do(ctx, req)
	if err != nil || resp.Status < 200 || resp.Status >= 300 || len(resp.Body) > 16384 {
		return fail()
	}
	access, err := url.Parse(strings.TrimSpace(string(resp.Body)))
	if err != nil || access.Scheme != "https" || access.User == nil || access.Host == "" || access.RawQuery != "" || access.Fragment != "" {
		return fail()
	}
	password, ok := access.User.Password()
	if !ok || access.User.Username() == "" || password == "" {
		return fail()
	}
	accountOrigin := requestOrigin(access)
	if accountOrigin == "" {
		return fail()
	}
	endpoint := strings.TrimSuffix(access.Path, "/") + "/accounts"
	return recipe{Origins: []string{accountOrigin}, Header: "Authorization", Scheme: "Basic ", Sensitivity: "financial", DailyLimit: 24, CacheSeconds: 3600, Endpoint: "/accounts"}, credential{Username: access.User.Username(), Password: password, Endpoint: endpoint}, nil
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
	r, c, err := s.readCredential(rec)
	if err != nil {
		return nil, err
	}
	if rec.Kind == "oauth" && !s.cfg.OAuthEnabled || rec.Kind == "vendor" && !s.cfg.VendorEnabled {
		return nil, ErrDisabled
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead && !r.AllowWrites {
		return nil, fmt.Errorf("%w: connection grants read access only", ErrEgress)
	}
	if r.Endpoint != "" && req.URL.EscapedPath() != r.Endpoint {
		return nil, fmt.Errorf("%w: this recipe only permits its accounts endpoint", ErrEgress)
	}
	if rec.Provider == "simplefin" {
		if err = validateSimpleFINWindow(req.URL); err != nil {
			return nil, err
		}
	}
	// A stable public alias keeps the claimed access URL's private path out of
	// artifact memory, transcripts and exports. Rewrite before the gate validates.
	if rec.Provider == "simplefin" {
		req = req.Clone(ctx)
		copied := *req.URL
		req.URL = &copied
		req.URL.Path = c.Endpoint
		req.URL.RawPath = ""
	}
	hash := requestCacheKey(req)
	cacheable := r.CacheSeconds > 0 && req.Method == http.MethodGet
	if cacheable {
		sealed, e := s.cfg.Store.GetConnectionCache(ctx, rec.ID, hash, s.cfg.Now())
		if e == nil {
			plain, e := s.cfg.Box.Decrypt(sealed)
			if e != nil {
				return nil, errors.New("connection cache could not be decrypted")
			}
			cached, e := decodeCachedResponse(plain)
			if e != nil {
				return nil, e
			}
			_ = s.cfg.Store.TouchConnection(ctx, rec.ID)
			return cached, nil
		} else if !errors.Is(e, store.ErrNotFound) {
			return nil, e
		}
	}
	if err = s.cfg.Store.ReserveConnectionRequest(ctx, rec.ID, r.DailyLimit, s.cfg.Now()); err != nil {
		return nil, err
	}
	var response *egress.Response
	{
		var engine Engine = NativeEngine{}
		if rec.Kind == "oauth" {
			engine = oauthEngine{s: s}
		}
		if rec.Kind == "vendor" {
			engine = vendorEngine{s: s}
		}
		response, err = gate.DoAuthorized(ctx, req, func(hop *http.Request) error {
			if requestOrigin(hop.URL) != target {
				return ErrEgress
			}
			if rec.Provider == "simplefin" && hop.URL.Path != c.Endpoint {
				return ErrEgress
			}
			return engine.Authorize(ctx, hop, rec, r, &c)
		})
	}
	if err != nil {
		if errors.Is(err, ErrEgress) || errors.Is(err, ErrStandin) || errors.Is(err, ErrDisabled) {
			return nil, err
		}
		return nil, errors.New("connection provider request failed")
	}
	if err = s.scrubResponse(rec, viewerID, r, c, response); err != nil {
		return nil, err
	}
	if err = s.cfg.Store.TouchConnection(ctx, rec.ID); err != nil {
		return nil, err
	}
	if cacheable && response.Status >= 200 && response.Status < 300 {
		raw, e := json.Marshal(response)
		if e != nil {
			return nil, e
		}
		if len(raw) > maxConnectionCacheJSONBytes {
			return nil, ErrResponseSize
		}
		sealed, e := s.cfg.Box.Encrypt(string(raw))
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
	sum := sha256.Sum256([]byte(req.Method + "\n" + req.URL.String() + "\n" + req.Header.Get("Accept")))
	return hex.EncodeToString(sum[:])
}
func validateSimpleFINWindow(u *url.URL) error {
	q := u.Query()
	var start, end int64
	_, e1 := fmt.Sscan(q.Get("start-date"), &start)
	_, e2 := fmt.Sscan(q.Get("end-date"), &end)
	if e1 == nil && e2 == nil && (end < start || end-start > int64(90*24*time.Hour/time.Second)) {
		return fmt.Errorf("%w: SimpleFIN windows may cover at most 90 days", ErrInvalid)
	}
	return nil
}
