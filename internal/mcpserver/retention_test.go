package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type retentionAPI struct {
	calls int
	fail  bool
}

func (a *retentionAPI) Do(_ context.Context, method, path string, _ any) (*Response, error) {
	a.calls++
	if a.fail {
		return &Response{Status: 404}, nil
	}
	return &Response{Status: 200, Body: []byte(`{"source_revision":"source","widget_revision":"widget","artifact":{"source_revision":"saved"}}`)}, nil
}

func TestPerArtifactToolsRejectInvalidIDsBeforeOtherWork(t *testing.T) {
	for _, id := range []string{"", "garbage", strings.Repeat("a", 2<<20), "01234567-89AB-4cde-8abc-0123456789ab", "0123456789ab4cde8abc0123456789ab", "01234567-89ab-4cde-8abc-0123456789ag"} {
		api := &retentionAPI{}
		tools := &tools{api: api, artifacts: make(map[string]*artifactRevisions)}
		ctx := context.Background()
		for name, call := range map[string]func() (*mcp.CallToolResult, any, error){
			"get_artifact": func() (*mcp.CallToolResult, any, error) { return tools.getArtifact(ctx, nil, artifactInput{id}) },
			"get_widget":   func() (*mcp.CallToolResult, any, error) { return tools.getWidget(ctx, nil, artifactInput{id}) },
			"get_state":    func() (*mcp.CallToolResult, any, error) { return tools.getState(ctx, nil, artifactInput{id}) },
			"write_artifact": func() (*mcp.CallToolResult, any, error) {
				return tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{id}})
			},
			"edit_artifact": func() (*mcp.CallToolResult, any, error) {
				return tools.editArtifact(ctx, nil, editsInput{artifactInput: artifactInput{id}})
			},
			"set_widget": func() (*mcp.CallToolResult, any, error) {
				return tools.setWidget(ctx, nil, setWidgetInput{artifactInput: artifactInput{id}})
			},
			"edit_widget": func() (*mcp.CallToolResult, any, error) {
				return tools.editWidget(ctx, nil, editsInput{artifactInput: artifactInput{id}})
			},
		} {
			_, _, err := call()
			if err == nil || !strings.Contains(err.Error(), "artifact_id") {
				t.Errorf("%s with id length %d: want artifact_id error, got %v", name, len(id), err)
			}
		}
		if api.calls != 0 || len(tools.artifacts) != 0 {
			t.Errorf("id length %d: calls=%d retained=%d", len(id), api.calls, len(tools.artifacts))
		}
	}
}

func retentionID(n int) string { return fmt.Sprintf("%08x-89ab-4cde-8abc-0123456789ab", n) }

func TestRevisionHistoryIgnoresRefusalsAndFailedCalls(t *testing.T) {
	api := &retentionAPI{fail: true}
	tools := &tools{api: api, artifacts: make(map[string]*artifactRevisions)}
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		id := retentionID(i)
		if _, _, err := tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{id}}); err == nil {
			t.Fatal("unread source save accepted")
		}
		if _, _, err := tools.setWidget(ctx, nil, setWidgetInput{artifactInput: artifactInput{id}}); err == nil {
			t.Fatal("unread widget save accepted")
		}
	}
	if api.calls != 0 || len(tools.artifacts) != 0 {
		t.Fatalf("unread saves: calls=%d retained=%d", api.calls, len(tools.artifacts))
	}
	for i := 0; i < 4; i++ {
		id := retentionID(i)
		if _, _, err := tools.getArtifact(ctx, nil, artifactInput{id}); err == nil {
			t.Fatal("failed source read accepted")
		}
		if _, _, err := tools.getWidget(ctx, nil, artifactInput{id}); err == nil {
			t.Fatal("failed widget read accepted")
		}
	}
	if len(tools.artifacts) != 0 {
		t.Fatalf("failed reads retained %d artifacts", len(tools.artifacts))
	}
	api.fail = false
	id := retentionID(0)
	if _, _, err := tools.getArtifact(ctx, nil, artifactInput{id}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tools.getWidget(ctx, nil, artifactInput{id}); err != nil {
		t.Fatal(err)
	}
	before := *tools.artifacts[id]
	api.fail = true
	tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{id}})
	tools.setWidget(ctx, nil, setWidgetInput{artifactInput: artifactInput{id}})
	if after := tools.artifacts[id]; *after != before {
		t.Fatal("failed saves changed remembered revisions")
	}
}

func TestRevisionHistoryEvictsLeastRecentlyUsedArtifact(t *testing.T) {
	api := &retentionAPI{}
	tools := &tools{api: api, artifacts: make(map[string]*artifactRevisions)}
	ctx := context.Background()
	read := func(i int) {
		t.Helper()
		if _, _, err := tools.getArtifact(ctx, nil, artifactInput{retentionID(i)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 256; i++ {
		read(i)
	}
	// A successful save refreshes recency, and a successful read does too.
	if _, _, err := tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{retentionID(0)}}); err != nil {
		t.Fatal(err)
	}
	read(1)
	read(256)
	if len(tools.artifacts) != 256 || tools.artifacts[retentionID(2)] != nil || tools.artifacts[retentionID(0)] == nil || tools.artifacts[retentionID(1)] == nil {
		t.Fatalf("wrong LRU eviction; retained %d artifacts", len(tools.artifacts))
	}
	calls := api.calls
	if _, _, err := tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{retentionID(2)}}); err == nil || !strings.Contains(err.Error(), "read the artifact") {
		t.Fatalf("evicted save: %v", err)
	}
	if api.calls != calls || len(tools.artifacts) != 256 {
		t.Fatal("evicted save contacted API or changed history size")
	}
	read(2)
	if _, _, err := tools.writeArtifact(ctx, nil, writeArtifactInput{artifactInput: artifactInput{retentionID(2)}}); err != nil {
		t.Fatalf("save after re-read: %v", err)
	}
}
