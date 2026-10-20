package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/websocket"
)

// workerTransport keeps the management credential on the outer API handshake.
// The tunneled HTTP request contains only the independent endpoint bearer token.
// No control-connection pool, pinger, open listener, or background health loop.
type workerTransport struct {
	origin string
	token  string
	http   *http.Transport
}

func (t *workerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme+"://"+request.URL.Host != t.origin || request.URL.User != nil {
		return nil, errors.New("worker transport refuses requests outside its assigned endpoint")
	}
	forwarded := request.Clone(request.Context())
	forwarded.URL.Scheme = "http" // TLS is provided by the outer wss:// API tunnel.
	forwarded.Header.Set("Authorization", "Bearer "+t.token)
	forwarded.Header.Set("Exhibit-Agent-Version", "1")
	forwarded.Header.Del("Proxy-Authorization")
	forwarded.Close = true
	return t.http.RoundTrip(forwarded)
}
func (t *workerTransport) CloseIdleConnections() { t.http.CloseIdleConnections() }

func (p *Provisioner) endpoint(request Request, resource Resource) (*Endpoint, error) {
	endpointURL, err := url.Parse(resource.URL)
	if err != nil || endpointURL.Scheme != "https" || endpointURL.Host == "" || endpointURL.User != nil || endpointURL.RawQuery != "" || endpointURL.Fragment != "" || (endpointURL.Path != "" && endpointURL.Path != "/") {
		return nil, errors.New("Sprites returned an invalid HTTPS worker URL")
	}
	if resource.URLSettings != nil && resource.URLSettings.Auth != "sprite" {
		return nil, errors.New("Sprite URL must use private sprite authentication")
	}
	transport := &http.Transport{
		DisableKeepAlives:     true,
		MaxConnsPerHost:       8,
		ResponseHeaderTimeout: p.config.RequestTimeout,
		DialContext:           func(ctx context.Context, _, _ string) (net.Conn, error) { return p.proxySocket(ctx, resource.Name) },
	}
	origin := endpointURL.Scheme + "://" + endpointURL.Host
	client := &http.Client{
		Transport:     &workerTransport{origin: origin, token: request.EndpointToken, http: transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Endpoint{Identity: request.Identity, Name: resource.Name, URL: origin, EndpointToken: request.EndpointToken, Client: client, Transport: client.Transport, Destroy: func(ctx context.Context) error {
		transport.CloseIdleConnections()
		return p.Destroy(ctx, request.Identity)
	}}, nil
}

func (p *Provisioner) proxySocket(ctx context.Context, name string) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()
	apiURL, _ := url.Parse(p.config.BaseURL)
	if apiURL.Scheme == "https" {
		apiURL.Scheme = "wss"
	} else {
		apiURL.Scheme = "ws"
	}
	apiURL.Path = "/v1/sprites/" + name + "/proxy"
	config, err := websocket.NewConfig(apiURL.String(), p.config.BaseURL)
	if err != nil {
		return nil, errors.New("invalid Sprites proxy configuration")
	}
	config.Header.Set("Authorization", "Bearer "+p.config.Token)
	connection, err := config.DialContext(dialCtx)
	if err != nil {
		if dialCtx.Err() != nil {
			return nil, dialCtx.Err()
		}
		return nil, errors.New("Sprites authenticated proxy connection failed")
	}
	connection.MaxPayloadBytes = 64 << 10
	// The initialization exchange must also obey cancellation, not only TCP/TLS.
	stop := context.AfterFunc(dialCtx, func() { _ = connection.SetDeadline(time.Now()) })
	_ = connection.SetDeadline(time.Now().Add(p.config.RequestTimeout))
	err = websocket.JSON.Send(connection, map[string]any{"host": "127.0.0.1", "port": p.config.Port})
	var response struct {
		Status string `json:"status"`
	}
	if err == nil {
		err = websocket.JSON.Receive(connection, &response)
	}
	stopped := stop()
	if err != nil || !stopped || response.Status != "connected" {
		_ = connection.Close()
		if dialCtx.Err() != nil {
			return nil, dialCtx.Err()
		}
		return nil, errors.New("Sprites TCP proxy initialization failed")
	}
	_ = connection.SetDeadline(time.Time{})
	connection.PayloadType = websocket.BinaryFrame
	return connection, nil
}

func (p *Provisioner) checkReady(ctx context.Context, endpoint *Endpoint) error {
	if p.config.CheckReady != nil {
		return p.config.CheckReady(ctx, endpoint)
	}
	request, err := http.NewRequestWithContext(ctx, "GET", endpoint.URL+p.config.ReadyPath, nil)
	if err != nil {
		return errors.New("invalid worker readiness request")
	}
	response, err := endpoint.Client.Do(request)
	if err != nil {
		return errors.New("worker readiness connection failed")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return ErrConflict
	}
	if response.StatusCode != 200 {
		return errors.New("worker is not ready")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return errors.New("worker readiness response interrupted")
	}
	var ready ReadyResponse
	if json.Unmarshal(data, &ready) != nil || ready.ExecutionID != endpoint.Identity.ExecutionID || ready.SessionID != endpoint.Identity.SessionID || ready.Version != "1" || ready.PiVersion != p.config.Release.PiVersion {
		return ErrConflict
	}
	return nil
}

func (p *Provisioner) awaitReady(ctx context.Context, endpoint *Endpoint) error {
	delay := p.config.Retry.InitialDelay
	for {
		checkCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
		err := p.checkReady(checkCtx, endpoint)
		cancel()
		endpoint.CloseIdleConnections()
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrConflict) {
			return ErrConflict
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := waitContext(ctx, delay); err != nil {
			return err
		}
		delay = min(delay*2, p.config.Retry.MaxDelay)
	}
}
