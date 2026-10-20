package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestHoldsUseLocalSocketAndExpireWithoutBackgroundHeartbeat(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Host != "sprite" || r.URL.Path != "/v1/tasks/attempt-1" {
			t.Error("incorrect local Tasks API target")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("local Tasks API must not need a management token")
		}
		if n <= 2 {
			if r.Method != "PUT" {
				t.Error("task acquire/renew must be an idempotent upsert")
			}
			var body map[string]int64
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["expire"] != 300 {
				t.Error("task expiry must be bounded seconds")
			}
			w.WriteHeader(200)
		} else {
			if r.Method != "DELETE" {
				t.Error("settlement must release hold")
			}
			w.WriteHeader(404)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	h := newHolds(socket)
	for _, call := range []func() error{
		func() error { return h.Acquire(context.Background(), "attempt-1", 5*time.Minute) },
		func() error { return h.Renew(context.Background(), "attempt-1", 5*time.Minute) },
		func() error { return h.Release(context.Background(), "attempt-1") },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	h.CloseIdleConnections()
	if calls.Load() != 3 {
		t.Error("unexpected Tasks API calls")
	}
	// No background renewer is created by Acquire/Renew; expiry bounds crash cost.
	for _, ttl := range []time.Duration{0, time.Hour + time.Second, 1500 * time.Millisecond} {
		if err := h.Acquire(context.Background(), "attempt-1", ttl); err == nil {
			t.Error("invalid task lifetime accepted")
		}
	}
	if err := h.Release(context.Background(), "../another-task"); err == nil {
		t.Error("unsafe task name accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Renew(ctx, "attempt-1", time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("hold cancellation ignored: %v", err)
	}
	if calls.Load() != 3 {
		t.Error("invalid or cancelled hold reached socket")
	}
}
