package bundle

import (
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func runNode(t *testing.T, app *Bundle, hook string, reuseDirectory ...string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is not installed")
	}
	var dir string
	if len(reuseDirectory) > 0 {
		dir = reuseDirectory[0]
	} else {
		dir = unpack(t, app)
	}
	args := []string{"serve.mjs"}
	if hook != "" {
		if err := os.WriteFile(filepath.Join(dir, "test-hooks.mjs"), []byte(hook), 0600); err != nil {
			t.Fatal(err)
		}
		args = []string{"--import", "./test-hooks.mjs", "serve.mjs"}
	}
	command := exec.Command(node, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "PORT=0", "HOST=127.0.0.1", "EXHIBIT_CONNECTIONS=[]", "EXHIBIT_PUBLIC_HOST=")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := regexp.MustCompile(`http://localhost:([0-9]+)/`).FindStringSubmatch(scanner.Text()); len(match) == 2 {
				ready <- "http://127.0.0.1:" + match[1]
				return
			}
		}
		ready <- ""
	}()
	select {
	case base := <-ready:
		if base == "" {
			t.Fatal("Node runner stopped before opening its listener")
		}
		return base
	case <-time.After(10 * time.Second):
		t.Fatal("Node runner did not start")
	}
	return ""
}

func TestNodeRunnerLocalConnectionAndNetworkBoundary(t *testing.T) {
	var calls atomic.Int32
	var otherCalls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/claim" {
			_, _ = io.WriteString(w, "https://simple-user:simple-password@api.example/private/local")
			return
		}
		if r.Host == "second.example" {
			otherCalls.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Error("Node forwarded credentials across redirect")
			}
			_, _ = io.WriteString(w, "redirected")
			return
		}
		if r.URL.Path == "/private/local/accounts" {
			user, password, _ := r.BasicAuth()
			if user != "simple-user" || password != "simple-password" {
				t.Error("missing local SimpleFIN credentials")
			}
			_, _ = io.WriteString(w, `{"accounts":[]}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer local-secret" {
			t.Errorf("wrong Node credential %q", r.Header.Get("Authorization"))
		}
		for _, header := range []string{"Cookie", "Origin", "Referer", "X-Forwarded-For", "X-Arbitrary"} {
			if r.Header.Get(header) != "" {
				t.Errorf("Node forwarded guest %s", header)
			}
		}
		if r.Header.Get("User-Agent") != "Exhibit-Relay/1" {
			t.Errorf("dishonest Node UA %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "https://second.example/final", 302)
		case "/private":
			http.Redirect(w, r, "https://169.254.169.254/latest/meta-data", 302)
		case "/unapproved":
			http.Redirect(w, r, "https://unapproved.example/final", 302)
		case "/compressed":
			w.Header().Set("Content-Encoding", "gzip")
			writer := gzip.NewWriter(w)
			_, _ = io.WriteString(writer, strings.Repeat("x", 5<<20))
			_ = writer.Close()
		default:
			w.Header().Set("Set-Cookie", "must=not-leave")
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0, 1, 2, 255})
		}
	}))
	defer provider.Close()
	// The production runner has no private-host or TLS bypass. These test-only
	// process imports route already-checked public DNS literals to our TLS
	// fixture, while preserving the original authority for header assertions.
	hook := fmt.Sprintf(`import dns from "node:dns/promises";import https from "node:https";
dns.lookup=async host=>host==="inward.example"?[{address:"8.8.8.8",family:4},{address:"127.0.0.1",family:4}]:[{address:"8.8.8.8",family:4}];
const original=https.request;
https.request=(url,options,callback)=>{const fixture=new URL(%q);fixture.pathname=url.pathname;fixture.search=url.search;return original(fixture,{...options,lookup:undefined,headers:{...options.headers,host:url.host},rejectUnauthorized:false},callback)};
`, provider.URL)
	app := New("Node example", "<!doctype html><html><head></head><body>Hello</body></html>", map[string]string{"saved": "yes"}, []string{"https://api.example", "https://second.example", "https://inward.example"}, []ConnectionRequirement{{Name: "Provider", Kind: "bearer", Origin: "https://api.example"}})
	nodeDirectory := unpack(t, app)
	base := runNode(t, app, hook, nodeDirectory)
	authority := strings.TrimPrefix(base, "http://")
	client := &http.Client{Timeout: 10 * time.Second}
	response := localRequest(t, client, base, authority, "GET", "/__exhibit/connect", "", "", "")
	if response.Header.Get("Referrer-Policy") != "same-origin" {
		t.Error("Node connect form must preserve its same-origin Origin header")
	}
	token := csrfFromPage(t, bodyText(t, response))
	response = localRequest(t, client, base, "evil.example", "GET", "/", "", "", "")
	if response.StatusCode != 403 {
		t.Fatal("Node accepted forged Host")
	}
	response.Body.Close()
	response = localRequest(t, client, base, authority, "GET", "/__exhibit/render", "", "", "")
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox allow-scripts") {
		t.Error("Node direct render is not opaque")
	}
	render := bodyText(t, response)
	if strings.Contains(render, token) {
		t.Error("Node frame has host token")
	}
	policyPattern := regexp.MustCompile(`const policy = "([A-Za-z0-9_-]+)"`)
	policy := policyPattern.FindStringSubmatch(render)[1]
	refreshPolicy := func() {
		t.Helper()
		response := localRequest(t, client, base, authority, "GET", "/__exhibit/render", "", "", "")
		render := bodyText(t, response)
		match := policyPattern.FindStringSubmatch(render)
		if len(match) != 2 {
			t.Fatal("Node render omitted its network policy token")
		}
		policy = match[1]
	}
	for _, origin := range []string{"null", "http://evil.example", ""} {
		request, _ := http.NewRequest("POST", base+"/__exhibit/state", strings.NewReader(`{"op":"clear"}`))
		request.Header.Set("Origin", origin)
		request.Header.Set("X-Exhibit-CSRF", token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 403 {
			t.Errorf("Node accepted forged origin %q", origin)
		}
		response.Body.Close()
	}
	form := url.Values{"csrf": {token}, "index": {"0"}, "kind": {"bearer"}, "secret": {"local-secret"}, "action": {"connect"}}
	response = localRequest(t, client, base, authority, "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
	if response.Header.Get("Referrer-Policy") != "same-origin" {
		t.Error("Node connect result must preserve Origin for reconnect and disconnect forms")
	}
	if strings.Contains(bodyText(t, response), "local-secret") {
		t.Error("Node connect page returned secret")
	}
	oldInput, _ := json.Marshal(relayRequest{URL: "https://api.example/data", Method: "GET", Policy: policy})
	response = localRequest(t, client, base, authority, "POST", "/__exhibit/relay", string(oldInput), token, "application/json")
	if response.StatusCode != 409 || !strings.Contains(bodyText(t, response), `"reload":true`) {
		t.Error("Node old frame retained authority after connection change")
	}
	refreshPolicy()
	request := func(target string) (int, relayResponse) {
		t.Helper()
		input := relayRequest{URL: target, Method: "GET", Policy: policy, Headers: map[string]string{"Accept": "application/json", "Cookie": "forged=yes", "Authorization": "Bearer guest", "Origin": "https://evil.example", "Referer": "https://evil.example", "X-Forwarded-For": "127.0.0.1", "User-Agent": "Browser", "X-Arbitrary": "bad"}}
		payload, _ := json.Marshal(input)
		response := localRequest(t, client, base, authority, "POST", "/__exhibit/relay", string(payload), token, "application/json")
		defer response.Body.Close()
		var result relayResponse
		_ = json.NewDecoder(response.Body).Decode(&result)
		return response.StatusCode, result
	}
	status, result := request("https://api.example/data")
	if status != 200 || result.Status != 200 || result.BodyBase64 != base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 255}) {
		t.Fatalf("Node relay failed: %d %#v", status, result)
	}
	if result.Headers["set-cookie"] != "" {
		t.Error("Node leaked Set-Cookie")
	}
	status, result = request("https://api.example/redirect")
	if status != 200 || result.URL != "https://second.example/final" {
		t.Fatalf("Node allowed redirect failed: %d %#v", status, result)
	}
	for _, target := range []string{"https://api.example/private", "https://api.example/unapproved", "https://api.example/compressed", "https://inward.example/data", "https://127.0.0.1/", "https://2130706433/", "https://0177.0.0.1/", "https://0x7f000001/", "https://[::ffff:127.0.0.1]/", "https://100.100.100.200/", "http://api.example/data"} {
		status, _ := request(target)
		if status == 200 {
			t.Errorf("Node allowed unsafe target %s", target)
		}
	}
	before := calls.Load()
	form = url.Values{"csrf": {token}, "index": {"0"}, "kind": {"simplefin"}, "setup": {base64.StdEncoding.EncodeToString([]byte("https://api.example/claim"))}, "action": {"connect"}}
	response = localRequest(t, client, base, authority, "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
	text := bodyText(t, response)
	if !strings.Contains(text, "Connected locally") || strings.Contains(text, "simple-password") || strings.Contains(text, "/private/local") {
		t.Fatalf("Node SimpleFIN claim failed or leaked secret: %s", text)
	}
	refreshPolicy()
	response = localRequest(t, client, base, authority, "GET", "/__exhibit/render", "", "", "")
	if strings.Contains(response.Header.Get("Content-Security-Policy"), "second.example") {
		t.Error("Node financial frame retained another data destination")
	}
	response.Body.Close()
	for i := 0; i < 2; i++ {
		status, result = request("https://api.example/accounts")
		if status != 200 || result.URL != "https://api.example/accounts" {
			t.Fatalf("Node SimpleFIN endpoint leaked or failed: %d %#v", status, result)
		}
	}
	if calls.Load() != before+2 {
		t.Errorf("Node SimpleFIN cache missed: %d calls", calls.Load()-before)
	}
	status, _ = request("https://second.example/data")
	if status == 200 {
		t.Error("Node allowed financial data egress to a different origin")
	}
	previousOtherCalls := otherCalls.Load()
	for _, action := range []string{"disconnect", "connect"} {
		form = url.Values{"csrf": {token}, "index": {"0"}, "action": {action}, "kind": {"bearer"}, "secret": {"local-secret"}}
		response = localRequest(t, client, base, authority, "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
		response.Body.Close()
		refreshPolicy()
		status, _ = request("https://second.example/data")
		if status == 200 {
			t.Errorf("Node %s widened financial egress", action)
		}
		if action == "connect" {
			status, _ = request("https://api.example/redirect")
			if status == 200 {
				t.Error("Node replacement credential permitted financial redirect egress")
			}
		}
		if otherCalls.Load() != previousOtherCalls {
			t.Fatalf("Node %s contacted a forbidden financial destination", action)
		}
	}
	restarted := runNode(t, app, hook, nodeDirectory)
	restartedAuthority := strings.TrimPrefix(restarted, "http://")
	response = localRequest(t, client, restarted, restartedAuthority, "GET", "/__exhibit/connect", "", "", "")
	restartedCSRF := csrfFromPage(t, bodyText(t, response))
	response = localRequest(t, client, restarted, restartedAuthority, "GET", "/__exhibit/render", "", "", "")
	restartedPolicy := policyPattern.FindStringSubmatch(bodyText(t, response))[1]
	payload, _ := json.Marshal(relayRequest{URL: "https://second.example/data", Method: "GET", Policy: restartedPolicy, Headers: map[string]string{}})
	response = localRequest(t, client, restarted, restartedAuthority, "POST", "/__exhibit/relay", string(payload), restartedCSRF, "application/json")
	if response.StatusCode == 200 || otherCalls.Load() != previousOtherCalls {
		t.Fatal("Node restart lost financial data-destination policy")
	}
	response.Body.Close()
}
