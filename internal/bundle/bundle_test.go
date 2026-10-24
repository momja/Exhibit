package bundle

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
)

func unpack(t *testing.T, app *Bundle) string {
	t.Helper()
	data, err := app.Bytes("example")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, entry := range zr.File {
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(entry.Name)), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBundleContainsRequirementsWithoutConnectionIdentityOrSecrets(t *testing.T) {
	app := New("Connected example", "<!doctype html><html><head></head><body>Hello</body></html>", map[string]string{"count": "2"}, []string{"https://api.example"}, []ConnectionRequirement{{Name: "Weather", Kind: "api-key", Origin: "https://api.example", Header: "X-API-Key"}})
	dir := unpack(t, app)
	for _, name := range []string{"serve.mjs", "host.js", "shim.js", "index.html", "state.json", "exhibit.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	read, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if read.Page != app.Page || read.State["count"] != "2" || len(read.Manifest.Connections) != 1 {
		t.Fatalf("bundle changed during read: %#v", read)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "exhibit.json"))
	for _, field := range []string{"secret", "connection_id", "owner_id", "artifact_id", "server.js"} {
		if bytes.Contains(raw, []byte(field)) {
			t.Errorf("export contains credential or original identity field %s", field)
		}
	}
	if _, err := app.Bytes("../escape"); err == nil {
		t.Fatal("unsafe directory accepted")
	}
}

func TestReadRejectsUnsafeAndDuplicateArchives(t *testing.T) {
	for _, names := range [][]string{{"../exhibit.json"}, {"root/../../escape"}, {"root/exhibit.json", "other/index.html"}, {"root/exhibit.json", "root/exhibit.json"}} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range names {
			w, _ := zw.Create(name)
			_, _ = io.WriteString(w, "{}")
		}
		_ = zw.Close()
		filename := filepath.Join(t.TempDir(), "app.zip")
		if err := os.WriteFile(filename, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(filename); err == nil {
			t.Fatalf("unsafe archive accepted: %v", names)
		}
	}
}

var tokenPattern = regexp.MustCompile(`name="csrf" value="([A-Za-z0-9_-]+)"`)

func csrfFromPage(t *testing.T, body string) string {
	t.Helper()
	match := tokenPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing session token in connect page: %s", body)
	}
	return match[1]
}
func localRequest(t *testing.T, client *http.Client, base, authority, method, path, body, token, contentType string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = authority
	if method == "POST" {
		request.Header.Set("Origin", "http://"+authority)
		request.Header.Set("Content-Type", contentType)
		if token != "" {
			request.Header.Set("X-Exhibit-CSRF", token)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func bodyText(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestGoRunnerHostOriginCSRFAndOpaqueRender(t *testing.T) {
	app := New("Local example", "<html><head></head><body>example</body></html>", map[string]string{"__proto__": "saved", "danger": "</script><img src=x>"}, []string{"https://api.example"}, nil)
	s, err := NewServer(app, ServerOptions{AllowedHosts: []string{"runner.example:8787"}, StatePath: filepath.Join(t.TempDir(), "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fixture := httptest.NewServer(s)
	defer fixture.Close()
	hostResponse := localRequest(t, fixture.Client(), fixture.URL, "runner.example:8787", "GET", "/", "", "", "")
	if !strings.Contains(hostResponse.Header.Get("Content-Security-Policy"), "frame-src 'self'") || !strings.Contains(hostResponse.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("host permits a frame to navigate to another origin")
	}
	hostResponse.Body.Close()
	response := localRequest(t, fixture.Client(), fixture.URL, "evil.example", "GET", "/", "", "", "")
	if response.StatusCode != 403 {
		t.Fatal("forged Host accepted")
	}
	response.Body.Close()
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example:8787", "GET", "/__exhibit/connect", "", "", "")
	if response.Header.Get("Referrer-Policy") != "same-origin" {
		t.Error("connect form must preserve its same-origin Origin header for CSRF validation")
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("connect page can be framed")
	}
	token := csrfFromPage(t, bodyText(t, response))
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example:8787", "GET", "/__exhibit/render", "", "", "")
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox allow-scripts") || !strings.Contains(response.Header.Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Error("direct render is not sandboxed")
	}
	render := bodyText(t, response)
	if strings.Contains(render, token) || strings.Contains(render, "</script><img src=x>") {
		t.Error("frame contains host authority or unescaped state")
	}
	for _, origin := range []string{"null", "http://evil.example", ""} {
		request, _ := http.NewRequest("POST", fixture.URL+"/__exhibit/state", strings.NewReader(`{"op":"set","key":"stolen","value":"yes"}`))
		request.Host = "runner.example:8787"
		request.Header.Set("Origin", origin)
		request.Header.Set("X-Exhibit-CSRF", token)
		response, err := fixture.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 403 {
			t.Errorf("accepted Origin %q", origin)
		}
		response.Body.Close()
	}
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example:8787", "POST", "/__exhibit/state", `{"op":"set","key":"count","value":"3"}`, "", "application/json")
	if response.StatusCode != 403 {
		t.Fatal("accepted missing CSRF")
	}
	response.Body.Close()
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example:8787", "POST", "/__exhibit/state", `{"op":"set","key":"count","value":"3"}`, token, "application/json")
	if response.StatusCode != 204 {
		t.Fatalf("state save failed: %s", bodyText(t, response))
	}
	response.Body.Close()
	raw, err := os.ReadFile(s.options.StatePath)
	if err != nil || !bytes.Contains(raw, []byte(`"count":"3"`)) {
		t.Fatalf("state was not persisted: %s %v", raw, err)
	}
}

func TestLocalResponseRedactsReflectedCredentials(t *testing.T) {
	response := &egress.Response{Body: []byte(`{"echo":"private-user:private-password","key":"a secret","path":"/sensitive/path/accounts"}`), URL: "https://api.example/sensitive/path/accounts", Header: http.Header{"Etag": {"a+secret"}}}
	scrubLocalResponse(localConnection{Kind: "basic", Username: "private-user", Password: "private-password", Secret: "a secret", BasePath: "/sensitive/path"}, response)
	for _, secret := range []string{"private-user", "private-password", "a secret", "sensitive/path"} {
		if bytes.Contains(response.Body, []byte(secret)) || strings.Contains(response.URL, secret) {
			t.Errorf("reflected credential leaked: %s", secret)
		}
	}
	if response.Header.Get("Etag") == "a+secret" {
		t.Error("encoded credential leaked in response header")
	}
}

func TestGoLocalConnectionStaysOutsideFrameAndUsesSharedGate(t *testing.T) {
	var providerCalls int
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		if r.Header.Get("Authorization") != "Bearer local-secret" {
			t.Errorf("incorrect credential %q", r.Header.Get("Authorization"))
		}
		for _, name := range []string{"Cookie", "Origin", "Referer", "X-Forwarded-For"} {
			if r.Header.Get(name) != "" {
				t.Errorf("forged %s left gate", name)
			}
		}
		w.Header().Set("Set-Cookie", "no=thanks")
		_, _ = io.WriteString(w, "connected")
	}))
	defer provider.Close()
	g, err := egress.NewForTests([]string{provider.URL}, egress.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	app := New("Connected example", "<html><head></head><body>Hello</body></html>", nil, []string{provider.URL}, []ConnectionRequirement{{Name: "Provider", Kind: "bearer", Origin: provider.URL}})
	s, err := NewServer(app, ServerOptions{AllowedHosts: []string{"runner.example"}, Gate: g})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fixture := httptest.NewServer(s)
	defer fixture.Close()
	response := localRequest(t, fixture.Client(), fixture.URL, "runner.example", "GET", "/__exhibit/connect", "", "", "")
	token := csrfFromPage(t, bodyText(t, response))
	input := relayRequest{URL: provider.URL + "/data", Method: "GET", Policy: s.framePolicy, Headers: map[string]string{"Authorization": "Bearer attacker", "Cookie": "bad=yes", "Origin": "https://evil.example", "X-Forwarded-For": "127.0.0.1"}}
	payload, _ := json.Marshal(input)
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/relay", string(payload), token, "application/json")
	if response.StatusCode != 409 || providerCalls != 0 {
		t.Fatal("missing local connection did not prompt")
	}
	response.Body.Close()
	form := url.Values{"csrf": {token}, "index": {"0"}, "kind": {"bearer"}, "secret": {"local-secret"}, "action": {"connect"}}
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
	if strings.Contains(bodyText(t, response), "local-secret") {
		t.Error("connect UI returned credential")
	}
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/relay", string(payload), token, "application/json")
	if response.StatusCode != 409 || !strings.Contains(bodyText(t, response), `"reload":true`) {
		t.Error("old frame retained authority after a connection change")
	}
	input.Policy = s.framePolicy
	payload, _ = json.Marshal(input)
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/relay", string(payload), token, "application/json")
	var result relayResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || result.Status != 200 || result.BodyBase64 != base64.StdEncoding.EncodeToString([]byte("connected")) || providerCalls != 1 {
		t.Fatalf("relay failed: %#v status %d", result, response.StatusCode)
	}
	if result.Headers["Set-Cookie"] != "" {
		t.Fatal("unsafe response headers forwarded")
	}
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "GET", "/__exhibit/render", "", "", "")
	if strings.Contains(bodyText(t, response), "local-secret") {
		t.Error("credential reached artifact frame")
	}
	data, err := app.Bytes("app")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("local-secret")) {
		t.Error("credential entered bundle export")
	}
}

func TestGoBasicAccessURLCacheQuotaAndNoPrivateEndpointLeak(t *testing.T) {
	calls := 0
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/private/access/accounts" {
			t.Errorf("wrong local access path %q", r.URL.Path)
		}
		user, password, _ := r.BasicAuth()
		if user != "username" || password != "secret" {
			t.Error("missing Basic credentials")
		}
		_, _ = io.WriteString(w, `{"accounts":[]}`)
	}))
	defer provider.Close()
	g, _ := egress.NewForTests([]string{provider.URL}, egress.Limits{})
	app := New("Connected API", "<p>Accounts</p>", nil, []string{provider.URL}, nil)
	s, err := NewServer(app, ServerOptions{AllowedHosts: []string{"runner.example"}, Gate: g})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fixture := httptest.NewServer(s)
	defer fixture.Close()
	response := localRequest(t, fixture.Client(), fixture.URL, "runner.example", "GET", "/__exhibit/connect", "", "", "")
	token := csrfFromPage(t, bodyText(t, response))
	access, _ := url.Parse(provider.URL + "/private/access")
	access.User = url.UserPassword("username", "secret")
	form := url.Values{"csrf": {token}, "index": {"0"}, "kind": {"basic"}, "access_url": {access.String()}, "daily_limit": {"1"}, "cache_minutes": {"10"}, "action": {"connect"}}
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
	connected := bodyText(t, response)
	if !strings.Contains(connected, "Connected locally") || strings.Contains(connected, "private/access") || strings.Contains(connected, "username") && strings.Contains(connected, access.String()) {
		t.Fatalf("connect failed or returned private access URL: %s", connected)
	}
	for i := 0; i < 2; i++ {
		input, _ := json.Marshal(relayRequest{URL: provider.URL + "/accounts", Method: "GET", Policy: s.framePolicy})
		response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/relay", string(input), token, "application/json")
		body := bodyText(t, response)
		if response.StatusCode != 200 || strings.Contains(body, "/private/access/") {
			t.Fatalf("private path leaked: %d %s", response.StatusCode, body)
		}
	}
	if calls != 1 {
		t.Fatalf("Basic response cache missed: %d calls", calls)
	}
	input, _ := json.Marshal(relayRequest{URL: provider.URL + "/accounts?fresh=1", Method: "GET", Policy: s.framePolicy})
	response = localRequest(t, fixture.Client(), fixture.URL, "runner.example", "POST", "/__exhibit/relay", string(input), token, "application/json")
	if response.StatusCode != 429 || calls != 1 {
		t.Fatalf("daily quota not enforced: %d calls, %s", calls, bodyText(t, response))
	}
	response.Body.Close()
}

func TestCredentialHeadersCannotControlTransport(t *testing.T) {
	for _, header := range []string{"Host", "Cookie", "Origin", "Referer", "Connection", "Proxy-Authorization", "X-Forwarded-Host", "User-Agent", "Sec-Fetch-Site", "Content-Length", "Bad Header"} {
		if credentialHeader(header) {
			t.Errorf("unsafe credential header accepted: %s", header)
		}
	}
	for _, header := range []string{"X-API-Key", "Authorization", "ApiKey"} {
		if !credentialHeader(header) {
			t.Errorf("credential header refused: %s", header)
		}
	}
}

func ExampleNew() {
	app := New("Weather", "<h1>Weather</h1>", nil, []string{"https://api.example"}, []ConnectionRequirement{{Name: "Weather API", Kind: "api-key", Origin: "https://api.example", Header: "X-API-Key"}})
	fmt.Println(app.Manifest.Format)
	// Output: exhibit-relay
}

// Both shipped runners use this fixture to exercise response reuse and quota
// through their public local HTTP interface, rather than internal counters.
func exerciseLocalConnectionSafety(t *testing.T, runner string) {
	t.Helper()
	var calls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		user, password, _ := r.BasicAuth()
		if user != "api-user" || password != "api-password" {
			t.Errorf("wrong Basic credentials: %q %q", user, password)
		}
		if !strings.HasPrefix(r.URL.Path, "/private<token>/") {
			t.Errorf("request left its private base path: %q", r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, "/private%3Ctoken%3E/resource", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Etag", r.URL.EscapedPath())
		if strings.HasSuffix(r.URL.Path, "/vary-star") {
			w.Header().Set("Vary", "*")
		}
		if strings.HasSuffix(r.URL.Path, "/no-store") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.Header.Get("Range") != "" || strings.HasSuffix(r.URL.Path, "/partial") {
			w.WriteHeader(http.StatusPartialContent)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Path, "escaped": r.URL.EscapedPath(), "slash_escaped": strings.ReplaceAll(r.URL.Path, "/", `\/`), "authorization": r.Header.Get("Authorization"), "language": r.Header.Get("Accept-Language")})
	}))
	defer provider.Close()
	origin := provider.URL
	var base, authority string
	client := &http.Client{Timeout: 10 * time.Second}
	if runner == "node" {
		origin = "https://api.example"
		app := New("Basic API", "<html><head></head><body>Accounts</body></html>", nil, []string{origin}, []ConnectionRequirement{{Name: "API", Kind: "basic", Origin: origin}})
		hook := fmt.Sprintf(`import dns from "node:dns/promises";import https from "node:https";
	dns.lookup=async()=>[{address:"8.8.8.8",family:4}];
	const original=https.request;https.request=(url,options,callback)=>{const fixture=new URL(%q);fixture.pathname=url.pathname;fixture.search=url.search;return original(fixture,{...options,lookup:undefined,rejectUnauthorized:false},callback)};`, provider.URL)
		base = runNode(t, app, hook)
		authority = strings.TrimPrefix(base, "http://")
	} else {
		gate, _ := egress.NewForTests([]string{origin}, egress.Limits{})
		app := New("Basic API", "<html><head></head><body>Accounts</body></html>", nil, []string{origin}, []ConnectionRequirement{{Name: "API", Kind: "basic", Origin: origin}})
		server, err := NewServer(app, ServerOptions{AllowedHosts: []string{"runner.example"}, Gate: gate})
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		fixture := httptest.NewServer(server)
		defer fixture.Close()
		base, authority = fixture.URL, "runner.example"
	}
	response := localRequest(t, client, base, authority, "GET", "/__exhibit/connect", "", "", "")
	token := csrfFromPage(t, bodyText(t, response))
	var policy string
	policyPattern := regexp.MustCompile(`(?:const policy = |var policy = )"([A-Za-z0-9_-]+)"`)
	connect := func(limit int) {
		t.Helper()
		access, _ := url.Parse(origin + "/private%3Ctoken%3E")
		access.User = url.UserPassword("api-user", "api-password")
		form := url.Values{"csrf": {token}, "index": {"0"}, "kind": {"basic"}, "access_url": {access.String()}, "cache_minutes": {"1"}, "daily_limit": {strconv.Itoa(limit)}, "action": {"connect"}}
		response := localRequest(t, client, base, authority, "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
		text := bodyText(t, response)
		if !strings.Contains(text, "Connected locally") || strings.Contains(text, "private%3Ctoken%3E") || strings.Contains(text, "api-password") {
			t.Fatalf("connection failed or returned access data: %s", text)
		}
		response = localRequest(t, client, base, authority, "GET", "/__exhibit/render", "", "", "")
		match := policyPattern.FindStringSubmatch(bodyText(t, response))
		if len(match) != 2 {
			t.Fatal("render omitted policy token")
		}
		policy = match[1]
	}
	request := func(path string, headers map[string]string) (int, relayResponse) {
		t.Helper()
		input, _ := json.Marshal(relayRequest{URL: origin + path, Method: "GET", Policy: policy, Headers: headers})
		response := localRequest(t, client, base, authority, "POST", "/__exhibit/relay", string(input), token, "application/json")
		defer response.Body.Close()
		var result relayResponse
		_ = json.NewDecoder(response.Body).Decode(&result)
		body, _ := base64.StdEncoding.DecodeString(result.BodyBase64)
		if len(body) > 0 && !json.Valid(body) {
			t.Errorf("response redaction corrupted provider JSON: %s", body)
		}
		serialized, _ := json.Marshal(result)
		for _, private := range []string{"private", "token", "api-user", "api-password", base64.StdEncoding.EncodeToString([]byte("api-user:api-password"))} {
			if bytes.Contains(body, []byte(private)) || bytes.Contains(serialized, []byte(private)) {
				t.Errorf("private access data reflected into relay response: %s / %s", body, serialized)
			}
		}
		return response.StatusCode, result
	}
	connect(0)
	for _, test := range []struct {
		path    string
		headers map[string]string
		status  int
		calls   int32
	}{
		{"/resource", nil, 200, 1},
		{"/resource", map[string]string{"Range": "bytes=0-9"}, 206, 2},
		{"/resource", map[string]string{"If-None-Match": "tag"}, 304, 2},
		{"/resource", map[string]string{"Accept-Language": "fr"}, 200, 1},
		{"/resource", map[string]string{"Accept-Language": "en"}, 200, 1},
		{"/vary-star", nil, 200, 2},
		{"/no-store", nil, 200, 2},
		{"/partial", nil, 206, 2},
	} {
		before := calls.Load()
		for range 2 {
			status, result := request(test.path, test.headers)
			if status != 200 || result.Status != test.status {
				t.Fatalf("request %s failed: %d %#v", test.path, status, result)
			}
		}
		if calls.Load()-before != test.calls {
			t.Errorf("%s %v: wanted %d provider contacts, got %d", test.path, test.headers, test.calls, calls.Load()-before)
		}
	}
	connect(1)
	before := calls.Load()
	status, _ := request("/redirect", nil)
	if status != 429 || calls.Load()-before != 1 {
		t.Fatalf("redirect exceeded connection quota: status %d, %d provider calls", status, calls.Load()-before)
	}
}

func TestGoLocalConnectionResponseReuseAndQuotaSafety(t *testing.T) {
	exerciseLocalConnectionSafety(t, "go")
}
