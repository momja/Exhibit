package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/api"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/mockllm"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/sprites"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

type testSpriteWorker struct {
	server   *httptest.Server
	service  *agentexec.Service
	calls    int
	commands int
}
type testSpriteFleet struct {
	mu          sync.Mutex
	journal     *workerJournal
	pi          string
	root        string
	workers     map[string]*testSpriteWorker
	provisioned int
	deleted     int
}

func (f *testSpriteFleet) Provision(ctx context.Context, request sprites.Request) (*sprites.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	persisted, ok := f.journal.snapshot()[request.Identity.ExecutionID]
	if !ok || persisted.Request != request {
		return nil, errors.New("provision before durable request")
	}
	if f.workers[request.Identity.ExecutionID] != nil {
		return nil, errors.New("duplicate provisioning")
	}
	service, err := agentexec.NewService(agentexec.ServiceConfig{Token: request.EndpointToken, ExecutionID: request.Identity.ExecutionID, SessionID: request.Identity.SessionID, Runner: agentexec.Local{PiBin: f.pi, WorkRoot: f.root}})
	if err != nil {
		return nil, err
	}
	worker := &testSpriteWorker{service: service}
	worker.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		worker.calls++
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commands") {
			worker.commands++
		}
		current := worker.service
		f.mu.Unlock()
		current.ServeHTTP(w, r)
	}))
	f.workers[request.Identity.ExecutionID] = worker
	f.provisioned++
	return &sprites.Endpoint{Identity: request.Identity, URL: worker.server.URL, EndpointToken: request.EndpointToken, Client: worker.server.Client(), Transport: worker.server.Client().Transport}, nil
}
func (f *testSpriteFleet) Wake(ctx context.Context, request sprites.Request) (*sprites.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	worker := f.workers[request.Identity.ExecutionID]
	if worker == nil {
		return nil, sprites.ErrNotFound
	}
	return &sprites.Endpoint{Identity: request.Identity, URL: worker.server.URL, EndpointToken: request.EndpointToken, Client: worker.server.Client(), Transport: worker.server.Client().Transport}, nil
}
func (f *testSpriteFleet) Query(ctx context.Context, id sprites.Identity) (sprites.Resource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.workers[id.ExecutionID] == nil {
		return sprites.Resource{}, sprites.ErrNotFound
	}
	return sprites.Resource{Name: id.ExecutionID}, nil
}
func (f *testSpriteFleet) Destroy(ctx context.Context, id sprites.Identity) error {
	f.mu.Lock()
	worker := f.workers[id.ExecutionID]
	delete(f.workers, id.ExecutionID)
	if worker != nil {
		f.deleted++
	}
	f.mu.Unlock()
	if worker != nil {
		err := worker.service.Shutdown(ctx)
		worker.server.Close()
		return err
	}
	return nil
}
func (f *testSpriteFleet) Reconcile(ctx context.Context, assignments []sprites.Assignment, before sprites.BeforeDestroy) (sprites.ReconcileResult, error) {
	var result sprites.ReconcileResult
	for _, assignment := range assignments {
		if assignment.Active {
			continue
		}
		id := assignment.Identity
		if err := before(ctx, sprites.Candidate{Identity: &id, Resource: sprites.Resource{Name: id.ExecutionID}}); err != nil {
			return result, err
		}
		if err := f.Destroy(ctx, id); err != nil {
			return result, err
		}
		result.Deleted = append(result.Deleted, id.ExecutionID)
	}
	return result, nil
}

func (f *testSpriteFleet) coldRestart(ctx context.Context, request sprites.Request) error {
	f.mu.Lock()
	worker := f.workers[request.Identity.ExecutionID]
	old := worker.service
	f.mu.Unlock()
	if err := old.Shutdown(ctx); err != nil {
		return err
	}
	service, err := agentexec.NewService(agentexec.ServiceConfig{Token: request.EndpointToken, ExecutionID: request.Identity.ExecutionID, SessionID: request.Identity.SessionID, Runner: agentexec.Local{PiBin: f.pi, WorkRoot: f.root}})
	if err != nil {
		return err
	}
	f.mu.Lock()
	worker.service = service
	f.mu.Unlock()
	return nil
}

func workerTestFixture(t *testing.T, limit int, configure ...func(*agent.Config)) (*workerCoordinator, *agent.Manager, *testSpriteFleet, *store.SQLiteStore) {
	t.Helper()
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pinned Pi unavailable")
	}
	directory := t.TempDir()
	st, err := store.OpenSQLite(directory + "/app.db")
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	box, err := secrets.Load("test-core-secrets", "")
	require.NoError(t, err)
	registry := agentscope.NewRegistry()
	config := agentExecutionConfig{Mode: "remote", MaxWorkers: limit, StartupTimeout: 10 * time.Second, IdleRetention: time.Minute}
	c, err := newWorkerCoordinator(context.Background(), config, directory, box, st, registry, false)
	require.NoError(t, err)
	fleet := &testSpriteFleet{journal: c.journal, pi: pi, root: t.TempDir(), workers: map[string]*testSpriteWorker{}}
	c.backend = fleet
	c.config.Mode = "sprites"
	mockProvider := mockllm.Handler()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		mockProvider.ServeHTTP(response, r)
		for name, values := range response.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(response.Code)
		usage := `data: {"id":"metered","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":240,"completion_tokens":60,"total_tokens":300,"prompt_tokens_details":{"cached_tokens":40}}}`
		_, _ = w.Write([]byte(strings.Replace(response.Body.String(), "data: [DONE]", usage+"\n\ndata: [DONE]", 1)))
	}))
	t.Cleanup(provider.Close)
	var router http.Handler
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) }))
	t.Cleanup(core.Close)
	runner, err := c.runner("/missing-core-pi", "/missing-core-scratch")
	require.NoError(t, err)
	managerConfig := agent.Config{Runner: runner, ReserveExecution: c.reserve, MaxSessions: config.MaxWorkers, Credentials: registry, APIBaseURL: core.URL, MockLLMURL: provider.URL}
	for _, apply := range configure {
		apply(&managerConfig)
	}
	manager, err := agent.New(managerConfig, st)
	require.NoError(t, err)
	blobs, err := blob.NewFSStore(directory + "/blobs")
	require.NoError(t, err)
	router = api.NewRouter(api.Config{Store: st, Blob: blobs, Secrets: box, AuthToken: "operator", Agent: manager, AgentControl: c.control, AgentCredentials: registry, AppOrigin: core.URL, RenderOrigin: "http://render.test", MockEnabled: true})
	c.start(manager)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
		close(c.stop)
		<-c.done
		_ = c.reconcile(ctx)
	})
	return c, manager, fleet, st
}
func workerTestPrompt(t *testing.T, session *agent.Session, events <-chan []byte, prompt string, want string) {
	t.Helper()
	require.NoError(t, session.Prompt(context.Background(), prompt, nil, nil))
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	found := want == ""
	for {
		select {
		case event := <-events:
			if strings.Contains(string(event), want) {
				found = true
			}
			if strings.Contains(string(event), `"type":"agent_settled"`) {
				require.True(t, found, "missing %s", want)
				return
			}
		case <-timeout.C:
			t.Fatal("worker failed to settle")
		}
	}
}
func TestWorkerCoordinatorRealHTTPConcurrentOwnersResumeAndCentralAdmission(t *testing.T) {
	c, manager, fleet, st := workerTestFixture(t, 2)
	sessions := make([]*agent.Session, 2)
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for i := range sessions {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			var err error
			sessions[i], err = manager.Create(context.Background(), agent.CreateOpts{OwnerID: int64(i + 1), Provider: "exhibit-mock", APIKey: "only-assigned-key"})
			errs <- err
		}(i)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	streams := map[*agent.Session]<-chan []byte{}
	for _, session := range sessions {
		events, unsubscribe := session.Subscribe()
		streams[session] = events
		t.Cleanup(unsubscribe)
	}
	send := func(session *agent.Session, prompt, want string) {
		events := streams[session]
		if events == nil {
			var unsubscribe func()
			events, unsubscribe = session.Subscribe()
			streams[session] = events
			t.Cleanup(unsubscribe)
		}
		workerTestPrompt(t, session, events, prompt, want)
	}
	fleet.mu.Lock()
	require.Equal(t, 2, fleet.provisioned)
	fleet.mu.Unlock()
	_, err := manager.Create(context.Background(), agent.CreateOpts{OwnerID: 3, Provider: "exhibit-mock", APIKey: "third"})
	require.True(t, errors.Is(err, agent.ErrInstanceSessionLimit) || errors.Is(err, agentcontrol.ErrFleetLimit), err)
	_, err = c.reserve(context.Background(), agentexec.Identity{OwnerID: 3, SessionID: "overflow", ExecutionID: "overflow-attempt"}, nil)
	require.ErrorIs(t, err, agentcontrol.ErrFleetLimit)
	for i, session := range sessions {
		group.Add(1)
		go func(i int, session *agent.Session) {
			defer group.Done()
			workerTestPrompt(t, session, streams[session], []string{"build a counter", "build another counter"}[i], "exhibit_artifact_saved")
		}(i, session)
	}
	group.Wait()
	require.NotEqual(t, sessions[0].ArtifactID(), sessions[1].ArtifactID())
	send(sessions[0], "make it blue", "exhibit_artifact_saved")
	send(sessions[0], "set state count to 7", "exhibit_state_changed")
	ctx := context.Background()
	state, err := st.GetState(ctx, 1, sessions[0].ArtifactID(), 1)
	require.NoError(t, err)
	require.Equal(t, "7", state["count"])
	require.Eventually(t, func() bool {
		records := c.journal.snapshot()
		for _, record := range records {
			if record.IdleSince.IsZero() {
				return false
			}
		}
		return true
	}, 3*time.Second, 10*time.Millisecond)

	countRequests := func() int {
		fleet.mu.Lock()
		defer fleet.mu.Unlock()
		count := 0
		for _, worker := range fleet.workers {
			count += worker.calls
		}
		return count
	}
	// The journal's IdleSince precedes the final /idle handshake. Wait for that
	// bounded handshake and post-settle usage reconciliation before measuring.
	lastCount, quietSince := countRequests(), time.Now()
	require.Eventually(t, func() bool {
		count := countRequests()
		if count != lastCount {
			lastCount = count
			quietSince = time.Now()
		}
		return time.Since(quietSince) >= 150*time.Millisecond
	}, 3*time.Second, 10*time.Millisecond)
	calls := countRequests()
	c.renew()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, calls, countRequests(), "idle lease renewal must not wake a worker")

	var firstRecord workerRecord
	for _, record := range c.journal.snapshot() {
		if record.OwnerID == 1 {
			firstRecord = record
		}
	}
	require.NoError(t, fleet.coldRestart(ctx, firstRecord.Request))
	send(sessions[0], "make it green after cold wake", "exhibit_artifact_saved")
	require.Eventually(t, func() bool {
		record := c.journal.snapshot()[firstRecord.Assignment.ExecutionID]
		return !record.IdleSince.IsZero()
	}, 3*time.Second, 10*time.Millisecond)
	// Destroy the complete Sprite after confirmed settlement. A new explicit
	// prompt provisions the immutable request and restores central history.
	require.NoError(t, fleet.Destroy(ctx, firstRecord.Request.Identity))
	send(sessions[0], "make it orange after complete replacement", "exhibit_artifact_saved")
	// The mock model's price table is intentionally zero. A trusted priced
	// receipt proves nonzero lifetime spend survives replacement, and retries
	// cannot double count it; real model tokens still prove new work occurred.
	supervisorToken, err := c.control.IssueSupervisor(ctx, firstRecord.Assignment.Identity, "user")
	require.NoError(t, err)
	supervisor, err := c.control.AuthenticateSupervisor(ctx, supervisorToken)
	require.NoError(t, err)
	usage := agentcontrol.Usage{EventID: "priced-tool-result", Source: "tool", Provider: "test", Model: "test", CostMicros: 1234}
	inserted, err := c.control.ReportUsage(ctx, supervisor, usage)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = c.control.ReportUsage(ctx, supervisor, usage)
	require.NoError(t, err)
	require.False(t, inserted)
	baseline, err := st.SessionAgentSpend(ctx, 1, sessions[0].ID)
	require.NoError(t, err)
	require.Greater(t, baseline.CostMicros, int64(0))
	firstID, artifactID := sessions[0].ID, sessions[0].ArtifactID()
	manager.Close(1, firstID)
	require.Eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return len(c.live) == 1 }, 5*time.Second, 10*time.Millisecond)
	var resumed *agent.Session
	require.Eventually(t, func() bool {
		resumed, err = manager.Create(ctx, agent.CreateOpts{OwnerID: 1, Provider: "exhibit-mock", APIKey: "current-assigned-key", ArtifactID: artifactID, ResumeSessionID: firstID})
		return !errors.Is(err, agent.ErrSessionLive)
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, firstID, resumed.ID)
	send(resumed, "make it red", "exhibit_artifact_saved")
	totals, err := st.SessionAgentSpend(ctx, 1, firstID)
	require.NoError(t, err)
	require.Equal(t, baseline.CostMicros, totals.CostMicros)
	require.Greater(t, totals.InputTokens+totals.OutputTokens, baseline.InputTokens+baseline.OutputTokens)
	_, err = st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: "local:owner1", Email: "owner1", PasswordHash: "test-only"})
	require.NoError(t, err)
	_, err = st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: "local:owner2", Email: "owner2", PasswordHash: "test-only"})
	require.NoError(t, err)
	require.NoError(t, st.SetUserAdmin(ctx, 2, true))
	_, err = st.DeleteAccount(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, c.deleteOwner(ctx, 1))
	for _, record := range c.journal.snapshot() {
		require.NotEqual(t, int64(1), record.OwnerID)
	}
	// Unrelated worker survives the other owner's erasure.
	require.NotNil(t, manager.Get(2, sessions[1].ID))
}

func TestRemoteWorkersShareCentralPlatformPolicyAndInstanceBudget(t *testing.T) {
	limit := int64(1_000_000_000)
	c, manager, fleet, st := workerTestFixture(t, 3, func(cfg *agent.Config) {
		cfg.Caps = agent.SpendCaps{InstanceMicrosPerMonth: &limit}
		cfg.Guardrail = &agent.Guardrail{Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "guard-only-key"}
		cfg.HideModelIdentity = true
	})
	c.paidBy = "platform"
	ctx := context.Background()
	first, err := manager.Create(ctx, agent.CreateOpts{OwnerID: 1, Provider: "exhibit-mock", APIKey: "assigned-platform-key", PlatformPaid: true})
	require.NoError(t, err)
	second, err := manager.Create(ctx, agent.CreateOpts{OwnerID: 2, Provider: "exhibit-mock", APIKey: "assigned-platform-key", PlatformPaid: true})
	require.NoError(t, err)
	events, unsubscribe := first.Subscribe()
	defer unsubscribe()
	require.NoError(t, first.Prompt(ctx, "mock-policy-violation", nil, nil))
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			return strings.Contains(string(event), "exhibit_guard_blocked")
		default:
			return false
		}
	}, 10*time.Second, 10*time.Millisecond)
	require.Empty(t, first.ArtifactID(), "screened prompt must not create an artifact")
	workerTestPrompt(t, first, events, "build a counter", "exhibit_artifact_saved")
	// Trusted late reporting from a different execution/owner consumes the SAME
	// instance budget; neither daemon has its own quota or ledger.
	require.NoError(t, st.RecordAgentUsage(ctx, store.AgentUsage{OwnerID: 9, SessionID: "terminated-execution", PaidBy: store.PaidByPlatform, Provider: "test", Model: "test", Source: store.UsageSourceModel, CostMicros: limit}))
	require.Eventually(t, func() bool { return manager.ProbeCaps(ctx, 2) != nil }, 2*time.Second, 10*time.Millisecond)
	refusal := manager.ProbeCaps(ctx, 2)
	require.Equal(t, "instance", refusal.Which)
	var secondExecution string
	for execution, record := range c.journal.snapshot() {
		if record.OwnerID == 2 {
			secondExecution = execution
		}
	}
	fleet.mu.Lock()
	calls := fleet.workers[secondExecution].commands
	fleet.mu.Unlock()
	err = second.Prompt(ctx, "this must never reach worker", nil, nil)
	var capRefusal *agent.CapRefusal
	require.ErrorAs(t, err, &capRefusal)
	require.Equal(t, "instance", capRefusal.Which)
	fleet.mu.Lock()
	after := fleet.workers[secondExecution].commands
	fleet.mu.Unlock()
	require.Equal(t, calls, after, "refused spending must not send a command to worker or guardrail")
	totals, err := st.InstanceAgentSpend(ctx, time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -time.Now().UTC().Day()+1))
	require.NoError(t, err)
	require.GreaterOrEqual(t, totals.CostMicros, limit)
}

func TestSlowExecutionGuardDoesNotBlockOtherLifecycleOrLeaseRenewal(t *testing.T) {
	c, _, _, _ := workerTestFixture(t, 2)
	ctx := context.Background()
	first, err := c.reserve(ctx, agentexec.Identity{OwnerID: 1, SessionID: "blocked-conversation", ExecutionID: "blocked-attempt"}, nil)
	require.NoError(t, err)
	second, err := c.reserve(ctx, agentexec.Identity{OwnerID: 2, SessionID: "other-conversation", ExecutionID: "other-attempt"}, nil)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, second)
	records := c.journal.snapshot()
	a, b := records["blocked-attempt"].Assignment.Identity, records["other-attempt"].Assignment.Identity
	releaseGuard, err := c.control.Guard(ctx, a)
	require.NoError(t, err)
	defer releaseGuard()
	blocked := make(chan error, 1)
	go func() { blocked <- c.active(ctx, a) }()
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	require.NoError(t, c.active(short, b), "one guarded save must not block another worker's activation")
	before, err := c.store.(agentcontrol.Repository).GetAgentExecution(ctx, b, time.Now())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	renewed := make(chan struct{})
	go func() { c.renew(); close(renewed) }()
	require.Eventually(t, func() bool {
		after, err := c.store.(agentcontrol.Repository).GetAgentExecution(ctx, b, time.Now())
		return err == nil && after.LeaseUntil.After(before.LeaseUntil)
	}, 300*time.Millisecond, 5*time.Millisecond)
	releaseGuard()
	require.NoError(t, <-blocked)
	<-renewed
	require.NoError(t, c.release(ctx, a, agentcontrol.Failed))
	require.NoError(t, c.release(ctx, b, agentcontrol.Failed))
}

type blockingCleanupFleet struct {
	sprites.Backend
	entered chan struct{}
	resume  chan struct{}
}

func (f blockingCleanupFleet) Reconcile(ctx context.Context, assignments []sprites.Assignment, before sprites.BeforeDestroy) (sprites.ReconcileResult, error) {
	close(f.entered)
	select {
	case <-f.resume:
		return sprites.ReconcileResult{}, nil
	case <-ctx.Done():
		return sprites.ReconcileResult{}, ctx.Err()
	}
}
func TestBlockedManagementCleanupDoesNotBlockWorkerActivation(t *testing.T) {
	c, _, fleet, _ := workerTestFixture(t, 2)
	ctx := context.Background()
	_, err := c.reserve(ctx, agentexec.Identity{OwnerID: 1, SessionID: "live", ExecutionID: "live-attempt"}, nil)
	require.NoError(t, err)
	id := c.journal.snapshot()["live-attempt"].Assignment.Identity
	blocker := blockingCleanupFleet{Backend: fleet, entered: make(chan struct{}), resume: make(chan struct{})}
	c.backend = blocker
	finished := make(chan error, 1)
	go func() { finished <- c.reconcile(ctx) }()
	<-blocker.entered
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	require.NoError(t, c.active(short, id))
	close(blocker.resume)
	require.NoError(t, <-finished)
	c.backend = fleet
	require.NoError(t, c.release(ctx, id, agentcontrol.Failed))
}

func TestCoreRestartFencesOldGrantAndCleansJournaledSprite(t *testing.T) {
	c, _, fleet, st := workerTestFixture(t, 2)
	ctx := context.Background()
	reservation, err := c.reserve(ctx, agentexec.Identity{OwnerID: 7, SessionID: "restart-conversation", ExecutionID: "restart-attempt"}, nil)
	require.NoError(t, err)
	_, err = c.acquire(ctx, agentexec.Identity{OwnerID: 7, SessionID: "restart-conversation", ExecutionID: "restart-attempt"})
	require.NoError(t, err)
	require.NotNil(t, c.registry.Resolve(reservation.Grant.Token()))
	// Simulate the replacement core before any runtime reattachment. It shares
	// only durable storage/key material; live worker/grant maps start empty.
	box, err := secrets.Load("test-core-secrets", "")
	require.NoError(t, err)
	config := c.config
	config.Mode = "remote"
	replacement, err := newWorkerCoordinator(ctx, config, filepath.Dir(filepath.Dir(c.journal.path)), box, st, agentscope.NewRegistry(), false)
	require.NoError(t, err)
	require.Nil(t, c.registry.Resolve(reservation.Grant.Token()), "prior worker must lose effective write authority on core restart")
	record := replacement.journal.snapshot()["restart-attempt"]
	require.Equal(t, int64(7), record.Assignment.OwnerID)
	require.Equal(t, agentcontrol.Interrupted, record.Assignment.Status)
	require.NotEmpty(t, record.Request.EndpointToken)
	replacement.backend = fleet
	require.NoError(t, replacement.reconcile(ctx))
	require.Empty(t, replacement.journal.snapshot(), "remove provisioning credentials only after confirmed resource cleanup")
	_, err = fleet.Query(ctx, record.Request.Identity)
	require.ErrorIs(t, err, sprites.ErrNotFound)
	// Retire the simulated old core's ephemeral maps so its test cleanup does
	// not overwrite the replacement's durable journal.
	c.retire("restart-attempt")
	require.NoError(t, c.journal.update("restart-attempt", nil))
}

type unavailableCleanupFleet struct {
	sprites.Backend
	unavailable bool
}

func (f *unavailableCleanupFleet) Reconcile(ctx context.Context, a []sprites.Assignment, b sprites.BeforeDestroy) (sprites.ReconcileResult, error) {
	if f.unavailable {
		return sprites.ReconcileResult{}, errors.New("management unavailable")
	}
	return f.Backend.Reconcile(ctx, a, b)
}
func (f *unavailableCleanupFleet) Destroy(ctx context.Context, id sprites.Identity) error {
	if f.unavailable {
		return errors.New("management unavailable")
	}
	return f.Backend.Destroy(ctx, id)
}

func TestFailedRestartCleanupPreservesFreshSessionForExistingOwner(t *testing.T) {
	old, _, fleet, st := workerTestFixture(t, 4)
	ctx := context.Background()
	_, err := st.UpsertUser(ctx, "existing-owner", "owner@example.test")
	require.NoError(t, err)
	_, err = old.reserve(ctx, agentexec.Identity{OwnerID: 1, SessionID: "old", ExecutionID: "old-attempt"}, nil)
	require.NoError(t, err)
	_, err = old.acquire(ctx, agentexec.Identity{OwnerID: 1, SessionID: "old", ExecutionID: "old-attempt"})
	require.NoError(t, err)
	box, err := secrets.Load("test-core-secrets", "")
	require.NoError(t, err)
	registry := agentscope.NewRegistry()
	config := old.config
	config.Mode = "remote"
	replacement, err := newWorkerCoordinator(ctx, config, filepath.Dir(filepath.Dir(old.journal.path)), box, st, registry, false)
	require.NoError(t, err)
	unavailable := &unavailableCleanupFleet{Backend: fleet, unavailable: true}
	replacement.backend = unavailable
	replacement.config.Mode = "sprites"
	fleet.journal = replacement.journal
	require.Error(t, replacement.reconcile(ctx))
	require.False(t, replacement.journal.snapshot()["old-attempt"].EraseOwner, "restart cleanup is not account erasure")
	runner, err := replacement.runner("/missing-core-pi", "/missing-core-scratch")
	require.NoError(t, err)
	manager, err := agent.New(agent.Config{Runner: runner, ReserveExecution: replacement.reserve, Credentials: registry, MockLLMURL: "http://unused.test", APIBaseURL: "http://unused.test"}, st)
	require.NoError(t, err)
	replacement.manager = manager
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
		unavailable.unavailable = false
		_ = replacement.reconcile(ctx)
		old.retire("old-attempt")
		_ = old.journal.update("old-attempt", nil)
	})
	fresh, err := manager.Create(ctx, agent.CreateOpts{OwnerID: 1, Provider: "exhibit-mock", APIKey: "assigned-fresh-key"})
	require.NoError(t, err)
	unavailable.unavailable = false
	require.NoError(t, replacement.reconcile(ctx))
	require.NotNil(t, manager.Get(1, fresh.ID), "old-attempt cleanup must preserve this owner's fresh execution")
	_, err = st.GetUser(ctx, 1)
	require.NoError(t, err)
}

func TestCoreRestartReclaimsStaticDaemonAssignmentCapacity(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pinned Pi unavailable")
	}
	ctx := context.Background()
	daemon, err := agentexec.NewService(agentexec.ServiceConfig{Token: strings.Repeat("static-worker-token-", 3), LocalAssignments: true, MaxAssignments: 1, Runner: agentexec.Local{PiBin: pi, WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(daemon)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = daemon.Shutdown(cleanup)
		server.Close()
	})
	directory := t.TempDir()
	st, err := store.OpenSQLite(directory + "/app.db")
	require.NoError(t, err)
	defer st.Close()
	box, err := secrets.Load("static-core-secret", "")
	require.NoError(t, err)
	config := agentExecutionConfig{Mode: "remote", WorkerURL: server.URL, WorkerToken: strings.Repeat("static-worker-token-", 3), MaxWorkers: 2, StartupTimeout: 10 * time.Second, IdleRetention: time.Minute}
	old, err := newWorkerCoordinator(ctx, config, directory, box, st, agentscope.NewRegistry(), false)
	require.NoError(t, err)
	start := func(c *workerCoordinator, id string) agentexec.Process {
		identity := agentexec.Identity{OwnerID: 1, SessionID: id, ExecutionID: id}
		reservation, err := c.reserve(ctx, identity, nil)
		require.NoError(t, err)
		runner, err := c.runner("/missing-core-pi", "/missing-core-scratch")
		require.NoError(t, err)
		process, err := runner.Start(ctx, agentexec.StartConfig{Identity: identity, Provider: "anthropic", APIKey: "assigned-test-key", APIBaseURL: "http://unused.test", ToolToken: reservation.Grant.Token(), SystemPrompt: "no built-in tools", ExpiresAt: reservation.ExpiresAt})
		require.NoError(t, err)
		go func() { _, _ = io.Copy(io.Discard, process.Stdout()) }()
		return process
	}
	first := start(old, "old-static-attempt")
	replacement, err := newWorkerCoordinator(ctx, config, directory, box, st, agentscope.NewRegistry(), false)
	require.NoError(t, err)
	require.Empty(t, replacement.journal.snapshot(), "confirmed static close/release removes restart cleanup intent")
	// The daemon has capacity ONE. Starting another assignment proves that the
	// fenced previous core's worker memory/scratch/capacity was reclaimed.
	second := start(replacement, "new-static-attempt")
	require.NoError(t, second.Close())
	require.NoError(t, replacement.release(ctx, replacement.journal.snapshot()["new-static-attempt"].Assignment.Identity, agentcontrol.Closed))
	require.NoError(t, second.Cleanup())
	_ = first.ErasureCleanup().Cleanup()
	require.NoError(t, replacement.reconcile(ctx))
}

func TestLateUsageHTTPStopsOtherActivePlatformWorker(t *testing.T) {
	limit := int64(1_000_000)
	c, manager, _, st := workerTestFixture(t, 3, func(cfg *agent.Config) { cfg.Caps = agent.SpendCaps{InstanceMicrosPerMonth: &limit} })
	c.paidBy = "platform"
	ctx := context.Background()
	session, err := manager.Create(ctx, agent.CreateOpts{OwnerID: 1, Provider: "exhibit-mock", APIKey: "assigned-key", PlatformPaid: true})
	require.NoError(t, err)
	events, unsubscribe := session.Subscribe()
	defer unsubscribe()
	require.NoError(t, session.Prompt(ctx, "mock-ask-questions", nil, nil))
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			return strings.Contains(string(event), `"type":"exhibit_questions"`)
		default:
			return false
		}
	}, 10*time.Second, 10*time.Millisecond)
	old, err := c.control.Reserve(ctx, agentcontrol.Reservation{OwnerID: 9, ConversationID: "terminated-conversation", ExecutionID: "terminated-attempt", WorkerID: "old-worker"})
	require.NoError(t, err)
	token, err := c.control.IssueSupervisor(ctx, old.Identity, "platform")
	require.NoError(t, err)
	require.NoError(t, c.control.Release(ctx, old.Identity, agentcontrol.Interrupted))
	// Router attaches the observer to the shared manager. An unrelated closed
	// attempt's receipt must stop this current worker without waiting for more
	// model events from it.
	router := api.NewRouter(api.Config{Store: st, Agent: manager, AgentControl: c.control})
	request := httptest.NewRequest("POST", api.AgentWorkerAuthorityPath+"/usage", strings.NewReader(`{"event_id":"late-paid-receipt","source":"model","cost_micros":1000000}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			return strings.Contains(string(event), `"type":"exhibit_spend_cap"`) && strings.Contains(string(event), `"cap":"instance"`)
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond)
}
