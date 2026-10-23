package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/momja/Exhibit/internal/egress"
)

// Nango exposes managed OAuth credentials, so provider traffic always uses
// Exhibit's own gate. No provider request or redirect is delegated to Nango.
type nangoEngine struct{ s *Service }

func (nangoEngine) ID() string           { return "nango" }
func (nangoEngine) DeliveryMode() string { return VendorInject }

func (e nangoEngine) Start(ctx context.Context, flow VendorFlowRequest) (*VendorSession, error) {
	raw, _ := json.Marshal(map[string]any{"end_user": map[string]string{"id": flow.OwnerTag}, "tags": map[string]string{"end_user_id": flow.OwnerTag, "exhibit_owner": flow.OwnerTag, "exhibit_flow": flow.ID}, "allowed_integrations": []string{flow.Provider}})
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), flow.Provider, http.MethodPost, "/connect/sessions", raw, "management")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data struct {
			Token   string `json:"token"`
			Link    string `json:"connect_link"`
			Expires string `json:"expires_at"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body, &result) != nil || result.Data.Token == "" {
		return nil, vendorFailure(e.ID(), VendorUpstreamError)
	}
	link := result.Data.Link
	if link == "" {
		link = "https://connect.nango.dev/?session_token=" + url.QueryEscape(result.Data.Token)
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != "connect.nango.dev" || u.Fragment != "" {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	return &VendorSession{URL: link, ExpiresAt: result.Data.Expires}, nil
}
func (e nangoEngine) Complete(ctx context.Context, flow VendorFlowRequest) (string, bool, error) {
	query := url.Values{"tags[exhibit_flow]": []string{flow.ID}, "tags[end_user_id]": []string{flow.OwnerTag}}
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), flow.Provider, http.MethodGet, "/connections?"+query.Encode(), nil, "management")
	if err != nil {
		return "", false, err
	}
	var listing struct {
		Connections []nangoConnection `json:"connections"`
	}
	if json.Unmarshal(response.Body, &listing) != nil {
		return "", false, vendorFailure(e.ID(), VendorUpstreamError)
	}
	ref := ""
	for _, remote := range listing.Connections {
		if remote.Provider != flow.Provider || remote.Tags["exhibit_flow"] != flow.ID || remote.Tags["end_user_id"] != flow.OwnerTag || remote.Tags["exhibit_owner"] != flow.OwnerTag {
			continue
		}
		if len(remote.Errors) > 0 {
			return "", false, vendorFailure(e.ID(), VendorNeedsReconnect)
		}
		spec := VendorConnectionSpec{OwnerTag: flow.OwnerTag, Provider: flow.Provider, ExternalRef: remote.ID, Spec: flow.Spec}
		verified, err := e.connection(ctx, spec)
		if err != nil {
			return "", false, err
		}
		if verified.Tags["exhibit_flow"] != flow.ID {
			return "", false, vendorFailure(e.ID(), VendorRefused)
		}
		if ref != "" && ref != remote.ID {
			return "", false, vendorFailure(e.ID(), VendorRefused)
		}
		ref = remote.ID
	}
	return ref, ref != "", nil
}

type nangoConnection struct {
	ID       string            `json:"connection_id"`
	Provider string            `json:"provider_config_key"`
	Tags     map[string]string `json:"tags"`
	Errors   []json.RawMessage `json:"errors"`
	EndUser  struct {
		ID string `json:"id"`
	} `json:"end_user"`
	Credentials struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Type         string `json:"type"`
	} `json:"credentials"`
}

func (e nangoEngine) connection(ctx context.Context, spec VendorConnectionSpec) (*nangoConnection, error) {
	if !validVendorReference(spec.ExternalRef) {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	path := "/connections/" + url.PathEscape(spec.ExternalRef) + "?provider_config_key=" + url.QueryEscape(spec.Provider)
	response, err := e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodGet, path, nil, "management")
	if err != nil {
		return nil, err
	}
	var result nangoConnection
	if json.Unmarshal(response.Body, &result) != nil || result.ID != spec.ExternalRef || result.Provider != spec.Provider {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	if result.Tags["exhibit_owner"] != spec.OwnerTag || result.EndUser.ID != spec.OwnerTag {
		return nil, vendorFailure(e.ID(), VendorRefused)
	}
	return &result, nil
}
func (e nangoEngine) Status(ctx context.Context, spec VendorConnectionSpec) (string, error) {
	remote, err := e.connection(ctx, spec)
	if err != nil {
		varErr, ok := err.(*VendorError)
		if ok && varErr.Code == VendorNeedsReconnect {
			return VendorNeedsReconnect, nil
		}
		if ok && varErr.HTTPStatus == http.StatusNotFound {
			return "revoked", nil
		}
		return "", err
	}
	if len(remote.Errors) > 0 || remote.Credentials.AccessToken == "" {
		return VendorNeedsReconnect, nil
	}
	return "ok", nil
}
func (e nangoEngine) Authorize(ctx context.Context, req *http.Request, spec VendorConnectionSpec, c *credential) error {
	if !slices.Contains(spec.Spec.Origins, requestOrigin(req.URL)) {
		return vendorFailure(e.ID(), VendorRefused)
	}
	remote, err := e.connection(ctx, spec)
	if err != nil {
		return err
	}
	if len(remote.Errors) > 0 || remote.Credentials.AccessToken == "" {
		return vendorFailure(e.ID(), VendorNeedsReconnect)
	}
	if remote.Credentials.Type != "" && !strings.EqualFold(remote.Credentials.Type, "OAUTH2") {
		return vendorFailure(e.ID(), VendorRefused)
	}
	if len(remote.Credentials.AccessToken)+len(remote.Credentials.RefreshToken) > 16384 || strings.ContainsAny(remote.Credentials.AccessToken+remote.Credentials.RefreshToken, "\r\n") {
		return vendorFailure(e.ID(), VendorUpstreamError)
	}
	c.Value = remote.Credentials.AccessToken
	c.Password = remote.Credentials.RefreshToken
	req.Header.Set("Authorization", "Bearer "+c.Value)
	return nil
}
func (e nangoEngine) Forward(context.Context, *http.Request, VendorConnectionSpec) (*egress.Response, error) {
	return nil, vendorFailure(e.ID(), VendorRefused)
}
func (e nangoEngine) Revoke(ctx context.Context, spec VendorConnectionSpec) error {
	if !validVendorReference(spec.ExternalRef) {
		return vendorFailure(e.ID(), VendorRefused)
	}
	// Recheck the saved owner and provider before destructive management I/O.
	// The same external reference may have been replaced outside Exhibit.
	if _, err := e.connection(ctx, spec); err != nil {
		var failure *VendorError
		if errors.As(err, &failure) && failure.HTTPStatus == http.StatusNotFound {
			return nil
		}
		return err
	}
	_, err := e.s.vendorAPIRequest(ctx, e.ID(), spec.Provider, http.MethodDelete, "/connections/"+url.PathEscape(spec.ExternalRef)+"?provider_config_key="+url.QueryEscape(spec.Provider), nil, "management")
	return err
}
