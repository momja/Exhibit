package egress

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestForwardedRequestUsesTheCommonHeaderAndBodyBoundary(t *testing.T) {
	g := testGate(t, []string{"https://api.example.test"}, Limits{MaxRequestBytes: 8, MaxResponseBytes: 8, UserAgent: "Exhibit-Relay/1"})
	req, _ := http.NewRequest(http.MethodPost, "https://api.example.test/data", strings.NewReader("payload"))
	req.Host = "forged.example"
	req.Header = http.Header{"Accept": {"application/json"}, "Cookie": {"session=private"}, "Authorization": {"Bearer forged"}, "X-Forwarded-For": {"127.0.0.1"}, "User-Agent": {"browser"}, "X-Api-Key": {"forged"}}
	response, err := g.DoForwarded(context.Background(), req, func(ctx context.Context, clean *http.Request) (*Response, error) {
		for _, key := range []string{"Cookie", "Authorization", "X-Forwarded-For", "X-Api-Key"} {
			if clean.Header.Get(key) != "" {
				t.Errorf("guest authority reached vendor: %s", key)
			}
		}
		if clean.Host != "api.example.test" || clean.Header.Get("Accept") != "application/json" || clean.Header.Get("User-Agent") != "Exhibit-Relay/1" {
			t.Errorf("incorrect sanitized request: %s %#v", clean.Host, clean.Header)
		}
		body, err := io.ReadAll(clean.Body)
		if err != nil || string(body) != "payload" || clean.Method != http.MethodPost {
			t.Errorf("lost request body or method: %q %v", body, err)
		}
		return &Response{Status: 200, URL: "https://untrusted.example/private", Header: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"secret=value"}, "Access-Control-Allow-Origin": {"*"}, "Connection": {"Etag"}, "Etag": {"hop"}}, Body: []byte("result")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.URL != req.URL.String() || response.Header.Get("Content-Type") != "application/json" || string(response.Body) != "result" {
		t.Fatalf("unexpected forwarded response: %#v", response)
	}
	for _, key := range []string{"Set-Cookie", "Access-Control-Allow-Origin", "Connection", "Etag"} {
		if response.Header.Get(key) != "" {
			t.Errorf("unsafe provider header delivered: %s", key)
		}
	}
}

func TestForwardedRequestsRefuseBeforeVendorHandoff(t *testing.T) {
	for _, scenario := range []struct {
		name, target, method, body string
		headers                    http.Header
		limits                     Limits
	}{
		{name: "unapproved", target: "https://other.example.test", method: "GET"},
		{name: "private literal", target: "https://127.0.0.1", method: "GET"},
		{name: "unsupported method", target: "https://api.example.test", method: "CONNECT"},
		{name: "oversized body", target: "https://api.example.test", method: "POST", body: "too long", limits: Limits{MaxRequestBytes: 4}},
		{name: "invalid header", target: "https://api.example.test", method: "GET", headers: http.Header{"Accept": {"json\r\nInjected: yes"}}},
		{name: "operator denial", target: "https://api.example.test", method: "GET", limits: Limits{OperatorDenyOrigins: []string{"https://api.example.test"}}},
		{name: "mutating operator hook", target: "https://api.example.test", method: "GET", limits: Limits{BeforeRequest: func(u *url.URL) error { u.Host = "other.example.test"; return nil }}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			g, err := New([]string{"https://api.example.test", "https://127.0.0.1"}, scenario.limits)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			g.lookup = func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
			}
			req, _ := http.NewRequest(scenario.method, scenario.target, strings.NewReader(scenario.body))
			req.Header = scenario.headers
			called := false
			_, err = g.DoForwarded(context.Background(), req, func(context.Context, *http.Request) (*Response, error) {
				called = true
				return &Response{Status: 200}, nil
			})
			if called || err == nil {
				t.Fatalf("refused request reached vendor: called=%v err=%v", called, err)
			}
		})
	}
}

func TestForwardedRequestChecksAllDNSAnswersWithoutDialing(t *testing.T) {
	g, err := New([]string{"https://api.example.test"}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	g.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.1")}, nil
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	_, err = g.DoForwarded(context.Background(), req, func(context.Context, *http.Request) (*Response, error) {
		t.Fatal("mixed private DNS answer reached vendor")
		return nil, nil
	})
	denied(t, err, false)
}

func TestForwardedResponseCapsAndDeadline(t *testing.T) {
	for _, response := range []*Response{
		{Status: 200, Body: []byte("exceeds four")},
		{Status: 200, Header: http.Header{"Content-Type": {strings.Repeat("x", 64<<10)}}},
		{Status: 200, Header: http.Header{strings.Repeat("X", 64<<10): nil}},
		{Status: 0},
	} {
		g := testGate(t, []string{"https://api.example.test"}, Limits{MaxResponseBytes: 4})
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
		_, err := g.DoForwarded(context.Background(), req, func(context.Context, *http.Request) (*Response, error) { return response, nil })
		if err == nil {
			t.Fatalf("invalid response delivered: %#v", response)
		}
	}
	g := testGate(t, []string{"https://api.example.test"}, Limits{Timeout: 10 * time.Millisecond})
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
	_, err := g.DoForwarded(context.Background(), req, func(ctx context.Context, _ *http.Request) (*Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("vendor call did not receive relay deadline: %v", err)
	}
}
