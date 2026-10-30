package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// API is the tools' only way into Exhibit: an authenticated request to the
// HTTP API, made with the agent grant the server was started with. Every write
// therefore takes the single write path a person's edits take, and the API's
// own scope check (agentScopeAllows) is the boundary, not this package.
type API interface {
	Do(ctx context.Context, method, path string, body any) (*Response, error)
}

// Response is a completed request: its status and body, whatever the status.
// Turning a status into something an agent can act on is the tools' job.
type Response struct {
	Status int
	Body   []byte
}

// HTTPAPI reaches an Exhibit instance over the network, the stdio server's
// transport (Exh-q6xo).
type HTTPAPI struct {
	// BaseURL is the instance's app origin, e.g. https://exhibit.example.com.
	BaseURL string
	// Token is the owner-issued agent grant.
	Token  string
	Client *http.Client
}

// maxResponseBytes bounds what one response may hand the agent. An artifact
// body is bounded by the instance's request limit on the way in; this is the
// same bound on the way out, generously.
const maxResponseBytes = 64 << 20

func (h *HTTPAPI) Do(ctx context.Context, method, path string, body any) (*Response, error) {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(h.BaseURL, "/")+path, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no response from Exhibit (%v); nothing confirms whether it was saved, so read it back before assuming either way", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Body: data}, nil
}
