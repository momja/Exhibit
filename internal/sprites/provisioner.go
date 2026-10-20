package sprites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var prefixPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,25}[a-z0-9]$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var versionPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,95}$`)
var checksumPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var runtimeVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var envPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var endpointTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/-]{32,512}=*$`)

// Provisioner has process-local startup/creation guards, not durable admission.
// Use one coordinator per prefix; the coordinator must persist assignments and
// enforce owner/global active-execution ceilings before invoking this backend.
type Provisioner struct {
	config     Config
	http       *http.Client
	startups   chan struct{}
	creates    chan struct{}
	lastCreate time.Time
	mu         sync.Mutex
	operations map[string]chan struct{}
}

var _ Backend = (*Provisioner)(nil)

func New(config Config) (*Provisioner, error) {
	if config.Token == "" || strings.ContainsAny(config.Token, "\r\n") {
		return nil, errors.New("Sprites management token is required")
	}
	if !prefixPattern.MatchString(config.Prefix) {
		return nil, errors.New("Sprites prefix must be 3–27 lowercase letters/digits/hyphens, starting with a letter and ending alphanumeric")
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.sprites.dev"
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	if config.Release.NodeVersion == "" {
		config.Release.NodeVersion = "22.22.0"
	}
	if config.Release.PiVersion == "" {
		config.Release.PiVersion = "1.3.0"
	}
	if !runtimeVersionPattern.MatchString(config.Release.NodeVersion) || !runtimeVersionPattern.MatchString(config.Release.PiVersion) {
		return nil, errors.New("Sprites Node/Pi releases require exact semantic versions")
	}
	apiURL, err := url.Parse(config.BaseURL)
	if err != nil || apiURL.Host == "" || apiURL.User != nil || apiURL.RawQuery != "" || apiURL.Fragment != "" || apiURL.Path != "" || (apiURL.Scheme != "https" && !(apiURL.Scheme == "http" && (apiURL.Hostname() == "127.0.0.1" || apiURL.Hostname() == "localhost" || apiURL.Hostname() == "::1"))) {
		return nil, errors.New("Sprites BaseURL must be an HTTPS origin (loopback HTTP allowed for tests)")
	}
	releaseURL, err := url.Parse(config.Release.URL)
	if err != nil || releaseURL.Scheme != "https" || releaseURL.Host == "" || releaseURL.User != nil || releaseURL.RawQuery != "" || releaseURL.Fragment != "" || !versionPattern.MatchString(config.Release.Version) || !checksumPattern.MatchString(config.Release.SHA256) {
		return nil, errors.New("Sprites release requires an exact version, HTTPS URL without credentials/query, and lowercase SHA256")
	}
	if strings.Contains(config.Release.URL, config.Token) {
		return nil, errors.New("Sprites management credential must not appear in release configuration")
	}
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 20
	}
	if config.MaxStartups == 0 {
		config.MaxStartups = min(2, config.MaxWorkers)
	}
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 2 * time.Minute
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.CleanupTimeout == 0 {
		config.CleanupTimeout = 15 * time.Second
	}
	if config.IdleRetention == 0 {
		config.IdleRetention = 15 * time.Minute
	}
	if config.OrphanGrace == 0 {
		config.OrphanGrace = 5 * time.Minute
	}
	if config.CreateInterval == 0 {
		config.CreateInterval = 6 * time.Second
	}
	if config.Retry.Attempts == 0 {
		config.Retry.Attempts = 4
	}
	if config.Retry.InitialDelay == 0 {
		config.Retry.InitialDelay = 250 * time.Millisecond
	}
	if config.Retry.MaxDelay == 0 {
		config.Retry.MaxDelay = 5 * time.Second
	}
	if config.ReadyPath == "" {
		config.ReadyPath = "/v1/ready"
	}
	readyURL, err := url.Parse(config.ReadyPath)
	if err != nil || readyURL.IsAbs() || readyURL.Host != "" || !strings.HasPrefix(config.ReadyPath, "/") || strings.HasPrefix(config.ReadyPath, "//") || readyURL.Fragment != "" {
		return nil, errors.New("Sprites readiness path must be an absolute path")
	}
	if config.Port < 1 || config.Port > 65535 || config.MaxWorkers < 1 || config.MaxStartups < 1 || config.MaxStartups > config.MaxWorkers || config.StartupTimeout <= 0 || config.StartupTimeout > 10*time.Minute || config.RequestTimeout <= 0 || config.CleanupTimeout <= 0 || config.IdleRetention <= 0 || config.OrphanGrace <= 0 || config.CreateInterval <= 0 || config.Retry.Attempts < 1 || config.Retry.Attempts > 10 || config.Retry.InitialDelay <= 0 || config.Retry.MaxDelay < config.Retry.InitialDelay || config.Retry.MaxDelay > time.Minute {
		return nil, errors.New("invalid Sprites limits/timeouts/retry policy")
	}
	environment := make(map[string]string, len(config.Environment))
	for key, value := range config.Environment {
		if !envPattern.MatchString(key) || strings.HasPrefix(key, "EXHIBIT_WORKER_") || key == "PATH" || key == "HOME" || key == "TMPDIR" || strings.Contains(value, config.Token) || strings.ContainsRune(value, 0) {
			return nil, errors.New("invalid or reserved Sprites worker environment")
		}
		environment[key] = value
	}
	config.Environment = environment
	config.WorkerArgs = append([]string(nil), config.WorkerArgs...)
	for _, arg := range config.WorkerArgs {
		if strings.Contains(arg, config.Token) || strings.ContainsRune(arg, 0) {
			return nil, errors.New("invalid Sprites worker argument")
		}
	}
	var client http.Client
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableKeepAlives = true
		client.Transport = transport
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Provisioner{config: config, http: &client, startups: make(chan struct{}, config.MaxStartups), creates: make(chan struct{}, 1), operations: map[string]chan struct{}{}}, nil
}

func validateIdentity(identity Identity) error {
	for _, value := range []string{identity.OwnerID, identity.SessionID, identity.ExecutionID} {
		if value == "" || len(value) > 512 || strings.ContainsRune(value, 0) {
			return errors.New("Sprites identity requires bounded owner/session/execution IDs")
		}
	}
	return nil
}

// Name is deterministic, contains no raw owner/session data, and cannot match a
// different owner or execution. It also allows recovery after uncertain creates.
func (p *Provisioner) Name(identity Identity) string {
	return p.config.Prefix + "-" + identityDigest(identity)
}
func (p *Provisioner) managedName(name string) bool {
	suffix, ok := strings.CutPrefix(name, p.config.Prefix+"-")
	return ok && digestPattern.MatchString(suffix)
}

func (p *Provisioner) validateRequest(request Request) error {
	if err := validateIdentity(request.Identity); err != nil {
		return err
	}
	if len(request.EndpointToken) > 512 || !endpointTokenPattern.MatchString(request.EndpointToken) || strings.Contains(request.EndpointToken, p.config.Token) {
		return errors.New("a separate, persisted random worker endpoint token is required")
	}
	return nil
}

func (p *Provisioner) lock(ctx context.Context, name string) (func(), error) {
	for {
		p.mu.Lock()
		wait, exists := p.operations[name]
		if !exists {
			done := make(chan struct{})
			p.operations[name] = done
			p.mu.Unlock()
			return func() { p.mu.Lock(); delete(p.operations, name); close(done); p.mu.Unlock() }, nil
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

func (p *Provisioner) startup(ctx context.Context) (context.Context, func(), error) {
	select {
	case p.startups <- struct{}{}:
	default:
		return nil, nil, fmt.Errorf("Sprites startup ceiling (%d): %w", p.config.MaxStartups, ErrCapacity)
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.StartupTimeout)
	return ctx, func() { cancel(); <-p.startups }, nil
}

// Provision is idempotent for a persisted Request. Never use it to rotate an
// existing endpoint credential or to upgrade a live release: use a new identity.
func (p *Provisioner) Provision(ctx context.Context, request Request) (_ *Endpoint, resultErr error) {
	if err := p.validateRequest(request); err != nil {
		return nil, err
	}
	ctx, finish, err := p.startup(ctx)
	if err != nil {
		return nil, err
	}
	defer finish()
	name := p.Name(request.Identity)
	unlock, err := p.lock(ctx, name)
	if err != nil {
		return nil, err
	}
	defer unlock()
	resource, fresh, err := p.ensureSprite(ctx, name)
	cleanup := fresh
	defer func() {
		if resultErr == nil || !cleanup {
			return
		}
		// Startup may have exhausted/cancelled its deadline. Cleanup still has its
		// own short budget; a failure is reported and left for metadata reconciliation.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), p.config.CleanupTimeout)
		defer cancel()
		if err := p.delete(cleanupCtx, name); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("failed startup cleanup: %w", err))
		}
	}()
	if err != nil {
		return nil, err
	}
	if resource.URLSettings != nil && resource.URLSettings.Auth != "sprite" {
		return nil, errors.New("Sprite URL must use private sprite authentication")
	}
	installed, err := p.install(ctx, request, name, fresh)
	if errors.Is(err, ErrConflict) {
		cleanup = false
		return nil, err
	}
	cleanup = cleanup || installed
	if err != nil {
		return nil, err
	}
	endpoint, err := p.endpoint(request, resource)
	if err != nil {
		return nil, err
	}
	if err := p.awaitReady(ctx, endpoint); err != nil {
		endpoint.CloseIdleConnections()
		return nil, err
	}
	return endpoint, nil
}

func (p *Provisioner) ensureSprite(ctx context.Context, name string) (Resource, bool, error) {
	select {
	case p.creates <- struct{}{}:
	case <-ctx.Done():
		return Resource{}, false, ctx.Err()
	}
	defer func() { <-p.creates }()
	resource, err := p.get(ctx, name)
	if err == nil {
		return resource, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Resource{}, false, err
	}
	inventory, err := p.inventory(ctx)
	if err != nil {
		return Resource{}, false, err
	}
	if len(inventory.Resources) >= p.config.MaxWorkers {
		return Resource{}, false, fmt.Errorf("Sprites retained-worker ceiling (%d): %w", p.config.MaxWorkers, ErrCapacity)
	}
	if inventory.Org.RunningLimit > 0 && inventory.Org.Running >= inventory.Org.RunningLimit {
		return Resource{}, false, fmt.Errorf("Sprites org running ceiling (%d): %w", inventory.Org.RunningLimit, ErrCapacity)
	}
	body := mustJSON(map[string]any{"name": name, "url_settings": map[string]string{"auth": "sprite"}})
	created := false
	err = p.retry(ctx, func() error {
		if !p.lastCreate.IsZero() {
			if err := waitContext(ctx, max(0, time.Until(p.lastCreate.Add(p.config.CreateInterval)))); err != nil {
				return err
			}
		}
		p.lastCreate = time.Now()
		data, err := p.request(ctx, "POST", "/v1/sprites", "application/json", body)
		if err == nil {
			created = true
			var response struct {
				Name string `json:"name"`
			}
			if err := decodeResponse(data, &response); err != nil {
				return err
			}
			if response.Name != name {
				return errors.New("Sprites create returned an unexpected name")
			}
			return nil
		}
		// A duplicate name or uncertain create is resolved by GET, never by minting a
		// second name. GET does not wake the Sprite. Unknown outcomes remain orphans
		// until reconciliation if the context is already exhausted.
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status == 409 || apiErr.Status >= 500 {
			existing, lookupErr := p.get(ctx, name)
			if lookupErr == nil {
				resource = existing
				return nil
			}
			if apiErr != nil && apiErr.Status == 409 && errors.Is(lookupErr, ErrNotFound) {
				return err
			}
		}
		return err
	})
	if err != nil {
		return Resource{}, created, err
	}
	if resource.Name == "" {
		resource, err = p.get(ctx, name)
	}
	return resource, created, err
}

// Query reads only management metadata. It does not connect to the Sprite, read
// files, or run a health check. Status cold/warm is platform metadata, not health.
func (p *Provisioner) Query(ctx context.Context, identity Identity) (Resource, error) {
	if err := validateIdentity(identity); err != nil {
		return Resource{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()
	return p.get(ctx, p.Name(identity))
}

// Wake reconnects on demand, checks the immutable manifest and readiness, and
// does not create a missing Sprite or replay an unfinished execution.
func (p *Provisioner) Wake(ctx context.Context, request Request) (*Endpoint, error) {
	if err := p.validateRequest(request); err != nil {
		return nil, err
	}
	ctx, finish, err := p.startup(ctx)
	if err != nil {
		return nil, err
	}
	defer finish()
	name := p.Name(request.Identity)
	unlock, err := p.lock(ctx, name)
	if err != nil {
		return nil, err
	}
	defer unlock()
	resource, err := p.get(ctx, name)
	if err != nil {
		return nil, err
	}
	existing, err := p.readManifest(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != expectedManifest(request.Identity, request.EndpointToken, p.bootstrapScript(request.Identity)) {
		return nil, ErrConflict
	}
	endpoint, err := p.endpoint(request, resource)
	if err != nil {
		return nil, err
	}
	if err := p.awaitReady(ctx, endpoint); err != nil {
		endpoint.CloseIdleConnections()
		return nil, err
	}
	return endpoint, nil
}

// Destroy is infrastructure-only. First stop/fence execution, flush acknowledged
// state and revoke grants in the parent coordinator. Missing Sprites are success.
func (p *Provisioner) Destroy(ctx context.Context, identity Identity) error {
	if err := validateIdentity(identity); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.CleanupTimeout)
	defer cancel()
	unlock, err := p.lock(ctx, p.Name(identity))
	if err != nil {
		return err
	}
	defer unlock()
	return p.delete(ctx, p.Name(identity))
}

// Reconcile uses the authoritative central snapshot and management metadata only.
// Serialize snapshot capture/reconciliation with central assignment admission;
// otherwise a stale snapshot could delete a newly admitted assignment.
func (p *Provisioner) Reconcile(ctx context.Context, assignments []Assignment, beforeDestroy BeforeDestroy) (ReconcileResult, error) {
	var result ReconcileResult
	if beforeDestroy == nil {
		return result, errors.New("Sprites reconciliation requires a flush/revoke hook")
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.StartupTimeout)
	defer cancel()
	desired := map[string]Assignment{}
	for _, assignment := range assignments {
		if err := validateIdentity(assignment.Identity); err != nil {
			return result, err
		}
		if !assignment.Active && assignment.IdleSince.IsZero() {
			return result, errors.New("idle Sprite assignment requires IdleSince")
		}
		name := p.Name(assignment.Identity)
		if _, duplicate := desired[name]; duplicate {
			return result, errors.New("duplicate Sprite assignment in reconciliation snapshot")
		}
		desired[name] = assignment
	}
	inventory, err := p.inventory(ctx)
	if err != nil {
		return result, err
	}
	result.Org = inventory.Org
	var failures []error
	now := time.Now()
	for _, resource := range inventory.Resources {
		assignment, known := desired[resource.Name]
		candidate := Candidate{Resource: resource, Reason: "orphan"}
		if known {
			if assignment.Active || now.Sub(assignment.IdleSince) < p.config.IdleRetention {
				continue
			}
			candidate.Identity = &assignment.Identity
			candidate.Reason = "idle"
		} else if resource.CreatedAt.IsZero() || now.Sub(resource.CreatedAt) < p.config.OrphanGrace {
			continue
		}
		// Never race an in-process startup or destroy. Next sweep can retry it.
		p.mu.Lock()
		_, busy := p.operations[resource.Name]
		if busy {
			p.mu.Unlock()
			continue
		}
		done := make(chan struct{})
		p.operations[resource.Name] = done
		p.mu.Unlock()
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, p.config.CleanupTimeout)
		cleanupErr := beforeDestroy(cleanupCtx, candidate)
		if cleanupErr != nil {
			// Hooks may contain remote error bodies: do not return them verbatim.
			cleanupErr = errors.New("central flush/revoke hook failed")
		} else {
			cleanupErr = p.delete(cleanupCtx, resource.Name)
		}
		cleanupCancel()
		p.mu.Lock()
		delete(p.operations, resource.Name)
		close(done)
		p.mu.Unlock()
		if cleanupErr != nil {
			result.Failed = append(result.Failed, resource.Name)
			failures = append(failures, fmt.Errorf("cleanup %s: %w", resource.Name, cleanupErr))
		} else {
			result.Deleted = append(result.Deleted, resource.Name)
		}
	}
	return result, errors.Join(failures...)
}
