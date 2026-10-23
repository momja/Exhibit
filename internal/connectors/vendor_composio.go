package connectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/egress"
)

// Composio's supported API redacts provider credentials for both managed and
// custom OAuth apps. Its engine therefore forwards through Proxy Execute; it
// never treats masked account.state fields as injectable credentials.
type composioEngine struct{ s *Service }

func (composioEngine) ID() string           { return "composio" }
func (composioEngine) DeliveryMode() string { return VendorForward }

func (e composioEngine) Start(ctx context.Context, flow VendorFlowRequest) (*VendorSession, error) {
	if flow.OwnerTag == "" || flow.Spec.AuthConfigID == "" {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	body, _ := json.Marshal(map[string]string{"auth_config_id": flow.Spec.AuthConfigID, "user_id": flow.OwnerTag})
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), flow.Provider, http.MethodPost, "/api/v3.1/connected_accounts/link", body, "management")
	if err != nil {
		return nil, err
	}
	var link struct {
		ID        string `json:"connected_account_id"`
		URL       string `json:"redirect_url"`
		ExpiresAt string `json:"expires_at"`
	}
	if json.Unmarshal(response.Body, &link) != nil || !composioReference(link.ID) {
		return nil, vendorFailure(e.ID(), VendorUpstreamError)
	}
	u, err := url.Parse(link.URL)
	if err != nil || u.Scheme != "https" || u.Host != "connect.composio.dev" || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/link/") {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	expires, err := time.Parse(time.RFC3339, link.ExpiresAt)
	if err != nil || !expires.After(e.s.cfg.Now()) {
		return nil, vendorFailure(e.ID(), VendorNeedsReconnect)
	}
	return &VendorSession{ID: flow.ID, URL: link.URL, Provider: flow.Provider, ExpiresAt: link.ExpiresAt, ExternalRef: link.ID}, nil
}

func (e composioEngine) Complete(ctx context.Context, flow VendorFlowRequest) (string, bool, error) {
	account, err := e.account(ctx, VendorConnectionSpec{OwnerTag: flow.OwnerTag, Provider: flow.Provider, ExternalRef: flow.ExternalRef, Spec: flow.Spec})
	if err != nil {
		return "", false, err
	}
	switch strings.ToUpper(account.Status) {
	case "INITIALIZING", "INITIATED":
		return "", false, nil
	case "ACTIVE":
		if account.Disabled || account.AuthConfig.Disabled {
			return "", false, vendorFailure(e.ID(), VendorNeedsReconnect)
		}
		return account.ID, true, nil
	default:
		return "", false, vendorFailure(e.ID(), VendorNeedsReconnect)
	}
}

func (e composioEngine) Status(ctx context.Context, spec VendorConnectionSpec) (string, error) {
	account, err := e.account(ctx, spec)
	if err != nil {
		var failure *VendorError
		if errors.As(err, &failure) {
			if failure.HTTPStatus == http.StatusNotFound {
				return "revoked", nil
			}
			if failure.Code == VendorNeedsReconnect {
				return VendorNeedsReconnect, nil
			}
		}
		return "", err
	}
	if strings.EqualFold(account.Status, "REVOKED") {
		return "revoked", nil
	}
	if !strings.EqualFold(account.Status, "ACTIVE") || account.Disabled || account.AuthConfig.Disabled {
		return "needs_reconnect", nil
	}
	return "ok", nil
}

func (e composioEngine) Authorize(context.Context, *http.Request, VendorConnectionSpec, *credential) error {
	return vendorFailure(e.ID(), VendorRefused)
}

func (e composioEngine) Forward(ctx context.Context, req *http.Request, spec VendorConnectionSpec) (*egress.Response, error) {
	// The common Exhibit gate runs first. Repeat the reviewed recipe check
	// here so this engine cannot become an unrestricted vendor proxy.
	if req == nil || req.URL == nil || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" || req.URL.Opaque != "" || !slices.Contains(spec.Spec.Origins, requestOrigin(req.URL)) {
		return nil, ErrEgress
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead:
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if !spec.Spec.AllowWrites {
			return nil, ErrEgress
		}
	default:
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	status, err := e.Status(ctx, spec)
	if err != nil {
		return nil, err
	}
	if status != "ok" {
		return nil, vendorFailure(e.ID(), VendorNeedsReconnect)
	}
	request := composioProxyRequest{ConnectionID: spec.ExternalRef, UserID: spec.OwnerTag, Endpoint: req.URL.String(), Method: req.Method}
	for _, key := range composioRequestHeaders {
		for _, value := range req.Header.Values(key) {
			request.Parameters = append(request.Parameters, composioParameter{Name: key, Value: value, In: "header"})
		}
	}
	if req.Body != nil {
		body, err := io.ReadAll(io.LimitReader(req.Body, (1<<20)+1))
		if err != nil {
			return nil, vendorFailure(e.ID(), VendorUpstreamError)
		}
		if len(body) > 1<<20 {
			return nil, vendorFailure(e.ID(), VendorRefused)
		}
		if len(body) > 0 {
			if json.Valid(body) && strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
				request.Body = body
			} else {
				request.BinaryBody = &composioBinaryBody{Base64: base64.StdEncoding.EncodeToString(body), ContentType: req.Header.Get("Content-Type")}
			}
		}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodPost, "/api/v3.1/tools/execute/proxy", body, "forward")
	if err != nil {
		return nil, err
	}
	var result struct {
		Status     int               `json:"status"`
		Headers    map[string]string `json:"headers"`
		Data       json.RawMessage   `json:"data"`
		BinaryData json.RawMessage   `json:"binary_data"`
	}
	if json.Unmarshal(response.Body, &result) != nil || result.Status < 200 || result.Status > 599 {
		return nil, vendorFailure(e.ID(), VendorUpstreamError)
	}
	if len(result.BinaryData) > 0 && string(result.BinaryData) != "null" {
		// Download URLs are another destination and may have separate expiry
		// and disclosure rules. Never fetch one outside the normal gate.
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	headers := make(http.Header, len(result.Headers))
	for key, value := range result.Headers {
		headers.Set(key, value)
	}
	if result.Status >= 400 {
		return nil, vendorHTTPError(e.ID(), result.Status, headers)
	}
	data := []byte(result.Data)
	mediaType, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	_, subtype, _ := strings.Cut(strings.ToLower(mediaType), "/")
	isJSON := subtype == "json" || strings.HasSuffix(subtype, "+json")
	if len(data) > 0 && data[0] == '"' && !isJSON {
		var text string
		if json.Unmarshal(data, &text) != nil {
			return nil, vendorFailure(e.ID(), VendorUpstreamError)
		}
		data = []byte(text)
	}
	if req.Method == http.MethodHead || result.Status == http.StatusNoContent || result.Status == http.StatusResetContent {
		data = nil
	}
	forwarded := &egress.Response{Status: result.Status, StatusText: http.StatusText(result.Status), Header: headers, Body: data, URL: req.URL.String()}
	if err := responseSizeAllowed(forwarded); err != nil {
		return nil, err
	}
	return forwarded, nil
}

func (e composioEngine) Revoke(ctx context.Context, spec VendorConnectionSpec) error {
	account, err := e.account(ctx, spec)
	if err != nil {
		var failure *VendorError
		if errors.As(err, &failure) && failure.HTTPStatus == http.StatusNotFound {
			return nil // Already removed remotely; local cleanup is safe.
		}
		return err
	}
	path := "/api/v3.1/connected_accounts/" + url.PathEscape(spec.ExternalRef)
	if !strings.EqualFold(account.Status, "REVOKED") {
		_, err = e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodPost, path+"/revoke", nil, "management")
		var failure *VendorError
		if err != nil {
			if !errors.As(err, &failure) || failure.HTTPStatus != http.StatusBadRequest && failure.HTTPStatus != http.StatusConflict && failure.HTTPStatus != http.StatusNotFound {
				return err
			}
		}
	}
	_, err = e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodDelete, path+"?revoke_on_delete=true", nil, "management")
	return err
}

type composioAccount struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Status     string `json:"status"`
	Disabled   bool   `json:"is_disabled"`
	AuthConfig struct {
		ID       string `json:"id"`
		Disabled bool   `json:"is_disabled"`
	} `json:"auth_config"`
	Toolkit struct {
		Slug string `json:"slug"`
	} `json:"toolkit"`
}

func (e composioEngine) account(ctx context.Context, spec VendorConnectionSpec) (*composioAccount, error) {
	if !composioReference(spec.ExternalRef) || spec.OwnerTag == "" || spec.Spec.AuthConfigID == "" {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodGet, "/api/v3.1/connected_accounts/"+url.PathEscape(spec.ExternalRef), nil, "management")
	if err != nil {
		return nil, err
	}
	var account composioAccount
	if json.Unmarshal(response.Body, &account) != nil || account.ID != spec.ExternalRef {
		return nil, vendorFailure(e.ID(), VendorUpstreamError)
	}
	toolkit := spec.Spec.Toolkit
	if toolkit == "" {
		toolkit = spec.Provider
	}
	if account.UserID != spec.OwnerTag || account.AuthConfig.ID != spec.Spec.AuthConfigID || account.Toolkit.Slug != toolkit {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	return &account, nil
}

func composioReference(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

type composioParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	In    string `json:"in"`
}

type composioBinaryBody struct {
	Base64      string `json:"base64"`
	ContentType string `json:"content_type,omitempty"`
}

type composioProxyRequest struct {
	ConnectionID string              `json:"connected_account_id"`
	UserID       string              `json:"user_id"`
	Endpoint     string              `json:"endpoint"`
	Method       string              `json:"method"`
	Parameters   []composioParameter `json:"parameters,omitempty"`
	Body         json.RawMessage     `json:"body,omitempty"`
	BinaryBody   *composioBinaryBody `json:"binary_body,omitempty"`
}

var composioRequestHeaders = []string{
	"Accept", "Accept-Language", "Content-Type", "If-Match", "If-None-Match",
	"If-Modified-Since", "If-Unmodified-Since", "Range", "If-Range", "Cache-Control",
}
