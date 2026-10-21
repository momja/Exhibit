package sprites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"
)

const TasksSocket = "/.sprite/api.sock"

var taskNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)

// Holds runs INSIDE the standalone worker. It does not need or accept a Sprites
// management token. Acquire before accepting work; Renew only during a bounded
// active execution; Release after settlement/abort. It starts no goroutines.
type Holds struct{ client *http.Client }

func NewHolds() *Holds {
	return newHolds(TasksSocket)
}

func newHolds(socketPath string) *Holds {
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	return &Holds{client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Acquire uses the documented upsert so retrying after a lost response is safe.
func (h *Holds) Acquire(ctx context.Context, name string, ttl time.Duration) error {
	return h.Renew(ctx, name, ttl)
}
func (h *Holds) Renew(ctx context.Context, name string, ttl time.Duration) error {
	if ttl < time.Second || ttl > time.Hour || ttl%time.Second != 0 {
		return errors.New("Sprite task expiry must be whole seconds from 1 second to 1 hour")
	}
	data, _ := json.Marshal(map[string]int64{"expire": int64(ttl / time.Second)})
	return h.call(ctx, "PUT", name, data)
}
func (h *Holds) Release(ctx context.Context, name string) error {
	return h.call(ctx, "DELETE", name, nil)
}
func (h *Holds) CloseIdleConnections() { h.client.CloseIdleConnections() }
func (h *Holds) call(ctx context.Context, method, name string, body []byte) error {
	if !taskNamePattern.MatchString(name) {
		return errors.New("invalid Sprite task name")
	}
	req, _ := http.NewRequestWithContext(ctx, method, "http://sprite/v1/tasks/"+name, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Sprite local Tasks API connection failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 204 || (method == "DELETE" && resp.StatusCode == 404) {
		return nil
	}
	return errors.New("Sprite local Tasks API refused hold operation")
}
