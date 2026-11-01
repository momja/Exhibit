package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

func TestRefusalsExplainRecovery(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		path   string
		body   string
		want   string
	}{
		{"source conflict", 409, "/api/artifacts/a", "", "get_artifact"},
		{"widget conflict", 409, "/api/artifacts/a/widget", "", "get_widget"},
		{"precondition", 428, "/api/artifacts/a", "", "Nothing was saved"},
		{"quota", 413, "/api/artifacts/a", `{"code":"storage_quota_exceeded","error":"over quota"}`, "earlier versions"},
		{"body limit", 413, "/api/artifacts/a", "", "make it smaller"},
		{"revoked", 401, "/api/artifacts/a", "", "new one"},
		{"forbidden", 403, "/api/artifacts/a", "", "owner's library"},
		{"not found", 404, "/api/artifacts/a", "", "does not exist in this library"},
		{"invalid edit", 400, "/api/artifacts/a", `{"error":"oldText does not match"}`, "oldText does not match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := refusal(http.MethodPatch, tt.path, &Response{Status: tt.status, Body: []byte(tt.body)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("refusal = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestArtifactPathCannotSelectAnotherRoute(t *testing.T) {
	for _, id := range []string{"", " ", ".", "..", "../shares", "a/state", `a\state`} {
		if path, err := artifactPath(id, "/state"); err == nil {
			t.Errorf("accepted %q as %q", id, path)
		}
	}
	path, err := artifactPath("01234567-89ab-4cde-8abc-0123456789ab", "/state")
	if err != nil || path != "/api/artifacts/01234567-89ab-4cde-8abc-0123456789ab/state" {
		t.Fatalf("literal id path = %q, %v", path, err)
	}
}

func TestStateReadRejectsIncompleteOrNonObjectResponses(t *testing.T) {
	for _, body := range []string{`{"keep":"value"`, `["key", "value"]`, `null`, `{"key":"value"}{"extra":"value"}`} {
		if _, _, err := orderedStringMap([]byte(body)); err == nil {
			t.Errorf("accepted malformed state %q", body)
		}
	}
	keys, values, err := orderedStringMap([]byte("{\"z\":\" { \\\"n\\\":1.00 } \",\"a\":\"last\"}\n"))
	if err != nil || len(keys) != 2 || keys[0] != "z" || keys[1] != "a" || values["z"] != " { \"n\":1.00 } " {
		t.Fatalf("state order/bytes not preserved: %v, %v, %v", keys, values, err)
	}
}
