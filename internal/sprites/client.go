package sprites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

var errTransport = errors.New("Sprites management transport failed")

// APIError deliberately excludes response bodies and headers: upstream errors
// and bootstrap logs may echo credentials or conversation data.
type APIError struct {
	Operation  string
	Status     int
	RetryAfter time.Duration
	capacity   bool
}

func (e *APIError) Error() string {
	if e.capacity {
		return fmt.Sprintf("Sprites %s refused capacity (HTTP %d): %v", e.Operation, e.Status, ErrCapacity)
	}
	return fmt.Sprintf("Sprites %s failed (HTTP %d)", e.Operation, e.Status)
}
func (e *APIError) Is(target error) bool {
	return (target == ErrCapacity && e.capacity) || (target == ErrNotFound && e.Status == http.StatusNotFound)
}

func retryable(err error) bool {
	if errors.Is(err, errTransport) {
		return true
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == 429 || apiErr.Status == 408 || apiErr.Status >= 500 || apiErr.capacity
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(t))
	}
	return 0
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *Provisioner) retry(ctx context.Context, operation func() error) error {
	delay := p.config.Retry.InitialDelay
	for attempt := 0; ; attempt++ {
		err := operation()
		if err == nil || !retryable(err) || attempt+1 >= p.config.Retry.Attempts {
			return err
		}
		// Full jitter, capped locally. Retry-After is a lower bound; if it exceeds
		// the operation deadline, fail promptly instead of retrying too early.
		pause := time.Duration(rand.Int64N(int64(delay) + 1))
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			pause = max(pause, apiErr.RetryAfter)
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= pause {
			return err
		}
		if err := waitContext(ctx, pause); err != nil {
			return err
		}
		delay = min(delay*2, p.config.Retry.MaxDelay)
	}
}

// request makes one bounded call, never follows redirects, and never returns an
// upstream body in an error. Callers retry only idempotent management operations.
func (p *Provisioner) request(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, p.config.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid Sprites management request")
	}
	req.Header.Set("Authorization", "Bearer "+p.config.Token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "exhibit-sprites/1")
	resp, err := p.http.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, requestCtx.Err()
		}
		return nil, errTransport
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, requestCtx.Err()
		}
		return nil, errTransport
	}
	if len(data) > maxResponseBytes {
		return nil, errors.New("Sprites management response exceeds limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		lower := strings.ToLower(string(data))
		quotaMessage := strings.Contains(lower, "max sprites per org exceeded") || strings.Contains(lower, "max_sprites_per_org") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "running_limit_exceeded")
		capacity := resp.StatusCode == 429 || (resp.StatusCode != 401 && quotaMessage)
		return nil, &APIError{Operation: method + " " + strings.Split(path, "?")[0], Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After")), capacity: capacity}
	}
	return data, nil
}

func decodeResponse(data []byte, into any) error {
	if err := json.Unmarshal(data, into); err != nil {
		return errors.New("invalid JSON in Sprites response")
	}
	return nil
}

func (p *Provisioner) jsonRequest(ctx context.Context, method, path string, body, result any) error {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return errors.New("invalid Sprites request body")
		}
	}
	return p.retry(ctx, func() error {
		response, err := p.request(ctx, method, path, "application/json", data)
		if err != nil {
			return err
		}
		if result != nil {
			return decodeResponse(response, result)
		}
		return nil
	})
}

func (p *Provisioner) get(ctx context.Context, name string) (Resource, error) {
	var resource Resource
	err := p.jsonRequest(ctx, http.MethodGet, "/v1/sprites/"+name, nil, &resource)
	if err == nil && resource.Name != name {
		err = errors.New("Sprites returned an unexpected resource name")
	}
	return resource, err
}

func (p *Provisioner) inventory(ctx context.Context) (Inventory, error) {
	var inventory Inventory
	continuation := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		query := url.Values{"max_results": {"500"}}
		if continuation != "" {
			query.Set("continuation_token", continuation)
		}
		// Enumerate the org, not a prefix-filtered subset: the spec's running/warm
		// counters describe returned resources. Limits apply to the entire org.
		var response struct {
			Sprites []Resource `json:"sprites"`
			OrgLimits
			Org     *OrgLimits `json:"org"` // current Go SDK response variant
			HasMore bool       `json:"has_more"`
			Next    string     `json:"next_continuation_token"`
		}
		if err := p.jsonRequest(ctx, "GET", "/v1/sprites?"+query.Encode(), nil, &response); err != nil {
			return inventory, err
		}
		inventory.Org.RunningLimit = max(inventory.Org.RunningLimit, response.RunningLimit)
		inventory.Org.WarmLimit = max(inventory.Org.WarmLimit, response.WarmLimit)
		for _, resource := range response.Sprites {
			if seen[resource.Name] {
				return inventory, errors.New("Sprites inventory repeats a resource")
			}
			seen[resource.Name] = true
			switch resource.Status {
			case "running":
				inventory.Org.Running++
			case "warm":
				inventory.Org.Warm++
			case "cold":
				inventory.Org.Cold++
			}
			if p.managedName(resource.Name) {
				inventory.Resources = append(inventory.Resources, resource)
			}
		}
		if response.Org != nil {
			inventory.Org.RunningLimit = max(inventory.Org.RunningLimit, response.Org.RunningLimit)
			inventory.Org.WarmLimit = max(inventory.Org.WarmLimit, response.Org.WarmLimit)
		}
		if !response.HasMore {
			return inventory, nil
		}
		if response.Next == "" || response.Next == continuation {
			return inventory, errors.New("invalid Sprites inventory continuation")
		}
		continuation = response.Next
	}
	return inventory, errors.New("Sprites inventory exceeds 100 pages")
}

func (p *Provisioner) writeFile(ctx context.Context, name, path, mode string, data []byte) error {
	// mkdir is in OpenAPI 0.1.12; mkdirParents is used by the current official Go
	// SDK. Sending both supports the deployed variants without shell exec.
	query := url.Values{"path": {path}, "workingDir": {"/"}, "mode": {mode}, "mkdir": {"true"}, "mkdirParents": {"true"}}
	return p.retry(ctx, func() error {
		_, err := p.request(ctx, "PUT", "/v1/sprites/"+name+"/fs/write?"+query.Encode(), "application/octet-stream", data)
		return err
	})
}

func (p *Provisioner) delete(ctx context.Context, name string) error {
	err := p.jsonRequest(ctx, "DELETE", "/v1/sprites/"+name, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
