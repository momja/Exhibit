package sprites

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestQuotaRetriesAndCapacityErrors(t *testing.T) {
	for _, scenario := range []string{"transient-quota", "exhausted-quota", "org-quota-400", "authorization", "retry-after", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFakeAPI(t)
			var attempts atomic.Int64
			config := testConfig(f)
			config.CreateInterval = 12 * time.Millisecond
			if scenario == "deadline" {
				config.StartupTimeout = 25 * time.Millisecond
			}
			f.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method != "POST" || r.URL.Path != "/v1/sprites" {
					return false
				}
				n := attempts.Add(1)
				switch scenario {
				case "transient-quota":
					if n > 2 {
						return false
					}
				case "authorization":
					http.Error(w, managementToken, 401)
					return true
				case "retry-after":
					w.Header().Set("Retry-After", "3600")
				case "org-quota-400":
					http.Error(w, `{"error":"max sprites per org exceeded"}`, 400)
					return true
				case "deadline":
					_, _ = io.Copy(io.Discard, r.Body)
					select {
					case <-r.Context().Done():
					case <-time.After(100 * time.Millisecond):
					}
					return true
				}
				http.Error(w, "rate limit exceeded", 429)
				return true
			}
			p, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			before := time.Now()
			endpoint, err := p.Provision(context.Background(), testRequest())
			if time.Since(before) > time.Second {
				t.Fatal("quota handling exceeded bounded startup")
			}
			wantAttempts := int64(3)
			switch scenario {
			case "transient-quota":
				if err != nil {
					t.Fatal(err)
				}
				endpoint.CloseIdleConnections()
			case "authorization":
				wantAttempts = 1
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Status != 401 {
					t.Errorf("expected auth error: %v", err)
				}
			case "retry-after":
				wantAttempts = 1
				if !errors.Is(err, ErrCapacity) {
					t.Errorf("expected capacity refusal: %v", err)
				}
			case "deadline":
				wantAttempts = 1
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("expected bounded deadline: %v", err)
				}
			default:
				if !errors.Is(err, ErrCapacity) {
					t.Errorf("expected capacity refusal: %v", err)
				}
			}
			if attempts.Load() != wantAttempts {
				t.Errorf("got %d create attempts, want %d", attempts.Load(), wantAttempts)
			}
			if scenario == "transient-quota" && time.Since(before) < 24*time.Millisecond {
				t.Error("creation-rate backoff was bypassed")
			}
			if err != nil && strings.Contains(err.Error(), managementToken) {
				t.Error("management error leaked credential")
			}
		})
	}
}

func TestInventoryLimitsPaginationAndPrefixIsolation(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "openapi", true: "go-sdk"}[nested], func(t *testing.T) {
			f := newFakeAPI(t)
			f.nestedOrg = nested
			f.pageSize = 1
			p := newTestProvisioner(t, f, func(c *Config) { c.MaxWorkers = 2 })
			first := testRequest().Identity
			second := first
			second.ExecutionID = "attempt-2"
			f.add(p.Name(first), "cold", time.Hour)
			f.add(p.Name(second), "cold", time.Hour)
			f.add("exhibit-test-unrelated-name", "running", time.Hour)
			f.add("another-deployment-00000000000000000000000000000000", "warm", time.Hour)
			inventory, err := p.inventory(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(inventory.Resources) != 2 || inventory.Org.Running != 1 || inventory.Org.Warm != 1 || inventory.Org.RunningLimit != 100 {
				t.Error("incorrect filtered inventory or organization limits")
			}
			request := testRequest()
			request.Identity.ExecutionID = "attempt-3"
			if _, err := p.Provision(context.Background(), request); !errors.Is(err, ErrCapacity) {
				t.Errorf("retained workers did not bound provisioning: %v", err)
			}
			if countCalls(f, "POST", "/v1/sprites") != 0 {
				t.Error("created beyond configured capacity")
			}
			if _, err := p.Reconcile(context.Background(), nil, func(context.Context, Candidate) error { return nil }); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			_, similar := f.sprites["exhibit-test-unrelated-name"]
			_, unrelated := f.sprites["another-deployment-00000000000000000000000000000000"]
			f.mu.Unlock()
			if !similar || !unrelated {
				t.Error("reconciliation deleted a resource outside its exclusive name pattern")
			}
		})
	}
}

func TestOrgCapacityIncludesOtherDeployments(t *testing.T) {
	f := newFakeAPI(t)
	f.runningLimit = 1
	f.add("unrelated-worker", "running", time.Hour)
	p := newTestProvisioner(t, f, nil)
	if _, err := p.Provision(context.Background(), testRequest()); !errors.Is(err, ErrCapacity) {
		t.Errorf("organization ceiling ignored: %v", err)
	}
	if countCalls(f, "POST", "/v1/sprites") != 0 {
		t.Error("create attempted at organization ceiling")
	}
}

func TestStartupsAreBoundedWithoutUnboundedQueue(t *testing.T) {
	f := newFakeAPI(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	p := newTestProvisioner(t, f, func(c *Config) {
		c.MaxStartups = 1
		c.CheckReady = func(ctx context.Context, _ *Endpoint) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
	done := make(chan error, 1)
	go func() { _, err := p.Provision(context.Background(), testRequest()); done <- err }()
	<-entered
	second := testRequest()
	second.Identity.ExecutionID = "other-attempt"
	if _, err := p.Provision(context.Background(), second); !errors.Is(err, ErrCapacity) {
		t.Errorf("startup ceiling ignored: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if countCalls(f, "POST", "/v1/sprites") != 1 {
		t.Error("capacity refusal still provisioned another worker")
	}
}

func TestReconcileRetentionOrphansAndFailedRevocation(t *testing.T) {
	f := newFakeAPI(t)
	p := newTestProvisioner(t, f, func(c *Config) { c.IdleRetention = time.Minute; c.OrphanGrace = 2 * time.Minute })
	identities := map[string]Identity{}
	for _, kind := range []string{"active", "recent-idle", "expired", "blocked", "orphan", "new-orphan"} {
		id := testRequest().Identity
		id.ExecutionID = kind
		identities[kind] = id
		age := time.Hour
		if kind == "new-orphan" {
			age = time.Second
		}
		f.add(p.Name(id), "cold", age)
	}
	hookCalls := map[string]bool{}
	hook := func(_ context.Context, candidate Candidate) error {
		hookCalls[candidate.Resource.Name] = true
		if countCalls(f, "DELETE", "/v1/sprites/"+candidate.Resource.Name) != 0 {
			t.Error("resource deleted before central flush/revocation")
		}
		if candidate.Resource.Name == p.Name(identities["blocked"]) {
			return errors.New(managementToken)
		}
		if candidate.Reason == "orphan" && candidate.Identity != nil {
			t.Error("orphan invented a central identity")
		}
		if candidate.Reason == "idle" && candidate.Identity == nil {
			t.Error("known assignment lost identity")
		}
		return nil
	}
	desired := []Assignment{
		{Identity: identities["active"], Active: true},
		{Identity: identities["recent-idle"], IdleSince: time.Now().Add(-30 * time.Second)},
		{Identity: identities["expired"], IdleSince: time.Now().Add(-time.Hour)},
		{Identity: identities["blocked"], IdleSince: time.Now().Add(-time.Hour)},
	}
	result, err := p.Reconcile(context.Background(), desired, hook)
	if err == nil || len(result.Failed) != 1 || len(result.Deleted) != 2 || len(hookCalls) != 3 {
		t.Fatalf("unexpected reconciliation result: deleted=%d failed=%d err=%v", len(result.Deleted), len(result.Failed), err)
	}
	if strings.Contains(err.Error(), managementToken) {
		t.Error("hook error leaked credential")
	}
	for _, kind := range []string{"active", "recent-idle", "blocked", "new-orphan"} {
		f.mu.Lock()
		exists := f.sprites[p.Name(identities[kind])] != nil
		f.mu.Unlock()
		if !exists {
			t.Errorf("should retain %s", kind)
		}
	}
	if f.tunnels.Load() != 0 {
		t.Error("reconciliation woke Sprites")
	}
	if countCalls(f, "GET", "/v1/sprites/"+p.Name(identities["active"])+"/fs/read") != 0 {
		t.Error("reconciliation read idle worker filesystem")
	}
	if _, err := p.Reconcile(context.Background(), desired, nil); err == nil {
		t.Error("reconciliation must require explicit central cleanup hook")
	}
}

func TestManagementRedirectAndOversizedResponse(t *testing.T) {
	for _, kind := range []string{"redirect", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			var followed atomic.Bool
			target := newFakeAPI(t)
			target.fault = func(w http.ResponseWriter, r *http.Request) bool {
				followed.Store(true)
				w.WriteHeader(200)
				return true
			}
			f := newFakeAPI(t)
			f.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if kind == "redirect" {
					http.Redirect(w, r, target.server.URL+"/steal", 307)
				} else {
					_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))
				}
				return true
			}
			p := newTestProvisioner(t, f, nil)
			if _, err := p.Query(context.Background(), testRequest().Identity); err == nil {
				t.Error("invalid upstream response accepted")
			}
			if followed.Load() {
				t.Error("authenticated management request followed redirect")
			}
		})
	}
}

func TestProxyInitializationDeadlineClosesConnection(t *testing.T) {
	f := newFakeAPI(t)
	wait := make(chan struct{})
	f.initWait = wait
	p := newTestProvisioner(t, f, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := p.proxySocket(ctx, "exhibit-test-deadline")
	close(wait)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("proxy initialization ignored cancellation: %v", err)
	}
	eventuallyClosed(t, f)
}

func TestConfigurationRejectsCredentialMixingAndMutableRelease(t *testing.T) {
	f := newFakeAPI(t)
	for _, change := range []func(*Config){
		func(c *Config) { c.Token = "" },
		func(c *Config) { c.Prefix = "../shared" },
		func(c *Config) { c.BaseURL = "http://untrusted.example" },
		func(c *Config) { c.Release.SHA256 = "" },
		func(c *Config) { c.Release.Version = "../latest" },
		func(c *Config) { c.Release.URL = "https://example/worker?token=secret" },
		func(c *Config) { c.Release.NodeVersion = "latest" },
		func(c *Config) { c.Environment = map[string]string{"EXHIBIT_WORKER_TOKEN_FILE": "unsafe"} },
		func(c *Config) { c.Environment = map[string]string{"SOME_TOKEN": managementToken} },
		func(c *Config) { c.WorkerArgs = []string{"--credential", managementToken} },
		func(c *Config) { c.ReadyPath = "//another-host/ready" },
		func(c *Config) { c.Retry.Attempts = 1000 },
	} {
		config := testConfig(f)
		change(&config)
		if _, err := New(config); err == nil {
			t.Error("unsafe configuration accepted")
		}
	}
	p := newTestProvisioner(t, f, nil)
	if _, err := New(func() Config { c := testConfig(f); c.MaxWorkers = 1; return c }()); err != nil {
		t.Errorf("single-worker deployment must have a usable default startup ceiling: %v", err)
	}
	request := testRequest()
	request.EndpointToken = managementToken
	if _, err := p.Provision(context.Background(), request); err == nil {
		t.Error("management credential accepted as worker token")
	}
	if countCalls(f, "POST", "/v1/sprites") != 0 {
		t.Error("invalid configuration reached provisioning API")
	}
	token, err := NewEndpointToken()
	if err != nil || len(token) < 32 {
		t.Fatal("could not generate endpoint credential")
	}
	if strings.Contains(testConfig(f).String(), managementToken) || strings.Contains(testRequest().String(), endpointToken) {
		t.Error("formatted configuration leaks credentials")
	}
}
