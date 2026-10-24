package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"golang.org/x/oauth2"
)

const standinPrefix = "exh_standin_"

const (
	maxConnectionResponseBytes         = 4 << 20
	maxConnectionResponseMetadataBytes = 64 << 10
	// Body bytes are base64 in the encrypted JSON envelope. This bound also
	// permits the metadata's worst-case JSON escaping without growing a cache
	// entry beyond a small, fixed multiple of the decoded response limit.
	maxConnectionCacheJSONBytes = 8 << 20
)

var ErrResponseSize = errors.New("connection response exceeds the 4 MiB body or 64 KiB metadata limit")

type standinBinding struct {
	viewerID     int64
	connectionID string
	origins      []string
	expires      time.Time
}
type Standin struct {
	ConnectionID string    `json:"connection_id"`
	Value        string    `json:"value"`
	AccessURL    string    `json:"access_url,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Standin returns an inert substitute, never the credential. It may be
// delivered to a bound frame after its connection grant was checked. There is
// no persistent exported stand-in registry: exports must reconnect locally.
func (s *Service) Standin(ctx context.Context, viewerID int64, artifactID, id string) (*Standin, error) {
	if !s.cfg.StandinsEnabled {
		return nil, ErrDisabled
	}
	rec, err := s.cfg.Store.GetGrantedConnection(ctx, viewerID, artifactID, id)
	if err != nil {
		return nil, err
	}
	if rec.Kind == "vendor" && rec.Status == "reconnect" {
		return nil, vendorFailure(rec.Engine, VendorNeedsReconnect)
	}
	r, _, err := s.readCredential(rec)
	if err != nil {
		return nil, err
	}
	token := standinPrefix + randomToken()
	expires := s.cfg.Now().Add(time.Hour)
	s.mu.Lock()
	for key, b := range s.standins {
		if !b.expires.After(s.cfg.Now()) {
			delete(s.standins, key)
		}
	}
	if len(s.standins) >= 4096 {
		s.mu.Unlock()
		return nil, errors.New("too many active stand-ins; try again later")
	}
	s.standins[token] = standinBinding{viewerID: viewerID, connectionID: id, origins: append([]string{}, r.Origins...), expires: expires}
	s.mu.Unlock()
	result := &Standin{ConnectionID: id, Value: token, ExpiresAt: expires}
	if rec.Provider == "basic" {
		u, _ := url.Parse(r.Origins[0])
		u.User = url.UserPassword("exhibit", token)
		u.Path = "" // The stand-in always names the public virtual root.
		result.AccessURL = u.String()
	}
	return result, nil
}

func (s *Service) checkStandinMisuse(viewerID int64, artifactID string, req *http.Request, target string) error {
	// Copy a bounded body before looking: the common gate applies its full
	// decoded/body limits too, but stand-ins in a body are forbidden outright.
	if req.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(req.Body, (1<<20)+1))
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(raw))
		if err != nil || len(raw) > 1<<20 {
			return ErrStandin
		}
		if bytes.Contains(raw, []byte(standinPrefix)) {
			return s.standinAlert(viewerID, "", target)
		}
	}
	if strings.Contains(req.URL.String(), standinPrefix) {
		return s.standinAlert(viewerID, "", target)
	}
	for header, values := range req.Header {
		for _, value := range values {
			decoded := value
			if strings.EqualFold(header, "Authorization") && strings.HasPrefix(value, "Basic ") {
				if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "Basic ")); err == nil {
					decoded = string(raw)
				}
			}
			if !strings.Contains(decoded, standinPrefix) {
				continue
			}
			s.mu.Lock()
			var token string
			var binding standinBinding
			for candidate, b := range s.standins {
				if strings.Contains(decoded, candidate) {
					token = candidate
					binding = b
					break
				}
			}
			s.mu.Unlock()
			if token == "" || binding.viewerID != viewerID || !binding.expires.After(s.cfg.Now()) || !slices.Contains(binding.origins, target) {
				return s.standinAlert(viewerID, binding.connectionID, target)
			}
			rec, err := s.cfg.Store.GetGrantedConnection(req.Context(), viewerID, artifactID, binding.connectionID)
			if err != nil {
				return s.standinAlert(viewerID, binding.connectionID, target)
			}
			var r recipe
			if json.Unmarshal([]byte(rec.RecipeJSON), &r) != nil {
				return ErrStandin
			}
			// Match the complete reviewed position, rather than replacing substrings
			// in echoed arbitrary fields. Every other occurrence is an exfil signal.
			allowed := strings.EqualFold(header, r.Header) && (value == token || value == r.Scheme+token || r.Scheme == "Basic " && decoded == "exhibit:"+token)
			if !allowed {
				return s.standinAlert(viewerID, binding.connectionID, target)
			}
		}
	}
	return nil
}
func (s *Service) standinAlert(viewerID int64, id, target string) error {
	slog.Warn("connection stand-in misuse blocked", slog.Int64("viewer_id", viewerID), slog.String("connection_id", id), slog.String("destination_origin", target))
	return ErrStandin
}

func (s *Service) responseScrubber(rec *store.ConnectionRecord, viewerID int64, r recipe, c credential) *strings.Replacer {
	replacement := "[credential redacted]"
	s.mu.Lock()
	for token, b := range s.standins {
		if b.viewerID == viewerID && b.connectionID == rec.ID && b.expires.After(s.cfg.Now()) {
			replacement = token
			break
		}
	}
	s.mu.Unlock()
	values := []string{c.Value, c.Password, c.Username, c.Username + ":" + c.Password, r.Scheme + c.Value}
	if c.Username != "" || c.Password != "" {
		values = append(values, base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password)))
	}
	if len(c.OAuthToken) > 0 {
		var token oauth2.Token
		if json.Unmarshal(c.OAuthToken, &token) == nil {
			values = append(values, token.AccessToken, token.RefreshToken)
		}
	}
	patterns := map[string]string{}
	addPattern := func(value, replacement string) {
		patterns[value] = replacement
		encoded := url.QueryEscape(value)
		if encoded != value {
			patterns[encoded] = replacement
		}
		escaped, _ := json.Marshal(value)
		if len(escaped) > 2 {
			patterns[string(escaped[1:len(escaped)-1])] = replacement
		}
	}
	if c.BasePath != "" && c.BasePath != "/" {
		// Providers may reflect an escaped URL, a decoded path, or a
		// JSON-escaped path. Every representation must preserve the alias.
		addPattern(strings.TrimRight(c.BasePath, "/")+"/", "/")
		addPattern(c.BasePath, "/")
		if decoded, err := url.PathUnescape(c.BasePath); err == nil && decoded != c.BasePath {
			addPattern(strings.TrimRight(decoded, "/")+"/", "/")
			addPattern(decoded, "/")
		}
	}
	for _, value := range values {
		if value == "" || value == ":" || value == r.Scheme {
			continue
		}
		addPattern(value, replacement)
	}
	for _, secret := range c.ExtraSecrets {
		if secret != "" {
			addPattern(secret, "[credential redacted]")
		}
	}
	ordered := make([]string, 0, len(patterns))
	for value := range patterns {
		ordered = append(ordered, value)
	}
	slices.SortFunc(ordered, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	pairs := make([]string, 0, len(ordered)*2)
	for _, value := range ordered {
		pairs = append(pairs, value, patterns[value])
	}
	return strings.NewReplacer(pairs...)
}

// replacementSize counts a replacer's exact output without allocating that
// output. WriteString keeps a repeated short credential from first growing a
// provider's bounded response into an unbounded intermediate byte slice.
type replacementSize struct{ size, limit int }

func (c *replacementSize) Write(p []byte) (int, error) {
	if len(p) > c.limit-c.size {
		return 0, ErrResponseSize
	}
	c.size += len(p)
	return len(p), nil
}
func (c *replacementSize) WriteString(p string) (int, error) {
	if len(p) > c.limit-c.size {
		return 0, ErrResponseSize
	}
	c.size += len(p)
	return len(p), nil
}

func boundedReplace(replacer *strings.Replacer, value string, limit int) (string, error) {
	if len(value) > limit {
		return "", ErrResponseSize
	}
	count := &replacementSize{limit: limit}
	if _, err := replacer.WriteString(count, value); err != nil {
		return "", err
	}
	var output strings.Builder
	output.Grow(count.size)
	if _, err := replacer.WriteString(&output, value); err != nil {
		return "", err
	}
	return output.String(), nil
}

func responseSizeAllowed(response *egress.Response) error {
	if len(response.Body) > maxConnectionResponseBytes {
		return ErrResponseSize
	}
	remaining := maxConnectionResponseMetadataBytes
	for _, value := range []string{response.StatusText, response.URL} {
		if len(value) > remaining {
			return ErrResponseSize
		}
		remaining -= len(value)
	}
	for key, values := range response.Header {
		if len(key)+4 > remaining {
			return ErrResponseSize
		}
		remaining -= len(key) + 4
		for _, value := range values {
			if len(value)+4 > remaining {
				return ErrResponseSize
			}
			remaining -= len(value) + 4
		}
	}
	return nil
}

func (s *Service) scrubResponse(rec *store.ConnectionRecord, viewerID int64, r recipe, c credential, response *egress.Response) error {
	if err := responseSizeAllowed(response); err != nil {
		return err
	}
	replacer := s.responseScrubber(rec, viewerID, r, c)
	body, err := boundedReplace(replacer, string(response.Body), maxConnectionResponseBytes)
	if err != nil {
		return err
	}
	response.Body = []byte(body)
	remaining := maxConnectionResponseMetadataBytes
	response.StatusText, err = boundedReplace(replacer, response.StatusText, remaining)
	if err != nil {
		return err
	}
	remaining -= len(response.StatusText)
	response.URL, err = boundedReplace(replacer, response.URL, remaining)
	if err != nil {
		return err
	}
	remaining -= len(response.URL)
	for key, values := range response.Header {
		if len(key)+4 > remaining {
			return ErrResponseSize
		}
		remaining -= len(key) + 4
		for i, value := range values {
			if remaining < 4 {
				return ErrResponseSize
			}
			replacement, err := boundedReplace(replacer, value, remaining-4)
			if err != nil {
				return err
			}
			values[i] = replacement
			remaining -= len(replacement) + 4
		}
		response.Header[key] = values
	}
	return nil
}
