package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

// APIProposalSpec is an agent-authored suggestion. It deliberately has no
// credential, arbitrary URL, authentication template, or permission fields.
type APIProposalSpec struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Header string `json:"header"`
	Auth   string `json:"auth"`
	Reason string `json:"reason"`
}

type APIProposal struct {
	ID           string          `json:"id"`
	ArtifactID   string          `json:"artifact_id"`
	Spec         APIProposalSpec `json:"spec"`
	State        string          `json:"state"`
	ConnectionID string          `json:"connection_id,omitempty"`
}

// ValidateAPIProposal checks a secret-free destination without contacting it.
// The relay independently checks all DNS answers and pins public IPs whenever
// a connection is used; an accepted proposal supplies no network authority.
func ValidateAPIProposal(spec APIProposalSpec) (APIProposalSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Reason = strings.TrimSpace(spec.Reason)
	if spec.Name == "" || len(spec.Name) > 100 || !utf8.ValidString(spec.Name) || strings.ContainsFunc(spec.Name, unsafeProposalText) {
		return APIProposalSpec{}, fmt.Errorf("%w: give the connection a name of at most 100 characters", ErrInvalid)
	}
	if spec.Reason == "" || utf8.RuneCountInString(spec.Reason) > 1000 || !utf8.ValidString(spec.Reason) || strings.ContainsFunc(spec.Reason, unsafeProposalText) {
		return APIProposalSpec{}, fmt.Errorf("%w: explain why the connection is needed in at most 1000 characters", ErrInvalid)
	}
	o, err := normalizeHTTPSOrigin(spec.Origin)
	if err != nil {
		return APIProposalSpec{}, err
	}
	u, err := url.Parse(spec.Origin)
	if err != nil || u.Path != "" || u.RawPath != "" || strings.Contains(spec.Origin, "#") || strings.HasSuffix(u.Host, ":") {
		return APIProposalSpec{}, fmt.Errorf("%w: enter only the HTTPS origin, without a path, query, fragment or credentials", ErrInvalid)
	}
	parsed, _ := url.Parse(o)
	if !publicProposalHostname(parsed.Hostname()) {
		return APIProposalSpec{}, fmt.Errorf("%w: choose a public HTTPS DNS hostname", ErrInvalid)
	}
	spec.Origin = o
	spec.Header = http.CanonicalHeaderKey(strings.TrimSpace(spec.Header))
	switch spec.Auth {
	case "api-key":
		if spec.Header == "" {
			spec.Header = "X-Api-Key"
		}
		if len(spec.Header) > 100 || !safeCredentialHeader(spec.Header) {
			return APIProposalSpec{}, fmt.Errorf("%w: choose a safe API credential header such as X-API-Key", ErrInvalid)
		}
	case "bearer":
		if spec.Header != "" && spec.Header != "Authorization" {
			return APIProposalSpec{}, fmt.Errorf("%w: Bearer authentication uses the Authorization header", ErrInvalid)
		}
		spec.Header = "Authorization"
	default:
		return APIProposalSpec{}, fmt.Errorf("%w: choose api-key or bearer authentication", ErrInvalid)
	}
	return spec, nil
}

func unsafeProposalText(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

func publicProposalHostname(host string) bool {
	if net.ParseIP(host) != nil || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, suffix := range []string{"localhost", "local", "internal", "intranet", "lan", "home", "home.arpa", "onion", "invalid", "test", "example"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return false
		}
	}
	labels := strings.Split(host, ".")
	// Browsers interpret a numeric final label as an IPv4 spelling, even when
	// earlier labels are unusual. Refuse these ambiguities as well as IPs.
	last := labels[len(labels)-1]
	if _, err := strconv.ParseUint(last, 0, 64); err == nil || strings.HasPrefix(last, "0x") {
		return false
	}
	allDigits := true
	for _, r := range last {
		allDigits = allDigits && r >= '0' && r <= '9'
	}
	if allDigits {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func apiProposal(p *store.ConnectorProposalRecord) *APIProposal {
	state := p.State
	if state == "approved" && p.ConnectionID == "" {
		state = "disconnected"
	}
	return &APIProposal{ID: p.ID, ArtifactID: p.ArtifactID, State: state, ConnectionID: p.ConnectionID,
		Spec: APIProposalSpec{Name: p.Name, Origin: p.Origin, Header: p.Header, Auth: p.Auth, Reason: p.Reason}}
}

func (s *Service) ProposeAPI(ctx context.Context, ownerID int64, artifactID string, spec APIProposalSpec) (*APIProposal, error) {
	if !s.cfg.CustomEnabled || s.cfg.Store == nil || s.cfg.Box == nil {
		return nil, ErrDisabled
	}
	if ownerID <= 0 {
		return nil, store.ErrNotFound
	}
	spec, err := ValidateAPIProposal(spec)
	if err != nil {
		return nil, err
	}
	p, err := s.cfg.Store.CreateConnectorProposal(ctx, &store.ConnectorProposalRecord{ID: uuid.NewString(), OwnerID: ownerID, ArtifactID: artifactID,
		Name: spec.Name, Origin: spec.Origin, Header: spec.Header, Auth: spec.Auth, Reason: spec.Reason})
	if err != nil {
		return nil, err
	}
	return apiProposal(p), nil
}

func (s *Service) GetAPIProposal(ctx context.Context, ownerID int64, artifactID, id string) (*APIProposal, error) {
	if !s.cfg.CustomEnabled || s.cfg.Store == nil || s.cfg.Box == nil {
		return nil, ErrDisabled
	}
	if ownerID <= 0 {
		return nil, store.ErrNotFound
	}
	p, err := s.cfg.Store.GetConnectorProposal(ctx, ownerID, artifactID, id)
	if err != nil {
		return nil, err
	}
	return apiProposal(p), nil
}

// ResolveAPIProposal accepts credentials only from the trusted app approval
// handler. An exact origin echo records the person's explicit trust decision;
// approval never accepts changes to the stored proposal or grants an artifact
// access to the resulting account.
func (s *Service) ResolveAPIProposal(ctx context.Context, ownerID int64, artifactID, id string, approve bool, trustedOrigin, secret string, limits Limits) (*APIProposal, error) {
	p, err := s.GetAPIProposal(ctx, ownerID, artifactID, id)
	if err != nil {
		return nil, err
	}
	if p.State != "pending" {
		return p, nil
	}
	var c *store.ConnectionRecord
	if approve {
		spec, err := ValidateAPIProposal(p.Spec)
		if err != nil {
			return nil, err
		}
		if trustedOrigin != spec.Origin {
			return nil, fmt.Errorf("%w: explicitly trust the exact destination origin before saving the key", ErrInvalid)
		}
		if strings.TrimSpace(secret) == "" || len(secret) > 16384 || !utf8.ValidString(secret) || strings.ContainsFunc(secret, unicode.IsControl) {
			return nil, fmt.Errorf("%w: enter a credential with a valid format", ErrInvalid)
		}
		// Agent proposals cannot override a recognized key's destination. A
		// mismatched key can still be set up through the normal advanced form,
		// which has its own explicit mismatch warning and confirmation.
		if err := credentialDestinationCheck(secret, []string{spec.Origin}, false); err != nil {
			return nil, err
		}
		r := recipe{Origins: []string{spec.Origin}, Header: spec.Header, Sensitivity: "personal"}
		if spec.Auth == "bearer" {
			r.Scheme = "Bearer "
		}
		policy, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(credential{Value: secret})
		if err != nil {
			return nil, err
		}
		sealed, err := s.cfg.Box.Encrypt(string(raw))
		if err != nil {
			return nil, fmt.Errorf("encrypt connection credential")
		}
		c = &store.ConnectionRecord{ID: uuid.NewString(), OwnerID: ownerID, Name: spec.Name, Provider: spec.Auth, Kind: "native", RecipeJSON: string(policy), EncryptedSecret: sealed}
	}
	resolved, err := s.cfg.Store.ResolveConnectorProposal(ctx, ownerID, artifactID, id, approve, c, limits.Native)
	if err != nil {
		return nil, err
	}
	return apiProposal(resolved), nil
}
