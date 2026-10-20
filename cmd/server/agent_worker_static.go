package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
)

// Prior static assignments outlive a core process. After central fencing they
// may be stopped/released without restoring their prompt or reading raw history.
// Credentials/endpoint are part of the encrypted journal, so config rotation
// cannot silently strand a previous daemon's assignment and provider keys.
func (c *workerCoordinator) cleanupStatic(ctx context.Context, record workerRecord) error {
	endpoint, token := record.StaticURL, record.StaticToken
	if endpoint == "" {
		endpoint, token = c.config.WorkerURL, c.config.WorkerToken
	}
	if endpoint == "" || token == "" {
		return errors.New("static worker cleanup requires its authenticated endpoint")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	body := `{"id":"core-reclaim"}`
	base := strings.TrimRight(endpoint, "/") + "/v1/executions/" + url.PathEscape(record.Assignment.ExecutionID)
	for _, operation := range []string{"close", "release"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+operation, strings.NewReader(body))
		if err != nil {
			return errors.New("invalid static cleanup endpoint")
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set(agentexec.VersionHeader, agentexec.ProtocolVersion)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return errors.New("static worker cleanup unavailable")
		}
		status := response.StatusCode
		var remoteError agentexec.WireError
		if status >= 400 {
			_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&remoteError)
		} else {
			// /close returns a potentially large snapshot. Discard it with a bound;
			// the last centrally acknowledged file remains the continuation authority.
			_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 2*agentexec.MaxSessionBytes+(1<<20)))
		}
		response.Body.Close()
		if status == 404 || remoteError.Code == "execution_retired" {
			return nil
		}
		if status < 200 || status >= 300 || err != nil {
			return errors.New("static worker cleanup not confirmed")
		}
	}
	return nil
}
