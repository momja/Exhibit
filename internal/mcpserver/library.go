package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type artifactSummary struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	UpdatedAt string   `json:"updated_at"`
	CreatedAt string   `json:"created_at"`
}

func (t *tools) searchArtifacts(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	query := url.Values{"q": {in.Query}, "limit": {strconv.Itoa(limit)}}
	if len(in.Tags) > 0 {
		query.Set("tags", strings.Join(in.Tags, ","))
	}
	var artifacts []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Tags  []struct {
			Name string `json:"name"`
		} `json:"tags"`
		UpdatedAt string `json:"updated_at"`
		CreatedAt string `json:"created_at"`
	}
	if err := t.call(ctx, http.MethodGet, "/api/artifacts?"+query.Encode(), nil, &artifacts); err != nil {
		return nil, nil, err
	}
	summaries := make([]artifactSummary, 0, len(artifacts))
	for _, a := range artifacts {
		tags := make([]string, 0, len(a.Tags))
		for _, tag := range a.Tags {
			tags = append(tags, tag.Name)
		}
		summaries = append(summaries, artifactSummary{ID: a.ID, Title: a.Title, Tags: tags, UpdatedAt: a.UpdatedAt, CreatedAt: a.CreatedAt})
	}
	return jsonText(summaries)
}

func (t *tools) createArtifact(ctx context.Context, _ *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, any, error) {
	var created patchResponse
	if err := t.call(ctx, http.MethodPost, "/api/artifacts", in, &created); err != nil {
		return nil, nil, err
	}
	if created.Artifact.ID == "" {
		return nil, nil, errors.New("Exhibit returned no artifact id after creation. Search the library before creating another artifact")
	}
	if _, err := artifactPath(created.Artifact.ID, ""); err != nil {
		return nil, nil, err
	}
	defer t.lockArtifact(created.Artifact.ID)()
	r := t.revisions(created.Artifact.ID)
	r.sourceRevision = created.Artifact.SourceRevision
	t.remember(created.Artifact.ID, r)
	return jsonText(struct {
		ID               string   `json:"id"`
		NetworkFootprint []string `json:"network_footprint"`
		Note             string   `json:"note"`
	}{
		ID:               created.Artifact.ID,
		NetworkFootprint: created.NetworkFootprint,
		Note:             "Created the artifact. Network origins found in the document must be approved by the owner in Exhibit before the artifact can reach them.",
	})
}

func jsonText(value any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return text(string(data)), nil, nil
}
