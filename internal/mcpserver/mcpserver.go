// Package mcpserver exposes one Exhibit artifact to an agent of the user's
// own (Claude Code, Cursor, Codex, ...) as Model Context Protocol tools
// (Exh-17a5).
//
// The tools are the Pi extension's (internal/agent/ext/exhibit.ts) minus what
// only makes sense inside Exhibit's own chat: no create_artifact, because the
// credential is issued for an artifact that exists; no get_selection, because
// there is no preview to select in; no ask_user, because the agent's own
// client already talks to the person. None takes an artifact id.
//
// Authority does not live here. Every tool is a request to the HTTP API made
// with an owner-issued agent grant, which the API resolves to one owner and
// one artifact and holds to agentScopeAllows. A rewritten copy of this
// package gets a 403, not another artifact.
//
// Untrusted text (the artifact's source, title, state and widget) reaches the
// agent only as a tool result, which is its own message role (av-5s7g). The
// server's instructions say so; nothing here wraps or fences a result.
package mcpserver

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"

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
	t := &tools{api: api}
	t.register(s)
	return s
}

// tools is one connection's view of its artifact: which one it is, and the
// revisions the agent last read, which every save must present.
type tools struct {
	api API

	mu         sync.Mutex
	artifactID string
	// sourceRevision and widgetRevision are the content revisions the agent
	// last saw. Empty means it has not read that resource, and a save of it
	// is refused until it does.
	sourceRevision string
	widgetRevision string
	// widgetRead says the agent has seen the widget's text, not only its
	// revision: edit_widget's oldText has to come from somewhere.
	widgetRead bool
}

type noInput struct{}

type editsInput struct {
	Edits []edit `json:"edits" jsonschema:"one or more targeted replacements, applied atomically"`
}

type edit struct {
	OldText string `json:"oldText" jsonschema:"exact text to find; must occur exactly once in the current source"`
	NewText string `json:"newText" jsonschema:"replacement text; empty deletes"`
}

type writeArtifactInput struct {
	Body  string `json:"body" jsonschema:"the complete replacement HTML document"`
	Title string `json:"title,omitempty" jsonschema:"a new title; omit to keep the current one"`
}

type setStateInput struct {
	Key   string `json:"key" jsonschema:"the state key to write"`
	Value string `json:"value" jsonschema:"the new value, verbatim; JSON-encode structured data yourself"`
}

type deleteStateInput struct {
	Key *string `json:"key,omitempty" jsonschema:"the key to delete; omit to erase ALL state"`
}

type setWidgetInput struct {
	Body string `json:"body" jsonschema:"the complete HTML document for the widget"`
}

func (t *tools) register(s *mcp.Server) {
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
		Description: "Read every key/value the artifact has stored (what its localStorage writes through to). " +
			"Values are shown verbatim; copy them back exactly via set_state except for what you were asked " +
			"to change.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.getState)
	mcp.AddTool(s, &mcp.Tool{Name: "set_state", Title: "Set state key",
		Description: "Write one state key, creating it if absent. Every other key is untouched. If the value " +
			"is JSON and you change one field, reproduce every other byte exactly as get_state showed it."}, t.setState)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_state", Title: "Delete artifact state",
		Description: "Delete one state key (idempotent), or omit key to erase ALL state for the artifact. " +
			"Erasing everything is irreversible: only do it when the user clearly asked to reset everything.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)}}, t.deleteState)
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

func boolPtr(b bool) *bool { return &b }

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
		return fmt.Errorf("Exhibit refused %s %s: %s. This token reaches only one artifact's source, widget and "+
			"saved data", method, path, firstNonEmpty(body.Error, "forbidden"))
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

// artifactPath is the API path of this connection's artifact plus sub. The id
// comes from the API's own answer about the credential, never from the agent.
func (t *tools) artifactPath(ctx context.Context, sub string) (string, error) {
	t.mu.Lock()
	id := t.artifactID
	t.mu.Unlock()
	if id == "" {
		var cred struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := t.call(ctx, http.MethodGet, "/api/agent/credential", nil, &cred); err != nil {
			return "", err
		}
		if cred.ArtifactID == "" {
			return "", errors.New("this token is not bound to an artifact")
		}
		id = cred.ArtifactID
		t.mu.Lock()
		t.artifactID = id
		t.mu.Unlock()
	}
	return "/api/artifacts/" + url.PathEscape(id) + sub, nil
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

func (t *tools) getArtifact(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	var a artifactView
	if err := t.call(ctx, http.MethodGet, path+"?body=true", nil, &a); err != nil {
		return nil, nil, err
	}
	if a.SourceRevision == "" {
		return nil, nil, errors.New("Exhibit returned no source revision; read the artifact again")
	}
	t.mu.Lock()
	t.sourceRevision = a.SourceRevision
	if a.WidgetRevision != t.widgetRevision {
		t.widgetRevision, t.widgetRead = a.WidgetRevision, false
	}
	t.mu.Unlock()
	meta := fmt.Sprintf("id: %s\ntitle: %s\nallowlist: [%s]\n%s", a.ID, oneLine(a.Title),
		strings.Join(a.NetworkAllowlist, ", "), t.assetSummary(ctx, path))
	return text("current source of the artifact this server edits:\n\n" + meta + "\n\n" + a.Body), nil, nil
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

func (t *tools) saveSource(ctx context.Context, patch map[string]any) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	revision := t.sourceRevision
	t.mu.Unlock()
	if revision == "" {
		return nil, nil, errors.New("read the artifact with get_artifact before changing it")
	}
	patch["expected_source_revision"] = revision
	var r patchResponse
	if err := t.call(ctx, http.MethodPatch, path, patch, &r); err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	if t.sourceRevision == revision {
		t.sourceRevision = r.Artifact.SourceRevision
	}
	t.mu.Unlock()
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
	msg := "Saved the artifact as a new version."
	if len(blocked) > 0 {
		msg += " Network footprint, blocked until the owner approves it in Exhibit: " + strings.Join(blocked, ", ") + "."
	}
	return text(msg), nil, nil
}

func (t *tools) editArtifact(ctx context.Context, _ *mcp.CallToolRequest, in editsInput) (*mcp.CallToolResult, any, error) {
	if len(in.Edits) == 0 {
		return nil, nil, errors.New("edits must contain at least one replacement")
	}
	return t.saveSource(ctx, map[string]any{"edits": in.Edits})
}

func (t *tools) writeArtifact(ctx context.Context, _ *mcp.CallToolRequest, in writeArtifactInput) (*mcp.CallToolResult, any, error) {
	patch := map[string]any{"body": in.Body}
	if title := strings.TrimSpace(in.Title); title != "" {
		patch["title"] = title
	}
	return t.saveSource(ctx, patch)
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

func (t *tools) getState(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "/state")
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
	if _, err := dec.Token(); err != nil {
		return nil, nil, err
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
	return keys, state, nil
}

func (t *tools) setState(ctx context.Context, _ *mcp.CallToolRequest, in setStateInput) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "/state")
	if err != nil {
		return nil, nil, err
	}
	if err := t.call(ctx, http.MethodPut, path, map[string]string{"key": in.Key, "value": in.Value}, nil); err != nil {
		return nil, nil, err
	}
	quoted, _ := json.Marshal(in.Key)
	return text(fmt.Sprintf("Set state key %s.", quoted)), nil, nil
}

func (t *tools) deleteState(ctx context.Context, _ *mcp.CallToolRequest, in deleteStateInput) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "/state")
	if err != nil {
		return nil, nil, err
	}
	// Presence, not emptiness: "" is a legitimate Web Storage key, and
	// reading it as absent would turn deleting it into erasing everything.
	if in.Key != nil {
		// A query value, never a path segment: a key of ".." as a segment
		// resolves away and lands on the artifact delete (av-hh1o).
		if err := t.call(ctx, http.MethodDelete, path+"?key="+url.QueryEscape(*in.Key), nil, nil); err != nil {
			return nil, nil, err
		}
		quoted, _ := json.Marshal(*in.Key)
		return text(fmt.Sprintf("Deleted state key %s.", quoted)), nil, nil
	}
	if err := t.call(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return nil, nil, err
	}
	return text("Erased all state on this artifact."), nil, nil
}

type widgetView struct {
	Body           string `json:"body"`
	WidgetRevision string `json:"widget_revision"`
}

func (t *tools) getWidget(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "/widget")
	if err != nil {
		return nil, nil, err
	}
	resp, err := t.api.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var w widgetView
	_ = json.Unmarshal(resp.Body, &w)
	switch {
	case resp.Status == http.StatusOK && w.WidgetRevision != "":
		t.mu.Lock()
		t.widgetRevision, t.widgetRead = w.WidgetRevision, true
		t.mu.Unlock()
		return text("current gallery widget of the artifact this server edits:\n\n" + w.Body), nil, nil
	case resp.Status == http.StatusNotFound && w.WidgetRevision != "":
		// No widget is an answer, not a failure: the revision is still the
		// one a first set_widget has to present.
		t.mu.Lock()
		t.widgetRevision, t.widgetRead = w.WidgetRevision, false
		t.mu.Unlock()
		return text("This artifact has no widget yet."), nil, nil
	}
	return nil, nil, refusal(http.MethodGet, path, resp)
}

type widgetSaveResponse struct {
	WidgetRevision string   `json:"widget_revision"`
	Unapproved     []string `json:"unapproved_origins"`
}

func (t *tools) saveWidget(ctx context.Context, payload map[string]any, needText bool) (*mcp.CallToolResult, any, error) {
	path, err := t.artifactPath(ctx, "/widget")
	if err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	revision, read := t.widgetRevision, t.widgetRead
	t.mu.Unlock()
	if revision == "" || (needText && !read) {
		return nil, nil, errors.New("read the widget with get_widget before changing it")
	}
	payload["expected_widget_revision"] = revision
	var r widgetSaveResponse
	if err := t.call(ctx, http.MethodPut, path, payload, &r); err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	if t.widgetRevision == revision {
		t.widgetRevision = r.WidgetRevision
		// A full write is text the agent now knows; edits leave what it
		// knew plus its own replacements, which it also knows.
		t.widgetRead = true
	}
	t.mu.Unlock()
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
	return t.saveWidget(ctx, map[string]any{"body": in.Body}, false)
}

func (t *tools) editWidget(ctx context.Context, _ *mcp.CallToolRequest, in editsInput) (*mcp.CallToolResult, any, error) {
	if len(in.Edits) == 0 {
		return nil, nil, errors.New("edits must contain at least one replacement")
	}
	return t.saveWidget(ctx, map[string]any{"edits": in.Edits}, true)
}
