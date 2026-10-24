package bundle

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/origin"
)

type ServerOptions struct {
	// Exact host:port authorities served by this listener, including loopback
	// aliases. Unknown Host headers fail before any content or mutation.
	AllowedHosts    []string
	StatePath       string
	ConnectionsJSON string
	// Gate is optional for embedders. Production defaults to the secure gate;
	// tests can explicitly use egress.NewForTests with fixture servers.
	Gate *egress.Gate
}
type localConnection struct {
	Origin       string `json:"origin"`
	Kind         string `json:"kind"`
	Header       string `json:"header,omitempty"`
	Secret       string `json:"secret,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessURL    string `json:"access_url,omitempty"`
	DailyLimit   int    `json:"daily_limit,omitempty"`
	CacheMinutes int    `json:"cache_minutes,omitempty"`
	BasePath     string `json:"-"`
	Day          string `json:"-"`
	Calls        int    `json:"-"`
}
type relayRequest struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64"`
	Policy     string            `json:"policy"`
}
type relayResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"body_base64"`
	URL        string            `json:"url"`
}
type cachedResponse struct {
	response *egress.Response
	expires  time.Time
	origin   string
}

type Server struct {
	app             *Bundle
	options         ServerOptions
	csrf            string
	framePolicy     string
	gate            *egress.Gate
	hosts           map[string]bool
	mu              sync.Mutex
	state           map[string]string
	connections     map[string]localConnection
	connectionLocks map[string]*sync.Mutex
	cache           map[string]cachedResponse
	active          chan struct{}
}

func NewServer(app *Bundle, options ServerOptions) (*Server, error) {
	if err := app.validate(); err != nil {
		return nil, err
	}
	if len(options.AllowedHosts) == 0 {
		return nil, errors.New("local runner requires explicit allowed hosts")
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	s := &Server{app: app, options: options, csrf: base64.RawURLEncoding.EncodeToString(token), framePolicy: rand.Text(), hosts: map[string]bool{}, state: map[string]string{}, connections: map[string]localConnection{}, connectionLocks: map[string]*sync.Mutex{}, cache: map[string]cachedResponse{}, active: make(chan struct{}, 8)}
	for _, host := range options.AllowedHosts {
		if strings.ContainsAny(host, "/\\?#@ \t\r\n") || host == "" {
			return nil, errors.New("invalid local runner host")
		}
		s.hosts[strings.ToLower(host)] = true
	}
	for key, value := range app.State {
		s.state[key] = value
	}
	if options.StatePath != "" {
		if raw, err := os.ReadFile(options.StatePath); err == nil {
			if err = json.Unmarshal(raw, &s.state); err != nil {
				return nil, fmt.Errorf("saved state: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if s.state == nil {
		s.state = map[string]string{}
	}
	if options.Gate != nil {
		s.gate = options.Gate
	} else {
		g, err := egress.New(app.Manifest.NetworkAllowlist, egress.Limits{})
		if err != nil {
			return nil, err
		}
		s.gate = g
	}
	if options.ConnectionsJSON != "" {
		var connections []localConnection
		if err := json.Unmarshal([]byte(options.ConnectionsJSON), &connections); err != nil {
			s.Close()
			return nil, errors.New("EXHIBIT_CONNECTIONS must be a JSON array")
		}
		for _, c := range connections {
			c, err := s.prepareConnection(c)
			if err != nil {
				s.Close()
				return nil, err
			}
			s.connections[c.Origin] = c
		}
	}
	return s, nil
}
func (s *Server) Close() { s.gate.Close() }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !s.hosts[strings.ToLower(r.Host)] {
		http.Error(w, "Unrecognized local runner host", http.StatusForbidden)
		return
	}
	if r.Method == "POST" && !s.authorized(r) {
		s.error(w, "Local request failed its origin or session check", 403)
		return
	}
	switch {
	case r.Method == "GET" && (r.URL.Path == "/" || r.URL.Path == "/index.html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		_ = hostTemplate.Execute(w, map[string]any{"Title": s.app.Manifest.Title, "Script": template.JS(strings.ReplaceAll(hostJS, "__EXHIBIT_CSRF__", jsonString(s.csrf)))})
	case r.Method == "GET" && r.URL.Path == "/__exhibit/render":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The header also forces a direct, top-level visit into an opaque
		// sandbox, where it cannot read the connect page or the host token.
		s.mu.Lock()
		origins := strings.Join(s.app.Manifest.NetworkAllowlist, " ")
		policy := s.framePolicy
		state, _ := json.Marshal(s.state)
		s.mu.Unlock()
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval' blob: data: "+origins+"; style-src 'unsafe-inline' "+origins+"; img-src data: blob: "+origins+"; font-src data: "+origins+"; media-src data: blob: "+origins+"; connect-src 'none'; worker-src blob: data:; frame-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
		shim := strings.ReplaceAll(shimJS, "__EXHIBIT_STATE_JSON__", jsonString(string(state)))
		shim = strings.ReplaceAll(shim, "__EXHIBIT_POLICY__", jsonString(policy))
		_, _ = io.WriteString(w, injectScript(s.app.Page, shim))
	case r.Method == "GET" && r.URL.Path == "/__exhibit/connect":
		s.connectPage(w, "")
	case r.Method == "GET" && r.URL.Path == "/__exhibit/connections":
		list := []map[string]any{}
		s.mu.Lock()
		for _, requirement := range s.requirements() {
			if connection, ok := s.connections[requirement.Origin]; ok {
				list = append(list, map[string]any{"name": requirement.Name, "provider": connection.Kind, "kind": "native", "origins": []string{connection.Origin}})
			}
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	case r.Method == "POST" && r.URL.Path == "/__exhibit/connect":
		s.connect(w, r)
	case r.Method == "POST" && r.URL.Path == "/__exhibit/relay":
		s.relay(w, r)
	case r.Method == "POST" && r.URL.Path == "/__exhibit/state":
		s.saveState(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if r.Header.Get("Origin") != "http://"+r.Host {
		return false
	}
	token := r.Header.Get("X-Exhibit-CSRF")
	if token == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
		if err := r.ParseForm(); err != nil {
			return false
		}
		token = r.PostForm.Get("csrf")
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.csrf)) == 1
}
func (s *Server) error(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func jsonString(value string) string { b, _ := json.Marshal(value); return string(b) }
func injectScript(page, script string) string {
	content := "<script>" + script + "</script>"
	if i := strings.Index(strings.ToLower(page), "<head"); i >= 0 {
		if j := strings.IndexByte(page[i:], '>'); j >= 0 {
			at := i + j + 1
			return page[:at] + content + page[at:]
		}
	}
	return content + page
}

func (s *Server) saveState(w http.ResponseWriter, r *http.Request) {
	var operation struct {
		Op    string `json:"op"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&operation); err != nil {
		s.error(w, "Invalid saved data operation", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]string, len(s.state))
	for key, value := range s.state {
		next[key] = value
	}
	switch operation.Op {
	case "set":
		next[operation.Key] = operation.Value
	case "delete":
		delete(next, operation.Key)
	case "clear":
		next = map[string]string{}
	default:
		s.error(w, "Unknown saved data operation", 400)
		return
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > 8<<20 || len(next) > 10000 {
		s.error(w, "Saved data exceeds the local limit", 413)
		return
	}
	if s.options.StatePath != "" {
		if err := os.WriteFile(s.options.StatePath+".tmp", raw, 0600); err != nil {
			s.error(w, "Could not save data", 500)
			return
		}
		if err := os.Rename(s.options.StatePath+".tmp", s.options.StatePath); err != nil {
			s.error(w, "Could not save data", 500)
			return
		}
	}
	s.state = next
	w.WriteHeader(204)
}

var errLocalQuota = errors.New("this connection's daily request limit has been reached")
var errLocalPolicy = errors.New("connections changed; reload the app")

func (s *Server) relay(w http.ResponseWriter, r *http.Request) {
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	default:
		s.error(w, "The local relay is busy; try again", 429)
		return
	}
	var input relayRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&input); err != nil {
		s.error(w, "Invalid relay request", 400)
		return
	}
	body, err := base64.StdEncoding.DecodeString(input.BodyBase64)
	if err != nil || len(body) > 1<<20 || len(input.Headers) > 64 {
		s.error(w, "Invalid relay body or headers", 400)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), input.Method, input.URL, bytes.NewReader(body))
	if err != nil {
		s.error(w, "Invalid relay URL", 400)
		return
	}
	for name, value := range input.Headers {
		req.Header.Set(name, value)
	}
	o, err := origin.NormalizeOrigin(req.URL.Scheme + "://" + req.URL.Host)
	if err != nil {
		s.error(w, "Invalid relay origin", 400)
		return
	}
	if !slices.Contains(s.app.Manifest.NetworkAllowlist, o) {
		s.error(w, "Destination is not in this bundle's allowlist", 403)
		return
	}
	// Serialize cache lookup, quota reservation and provider contact for each
	// connection, including concurrent requests from multiple frame documents.
	s.mu.Lock()
	if s.connectionLocks[o] == nil {
		s.connectionLocks[o] = &sync.Mutex{}
	}
	connectionLock := s.connectionLocks[o]
	s.mu.Unlock()
	connectionLock.Lock()
	defer connectionLock.Unlock()
	s.mu.Lock()
	if subtle.ConstantTimeCompare([]byte(input.Policy), []byte(s.framePolicy)) != 1 {
		s.mu.Unlock()
		s.policyError(w)
		return
	}
	allowed := s.app.Manifest.NetworkAllowlist
	connection, connected := s.connections[o]
	for _, requirement := range s.app.Manifest.Connections {
		if requirement.Origin == o && !connected {
			s.mu.Unlock()
			s.error(w, "Connect this app locally using Connections in the page header", 409)
			return
		}
	}
	headerBytes, _ := json.Marshal(input.Headers)
	cacheKey := fmt.Sprintf("%s %s %x", req.Method, req.URL.String(), sha256.Sum256(headerBytes))
	cacheable := connected && connection.CacheMinutes > 0 && (req.Method == "GET" || req.Method == "HEAD") && len(body) == 0 && !strings.Contains(strings.ToLower(req.Header.Get("Cache-Control")), "no-")
	for _, header := range []string{"Range", "If-Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
		cacheable = cacheable && req.Header.Get(header) == ""
	}
	if cacheable {
		if cached, ok := s.cache[cacheKey]; ok && time.Now().Before(cached.expires) {
			s.mu.Unlock()
			s.writeRelay(w, cached.response)
			return
		}
	}
	s.mu.Unlock()
	if connected && connection.BasePath != "" {
		if !safeAccessPath(req.URL.Path) {
			s.error(w, "Invalid path for this connection", 400)
			return
		}
		req.URL.Path = connection.BasePath + "/" + strings.TrimLeft(req.URL.Path, "/")
		req.URL.RawPath = ""
	}
	response, err := s.gate.DoAuthorized(r.Context(), req, func(hop *http.Request) error {
		hopOrigin, err := origin.NormalizeOrigin(hop.URL.Scheme + "://" + hop.URL.Host)
		if err != nil || !slices.Contains(allowed, hopOrigin) {
			return errors.New("destination is not approved for this app")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if subtle.ConstantTimeCompare([]byte(input.Policy), []byte(s.framePolicy)) != 1 {
			return errLocalPolicy
		}
		if !connected {
			return nil
		}
		if hopOrigin != o {
			return errors.New("connector redirects must stay at their authorized origin")
		}
		if connection.BasePath != "" && (!safeAccessPath(hop.URL.Path) || (hop.URL.Path != connection.BasePath && !strings.HasPrefix(hop.URL.Path, connection.BasePath+"/"))) {
			return errors.New("redirect leaves the connection's private base path")
		}
		// Authorization runs only after this hop passes the common gate. A
		// cached response costs no call; every provider redirect costs a call.
		current := s.connections[o]
		day := time.Now().UTC().Format("2006-01-02")
		if current.Day != day {
			current.Day, current.Calls = day, 0
		}
		if current.DailyLimit > 0 && current.Calls >= current.DailyLimit {
			return errLocalQuota
		}
		current.Calls++
		s.connections[o] = current
		switch connection.Kind {
		case "bearer":
			hop.Header.Set("Authorization", "Bearer "+connection.Secret)
		case "basic":
			hop.SetBasicAuth(connection.Username, connection.Password)
		case "api-key":
			hop.Header.Set(connection.Header, connection.Secret)
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errLocalQuota):
			s.error(w, errLocalQuota.Error(), 429)
		case errors.Is(err, errLocalPolicy):
			s.policyError(w)
		default:
			s.error(w, "The local relay refused or could not complete this request", 502)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if subtle.ConstantTimeCompare([]byte(input.Policy), []byte(s.framePolicy)) != 1 {
		s.policyError(w)
		return
	}
	if connected {
		if connection.BasePath != "" {
			response.URL = input.URL
		}
		scrubLocalResponse(connection, response)
	}
	if cacheable && response.Status >= 200 && response.Status < 300 && response.Status != 206 && !strings.Contains(response.Header.Get("Vary"), "*") && !strings.Contains(strings.ToLower(response.Header.Get("Cache-Control")), "no-store") {
		for key, cached := range s.cache {
			if time.Now().After(cached.expires) || len(s.cache) >= 128 {
				delete(s.cache, key)
			}
		}
		s.cache[cacheKey] = cachedResponse{response, time.Now().Add(time.Duration(connection.CacheMinutes) * time.Minute), o}
	}
	s.writeRelay(w, response)
}

func (s *Server) policyError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(409)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "Connections changed; reloading the app", "reload": true})
}

func scrubLocalResponse(connection localConnection, response *egress.Response) {
	values := []string{connection.Secret, connection.Username, connection.Password}
	if connection.Username != "" || connection.Password != "" {
		values = append(values, connection.Username+":"+connection.Password, base64.StdEncoding.EncodeToString([]byte(connection.Username+":"+connection.Password)))
	}
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	scrub := func(data []byte) []byte {
		for _, value := range values {
			if value == "" {
				continue
			}
			encoded, _ := json.Marshal(value)
			for _, variation := range []string{value, url.QueryEscape(value), url.PathEscape(value), string(encoded[1 : len(encoded)-1])} {
				if variation != "" {
					replacement := "[credential redacted]"
					if len(variation) < len(replacement) {
						replacement = "*"
					}
					data = bytes.ReplaceAll(data, []byte(variation), []byte(replacement))
				}
			}
		}
		if connection.BasePath != "" {
			escapedPath := (&url.URL{Path: connection.BasePath}).EscapedPath()
			for _, path := range []string{connection.BasePath, escapedPath, url.QueryEscape(connection.BasePath), url.PathEscape(connection.BasePath)} {
				encoded, _ := json.Marshal(path)
				jsonPath := string(encoded[1 : len(encoded)-1])
				for _, variation := range []string{path, jsonPath, strings.ReplaceAll(path, "/", `\/`), strings.ReplaceAll(jsonPath, "/", `\/`)} {
					data = bytes.ReplaceAll(data, []byte(variation), nil)
				}
			}
		}
		return data
	}
	response.Body = scrub(response.Body)
	response.URL = string(scrub([]byte(response.URL)))
	for name, values := range response.Header {
		for i, value := range values {
			values[i] = string(scrub([]byte(value)))
		}
		response.Header[name] = values
	}
}
func (s *Server) writeRelay(w http.ResponseWriter, response *egress.Response) {
	headers := map[string]string{}
	for name, values := range response.Header {
		headers[name] = strings.Join(values, ", ")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(relayResponse{response.Status, response.StatusText, headers, base64.StdEncoding.EncodeToString(response.Body), response.URL})
}

func credentialHeader(header string) bool {
	if header == "" || len(header) > 128 {
		return false
	}
	for _, r := range header {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	name := strings.ToLower(header)
	if strings.HasPrefix(name, "proxy-") || strings.HasPrefix(name, "sec-") || strings.HasPrefix(name, "x-forwarded-") {
		return false
	}
	switch name {
	case "cookie", "set-cookie", "host", "origin", "referer", "connection", "content-length", "transfer-encoding", "te", "trailer", "upgrade", "accept-encoding", "user-agent":
		return false
	}
	return true
}
func safeAccessPath(path string) bool {
	for range 4 {
		if strings.Contains(path, "\\") {
			return false
		}
		for _, part := range strings.Split(path, "/") {
			if part == "." || part == ".." {
				return false
			}
		}
		decoded, err := url.PathUnescape(path)
		if err != nil {
			return false
		}
		if decoded == path {
			return true
		}
		path = decoded
	}
	return false
}

func (s *Server) prepareConnection(c localConnection) (localConnection, error) {
	if c.AccessURL != "" {
		access, err := url.Parse(strings.TrimSpace(c.AccessURL))
		if err != nil || len(c.AccessURL) > 8192 || c.Kind != "basic" || access.Scheme != "https" || access.User == nil || access.RawQuery != "" || access.Fragment != "" || !safeAccessPath(access.Path) {
			return c, errors.New("enter an HTTPS Basic access URL with embedded username and password, without query or fragment")
		}
		accessOrigin, err := origin.NormalizeOrigin(access.Scheme + "://" + access.Host)
		if err != nil || (c.Origin != "" && accessOrigin != c.Origin) {
			return c, errors.New("the Basic access URL must use this connection's allowed origin")
		}
		c.Origin, c.Username = accessOrigin, access.User.Username()
		c.Password, _ = access.User.Password()
		c.BasePath = strings.TrimRight(access.Path, "/")
		c.AccessURL = ""
	}
	o, err := origin.NormalizeOrigin(c.Origin)
	if err != nil || o != c.Origin || !strings.HasPrefix(o, "https://") || !slices.Contains(s.app.Manifest.NetworkAllowlist, o) {
		return c, errors.New("local connection requires an allowed normalized HTTPS origin")
	}
	if c.DailyLimit < 0 || c.CacheMinutes < 0 || c.CacheMinutes > 525600 {
		return c, errors.New("enter nonnegative request and response reuse limits (reuse at most 525600 minutes)")
	}
	switch c.Kind {
	case "bearer":
		if c.Secret == "" {
			return c, errors.New("enter a bearer token")
		}
	case "api-key":
		if c.Secret == "" || !credentialHeader(c.Header) {
			return c, errors.New("enter an API key and a permitted header")
		}
	case "basic":
		if c.Username == "" || c.Password == "" || strings.Contains(c.Username, ":") {
			return c, errors.New("enter a Basic access URL, or a username and password")
		}
	default:
		return c, errors.New("unsupported local connection kind; reconnect using a generic connection")
	}
	if strings.ContainsAny(c.Secret+c.Username+c.Password, "\r\n") {
		return c, errors.New("credentials cannot contain line breaks")
	}
	return c, nil
}

func (s *Server) requirements() []ConnectionRequirement {
	result := append([]ConnectionRequirement(nil), s.app.Manifest.Connections...)
	for _, o := range s.app.Manifest.NetworkAllowlist {
		found := false
		for _, r := range result {
			if r.Origin == o {
				found = true
			}
		}
		if !found && strings.HasPrefix(o, "https://") {
			result = append(result, ConnectionRequirement{Name: o, Origin: o, Kind: "bearer", Header: "X-API-Key"})
		}
	}
	return result
}
func (s *Server) connectPage(w http.ResponseWriter, message string) {
	// A native form's Origin may become null under no-referrer. Keep its
	// same-origin evidence for CSRF validation without disclosing it off-site.
	w.Header().Set("Referrer-Policy", "same-origin")
	type row struct {
		ConnectionRequirement
		Connected bool
		Index     int
	}
	rows := []row{}
	s.mu.Lock()
	for i, requirement := range s.requirements() {
		connection, connected := s.connections[requirement.Origin]
		if connected {
			requirement.Kind, requirement.DailyLimit, requirement.CacheMinutes = connection.Kind, connection.DailyLimit, connection.CacheMinutes
			if connection.Header != "" {
				requirement.Header = connection.Header
			}
		}
		entry := row{ConnectionRequirement: requirement, Connected: connected, Index: i}

		rows = append(rows, entry)
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	_ = connectTemplate.Execute(w, map[string]any{"Title": s.app.Manifest.Title, "CSRF": s.csrf, "Rows": rows, "Message": message, "Origins": strings.Join(s.app.Manifest.NetworkAllowlist, ", "), "Script": template.JS(connectJS)})
}
func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.error(w, "Invalid connection form", 400)
		return
	}
	var index int
	if _, err := fmt.Sscanf(r.PostForm.Get("index"), "%d", &index); err != nil || index < 0 || index >= len(s.requirements()) {
		s.error(w, "Unknown local connection", 400)
		return
	}
	requirement := s.requirements()[index]
	if r.PostForm.Get("action") == "disconnect" {
		s.mu.Lock()
		delete(s.connections, requirement.Origin)
		s.clearConnectionCacheLocked(requirement.Origin)
		s.framePolicy = rand.Text()
		s.mu.Unlock()
		s.connectPage(w, "Connection removed from this runner.")
		return
	}
	readLimit := func(name string) (int, error) {
		value := strings.TrimSpace(r.PostForm.Get(name))
		if value == "" {
			return 0, nil
		}
		return strconv.Atoi(value)
	}
	dailyLimit, dailyErr := readLimit("daily_limit")
	cacheMinutes, cacheErr := readLimit("cache_minutes")
	if dailyErr != nil || cacheErr != nil {
		s.connectPage(w, "Enter whole numbers for request and response reuse limits.")
		return
	}
	c, err := s.prepareConnection(localConnection{Origin: requirement.Origin, Kind: r.PostForm.Get("kind"), Header: r.PostForm.Get("header"), Secret: r.PostForm.Get("secret"), Username: r.PostForm.Get("username"), Password: r.PostForm.Get("password"), AccessURL: r.PostForm.Get("access_url"), DailyLimit: dailyLimit, CacheMinutes: cacheMinutes})
	if err != nil {
		s.connectPage(w, err.Error())
		return
	}
	s.mu.Lock()
	s.connections[c.Origin] = c
	s.clearConnectionCacheLocked(c.Origin)
	s.framePolicy = rand.Text()
	s.mu.Unlock()
	s.connectPage(w, "Connected locally. Credentials remain in the runner's memory until it stops.")
}

func (s *Server) clearConnectionCacheLocked(connectedOrigin string) {
	for key, cached := range s.cache {
		if cached.origin == connectedOrigin {
			delete(s.cache, key)
		}
	}
}

var hostTemplate = template.Must(template.New("host").Parse(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title><style>html,body{height:100%;margin:0;font:15px system-ui;background:#f6f7f8;color:#172126}body{display:flex;flex-direction:column}header{display:flex;align-items:center;gap:20px;padding:12px 20px;border-bottom:1px solid #ddd}header strong{flex:1}a{color:#146350}iframe{border:0;width:100%;flex:1;background:white}.label{font-size:12px;color:#5c676c}</style></head><body><header><strong>{{.Title}}</strong><span class="label">Exhibit · local relay</span><a href="/__exhibit/connect" target="_blank" rel="noopener">Connections</a></header><iframe id="exhibit-app" title="{{.Title}}" sandbox="allow-scripts" src="/__exhibit/render"></iframe><script type="module">{{.Script}}</script></body></html>`))
var connectTemplate = template.Must(template.New("connect").Parse(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Local connections · {{.Title}}</title><style>body{margin:40px auto;padding:0 20px;max-width:720px;font:16px/1.5 system-ui;color:#172126;background:#f6f7f8}a{color:#146350}form{padding:24px;background:white;border:1px solid #dce0e1;border-radius:12px;margin:20px 0}h2{font-size:19px;margin:0}label{display:block;margin:14px 0}input,select,button{font:inherit;padding:9px;border:1px solid #bcc6c5;border-radius:6px;max-width:100%;box-sizing:border-box}input{display:block;width:100%}button{cursor:pointer;background:#146350;color:white;margin-right:8px}.muted{font-size:14px;color:#52615f}.message{padding:12px;background:#e2eee8}code{overflow-wrap:anywhere}small{display:block}</style></head><body><a href="/">← Back to app</a><h1>Local connections</h1><p>Connect {{.Title}} on this machine. Credentials stay outside the app frame and are held only until this runner stops. Reconnect after restarting.</p><p class="muted">Connections add authentication for their own origin. The app can also call other approved origins without a connection. The app's allowed origins are: {{.Origins}}.</p>{{if .Message}}<p class="message" role="status">{{.Message}}</p>{{end}}{{range .Rows}}<form method="post" action="/__exhibit/connect" autocomplete="off" data-connection-form><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="index" value="{{.Index}}"><h2>{{.Name}}</h2><code>{{.Origin}}</code><p class="muted">{{if .Connected}}Connected locally{{else}}No local credentials{{end}}</p><label>Connection kind<select name="kind"><option value="bearer" {{if eq .Kind "bearer"}}selected{{end}}>Bearer token</option><option value="api-key" {{if eq .Kind "api-key"}}selected{{end}}>API key header</option><option value="basic" {{if eq .Kind "basic"}}selected{{end}}>Basic authentication</option></select></label><div data-kinds="bearer api-key"><label>API key or bearer token<input type="password" name="secret" autocomplete="new-password"></label></div><div data-kinds="api-key"><label>API key header<input name="header" value="{{if .Header}}{{.Header}}{{else}}X-API-Key{{end}}"></label></div><div data-kinds="basic"><label>Basic access URL (HTTPS with embedded username and password)<input type="password" name="access_url" autocomplete="new-password" placeholder="https://user:password@api.example/private-base"></label><p class="muted">Or enter the credentials separately when the API has no private base path.</p><label>Basic username<input name="username" autocomplete="off"></label><label>Basic password<input type="password" name="password" autocomplete="new-password"></label></div><label>Maximum provider requests per UTC day (optional)<input type="number" name="daily_limit" min="0" step="1" value="{{if .DailyLimit}}{{.DailyLimit}}{{end}}"></label><label>Reuse successful GET/HEAD responses for minutes (optional)<input type="number" name="cache_minutes" min="0" max="525600" step="1" value="{{if .CacheMinutes}}{{.CacheMinutes}}{{end}}"></label><small class="muted">Credentials are used only for this origin. Reconnecting replaces the current local connection. Limits and reuse apply during this runner session.</small><p><button name="action" value="connect">Connect locally</button>{{if .Connected}}<button name="action" value="disconnect">Disconnect</button>{{end}}</p></form>{{else}}<p>This app has no allowed external origins.</p>{{end}}<script type="module">{{.Script}}</script></body></html>`))
