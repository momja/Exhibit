package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/sprites"
	"github.com/momja/Exhibit/internal/store"
)

const workerLeaseTTL = 2 * time.Minute

// workerCoordinator owns core lifecycle wiring. Control is the durable
// admission/write authority; journal is the encrypted provisioning intent.
// mu protects ephemeral maps only. Execution gates order one worker's changes
// without holding a fleet lock during authority waits or infrastructure calls.
type workerCoordinator struct {
	mu        sync.Mutex
	config    agentExecutionConfig
	control   *agentcontrol.Control
	registry  *agentscope.Registry
	journal   *workerJournal
	backend   sprites.Backend
	manager   *agent.Manager
	live      map[string]agentcontrol.Identity
	processes map[string]agentexec.Process
	store     store.Store
	kinds     map[string]string
	gates     map[string]*workerGate
	paidBy    string
	stop      chan struct{}
	done      chan struct{}
}

// A gate orders one execution's lifecycle/journal updates. No fleet mutex is
// held while waiting for authority, worker HTTP, or management APIs.
type workerGate struct {
	held       chan struct{}
	references int
}

func (c *workerCoordinator) lockExecution(ctx context.Context, id string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.gates == nil {
		c.gates = map[string]*workerGate{}
	}
	gate := c.gates[id]
	if gate == nil {
		gate = &workerGate{held: make(chan struct{}, 1)}
		c.gates[id] = gate
	}
	gate.references++
	c.mu.Unlock()
	drop := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		gate.references--
		if gate.references == 0 {
			delete(c.gates, id)
		}
	}
	select {
	case gate.held <- struct{}{}:
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() { once.Do(func() { <-gate.held; drop() }) }, nil
}

func (c *workerCoordinator) liveIdentity(execution string) (agentcontrol.Identity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.live[execution]
	return id, ok
}
func (c *workerCoordinator) retire(execution string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.live, execution)
	delete(c.processes, execution)
	delete(c.kinds, execution)
}

func newWorkerCoordinator(ctx context.Context, config agentExecutionConfig, dataDir string, box *secrets.Box, st *store.SQLiteStore, registry *agentscope.Registry, platform bool) (*workerCoordinator, error) {
	journal, err := openWorkerJournal(dataDir, box)
	if err != nil {
		return nil, err
	}
	c := &workerCoordinator{config: config, registry: registry, journal: journal, live: map[string]agentcontrol.Identity{}, processes: map[string]agentexec.Process{}, store: st, kinds: map[string]string{}, paidBy: "user", stop: make(chan struct{}), done: make(chan struct{})}
	if platform {
		c.paidBy = "platform"
	}
	c.control = agentcontrol.New(st, registry, agentcontrol.Config{FleetLimit: config.MaxWorkers, LeaseTTL: workerLeaseTTL})
	targets, fenceErr := c.control.InterruptAll(ctx)
	// Completed targets remain useful even if another conversation's fence failed.
	for _, target := range targets {
		if record, ok := journal.snapshot()[target.ExecutionID]; ok {
			record.Assignment = target
			record.IdleSince = time.Now().Add(-config.IdleRetention)
			if err := journal.update(target.ExecutionID, &record); err != nil {
				return nil, err
			}
		}
	}
	if fenceErr != nil {
		return nil, fmt.Errorf("fence prior worker assignments: %w", fenceErr)
	}
	if config.Mode == "sprites" {
		for _, record := range journal.snapshot() {
			if record.Request.EndpointToken != "" && record.Prefix != "" && record.Prefix != config.SpritePrefix {
				return nil, errors.New("clean up pending workers before changing AGENT_SPRITE_PREFIX")
			}
		}
		c.backend, err = sprites.New(sprites.Config{Token: config.SpriteToken, Prefix: config.SpritePrefix, Release: sprites.Release{URL: config.ReleaseURL, Version: config.ReleaseVersion, SHA256: config.ReleaseSHA256}, MaxWorkers: config.MaxWorkers, StartupTimeout: config.StartupTimeout, IdleRetention: config.IdleRetention, WorkerArgs: []string{"--sprite-holds", "required"}})
		if err != nil {
			return nil, err
		}
	}
	// None of the prior process's workers may reattach. Preserve failed deletions
	// in the encrypted journal for management-only retries after startup.
	for id, record := range journal.snapshot() {
		record.Assignment.Status = agentcontrol.Interrupted
		record.EraseOwner = true
		record.IdleSince = time.Now().Add(-config.IdleRetention)
		if err := journal.update(id, &record); err != nil {
			return nil, err
		}
	}
	if c.backend != nil {
		if err := c.reconcile(ctx); err != nil {
			slog.Warn("worker restart cleanup pending")
		}
	}
	return c, nil
}

func (c *workerCoordinator) runner(piBin, workRoot string) (agentexec.Runner, error) {
	switch c.config.Mode {
	case "off":
		return nil, nil
	case "local":
		path, err := exec.LookPath(piBin)
		if err != nil {
			return nil, err
		}
		return boundedWorkerRunner{runner: agentexec.Local{PiBin: path, WorkRoot: workRoot}, timeout: c.config.StartupTimeout, started: func(ctx context.Context, id agentexec.Identity) error {
			return c.active(ctx, c.journal.snapshot()[id.ExecutionID].Assignment.Identity)
		}}, nil
	case "remote", "sprites":
		return boundedWorkerRunner{runner: agentexec.Remote{Provisioner: agentexec.ProvisionFunc(c.acquire)}, timeout: c.config.StartupTimeout}, nil
	default:
		return nil, errors.New("unknown agent execution mode")
	}
}

type boundedWorkerRunner struct {
	runner  agentexec.Runner
	timeout time.Duration
	started func(context.Context, agentexec.Identity) error
}

func (r boundedWorkerRunner) Start(ctx context.Context, config agentexec.StartConfig) (agentexec.Process, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	process, err := r.runner.Start(ctx, config)
	if err != nil {
		return nil, err
	}
	if r.started != nil {
		if err := r.started(ctx, config.Identity); err != nil {
			_ = process.Kill()
			_ = process.Cleanup()
			return nil, err
		}
	}
	return process, nil
}

func (c *workerCoordinator) reserve(ctx context.Context, id agentexec.Identity, original *agentscope.Grant) (*agent.ExecutionReservation, error) {
	unlock, err := c.lockExecution(ctx, id.ExecutionID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	assignment, err := c.control.Reserve(ctx, agentcontrol.Reservation{OwnerID: id.OwnerID, ConversationID: id.SessionID, ExecutionID: id.ExecutionID, WorkerID: id.ExecutionID, ArtifactID: id.ArtifactID, Resume: id.Resume})
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = c.control.Release(context.Background(), assignment.Identity, agentcontrol.Failed)
			c.retire(id.ExecutionID)
			if record, ok := c.journal.snapshot()[id.ExecutionID]; ok {
				record.Assignment.Status = agentcontrol.Failed
				record.IdleSince = time.Now()
				_ = c.journal.update(id.ExecutionID, &record)
			}
		}
	}()
	grant, err := c.control.IssueTool(ctx, assignment.Identity)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !succeeded {
			c.registry.Revoke(grant)
		}
	}()
	if _, err := c.control.IssueSupervisor(ctx, assignment.Identity, c.paidBy); err != nil {
		return nil, err
	}
	request := sprites.Request{Identity: sprites.Identity{OwnerID: strconv.FormatInt(id.OwnerID, 10), SessionID: id.SessionID, ExecutionID: id.ExecutionID}}
	if c.config.Mode == "sprites" {
		request.EndpointToken, err = sprites.NewEndpointToken()
		if err != nil {
			return nil, err
		}
	}
	record := workerRecord{OwnerID: id.OwnerID, Prefix: c.config.SpritePrefix, Assignment: assignment, Request: request}
	if err := c.journal.update(id.ExecutionID, &record); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.live[id.ExecutionID] = assignment.Identity
	c.mu.Unlock()
	reservation := &agent.ExecutionReservation{Grant: grant, ExpiresAt: assignment.LeaseUntil,
		Started: func(process agentexec.Process) {
			c.mu.Lock()
			c.processes[id.ExecutionID] = process
			c.mu.Unlock()
		},
		BeginClose:    func(ctx context.Context) error { return c.control.BeginClose(ctx, assignment.Identity) },
		StartupFailed: func(ctx context.Context) error { return c.release(ctx, assignment.Identity, agentcontrol.Failed) },
		BeforePrompt: func(ctx context.Context) (time.Time, error) {
			deadline := time.Now().Add(workerLeaseTTL)
			if err := c.control.Renew(ctx, assignment.Identity); err != nil {
				return time.Time{}, err
			}
			return deadline, nil
		},
		Checkpoint: func(ctx context.Context, checkpoint agent.ExecutionCheckpoint) error {
			if checkpoint.SessionFile == "" {
				return nil
			}
			_, err := c.control.WriteCheckpoint(ctx, assignment.Identity, agentcontrol.Checkpoint{Revision: assignment.Revision + int64(checkpoint.Sequence), Kind: checkpoint.Kind, Title: strings.Join(strings.Fields(checkpoint.Title), " "), SessionFile: checkpoint.SessionFile})
			if err == nil {
				c.mu.Lock()
				c.kinds[id.ExecutionID] = checkpoint.Kind
				c.mu.Unlock()
			}
			return err
		},
		Release: func(ctx context.Context) error { return c.release(ctx, assignment.Identity, "") },
	}
	if id.Resume {
		snapshot, err := c.control.ReadSnapshot(ctx, assignment.Identity)
		if err != nil {
			return nil, err
		}
		baseline, err := c.store.SessionAgentSpend(ctx, id.OwnerID, id.SessionID)
		if err != nil {
			return nil, err
		}
		reservation.Resume = &agent.ResumeSnapshot{SessionFile: snapshot.SessionFile, Title: snapshot.Title, Kind: snapshot.Kind, Revision: snapshot.Revision, PreviousStatus: snapshot.PreviousStatus, Interrupted: snapshot.Kind == agentcontrol.CheckpointInterrupted || snapshot.PreviousStatus == agentcontrol.Interrupted || snapshot.PreviousStatus == agentcontrol.Failed}
		reservation.UsageBaseline = &baseline
	}
	succeeded = true
	return reservation, nil
}

func (c *workerCoordinator) acquire(ctx context.Context, id agentexec.Identity) (agentexec.Endpoint, error) {
	unlock, err := c.lockExecution(ctx, id.ExecutionID)
	if err != nil {
		return agentexec.Endpoint{}, err
	}
	defer unlock()
	record, ok := c.journal.snapshot()[id.ExecutionID]
	if !ok {
		return agentexec.Endpoint{}, agentcontrol.ErrStale
	}
	release, err := c.control.Guard(ctx, record.Assignment.Identity)
	if err != nil {
		return agentexec.Endpoint{}, err
	}
	release()
	endpoint := agentexec.Endpoint{URL: c.config.WorkerURL, Token: c.config.WorkerToken, AllowInsecure: c.config.AllowInsecure, AllowUnassigned: c.config.Mode == "remote"}
	if c.backend != nil {
		var remote *sprites.Endpoint
		if record.Assignment.Status == agentcontrol.Reserved {
			remote, err = c.backend.Provision(ctx, record.Request)
		} else {
			remote, err = c.backend.Wake(ctx, record.Request)
		}
		if err != nil {
			return endpoint, err
		}
		endpoint.URL, endpoint.Token, endpoint.Transport = remote.URL, remote.EndpointToken, remote.Transport
		endpoint.Client = remote.Client
	}
	endpoint.Active = func(ctx context.Context) error { return c.active(ctx, record.Assignment.Identity) }
	endpoint.Idle = func(ctx context.Context, snapshot agentexec.Snapshot) error {
		return c.idle(ctx, record.Assignment.Identity, snapshot)
	}
	endpoint.LoadSnapshot = func(ctx context.Context, _ agentexec.Identity) (agentexec.RestoreState, error) {
		snapshot, err := c.control.ReadSnapshot(ctx, record.Assignment.Identity)
		if err != nil {
			return agentexec.RestoreState{}, err
		}
		grant, err := c.control.IssueTool(ctx, record.Assignment.Identity)
		if err != nil {
			return agentexec.RestoreState{}, err
		}
		defer c.registry.Revoke(grant)
		return agentexec.RestoreState{SessionFile: snapshot.SessionFile, ArtifactID: grant.Scope().ArtifactID}, nil
	}
	return endpoint, nil
}

func (c *workerCoordinator) active(ctx context.Context, id agentcontrol.Identity) error {
	unlock, err := c.lockExecution(ctx, id.ExecutionID)
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.control.Activate(ctx, id); err != nil {
		return err
	}
	record, ok := c.journal.snapshot()[id.ExecutionID]
	if !ok {
		return agentcontrol.ErrStale
	}
	record.Assignment.Status = agentcontrol.Active
	record.IdleSince = time.Time{}
	return c.journal.update(id.ExecutionID, &record)
}

func (c *workerCoordinator) idle(ctx context.Context, id agentcontrol.Identity, snapshot agentexec.Snapshot) error {
	unlock, err := c.lockExecution(ctx, id.ExecutionID)
	if err != nil {
		return err
	}
	defer unlock()
	release, err := c.control.Guard(ctx, id)
	if err != nil {
		return err
	}
	release()
	if snapshot.Settled && snapshot.SessionFile != "" {
		acknowledged, err := c.control.ReadSnapshot(ctx, id)
		if err != nil {
			return err
		}
		if acknowledged.SessionFile != snapshot.SessionFile {
			return errors.New("worker settled snapshot is not durably acknowledged")
		}
	}
	record, ok := c.journal.snapshot()[id.ExecutionID]
	if !ok {
		return agentcontrol.ErrStale
	}
	record.IdleSince = time.Now()
	return c.journal.update(id.ExecutionID, &record)
}

func (c *workerCoordinator) release(ctx context.Context, id agentcontrol.Identity, terminal string) error {
	unlock, err := c.lockExecution(ctx, id.ExecutionID)
	if err != nil {
		return err
	}
	defer unlock()
	record, ok := c.journal.snapshot()[id.ExecutionID]
	if terminal == "" {
		terminal = agentcontrol.Closed
		c.mu.Lock()
		kind := c.kinds[id.ExecutionID]
		c.mu.Unlock()
		if kind == agentcontrol.CheckpointInterrupted {
			terminal = agentcontrol.Interrupted
		}
	}
	if err := c.control.Release(ctx, id, terminal); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.live, id.ExecutionID)
	c.mu.Unlock()
	if !ok {
		return nil
	}
	record.Assignment.Status, record.IdleSince = terminal, time.Now()
	if err := c.journal.update(id.ExecutionID, &record); err != nil {
		return err
	}
	c.retire(id.ExecutionID)
	return nil
}

func (c *workerCoordinator) start(manager *agent.Manager) {
	c.manager = manager
	go func() {
		defer close(c.done)
		renew := time.NewTicker(workerLeaseTTL / 3)
		cleanup := time.NewTicker(time.Minute)
		defer renew.Stop()
		defer cleanup.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-renew.C:
				c.renew()
			case <-cleanup.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := c.reconcile(ctx); err != nil {
					slog.Warn("worker retention cleanup pending")
				}
				cancel()
			}
		}
	}()
}

func (c *workerCoordinator) renew() {
	c.mu.Lock()
	live := make(map[string]agentcontrol.Identity, len(c.live))
	processes := make(map[string]agentexec.Process, len(c.processes))
	for execution, id := range c.live {
		live[execution] = id
		processes[execution] = c.processes[execution]
	}
	c.mu.Unlock()
	var pending sync.WaitGroup
	for execution, id := range live {
		pending.Add(1)
		go func(execution string, id agentcontrol.Identity) {
			defer pending.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			deadline := time.Now().Add(workerLeaseTTL)
			err := c.control.Renew(ctx, id)
			if err == nil {
				if process, ok := processes[execution].(agentexec.LeaseProcess); ok {
					err = process.SetLease(ctx, deadline)
				}
			}
			if err != nil {
				if _, live := c.liveIdentity(execution); live && c.manager != nil {
					go c.manager.Close(id.OwnerID, id.ConversationID)
				}
				slog.Warn("worker lease renewal failed", "execution_id", execution)
			}
		}(execution, id)
	}
	pending.Wait()
}

func (c *workerCoordinator) reconcile(ctx context.Context) error {
	erased := map[int64]bool{}
	for _, record := range c.journal.snapshot() {
		if record.EraseOwner {
			erased[record.OwnerID] = true
		}
	}
	var failures []error
	for owner := range erased {
		if err := c.deleteOwner(ctx, owner); err != nil {
			failures = append(failures, err)
		}
	}
	targets, err := c.control.ReconcileExpired(ctx)
	if err != nil {
		failures = append(failures, err)
	}
	for _, target := range targets {
		unlock, err := c.lockExecution(ctx, target.ExecutionID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		c.mu.Lock()
		delete(c.live, target.ExecutionID)
		c.mu.Unlock()
		if record, ok := c.journal.snapshot()[target.ExecutionID]; ok {
			record.Assignment = target
			record.IdleSince = time.Now()
			if err := c.journal.update(target.ExecutionID, &record); err != nil {
				failures = append(failures, err)
			}
		}
		unlock()
		if c.manager != nil {
			go c.manager.Close(target.OwnerID, target.ConversationID)
		}
	}
	records := c.journal.snapshot()
	assignments := make([]sprites.Assignment, 0, len(records))
	for execution, record := range records {
		_, live := c.liveIdentity(execution)
		if live && !record.IdleSince.IsZero() && c.config.IdleRetention > 0 && time.Since(record.IdleSince) >= c.config.IdleRetention && c.manager != nil {
			go c.manager.Close(record.OwnerID, record.Assignment.ConversationID)
		}
		if c.backend == nil {
			if !live && record.Request.EndpointToken == "" && !record.EraseOwner {
				if err := c.journal.update(execution, nil); err != nil {
					failures = append(failures, err)
				}
			}
			continue
		}
		assignments = append(assignments, sprites.Assignment{Identity: record.Request.Identity, Active: live || record.IdleSince.IsZero(), IdleSince: record.IdleSince})
	}
	if c.backend == nil {
		return errors.Join(failures...)
	}
	_, err = c.backend.Reconcile(ctx, assignments, func(ctx context.Context, candidate sprites.Candidate) error {
		// Revalidate the CURRENT desired set after the management inventory read.
		// This closes the stale-snapshot race without serializing fleet networking.
		if candidate.Identity != nil {
			if _, live := c.liveIdentity(candidate.Identity.ExecutionID); live {
				return errors.New("execution still draining")
			}
		} else if provisioner, ok := c.backend.(*sprites.Provisioner); ok {
			for execution, record := range c.journal.snapshot() {
				if provisioner.Name(record.Request.Identity) == candidate.Resource.Name {
					if _, live := c.liveIdentity(execution); live {
						return errors.New("execution admitted after inventory snapshot")
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		failures = append(failures, err)
	}
	// Confirm absent resources through management metadata only. Journal removal
	// is serialized per execution and rechecks liveness; no worker is polled.
	for execution, record := range records {
		if _, live := c.liveIdentity(execution); live {
			continue
		}
		if _, err := c.backend.Query(ctx, record.Request.Identity); !errors.Is(err, sprites.ErrNotFound) {
			continue
		}
		unlock, err := c.lockExecution(ctx, execution)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if _, live := c.liveIdentity(execution); !live {
			if err := c.journal.update(execution, nil); err != nil {
				failures = append(failures, err)
			}
		}
		unlock()
	}
	return errors.Join(failures...)
}

func (c *workerCoordinator) deleteOwner(ctx context.Context, owner int64) error {
	// The account is already durably deleted. Erasure discards recovery bytes;
	// ordinary Close/Shutdown still preserve unacknowledged state.
	var failures []error
	var eraseErr error
	if c.manager != nil {
		eraseErr = c.manager.EraseOwner(ctx, owner)
		if eraseErr != nil {
			failures = append(failures, eraseErr)
		}
	}
	for execution, record := range c.journal.snapshot() {
		if record.OwnerID != owner {
			continue
		}
		unlock, err := c.lockExecution(ctx, execution)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		err = c.control.Release(ctx, record.Assignment.Identity, agentcontrol.Interrupted)
		if err != nil && !errors.Is(err, agentcontrol.ErrStale) {
			failures = append(failures, err)
			unlock()
			continue
		}
		c.retire(execution)
		record.Assignment.Status = agentcontrol.Interrupted
		record.EraseOwner = true
		record.IdleSince = time.Now().Add(-c.config.IdleRetention)
		if err := c.journal.update(execution, &record); err != nil {
			failures = append(failures, err)
			unlock()
			continue
		}
		if c.backend == nil {
			if eraseErr != nil {
				unlock()
				continue
			}
			if record.Request.EndpointToken != "" {
				failures = append(failures, errors.New("Sprite cleanup requires the configured management backend"))
				unlock()
				continue
			}
		} else if err := c.backend.Destroy(ctx, record.Request.Identity); err != nil {
			failures = append(failures, err)
			unlock()
			continue
		}
		if err := c.journal.update(execution, nil); err != nil {
			failures = append(failures, err)
		}
		unlock()
	}
	return errors.Join(failures...)
}
