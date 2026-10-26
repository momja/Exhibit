package sprites

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

const managementToken = "test-management-credential-never-enters-sprite"
const endpointToken = "test-worker-endpoint-credential-0123456789"

type fakeSprite struct {
	resource Resource
	files    map[string][]byte
	service  []byte
}
type fakeAPI struct {
	t              *testing.T
	server         *httptest.Server
	mu             sync.Mutex
	sprites        map[string]*fakeSprite
	calls          []string
	createTimes    []time.Time
	fault          func(http.ResponseWriter, *http.Request) bool
	serviceOutput  string
	readyOverride  *ReadyResponse
	initWait       <-chan struct{}
	nestedOrg      bool
	pageSize       int
	runningLimit   int
	tunnels        atomic.Int64
	closedTunnels  atomic.Int64
	workerRequests atomic.Int64
	// Only records whether credentials crossed the boundary, never their values.
	leaked atomic.Bool
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, sprites: map[string]*fakeSprite{}, serviceOutput: "{\"type\":\"started\"}\n{\"type\":\"complete\"}\n", runningLimit: 100}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}
func (f *fakeAPI) add(name, status string, age time.Duration) {
	resource := Resource{Name: name, ID: "sprite-" + name, Status: status, URL: "https://" + name + ".sprites.app", CreatedAt: time.Now().Add(-age)}
	resource.URLSettings = &struct {
		Auth string `json:"auth"`
	}{"sprite"}
	f.sprites[name] = &fakeSprite{resource: resource, files: map[string][]byte{}}
}
func (f *fakeAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+managementToken {
		http.Error(w, "unauthorized", 401)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	fault := f.fault
	f.mu.Unlock()
	if fault != nil && fault(w, r) {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/proxy") {
		websocket.Handler(f.proxy).ServeHTTP(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v1/sprites" {
		if r.Method == "POST" {
			f.createTimes = append(f.createTimes, time.Now())
			var input struct {
				Name        string `json:"name"`
				URLSettings struct {
					Auth string `json:"auth"`
				} `json:"url_settings"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Name == "" || input.URLSettings.Auth != "sprite" {
				http.Error(w, "bad create", 400)
				return
			}
			if f.sprites[input.Name] != nil {
				http.Error(w, "duplicate", 409)
				return
			}
			f.add(input.Name, "cold", 0)
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": input.Name})
		} else {
			if r.URL.Query().Get("max_results") != "500" {
				f.t.Error("missing bounded pagination")
			}
			names := make([]string, 0, len(f.sprites))
			for name := range f.sprites {
				names = append(names, name)
			}
			sort.Strings(names)
			offset := 0
			if next := r.URL.Query().Get("continuation_token"); next != "" {
				_, _ = fmt.Sscan(next, &offset)
			}
			end := len(names)
			if f.pageSize > 0 {
				end = min(end, offset+f.pageSize)
			}
			resources := []Resource{}
			for _, name := range names[offset:end] {
				resources = append(resources, f.sprites[name].resource)
			}
			response := map[string]any{"sprites": resources, "has_more": end < len(names), "running_limit": f.runningLimit, "warm_limit": 100}
			if end < len(names) {
				response["next_continuation_token"] = fmt.Sprint(end)
			}
			if f.nestedOrg {
				delete(response, "running_limit")
				delete(response, "warm_limit")
				response["org"] = map[string]int{"running_limit": f.runningLimit, "warm_limit": 100}
			}
			_ = json.NewEncoder(w).Encode(response)
		}
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/sprites/"), "/")
	name := parts[0]
	sprite := f.sprites[name]
	if sprite == nil {
		http.Error(w, "not found", 404)
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			_ = json.NewEncoder(w).Encode(sprite.resource)
		case "DELETE":
			delete(f.sprites, name)
			w.WriteHeader(204)
		default:
			http.Error(w, "bad method", 405)
		}
		return
	}
	if parts[1] == "fs" {
		path := r.URL.Query().Get("path")
		if r.URL.Query().Get("workingDir") != "/" {
			f.t.Error("file request needs workingDir")
		}
		switch parts[2] {
		case "write":
			if r.Method != "PUT" || r.URL.Query().Get("mkdir") != "true" || r.URL.Query().Get("mkdirParents") != "true" {
				f.t.Error("incorrect write schema")
			}
			expectedMode := "0600"
			if strings.HasSuffix(path, "bootstrap.sh") {
				expectedMode = "0700"
			}
			if r.URL.Query().Get("mode") != expectedMode {
				f.t.Error("incorrect private file mode")
			}
			data, _ := io.ReadAll(r.Body)
			if path != workerDirectory+"/endpoint-token" && bytes.Contains(data, []byte(endpointToken)) {
				f.t.Error("worker token leaked outside private token file")
			}
			if bytes.Contains(data, []byte(managementToken)) {
				f.leaked.Store(true)
			}
			sprite.files[path] = data
			_, _ = io.WriteString(w, "{}")
		case "read":
			data, ok := sprite.files[path]
			if !ok {
				http.Error(w, "missing file", 404)
				return
			}
			_, _ = w.Write(data)
		}
		return
	}
	if parts[1] == "services" {
		switch r.Method {
		case "PUT":
			data, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			expected := serviceDefinition(8080)
			if !reflect.DeepEqual(body, decodeMap(expected)) {
				f.t.Error("incorrect Service request")
			}
			if bytes.Contains(data, []byte(managementToken)) || bytes.Contains(data, []byte(endpointToken)) {
				f.leaked.Store(true)
			}
			sprite.service = data
			_, _ = io.WriteString(w, f.serviceOutput)
		case "GET":
			if sprite.service == nil {
				http.Error(w, "missing service", 404)
				return
			}
			_, _ = w.Write(sprite.service)
		}
		return
	}
	http.Error(w, "unexpected route", 404)
}
func decodeMap(value any) map[string]any {
	var result map[string]any
	_ = json.Unmarshal(mustJSON(value), &result)
	return result
}
func (f *fakeAPI) proxy(ws *websocket.Conn) {
	defer ws.Close()
	f.tunnels.Add(1)
	defer f.closedTunnels.Add(1)
	_ = ws.SetDeadline(time.Now().Add(3 * time.Second))
	var init struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if websocket.JSON.Receive(ws, &init) != nil {
		return
	}
	if init.Host != "127.0.0.1" || init.Port != 8080 {
		f.t.Error("incorrect TCP proxy init")
	}
	f.mu.Lock()
	wait := f.initWait
	f.mu.Unlock()
	if wait != nil {
		<-wait
	}
	if websocket.JSON.Send(ws, map[string]string{"status": "connected"}) != nil {
		return
	}
	ws.PayloadType = websocket.BinaryFrame
	request, err := http.ReadRequest(bufio.NewReader(ws))
	if err != nil {
		return
	}
	defer request.Body.Close()
	data, _ := io.ReadAll(request.Body)
	f.workerRequests.Add(1)
	if request.Header.Get("Authorization") != "Bearer "+endpointToken {
		f.t.Error("tunneled request lacks per-worker authorization")
	}
	for _, values := range request.Header {
		for _, v := range values {
			if strings.Contains(v, managementToken) {
				f.leaked.Store(true)
			}
		}
	}
	if bytes.Contains(data, []byte(managementToken)) {
		f.leaked.Store(true)
	}
	if request.Header.Get("Exhibit-Agent-Version") != "1" {
		f.t.Error("missing worker protocol version")
	}
	if request.Header.Get("Connection") != "close" {
		f.t.Error("worker connection must close after request")
	}
	var response []byte
	if request.URL.Path == "/v1/ready" {
		name := strings.Split(strings.TrimPrefix(ws.Request().URL.Path, "/v1/sprites/"), "/")[0]
		f.mu.Lock()
		sprite := f.sprites[name]
		var stored manifest
		if sprite != nil {
			_ = json.Unmarshal(sprite.files[workerDirectory+"/manifest.json"], &stored)
		}
		ready := ReadyResponse{Version: "1", PiVersion: "1.3.0", ExecutionID: stored.Identity.ExecutionID, SessionID: stored.Identity.SessionID, Capabilities: []string{"commands", "snapshots"}}
		if f.readyOverride != nil {
			ready = *f.readyOverride
		}
		f.mu.Unlock()
		response = mustJSON(ready)
	} else {
		response = []byte(`{"ok":true}`)
	}
	resp := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{"Content-Type": {"application/json"}}, ContentLength: int64(len(response)), Body: io.NopCloser(bytes.NewReader(response)), Close: true}
	if resp.Write(ws) != nil {
		return
	}
	// Client must close the socket on response completion, even while the worker
	// would otherwise keep it open. No heartbeat/pool is allowed to hold it active.
	var b [1]byte
	_, _ = ws.Read(b[:])
}
func testConfig(f *fakeAPI) Config {
	return Config{Token: managementToken, BaseURL: f.server.URL, Prefix: "exhibit-test", Release: Release{Version: "revision-123", URL: "https://releases.example/worker/revision-123.tar.gz", SHA256: strings.Repeat("a", 64)}, StartupTimeout: time.Second, RequestTimeout: 300 * time.Millisecond, CleanupTimeout: 300 * time.Millisecond, CreateInterval: time.Millisecond, Retry: RetryPolicy{Attempts: 3, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}}
}
func testRequest() Request {
	return Request{Identity: Identity{OwnerID: "owner-1", SessionID: "conversation-1", ExecutionID: "attempt-1"}, EndpointToken: endpointToken}
}
func newTestProvisioner(t *testing.T, f *fakeAPI, configure func(*Config)) *Provisioner {
	config := testConfig(f)
	if configure != nil {
		configure(&config)
	}
	p, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func countCalls(f *fakeAPI, method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if call == method+" "+path {
			n++
		}
	}
	return n
}
func eventuallyClosed(t *testing.T, f *fakeAPI) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for f.closedTunnels.Load() != f.tunnels.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.closedTunnels.Load() != f.tunnels.Load() {
		t.Error("worker tunnel remained open after response")
	}
}

func TestProvisionBootstrapAuthorizationAndIdempotence(t *testing.T) {
	f := newFakeAPI(t)
	p := newTestProvisioner(t, f, nil)
	request := testRequest()
	endpoint, err := p.Provision(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.CloseIdleConnections()
	if endpoint.Name != p.Name(request.Identity) || endpoint.EndpointToken != endpointToken || endpoint.Client == nil || endpoint.Destroy == nil {
		t.Fatal("incomplete endpoint")
	}
	httpRequest, _ := http.NewRequest("POST", endpoint.URL+"/v1/commands", strings.NewReader(`{"id":"one"}`))
	// A caller cannot cause the org token to enter the worker HTTP request.
	httpRequest.Header.Set("Authorization", "Bearer "+managementToken)
	response, err := endpoint.Client.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	duplicate, err := p.Provision(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.CloseIdleConnections()
	if countCalls(f, "POST", "/v1/sprites") != 1 || countCalls(f, "PUT", "/v1/sprites/"+endpoint.Name+"/services/"+serviceName) != 1 {
		t.Error("idempotent provision recreated or restarted worker")
	}
	for _, method := range []string{"GET", "POST"} {
		cross, _ := http.NewRequest(method, "https://other.example/v1/commands", nil)
		if _, err := endpoint.Client.Do(cross); err == nil {
			t.Error("transport accepted another origin")
		}
	}
	if f.leaked.Load() {
		t.Error("management credential crossed worker boundary")
	}
	eventuallyClosed(t, f)
	if err := endpoint.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(context.Background(), request.Identity); err != nil {
		t.Fatal("destroy must tolerate already missing Sprite")
	}
}

func TestQueryDoesNotWakeAndWakeRevalidates(t *testing.T) {
	f := newFakeAPI(t)
	p := newTestProvisioner(t, f, nil)
	request := testRequest()
	endpoint, err := p.Provision(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.CloseIdleConnections()
	beforeTunnels := f.tunnels.Load()
	beforeReads := countCalls(f, "GET", "/v1/sprites/"+endpoint.Name+"/fs/read")
	f.mu.Lock()
	f.sprites[endpoint.Name].resource.Status = "cold"
	f.mu.Unlock()
	resource, err := p.Query(context.Background(), request.Identity)
	if err != nil || resource.Status != "cold" {
		t.Fatalf("query failed: %v", err)
	}
	if f.tunnels.Load() != beforeTunnels || countCalls(f, "GET", "/v1/sprites/"+endpoint.Name+"/fs/read") != beforeReads {
		t.Error("metadata query woke idle Sprite")
	}
	awake, err := p.Wake(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	awake.CloseIdleConnections()
	wrong := request
	wrong.EndpointToken = strings.Repeat("b", 40)
	if _, err := p.Wake(context.Background(), wrong); !errors.Is(err, ErrConflict) {
		t.Errorf("wrong endpoint token: %v", err)
	}
	if countCalls(f, "POST", "/v1/sprites") != 1 {
		t.Error("wake unexpectedly created a Sprite")
	}
	if err := p.Destroy(context.Background(), request.Identity); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Wake(context.Background(), request); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing Sprite wake: %v", err)
	}
	if countCalls(f, "POST", "/v1/sprites") != 1 {
		t.Error("missing wake silently replaced an assignment")
	}
	eventuallyClosed(t, f)
}

func TestNoCrossOwnerOrCredentialReuse(t *testing.T) {
	f := newFakeAPI(t)
	p := newTestProvisioner(t, f, nil)
	first := testRequest()
	endpoint, err := p.Provision(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.CloseIdleConnections()
	rotated := first
	rotated.EndpointToken = strings.Repeat("c", 40)
	if _, err := p.Provision(context.Background(), rotated); !errors.Is(err, ErrConflict) {
		t.Errorf("credential rotation must require a new identity: %v", err)
	}
	if countCalls(f, "DELETE", "/v1/sprites/"+endpoint.Name) != 0 {
		t.Error("conflict destroyed existing assignment")
	}
	other := first
	other.Identity.OwnerID = "owner-2"
	second, err := p.Provision(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	second.CloseIdleConnections()
	if endpoint.Name == second.Name {
		t.Error("owner change reused a Sprite")
	}
	f.mu.Lock()
	f.sprites[endpoint.Name].files[workerDirectory+"/manifest.json"] = mustJSON(manifest{Identity: other.Identity})
	f.mu.Unlock()
	if _, err := p.Provision(context.Background(), first); !errors.Is(err, ErrConflict) {
		t.Errorf("tampered assignment: %v", err)
	}
	if countCalls(f, "DELETE", "/v1/sprites/"+endpoint.Name) != 0 {
		t.Error("identity conflict destroyed another assignment")
	}
}

func TestBootstrapFailureCleanupAndSanitizedErrors(t *testing.T) {
	for _, scenario := range []string{"service-exit", "upload", "canceled-ready", "bad-ready", "get-after-create", "malformed-create"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFakeAPI(t)
			config := testConfig(f)
			request := testRequest()
			switch scenario {
			case "service-exit":
				f.serviceOutput = "{\"type\":\"stderr\",\"data\":\"" + managementToken + "\"}\n{\"type\":\"exit\",\"exit_code\":1}\n"
			case "upload":
				f.fault = func(w http.ResponseWriter, r *http.Request) bool {
					if strings.HasSuffix(r.URL.Path, "/fs/write") {
						http.Error(w, managementToken+endpointToken, 403)
						return true
					}
					return false
				}
			case "canceled-ready":
				// Cancel only after creation/bootstrap. A short wall-clock deadline
				// can expire before creation on a busy host, leaving nothing to delete.
				config.StartupTimeout = 10 * time.Second
				config.CheckReady = func(ctx context.Context, _ *Endpoint) error {
					cancel()
					<-ctx.Done()
					return ctx.Err()
				}
			case "bad-ready":
				f.readyOverride = &ReadyResponse{Version: "1", PiVersion: "1.3.0", ExecutionID: "wrong", SessionID: request.Identity.SessionID}
			case "get-after-create":
				f.fault = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method == "GET" && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/v1/sprites/"), "/") && r.URL.Path != "/v1/sprites" {
						f.mu.Lock()
						exists := f.sprites[strings.TrimPrefix(r.URL.Path, "/v1/sprites/")] != nil
						f.mu.Unlock()
						if exists {
							http.Error(w, "denied", 403)
							return true
						}
					}
					return false
				}
			case "malformed-create":
				f.fault = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method == "POST" && r.URL.Path == "/v1/sprites" {
						var in struct {
							Name string `json:"name"`
						}
						_ = json.NewDecoder(r.Body).Decode(&in)
						f.mu.Lock()
						f.add(in.Name, "cold", 0)
						f.mu.Unlock()
						w.WriteHeader(201)
						_, _ = io.WriteString(w, "bad response")
						return true
					}
					return false
				}
			}
			p, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Provision(ctx, request)
			if err == nil {
				t.Fatal("expected startup failure")
			}
			if strings.Contains(err.Error(), managementToken) || strings.Contains(err.Error(), endpointToken) {
				t.Error("secret in startup error")
			}
			if countCalls(f, "DELETE", "/v1/sprites/"+p.Name(request.Identity)) != 1 {
				t.Error("failed startup did not clean up its Sprite")
			}
			if scenario == "canceled-ready" && !errors.Is(err, context.Canceled) {
				t.Errorf("missing startup cancellation: %v", err)
			}
		})
	}
}

func TestExpiredStartupDoesNotDeleteAnUncreatedSprite(t *testing.T) {
	f := newFakeAPI(t)
	p := newTestProvisioner(t, f, nil)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := p.Provision(ctx, testRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected expired startup deadline, got %v", err)
	}
	if countCalls(f, "POST", "/v1/sprites") != 0 || countCalls(f, "DELETE", "/v1/sprites/"+p.Name(testRequest().Identity)) != 0 {
		t.Fatal("startup expired before creation must not create or delete a Sprite")
	}
}

func TestFailedCleanupReportsOrphan(t *testing.T) {
	f := newFakeAPI(t)
	f.serviceOutput = "{\"type\":\"error\",\"data\":\"private\"}\n"
	f.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "DELETE" {
			http.Error(w, managementToken, 503)
			return true
		}
		return false
	}
	p := newTestProvisioner(t, f, nil)
	_, err := p.Provision(context.Background(), testRequest())
	if err == nil || !strings.Contains(err.Error(), "failed startup cleanup") {
		t.Fatalf("missing cleanup failure: %v", err)
	}
	if strings.Contains(err.Error(), managementToken) {
		t.Error("upstream error body escaped")
	}
	f.mu.Lock()
	remaining := len(f.sprites)
	f.mu.Unlock()
	if remaining != 1 {
		t.Error("test must retain an orphan for later reconciliation")
	}
}

func TestPartialAndUncertainCreateRecovery(t *testing.T) {
	for _, scenario := range []string{"partial", "uncertain", "lost-response", "conflict"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFakeAPI(t)
			p := newTestProvisioner(t, f, nil)
			request := testRequest()
			name := p.Name(request.Identity)
			if scenario == "partial" {
				f.add(name, "cold", time.Minute)
			} else {
				var fired atomic.Bool
				f.fault = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method != "POST" || r.URL.Path != "/v1/sprites" || fired.Swap(true) {
						return false
					}
					f.mu.Lock()
					f.add(name, "cold", 0)
					f.mu.Unlock()
					if scenario == "lost-response" {
						connection, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = connection.Close()
						}
						return true
					}
					status := 500
					if scenario == "conflict" {
						status = 409
					}
					http.Error(w, "ambiguous create", status)
					return true
				}
			}
			endpoint, err := p.Provision(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			endpoint.CloseIdleConnections()
			if countCalls(f, "POST", "/v1/sprites") > 1 {
				t.Error("uncertain create was repeated instead of queried")
			}
			f.mu.Lock()
			n := len(f.sprites)
			f.mu.Unlock()
			if n != 1 {
				t.Error("create recovery duplicated a Sprite")
			}
		})
	}
}
