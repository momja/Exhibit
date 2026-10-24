package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) createRelaySchedule(w http.ResponseWriter, r *http.Request) {
	a, s := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	if !ro.cfg.Relay.SchedulesEnabled {
		writeError(w, 404, "experimental schedules are disabled")
		return
	}
	var input struct {
		URL           string `json:"url"`
		StateKey      string `json:"state_key"`
		IntervalHours int64  `json:"interval_hours"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.IntervalHours < 1 || input.IntervalHours > 24*365 {
		writeError(w, 400, "choose an interval between 1 hour and 1 year")
		return
	}
	if len(input.StateKey) > 1024 || !utf8.ValidString(input.StateKey) {
		writeError(w, 400, "state key is too long or invalid")
		return
	}
	req, err := http.NewRequest("GET", input.URL, nil)
	if err != nil || req.URL.User != nil || req.URL.Fragment != "" {
		writeError(w, 400, "use an API URL without credentials or a fragment")
		return
	}
	gate, err := ro.cfg.Relay.Gate(a.NetworkAllowlist, ro.cfg.AppOrigin)
	if err != nil {
		serverError(w, r, "schedule gate", err)
		return
	}
	defer gate.Close()
	if !gate.Allows(req.URL) {
		writeError(w, 400, "approve this HTTPS origin in the artifact's network settings first")
		return
	}
	viewer := ownerIDFromCtx(r.Context())
	g, err := s.GetRelayGrant(r.Context(), viewer, a.ID)
	if err != nil {
		serverError(w, r, "schedule grant", err)
		return
	}
	if !g.Reads {
		writeError(w, 403, "allow relay reads before scheduling a refresh")
		return
	}
	allowance, err := ro.resolveAllowance(r.Context(), viewer)
	if err != nil {
		serverError(w, r, "schedule allowance", err)
		return
	}
	interval := input.IntervalHours * 3600
	// A random offset prevents a fleet of daily refreshes hitting providers at
	// midnight. Once assigned, that minute is stable across future ticks.
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		serverError(w, r, "schedule time", err)
		return
	}
	next := time.Now().Unix() + int64(binary.LittleEndian.Uint64(random[:])%uint64(interval))
	action := store.RelaySchedule{ID: uuid.NewString(), ArtifactID: a.ID, ViewerID: viewer, URL: input.URL, StateKey: input.StateKey, IntervalSeconds: interval, NextRunAt: next, Enabled: true}
	if err = s.PutRelaySchedule(r.Context(), action, allowance.Schedules); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, action)
}

func (ro *Router) deleteRelaySchedule(w http.ResponseWriter, r *http.Request) {
	a, s := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	if err := s.DeleteRelaySchedule(r.Context(), ownerIDFromCtx(r.Context()), a.ID, urlParamID(r, "scheduleID")); err != nil {
		serverError(w, r, "delete schedule", err)
		return
	}
	w.WriteHeader(204)
}

func (ro *Router) listRelaySchedules(w http.ResponseWriter, r *http.Request) {
	a, s := ro.relayArtifact(w, r)
	if a == nil {
		return
	}
	schedules, err := s.ListRelaySchedules(r.Context(), ownerIDFromCtx(r.Context()), a.ID)
	if err != nil {
		serverError(w, r, "list schedules", err)
		return
	}
	writeJSON(w, 200, schedules)
}

// StartRelaySchedules runs declarative fetches only. It never evaluates any
// source from an artifact. Cancellation stops the worker and in-flight I/O.
func (ro *Router) StartRelaySchedules(ctx context.Context) {
	if !ro.cfg.Relay.Enabled || !ro.cfg.Relay.SchedulesEnabled {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		ro.runRelaySchedules(ctx, time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				ro.runRelaySchedules(ctx, now)
			}
		}
	}()
}

func (ro *Router) StartConnectorMaintenance(ctx context.Context) {
	if ro.cfg.Connections == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			if count, err := ro.cfg.Connections.PruneIdle(ctx); err != nil {
				slog.WarnContext(ctx, "prune idle managed connections failed")
			} else if count > 0 {
				slog.InfoContext(ctx, "pruned idle managed connections", slog.Int("count", count))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (ro *Router) runRelaySchedules(ctx context.Context, now time.Time) {
	s, ok := ro.relayStore()
	if !ok || !ro.cfg.Relay.Enabled || !ro.cfg.Relay.SchedulesEnabled {
		return
	}
	work, err := s.ClaimRelaySchedules(ctx, now, 20)
	if err != nil {
		slog.ErrorContext(ctx, "claim relay schedules", slog.String("err", err.Error()))
		return
	}
	for _, job := range work {
		if ctx.Err() != nil {
			return
		}
		status, reason := ro.refreshSchedule(ctx, job)
		if err := s.FinishRelaySchedule(ctx, job, status, reason); err != nil {
			slog.ErrorContext(ctx, "record scheduled refresh", slog.String("schedule_id", job.ID))
		}
	}
}

func (ro *Router) refreshSchedule(ctx context.Context, job store.RelaySchedule) (int, string) {
	a, err := ro.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(job.ViewerID), job.ArtifactID)
	if err != nil || a == nil {
		return 0, "Access to this artifact is no longer available."
	}
	// A changed entitlement is applied to existing work as well as creation.
	allowance, err := ro.resolveAllowance(ctx, job.ViewerID)
	if err != nil {
		return 0, "Could not verify the schedule allowance."
	}
	s, _ := ro.relayStore()
	count, err := s.CountRelaySchedules(ctx, job.ViewerID)
	if err != nil {
		return 0, "Could not read schedules."
	}
	if allowance.Schedules >= 0 && count > allowance.Schedules {
		return 0, "The schedule allowance has been exceeded."
	}
	policy, err := ro.relayPolicy(ctx, job.ViewerID, a)
	if err != nil {
		return 0, "Could not verify the artifact's current access."
	}
	req, err := http.NewRequestWithContext(ctx, "GET", job.URL, nil)
	if err != nil {
		return 0, "Invalid API URL."
	}
	response, err := ro.executeRelay(ctx, job.ViewerID, a, req)
	if err != nil {
		return 0, "Refresh was refused or the API was unavailable. Review relay access and connection limits."
	}
	if response.Status < 200 || response.Status >= 300 {
		return response.Status, fmt.Sprintf("The API returned HTTP %d. The previous data was kept.", response.Status)
	}
	if len(response.Body) > 1<<20 || !utf8.Valid(response.Body) || strings.ContainsRune(string(response.Body), 0) {
		return response.Status, "The response must be text or JSON no larger than 1 MiB. The previous data was kept."
	}
	ro.connectionDataMu.Lock()
	defer ro.connectionDataMu.Unlock()
	current, err := ro.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(job.ViewerID), a.ID)
	if err != nil || current == nil {
		return response.Status, "Access changed before the response could be saved."
	}
	currentPolicy, err := ro.relayPolicy(ctx, job.ViewerID, current)
	if err != nil || !hmac.Equal([]byte(policy), []byte(currentPolicy)) {
		return response.Status, "Data access changed during the refresh. The response was not saved."
	}
	if err = ro.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(job.ViewerID), a.ID, job.StateKey, string(response.Body), a.VersionSeq); err != nil {
		return response.Status, "Could not save the response. The previous data was kept."
	}
	return response.Status, ""
}
