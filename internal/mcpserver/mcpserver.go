// Package mcpserver exposes an owner's Exhibit library as MCP tools.
// Every operation uses the HTTP API; authorization remains in Exhibit.
// Create one server per connection to keep each client's revision history separate.
package mcpserver

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"sync"
	"text/template"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/momja/Exhibit/internal/agentguidance"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

// Instructions is what the server tells a connecting client about Exhibit,
// sharing what an artifact is, the widget rules and the data contract with
// the Pi system prompt (agentguidance).
var Instructions = func() string {
	t := template.Must(template.Must(template.New("").Option("missingkey=error").
		ParseFS(promptFS, "prompts/*.tmpl")).ParseFS(agentguidance.FS, "*.tmpl"))
	var out bytes.Buffer
	if err := t.ExecuteTemplate(&out, "instructions.tmpl", nil); err != nil {
		panic("mcpserver: render instructions: " + err.Error())
	}
	return strings.TrimSuffix(out.String(), "\n")
}()

// New returns an MCP server whose tools act through api. A server holds the
// revisions its tools last read, so it serves one client: make one per
// connection.
func New(api API, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "exhibit", Title: "Exhibit", Version: version},
		&mcp.ServerOptions{Instructions: Instructions, Capabilities: &mcp.ServerCapabilities{}})
	t := &tools{api: api, artifacts: make(map[string]*artifactRevisions)}
	t.register(s)
	return s
}

// tools remembers only successful reads and saves from this connection.
// Fixed lock stripes serialize calls for the same artifact without allocating
// a lock for every supplied id. The history lock is never held during API calls.
type tools struct {
	api       API
	locks     [64]sync.Mutex
	mu        sync.Mutex
	artifacts map[string]*artifactRevisions
	clock     uint64
}

const maxRememberedArtifacts = 256

type artifactRevisions struct {
	sourceRevision string
	widgetRevision string
	widgetRead     bool
	lastUsed       uint64
}

func (t *tools) lockArtifact(id string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	lock := &t.locks[hash.Sum32()%uint32(len(t.locks))]
	lock.Lock()
	return lock.Unlock
}

// revisions returns a working copy. Failed calls never change the history.
// The caller holds the artifact's lock until it has finished remembering it.
func (t *tools) revisions(id string) artifactRevisions {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r := t.artifacts[id]; r != nil {
		return *r
	}
	return artifactRevisions{}
}

func (t *tools) remember(id string, r artifactRevisions) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.artifacts[id] == nil && len(t.artifacts) >= maxRememberedArtifacts {
		var oldest string
		for key, entry := range t.artifacts {
			if oldest == "" || entry.lastUsed < t.artifacts[oldest].lastUsed {
				oldest = key
			}
		}
		delete(t.artifacts, oldest)
	}
	t.clock++
	r.lastUsed = t.clock
	t.artifacts[strings.Clone(id)] = &r
}

type artifactInput struct {
	ArtifactID string `json:"artifact_id" jsonschema:"the artifact id returned by search_artifacts or create_artifact; never guess"`
}

type searchInput struct {
	Query string   `json:"query" jsonschema:"search title, visible text and tags; empty lists recent artifacts"`
	Tags  []string `json:"tags,omitempty" jsonschema:"optional tag names to filter by"`
	Limit int      `json:"limit,omitempty" jsonschema:"maximum results; defaults to 20, capped at 50"`
}

type createInput struct {
	Title string `json:"title" jsonschema:"the new artifact's title"`
	Body  string `json:"body" jsonschema:"a complete self-contained HTML document"`
}

type editsInput struct {
	artifactInput
	Edits []edit `json:"edits" jsonschema:"one or more targeted replacements, applied atomically"`
}

type edit struct {
	OldText string `json:"oldText" jsonschema:"exact text to find; must occur exactly once in the current source"`
	NewText string `json:"newText" jsonschema:"replacement text; empty deletes"`
}

type writeArtifactInput struct {
	artifactInput
	Body  string `json:"body" jsonschema:"the complete replacement HTML document"`
	Title string `json:"title,omitempty" jsonschema:"a new title; omit to keep the current one"`
}

type setWidgetInput struct {
	artifactInput
	Body string `json:"body" jsonschema:"the complete HTML document for the widget"`
}

func (t *tools) register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "search_artifacts", Title: "Search library",
		Description: "Search the owner's library by title, visible text and tags. An empty query lists recent artifacts. Search first; never guess ids. When several results plausibly match, ask the user which one they mean.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.searchArtifacts)
	mcp.AddTool(s, &mcp.Tool{Name: "create_artifact", Title: "Create artifact",
		Description: "Create a new artifact with a title and complete self-contained HTML document. Returns its id; network origins require owner approval in Exhibit before the artifact can reach them."}, t.createArtifact)
	mcp.AddTool(s, &mcp.Tool{Name: "get_artifact", Title: "Read artifact",
		Description: "Read the artifact's current HTML source and metadata (title, network allowlist, " +
			"stored assets). Read before editing: saves carry the revision this returns.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.getArtifact)
	mcp.AddTool(s, &mcp.Tool{Name: "edit_artifact", Title: "Edit artifact",
		Description: "Make targeted in-place edits to the artifact source. Each edit's oldText must match " +
			"the current source exactly and only once: copy it from get_artifact with enough context to be " +
			"unique. Put several disjoint changes in one call; overlapping or touching edits must be merged. " +
			"Prefer this over write_artifact for anything short of a full rewrite."}, t.editArtifact)
	mcp.AddTool(s, &mcp.Tool{Name: "write_artifact", Title: "Write artifact",
		Description: "Replace the artifact source wholesale with a complete, self-contained HTML document " +
			"(never a fragment or diff). Use only for a full rewrite; optionally retitle it."}, t.writeArtifact)
	mcp.AddTool(s, &mcp.Tool{Name: "get_state", Title: "Read artifact state",
		Description: "Read every key/value the artifact has stored (what its localStorage writes through to), " +
			"verbatim. Saved data is read-only here: it is what the person entered into their tool, and an " +
			"agent never writes or erases it. Use it to understand the data the source must keep working with. " +
			"The owner decides per artifact whether agents may read it at all; when they have not, the call " +
			"is refused and retrying will not help.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.getState)
	mcp.AddTool(s, &mcp.Tool{Name: "get_widget", Title: "Read widget",
		Description: "Read the artifact's gallery widget source, or learn that it has none yet.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.getWidget)
	mcp.AddTool(s, &mcp.Tool{Name: "set_widget", Title: "Set widget",
		Description: "Save or replace the artifact's gallery widget: the small glanceable tile its library card " +
			"shows. body is a complete self-contained HTML document that reads the artifact's localStorage " +
			"keys, cannot write state and is not interactive. Read with get_widget or get_artifact first. " +
			"Prefer edit_widget for small changes to an existing tile."}, t.setWidget)
	mcp.AddTool(s, &mcp.Tool{Name: "edit_widget", Title: "Edit widget",
		Description: "Make targeted in-place edits to the artifact's gallery widget, like edit_artifact does for " +
			"the source. Read it with get_widget first; if there is no widget yet, create it with set_widget."}, t.editWidget)
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// call makes one API request and decodes a successful JSON answer into out,
// which may be nil. A refusal comes back as an error worded for the agent.
func (t *tools) call(ctx context.Context, method, path string, body, out any) error {
	resp, err := t.api.Do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return refusal(method, path, resp)
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Body, out)
}

// apiError is the JSON shape of the API's refusals.
type apiError struct {
	Error    string `json:"error"`
	Code     string `json:"code"`
	Resource string `json:"resource"`
}

// refusal turns a non-2xx answer into what the agent should do next. Nothing
// here retries: a conflict means somebody else changed the artifact, and only
// the agent, having read what they did, can decide how to redo its change.
func refusal(method, path string, resp *Response) error {
	var body apiError
	_ = json.Unmarshal(resp.Body, &body)
	resource := "artifact"
	if strings.HasSuffix(path, "/widget") {
		resource = "widget"
	}
	switch resp.Status {
	case http.StatusConflict, http.StatusPreconditionRequired:
		return fmt.Errorf("the %s changed since you read it, or you have not read it yet. Nothing was saved. "+
			"Read it again with get_%s, reapply your change to what is there now, and save again", resource, resource)
	case http.StatusRequestEntityTooLarge:
		if body.Code == "storage_quota_exceeded" {
			return fmt.Errorf("Exhibit refused the save: %s. Nothing was saved. Storage includes earlier versions, "+
				"so a smaller document still adds bytes; the owner has to free storage or raise the allowance", body.Error)
		}
		return fmt.Errorf("Exhibit refused the save as too large (%s). Nothing was saved, and the same content "+
			"will fail the same way: make it smaller. Inline base64 images, fonts or wasm are the usual cause", firstNonEmpty(body.Error, "request body too large"))
	case http.StatusUnauthorized:
		return errors.New("Exhibit no longer accepts this token: the owner may have revoked it. Ask them to issue a new one")
	case http.StatusForbidden:
		// Saved data is withheld per artifact by its owner (Exh-dabd), which is
		// a decision about this artifact, not about the token's reach.
		if strings.HasSuffix(path, "/state") {
			return fmt.Errorf("Exhibit did not share this artifact's saved data: %s", firstNonEmpty(body.Error, "forbidden"))
		}
		return fmt.Errorf("Exhibit refused %s %s: %s. This token reaches only its owner's library", method, path, firstNonEmpty(body.Error, "forbidden"))
	case http.StatusNotFound:
		return errors.New("the artifact does not exist in this library. Search with search_artifacts; artifacts shared with the owner are not accessible through this connection")
	case http.StatusBadRequest:
		return fmt.Errorf("Exhibit refused the request: %s. Nothing was saved", firstNonEmpty(body.Error, strings.TrimSpace(string(resp.Body))))
	}
	return fmt.Errorf("Exhibit answered %s %s with %d: %s", method, path, resp.Status, truncate(strings.TrimSpace(string(resp.Body)), 300))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// artifactPath accepts only the canonical UUID strings generated by Exhibit.
func artifactPath(id, sub string) (string, error) {
	if len(id) != 36 {
		return "", errors.New("artifact_id must be a canonical UUID from search_artifacts or create_artifact")
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return "", errors.New("artifact_id must be a canonical UUID from search_artifacts or create_artifact")
	}
	return "/api/artifacts/" + id + sub, nil
}

// A title is one line of display text, and on a URL-ingested artifact it is
// the most attacker-controllable field there is: no newline to start what
// looks like a message, and no length to bury the rest under.
const maxTitleChars = 200

func oneLine(s string) string {
	return truncate(strings.Join(strings.Fields(s), " "), maxTitleChars)
}

type artifactView struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	NetworkAllowlist []string `json:"network_allowlist"`
	SourceRevision   string   `json:"source_revision"`
	WidgetRevision   string   `json:"widget_revision"`
}

func (t *tools) getArtifact(ctx context.Context, _ *mcp.CallToolRequest, in artifactInput) (*mcp.CallToolResult, any, error) {
	path, err := artifactPath(in.ArtifactID, "")
	if err != nil {
		return nil, nil, err
	}
	defer t.lockArtifact(in.ArtifactID)()
	rstate := t.revisions(in.ArtifactID)
	var a artifactView
	if err := t.call(ctx, http.MethodGet, path+"?body=true", nil, &a); err != nil {
		return nil, nil, err
	}
	if a.SourceRevision == "" {
		return nil, nil, errors.New("Exhibit returned no source revision; read the artifact again")
	}
	rstate.sourceRevision = a.SourceRevision
	if a.WidgetRevision != rstate.widgetRevision {
		rstate.widgetRevision, rstate.widgetRead = a.WidgetRevision, false
	}
	t.remember(in.ArtifactID, rstate)
	meta := fmt.Sprintf("id: %s\ntitle: %s\nallowlist: [%s]\n%s", a.ID, oneLine(a.Title),
		strings.Join(a.NetworkAllowlist, ", "), t.assetSummary(ctx, path))
	return text("current artifact source:\n\n" + meta + "\n\n" + a.Body), nil, nil
}

// assetSummary lists the artifact's out-of-line payloads (av-20fk), never
// their bytes. Without it the agent reads a bare fetch('…/app.wasm') in a
// document with no such file and "fixes" code that works: the redirect to the
// stored copy is injected at render, outside the source.
func (t *tools) assetSummary(ctx context.Context, path string) string {
	var r struct {
		Assets []struct {
			SourceURL   string `json:"source_url"`
			ContentType string `json:"content_type"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"assets"`
	}
	if err := t.call(ctx, http.MethodGet, path+"/assets", nil, &r); err != nil {
		// Never fail a read over metadata: the source is what was asked for.
		return "assets: unavailable"
	}
	if len(r.Assets) == 0 {
		return "assets: none"
	}
	lines := []string{"assets: these URLs are served from stored copies at render time, so the fetches\n" +
		"below work even though the files are not in the source. Leave the fetch calls as\n" +
		"they are; the redirect is injected outside this document."}
	for _, a := range r.Assets {
		lines = append(lines, fmt.Sprintf("  - %s (%s, %d bytes)", a.SourceURL, a.ContentType, a.SizeBytes))
	}
	return strings.Join(lines, "\n")
}

type patchResponse struct {
	Artifact         artifactView `json:"artifact"`
	NetworkFootprint []string     `json:"network_footprint"`
}

func (t *tools) saveSource(ctx context.Context, id string, patch map[string]any) (*mcp.CallToolResult, any, error) {
	path, err := artifactPath(id, "")
	if err != nil {
		return nil, nil, err
	}
	defer t.lockArtifact(id)()
	rstate := t.revisions(id)
	revision := rstate.sourceRevision
	if revision == "" {
		return nil, nil, errors.New("read the artifact with get_artifact before changing it")
	}
	patch["expected_source_revision"] = revision
	var r patchResponse
	if err := t.call(ctx, http.MethodPatch, path, patch, &r); err != nil {
		return nil, nil, err
	}
	rstate.sourceRevision = r.Artifact.SourceRevision
	t.remember(id, rstate)
	// Report only what the new body leaves blocked: an approved origin is
	// not blocked, and re-flagging it would be a false alarm.
	approved := map[string]bool{}
	for _, o := range r.Artifact.NetworkAllowlist {
		approved[o] = true
	}
	var blocked []string
	for _, o := range r.NetworkFootprint {
		if !approved[o] {
			blocked = append(blocked, o)
		}
	}
	msg := "Saved the artifact."
	if len(blocked) > 0 {
		msg += " Network footprint, blocked until the owner approves it in Exhibit: " + strings.Join(blocked, ", ") + "."
	}
	return text(msg), nil, nil
}

func (t *tools) editArtifact(ctx context.Context, _ *mcp.CallToolRequest, in editsInput) (*mcp.CallToolResult, any, error) {
	if _, err := artifactPath(in.ArtifactID, ""); err != nil {
		return nil, nil, err
	}
	if len(in.Edits) == 0 {
		return nil, nil, errors.New("edits must contain at least one replacement")
	}
	return t.saveSource(ctx, in.ArtifactID, map[string]any{"edits": in.Edits})
}

func (t *tools) writeArtifact(ctx context.Context, _ *mcp.CallToolRequest, in writeArtifactInput) (*mcp.CallToolResult, any, error) {
	if _, err := artifactPath(in.ArtifactID, ""); err != nil {
		return nil, nil, err
	}
	patch := map[string]any{"body": in.Body}
	if title := strings.TrimSpace(in.Title); title != "" {
		patch["title"] = title
	}
	return t.saveSource(ctx, in.ArtifactID, patch)
}

// formatState renders each value as its exact bytes under a delimiter rather
// than as a JSON object, which would backslash-escape every quote in a
// JSON-shaped value and invite the agent to send the escaped text back.
func formatState(state map[string]string, keys []string) string {
	if len(keys) == 0 {
		return "(no state stored for this artifact)"
	}
	blocks := make([]string, len(keys))
	for i, k := range keys {
		quoted, _ := json.Marshal(k)
		blocks[i] = fmt.Sprintf("--- key: %s ---\n%s", quoted, state[k])
	}
	return strings.Join(blocks, "\n\n")
}

func (t *tools) getState(ctx context.Context, _ *mcp.CallToolRequest, in artifactInput) (*mcp.CallToolResult, any, error) {
	path, err := artifactPath(in.ArtifactID, "/state")
	if err != nil {
		return nil, nil, err
	}
	resp, err := t.api.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, nil, refusal(http.MethodGet, path, resp)
	}
	// Decode preserving the server's key order, which is the order the
	// agent should see and keep.
	keys, state, err := orderedStringMap(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return text(formatState(state, keys)), nil, nil
}

func orderedStringMap(data []byte) ([]string, map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	opening, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if opening != json.Delim('{') {
		return nil, nil, errors.New("Exhibit returned state that is not a JSON object")
	}
	state := map[string]string{}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		k, _ := tok.(string)
		var v string
		if err := dec.Decode(&v); err != nil {
			return nil, nil, err
		}
		keys = append(keys, k)
		state[k] = v
	}
	closing, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if closing != json.Delim('}') {
		return nil, nil, errors.New("Exhibit returned an incomplete state object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, errors.New("Exhibit returned trailing data after state")
	}
	return keys, state, nil
}

type widgetView struct {
	Body           string `json:"body"`
	WidgetRevision string `json:"widget_revision"`
}

func (t *tools) getWidget(ctx context.Context, _ *mcp.CallToolRequest, in artifactInput) (*mcp.CallToolResult, any, error) {
	path, err := artifactPath(in.ArtifactID, "/widget")
	if err != nil {
		return nil, nil, err
	}
	defer t.lockArtifact(in.ArtifactID)()
	rstate := t.revisions(in.ArtifactID)
	resp, err := t.api.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var w widgetView
	_ = json.Unmarshal(resp.Body, &w)
	switch {
	case resp.Status == http.StatusOK && w.WidgetRevision != "":
		rstate.widgetRevision, rstate.widgetRead = w.WidgetRevision, true
		t.remember(in.ArtifactID, rstate)
		return text("current gallery widget:\n\n" + w.Body), nil, nil
	case resp.Status == http.StatusNotFound && w.WidgetRevision != "":
		// No widget is an answer, not a failure: the revision is still the
		// one a first set_widget has to present.
		rstate.widgetRevision, rstate.widgetRead = w.WidgetRevision, false
		t.remember(in.ArtifactID, rstate)
		return text("This artifact has no widget yet."), nil, nil
	}
	return nil, nil, refusal(http.MethodGet, path, resp)
}

type widgetSaveResponse struct {
	WidgetRevision string   `json:"widget_revision"`
	Unapproved     []string `json:"unapproved_origins"`
}

func (t *tools) saveWidget(ctx context.Context, id string, payload map[string]any, needText bool) (*mcp.CallToolResult, any, error) {
	path, err := artifactPath(id, "/widget")
	if err != nil {
		return nil, nil, err
	}
	defer t.lockArtifact(id)()
	rstate := t.revisions(id)
	revision, read := rstate.widgetRevision, rstate.widgetRead
	if revision == "" || (needText && !read) {
		return nil, nil, errors.New("read the widget with get_widget before changing it")
	}
	payload["expected_widget_revision"] = revision
	var r widgetSaveResponse
	if err := t.call(ctx, http.MethodPut, path, payload, &r); err != nil {
		return nil, nil, err
	}
	rstate.widgetRevision = r.WidgetRevision
	// A full write is text the agent now knows; edits leave what it
	// knew plus its own replacements, which it also knows.
	rstate.widgetRead = true
	t.remember(id, rstate)
	msg := "Saved the gallery widget."
	if len(r.Unapproved) > 0 {
		// The widget shares the artifact's allowlist, so an origin the
		// artifact was never approved for is blocked at render. Say so
		// rather than let it surface as a blank tile.
		msg += " Warning: it references " + strings.Join(r.Unapproved, ", ") + ", which the artifact's network " +
			"allowlist does not cover; the browser will block those. Prefer a widget with no external references."
	}
	return text(msg), nil, nil
}

func (t *tools) setWidget(ctx context.Context, _ *mcp.CallToolRequest, in setWidgetInput) (*mcp.CallToolResult, any, error) {
	if _, err := artifactPath(in.ArtifactID, ""); err != nil {
		return nil, nil, err
	}
	return t.saveWidget(ctx, in.ArtifactID, map[string]any{"body": in.Body}, false)
}

func (t *tools) editWidget(ctx context.Context, _ *mcp.CallToolRequest, in editsInput) (*mcp.CallToolResult, any, error) {
	if _, err := artifactPath(in.ArtifactID, ""); err != nil {
		return nil, nil, err
	}
	if len(in.Edits) == 0 {
		return nil, nil, errors.New("edits must contain at least one replacement")
	}
	return t.saveWidget(ctx, in.ArtifactID, map[string]any{"edits": in.Edits}, true)
}
