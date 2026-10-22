package egress

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testGate(t *testing.T, origins []string, limits Limits) *Gate {
	t.Helper()
	g, err := NewForTests(origins, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g
}
func fetch(t *testing.T, g *Gate, target string) (*Response, error) {
	t.Helper()
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		t.Fatal(err)
	}
	return g.Do(context.Background(), req)
}
func denied(t *testing.T, err error, approvable bool) *DeniedError {
	t.Helper()
	var d *DeniedError
	if !errors.As(err, &d) {
		t.Fatalf("expected denial, got %v", err)
	}
	if (d.Origin != "") != approvable {
		t.Fatalf("unexpected approvability: %#v", d)
	}
	return d
}

func TestHostileGuestHeadersNeverLeaveRelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Cookie", "Authorization", "Origin", "Referer", "X-Forwarded-For", "X-Api-Key", "Proxy-Authorization", "Connection"} {
			if got := r.Header.Get(name); got != "" {
				t.Errorf("forwarded %s: %q", name, got)
			}
		}
		if r.Host == "attacker.example" {
			t.Error("forwarded forged Host")
		}
		if r.Header.Get("User-Agent") != "Exhibit-Relay/1 (+https://exhibit.example)" {
			t.Errorf("incorrect UA %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("lost permitted header")
		}
		w.Header().Set("Set-Cookie", "stolen=secret")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Connection", "ETag")
		w.Header().Set("ETag", "hop-by-hop")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{ "ok": true }`)
	}))
	defer srv.Close()
	g := testGate(t, []string{srv.URL}, Limits{UserAgent: "Exhibit-Relay/1 (+https://exhibit.example)"})
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Host = "attacker.example"
	req.Header = http.Header{"Accept": {"application/json"}, "Cookie": {"session=bad"}, "authorization": {"Bearer bad"}, "Host": {"attacker.example"}, "Origin": {"https://evil.example"}, "Referer": {"https://evil.example"}, "X-Forwarded-For": {"127.0.0.1"}, "X-Api-Key": {"bad"}, "Proxy-Authorization": {"bad"}, "User-Agent": {"browser"}}
	resp, err := g.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Set-Cookie", "Access-Control-Allow-Origin", "Connection", "ETag"} {
		if resp.Header.Get(h) != "" {
			t.Errorf("unsafe response header %s", h)
		}
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Error("missing safe response header")
	}
}

func TestEveryRedirectCheckedAndCredentialsReauthorized(t *testing.T) {
	var secondCalls int
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Secret") != "" {
			t.Error("credential crossed origin")
		}
		_, _ = io.WriteString(w, "second")
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer first-only" {
			t.Error("missing trusted credential")
		}
		http.Redirect(w, r, second.URL+"/final", 302)
	}))
	defer first.Close()
	req, _ := http.NewRequest("GET", first.URL, nil)
	for _, allowed := range [][]string{{first.URL}, {first.URL, second.URL}} {
		g := testGate(t, allowed, Limits{})
		var authorized int
		resp, err := g.DoAuthorized(context.Background(), req, func(hop *http.Request) error {
			authorized++
			if strings.HasPrefix(hop.URL.String(), first.URL) {
				hop.Header.Set("Authorization", "Bearer first-only")
				hop.Header.Set("X-Secret", "secret")
			}
			return nil
		})
		if len(allowed) == 1 {
			d := denied(t, err, true)
			if d.Origin != second.URL || authorized != 1 || secondCalls != 0 {
				t.Fatalf("redirect was not refused before authorization: %v %d", d, authorized)
			}
		} else if err != nil || authorized != 2 || resp.URL != second.URL+"/final" {
			t.Fatalf("wrong authorized redirect response %#v, %v", resp, err)
		}
	}
}

func TestOnlyHeaderMutationsAreAcceptedFromAuthorization(t *testing.T) {
	g := testGate(t, []string{"https://provider.example"}, Limits{})
	req, _ := http.NewRequest("GET", "https://provider.example/", nil)
	_, err := g.DoAuthorized(context.Background(), req, func(r *http.Request) error { r.URL.Host = "other.example"; return nil })
	denied(t, err, false)
}

func TestTrustedBeforeRequestHookStopsRedirectBeforeAuthorization(t *testing.T) {
	second := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("backed-off origin was contacted") }))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, second.URL+"/target", 302) }))
	defer first.Close()
	blocked := errors.New("operator backoff")
	checked, authorized := 0, 0
	g := testGate(t, []string{first.URL, second.URL}, Limits{BeforeRequest: func(target *url.URL) error {
		checked++
		if strings.HasPrefix(target.String(), second.URL) {
			return blocked
		}
		return nil
	}})
	req, _ := http.NewRequest("GET", first.URL, nil)
	_, err := g.DoAuthorized(context.Background(), req, func(*http.Request) error { authorized++; return nil })
	if !errors.Is(err, blocked) || checked != 2 || authorized != 1 {
		t.Fatalf("hook did not stop before credentials: %v, checked=%d authorized=%d", err, checked, authorized)
	}
}

func TestProductionDestinationRefusalsCannotBeApproved(t *testing.T) {
	for _, target := range []string{
		"http://example.com/", "http://localhost/", "https://localhost/", "https://LOCALHOST./",
		"https://127.0.0.1/", "https://127.1/", "https://2130706433/", "https://0177.0.0.1/", "https://0x7f000001/", "https://0x7f.0.0.1/",
		"https://[::1]/", "https://[::ffff:127.0.0.1]/", "https://10.0.0.1/", "https://169.254.169.254/", "https://100.100.100.200/",
		"https://[64:ff9b::7f00:1]/", "https://[2002:7f00:1::]/", "https://198.18.0.1/", "https://user:pass@example.com/", "file:///etc/passwd",
	} {
		t.Run(target, func(t *testing.T) {
			g, err := New([]string{"https://example.com"}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			_, err = fetch(t, g, target)
			denied(t, err, false)
		})
	}
}

func TestDialChecksEveryDNSAnswerAndDialsLiteralWithoutRebinding(t *testing.T) {
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("10.0.0.1")},
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("::ffff:169.254.169.254")},
	} {
		g, _ := New([]string{"https://public.example"}, Limits{})
		g.lookup = func(context.Context, string) ([]netip.Addr, error) { return answers, nil }
		g.dial = func(context.Context, string, string) (net.Conn, error) {
			t.Error("unsafe DNS answer was dialed")
			return nil, nil
		}
		_, err := g.dialContext(context.Background(), "tcp", "public.example:443")
		denied(t, err, false)
		g.Close()
	}
	g, _ := New([]string{"https://public.example"}, Limits{})
	defer g.Close()
	lookups := 0
	g.lookup = func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		if lookups > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	g.dial = func(_ context.Context, _, address string) (net.Conn, error) {
		if address != "8.8.8.8:443" {
			t.Errorf("dial re-resolves unchecked address %s", address)
		}
		return nil, nil
	}
	_, err := g.dialContext(context.Background(), "tcp", "public.example:443")
	if err != nil || lookups != 1 {
		t.Fatalf("unexpected lookup: %d %v", lookups, err)
	}
}

func TestDecodedResponseAndRequestLimits(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, _ = gz.Write(bytes.Repeat([]byte("x"), 1024))
	_ = gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	defer srv.Close()
	g := testGate(t, []string{srv.URL}, Limits{MaxResponseBytes: 100, MaxRequestBytes: 10})
	_, err := fetch(t, g, srv.URL)
	denied(t, err, false)
	req, _ := http.NewRequest("POST", srv.URL, strings.NewReader(strings.Repeat("a", 11)))
	_, err = g.Do(context.Background(), req)
	denied(t, err, false)
}

func TestTimeoutAndNoEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(100 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	g := testGate(t, []string{srv.URL}, Limits{Timeout: 10 * time.Millisecond})
	if _, err := fetch(t, g, srv.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := fetch(t, g, srv.URL+"/slow"); err == nil {
		t.Fatal("expected timeout")
	}
	if g.transport.Proxy != nil {
		t.Error("environment proxy enabled")
	}
}

func TestNormalizedAllowlistAndOperatorPolicy(t *testing.T) {
	g, err := New([]string{"HTTPS://API.Example.:0443"}, Limits{OperatorDenyOrigins: []string{"https://blocked.example"}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for target, want := range map[string]bool{"https://api.example/": true, "https://api.example:443/path": true, "https://api.example:444/": false, "https://blocked.example/": false} {
		req, _ := http.NewRequest("GET", target, nil)
		if g.Allows(req.URL) != want {
			t.Errorf("Allows(%q) != %v", target, want)
		}
	}
	if _, err := New([]string{"https://api.example/not-an-origin"}, Limits{}); err == nil {
		t.Fatal("path-bearing allowlist accepted")
	}
}

func TestRedirectCannotReachPrivateDestination(t *testing.T) {
	g, _ := New([]string{"https://public.example", "https://127.0.0.1"}, Limits{})
	defer g.Close()
	called := 0
	g.client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://127.0.0.1/secret"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	_, err := fetch(t, g, "https://public.example/")
	denied(t, err, false)
	if called != 1 {
		t.Fatalf("redirect target reached: %d calls", called)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
