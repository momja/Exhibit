package bundle

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
)

type localProviderRequest struct {
	service, path, authorization, apiKey string
}

// Exercise both runners through their local HTTP interface. Each service has
// a distinct TLS origin; the Node test-only hook pins reviewed public names to
// those fixtures without weakening the production runner's DNS boundary.
func TestPortableConnectionsAddAuthenticationWithoutNarrowingNetwork(t *testing.T) {
	for _, runner := range []string{"go", "node"} {
		t.Run(runner, func(t *testing.T) {
			var mu sync.Mutex
			var received []localProviderRequest
			origins := map[string]string{}
			fixtures := map[string]string{}
			for _, service := range []string{"one", "two", "public", "cdn", "unapproved"} {
				fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					received = append(received, localProviderRequest{service, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-API-Key")})
					mu.Unlock()
					if destination, redirect := strings.CutPrefix(r.URL.Path, "/redirect/"); redirect {
						http.Redirect(w, r, origins[destination]+"/redirect-target", http.StatusFound)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"service": service, "authorization": r.Header.Get("Authorization"), "api_key": r.Header.Get("X-API-Key")})
				}))
				t.Cleanup(fixture.Close)
				fixtures[service] = fixture.URL
				origins[service] = fixture.URL
				if runner == "node" {
					origins[service] = "https://" + service + ".example"
				}
			}
			allowlist := []string{origins["one"], origins["two"], origins["public"], origins["cdn"]}
			app := New("Independent connections", "<!doctype html><html><head></head><body>Connected app</body></html>", map[string]string{"snapshot": "existing saved data"}, allowlist, []ConnectionRequirement{
				{Name: "First API", Kind: "bearer", Origin: origins["one"], DataOrigins: []string{origins["one"], origins["unapproved"]}},
				{Name: "Second API", Kind: "api-key", Header: "X-API-Key", Origin: origins["two"], DataOrigins: []string{origins["two"]}},
			})
			directory := unpack(t, app)
			policy := []byte(fmt.Sprintf(`{"data_policies":{%q:[%q],%q:[%q]},"financial_origins":[%q]}`, origins["one"], origins["one"], origins["two"], origins["two"], origins["one"]))
			statePath := filepath.Join(directory, "state.json")
			if err := os.WriteFile(statePath+".policy.json", policy, 0600); err != nil {
				t.Fatal(err)
			}
			initialState, _ := os.ReadFile(statePath)
			var hook string
			if runner == "node" {
				mapping := map[string]string{}
				for service, origin := range origins {
					mapping[origin] = fixtures[service]
				}
				raw, _ := json.Marshal(mapping)
				hook = fmt.Sprintf(`import dns from "node:dns/promises";import https from "node:https";
	dns.lookup=async()=>[{address:"8.8.8.8",family:4}];
	const destinations=%s;const original=https.request;
	https.request=(url,options,callback)=>{const fixture=new URL(destinations[url.origin]);fixture.pathname=url.pathname;fixture.search=url.search;return original(fixture,{...options,lookup:undefined,headers:{...options.headers,host:url.host},rejectUnauthorized:false},callback)};`, raw)
			}
			var base, authority, token, framePolicy string
			client := &http.Client{Timeout: 10 * time.Second}
			policyPattern := regexp.MustCompile(`const policy = "([A-Za-z0-9_-]+)"`)
			refreshFrame := func() {
				t.Helper()
				response := localRequest(t, client, base, authority, "GET", "/__exhibit/render", "", "", "")
				for _, allowed := range allowlist {
					if !strings.Contains(response.Header.Get("Content-Security-Policy"), allowed) {
						t.Errorf("connection narrowed CSP for %s", allowed)
					}
				}
				render := bodyText(t, response)
				if !strings.Contains(render, "existing saved data") {
					t.Error("connecting or restarting discarded saved data")
				}
				match := policyPattern.FindStringSubmatch(render)
				if len(match) != 2 {
					t.Fatal("render omitted its document policy token")
				}
				framePolicy = match[1]
			}
			start := func() {
				t.Helper()
				if runner == "node" {
					base = runNode(t, app, hook, directory)
					authority = strings.TrimPrefix(base, "http://")
				} else {
					gate, err := egress.NewForTests(allowlist, egress.Limits{})
					if err != nil {
						t.Fatal(err)
					}
					server, err := NewServer(app, ServerOptions{AllowedHosts: []string{"runner.example"}, StatePath: statePath, Gate: gate})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(server.Close)
					fixture := httptest.NewServer(server)
					t.Cleanup(fixture.Close)
					base, authority = fixture.URL, "runner.example"
				}
				response := localRequest(t, client, base, authority, "GET", "/__exhibit/connect", "", "", "")
				page := bodyText(t, response)
				if strings.Contains(page, `name="data_origins"`) || strings.Contains(page, "Data may go to") {
					t.Error("local connection page still configures a destination ceiling")
				}
				token = csrfFromPage(t, page)
				refreshFrame()
			}
			connect := func(index int, kind, secret, action string) {
				t.Helper()
				form := url.Values{"csrf": {token}, "index": {strconv.Itoa(index)}, "kind": {kind}, "secret": {secret}, "header": {"X-API-Key"}, "daily_limit": {"3"}, "cache_minutes": {"1"}, "action": {action}, "data_origins": {origins["unapproved"]}}
				response := localRequest(t, client, base, authority, "POST", "/__exhibit/connect", form.Encode(), "", "application/x-www-form-urlencoded")
				page := bodyText(t, response)
				if response.StatusCode != 200 || action == "connect" && !strings.Contains(page, "Connected locally") || secret != "" && strings.Contains(page, secret) {
					t.Fatalf("connection change failed or exposed its secret: %d %s", response.StatusCode, page)
				}
				refreshFrame()
			}
			request := func(service, path string) (int, relayResponse, []localProviderRequest) {
				t.Helper()
				mu.Lock()
				before := len(received)
				mu.Unlock()
				input, _ := json.Marshal(relayRequest{URL: origins[service] + path, Method: "GET", Policy: framePolicy, Headers: map[string]string{"Authorization": "Bearer forged-guest", "X-API-Key": "forged-guest", "Cookie": "forged=yes"}})
				response := localRequest(t, client, base, authority, "POST", "/__exhibit/relay", string(input), token, "application/json")
				var result relayResponse
				_ = json.NewDecoder(response.Body).Decode(&result)
				response.Body.Close()
				body, _ := base64.StdEncoding.DecodeString(result.BodyBase64)
				for _, secret := range []string{"token-one", "key-two", "replacement-one", "forged-guest"} {
					if bytes.Contains(body, []byte(secret)) {
						t.Errorf("relay response exposed %s", secret)
					}
				}
				mu.Lock()
				calls := append([]localProviderRequest(nil), received[before:]...)
				mu.Unlock()
				return response.StatusCode, result, calls
			}
			assertCall := func(service, path, authorization, key string) {
				t.Helper()
				status, result, calls := request(service, path)
				if status != 200 || result.Status != 200 || len(calls) != 1 || calls[0].authorization != authorization || calls[0].apiKey != key {
					t.Fatalf("wrong authentication for %s: status %d, result %#v, contacts %#v", service, status, result, calls)
				}
			}
			assertCached := func(service, path string) {
				t.Helper()
				status, result, calls := request(service, path)
				if status != 200 || result.Status != 200 || len(calls) != 0 {
					t.Fatalf("another connection invalidated %s's reusable response: status %d, contacts %#v", service, status, calls)
				}
			}
			start()
			assertCall("public", "/data", "", "")
			assertCall("cdn", "/asset.js", "", "")
			connect(0, "bearer", "token-one", "connect")
			assertCall("one", "/data", "Bearer token-one", "")
			status, result, calls := request("one", "/data")
			if status != 200 || result.Status != 200 || len(calls) != 0 {
				t.Fatal("local connection response reuse stopped working")
			}
			connect(1, "api-key", "key-two", "connect")
			assertCall("two", "/data", "", "key-two")
			assertCached("one", "/data")
			for _, destination := range []string{"two", "public"} {
				status, _, calls = request("one", "/redirect/"+destination)
				if status == 200 || len(calls) != 1 || calls[0].service != "one" || calls[0].authorization != "Bearer token-one" {
					t.Fatalf("connector redirect escaped its credential scope: status %d, contacts %#v", status, calls)
				}
			}
			status, _, calls = request("one", "/over-budget")
			if status != 429 || len(calls) != 0 {
				t.Fatal("per-connection quota no longer refused provider contact")
			}
			assertCall("two", "/still-available", "", "key-two")
			assertCall("public", "/still-available", "", "")
			assertCall("cdn", "/still-available.js", "", "")
			status, _, calls = request("unapproved", "/data")
			if status == 200 || len(calls) != 0 {
				t.Fatal("legacy DataOrigins expanded the artifact allowlist")
			}
			status, _, calls = request("public", "/redirect/unapproved")
			if status == 200 || len(calls) != 1 || calls[0].service != "public" {
				t.Fatal("standalone redirect reached an unapproved origin")
			}
			status, result, calls = request("public", "/redirect/one")
			if status != 200 || result.Status != 200 || len(calls) != 2 || calls[1].service != "one" || calls[1].authorization != "" || calls[1].apiKey != "" {
				t.Fatalf("standalone redirect acquired another origin's credential: status %d contacts %#v", status, calls)
			}
			connect(0, "bearer", "", "disconnect")
			status, _, calls = request("one", "/data")
			if status != 409 || len(calls) != 0 {
				t.Fatal("removed required connection did not prompt for reconnection")
			}
			assertCached("two", "/data")
			assertCall("two", "/after-other-disconnect", "", "key-two")
			assertCall("public", "/after-disconnect", "", "")
			connect(0, "bearer", "replacement-one", "connect")
			assertCall("one", "/replacement", "Bearer replacement-one", "")
			assertCached("two", "/data")
			connect(1, "api-key", "", "disconnect")
			assertCall("one", "/after-second-disconnect", "Bearer replacement-one", "")
			assertCall("cdn", "/after-disconnect.js", "", "")
			start()
			assertCall("public", "/after-restart", "", "")
			assertCall("cdn", "/after-restart.js", "", "")
			for _, service := range []string{"one", "two"} {
				status, _, calls = request(service, "/after-restart")
				if status != 409 || len(calls) != 0 {
					t.Fatalf("%s credentials survived runner restart", service)
				}
			}
			currentState, _ := os.ReadFile(statePath)
			legacyPolicy, _ := os.ReadFile(statePath + ".policy.json")
			if !bytes.Equal(currentState, initialState) || !bytes.Equal(legacyPolicy, policy) {
				t.Error("connection changes required legacy state or policy cleanup")
			}
		})
	}
}

func TestConnectionRequirementStillNeedsArtifactOriginApproval(t *testing.T) {
	app := New("No implicit approval", "<p>Example</p>", nil, []string{"https://public.example"}, []ConnectionRequirement{{Name: "Private API", Kind: "bearer", Origin: "https://unapproved.example", DataOrigins: []string{"https://public.example"}}})
	if _, err := app.Bytes("example"); err == nil {
		t.Fatal("connection requirement silently expanded the artifact allowlist")
	}
}
