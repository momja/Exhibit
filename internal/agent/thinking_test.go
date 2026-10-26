package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThinkingLevelFromEnv(t *testing.T) {
	for _, level := range []string{"", "off", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run("level="+level, func(t *testing.T) {
			t.Setenv("AGENT_THINKING_LEVEL", " \t"+level+"\n")
			got, err := ThinkingLevelFromEnv()
			require.NoError(t, err)
			want := level
			if want == "" {
				want = "medium"
			}
			assert.Equal(t, want, got)
		})
	}
	for _, invalid := range []string{"auto", "HIGH", "extra-high", "high medium", "--help"} {
		t.Run("invalid="+invalid, func(t *testing.T) {
			t.Setenv("AGENT_THINKING_LEVEL", invalid)
			_, err := ThinkingLevelFromEnv()
			require.ErrorContains(t, err, "AGENT_THINKING_LEVEL")
			assert.Contains(t, err.Error(), invalid)
		})
	}
}

// Read Pi's actual RPC state without making a provider request. In particular,
// an explicit "off" must override saved thinking just like any other level.
func TestRealPiThinkingLevelFromEnvironment(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi is not installed")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"artifact","title":"Title","body":"<p>Source</p>","source_revision":"r1","assets":[]}`)
	}))
	defer app.Close()

	for _, tt := range []struct {
		name       string
		level      string
		ownerLevel string
		hosted     bool
		resume     bool
		want       string
	}{
		{name: "new default", want: "medium"},
		{name: "new configured", level: "high", want: "high"},
		{name: "new disabled", level: "off", want: "off"},
		{name: "new owner preference", level: "high", ownerLevel: "low", want: "low"},
		{name: "new owner disabled", level: "high", ownerLevel: "off", want: "off"},
		{name: "hosted ignores owner preference", level: "low", ownerLevel: "high", hosted: true, want: "low"},
		{name: "resume default baseline", resume: true, want: "medium"},
		{name: "resume overridden", level: "low", resume: true, want: "low"},
		{name: "resume disabled", level: "off", resume: true, want: "off"},
		{name: "resume owner preference", level: "high", ownerLevel: "low", resume: true, want: "low"},
		{name: "hosted resume ignores owner preference", ownerLevel: "off", hosted: true, resume: true, want: "medium"},
	} {
		for _, transport := range []string{"local", "remote"} {
			t.Run(tt.name+"/"+transport, func(t *testing.T) {
				t.Setenv("AGENT_THINKING_LEVEL", tt.level)
				level, err := ThinkingLevelFromEnv()
				require.NoError(t, err)
				m := newLimitedManager(t)
				m.cfg.PiBin = pi
				m.cfg.APIBaseURL = app.URL
				m.cfg.ThinkingLevel = level
				if transport == "remote" {
					const token = "thinking-worker-authentication-0123456789abcdef"
					worker, err := agentexec.NewService(agentexec.ServiceConfig{
						LocalAssignments: true, Token: token,
						Runner: agentexec.Local{PiBin: pi, WorkRoot: t.TempDir()},
					})
					require.NoError(t, err)
					server := httptest.NewServer(worker)
					m.cfg.Runner = agentexec.Remote{Endpoint: agentexec.Endpoint{URL: server.URL, Token: token, AllowUnassigned: true}}
					m.cfg.PiBin, m.cfg.WorkRoot = "/missing/core/pi", "/missing/core/scratch"
					t.Cleanup(func() {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						require.NoError(t, m.Shutdown(ctx))
						require.NoError(t, worker.Shutdown(ctx))
						server.Close()
					})
				}
				opts := CreateOpts{OwnerID: 7, Provider: "anthropic", Model: "claude-sonnet-4-5", APIKey: "not-used"}
				opts.ThinkingLevel, opts.PlatformPaid = tt.ownerLevel, tt.hosted
				if tt.resume {
					require.NoError(t, m.st.PutArtifact(t.Context(), &store.Artifact{
						ID: "artifact", OwnerID: 7, Title: "Title", SourceBlobID: "body", Tier: 1,
					}))
					saved := strings.Join([]string{
						`{"type":"session","version":3,"id":"conversation","timestamp":"2026-10-25T00:00:00Z","cwd":"/"}`,
						`{"type":"model_change","id":"model","parentId":null,"timestamp":"2026-10-25T00:00:00Z","provider":"anthropic","modelId":"claude-sonnet-4-5"}`,
						`{"type":"thinking_level_change","id":"thinking","parentId":"model","timestamp":"2026-10-25T00:00:00Z","thinkingLevel":"high"}`,
						`{"type":"message","id":"user","parentId":"thinking","timestamp":"2026-10-25T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Earlier request"}],"timestamp":1792886400000}}`,
					}, "\n") + "\n"
					require.NoError(t, m.st.SaveTranscript(t.Context(), 7, store.Transcript{
						ArtifactID: "artifact", SessionID: "conversation", SessionFile: saved,
					}))
					opts.ArtifactID, opts.ResumeSessionID = "artifact", "conversation"
				}
				s, err := m.Create(t.Context(), opts)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				line, err := s.roundTrip(ctx, map[string]any{"type": "get_state"})
				require.NoError(t, err)
				var response struct {
					Success bool `json:"success"`
					Data    struct {
						ThinkingLevel string `json:"thinkingLevel"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(line, &response))
				require.True(t, response.Success, string(line))
				assert.Equal(t, tt.want, response.Data.ThinkingLevel)
			})
		}
	}
}
