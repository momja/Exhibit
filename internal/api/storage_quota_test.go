package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quotaRefusal(t *testing.T, w *httptest.ResponseRecorder, limit, usage, size, additional int64) {
	t.Helper()
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	var got struct {
		storageQuotaError
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "storage_quota_exceeded", got.Code)
	assert.Equal(t, storageQuotaError{limit, usage, size, additional}, got.storageQuotaError)
	assert.Contains(t, got.Error, "storage quota exceeded")
}

func quotaBlobContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	contents := make(map[string]string)
	for _, name := range blobFileNames(t, dir) {
		body, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		contents[name] = string(body)
	}
	return contents
}

func TestStorageQuotaRefusesEveryArtifactWriteBeforeStoringBytes(t *testing.T) {
	withUnguardedFetcher(t)
	for _, agent := range []bool{false, true} {
		for _, operation := range []string{"create", "body", "widget", "refetch"} {
			if agent && operation == "refetch" {
				continue // Agent grants cannot reach refetch; they write through PATCH.
			}
			t.Run(operation+map[bool]string{false: "/owner", true: "/agent"}[agent], func(t *testing.T) {
				r, dir := newTestRouterWithBlobDir(t)
				upstream := newMutableUpstream(t)
				id := createArtifact(t, r, map[string]any{"body": "old body", "url": upstream.pageURL(), "title": "Original"})
				used := storageUsed(t, r)
				r.cfg.Entitlements = enforcing(used + 1)
				before := quotaBlobContents(t, dir)
				token := "secret"
				if agent {
					reg := agentscope.NewRegistry()
					r.cfg.AgentCredentials = reg
					artifactID := id
					if operation == "create" {
						artifactID = ""
					}
					grant, err := reg.Issue(1, artifactID, "")
					require.NoError(t, err)
					token = grant.Token()
				}
				method, path := "POST", "/api/artifacts"
				var payload any = map[string]any{"body": "new body"}
				switch operation {
				case "body":
					method, path = "PATCH", "/api/artifacts/"+id
					payload = map[string]any{"body": "new body", "title": "Must not change", "expected_source_revision": contentRevisionForTest(t, r, id)}
				case "widget":
					method, path = "PUT", "/api/artifacts/"+id+"/widget"
					payload = map[string]any{"body": "new body", "expected_widget_revision": widgetSnapshot(t, r, id).WidgetRevision}
				case "refetch":
					path, payload = "/api/artifacts/"+id+"/refetch", nil
				}
				w := doWithToken(t, r, method, path, token, payload)
				if operation == "refetch" {
					require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
					var refused storageQuotaError
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &refused))
					assert.Greater(t, refused.WriteBytes, int64(len(refetchV1Page)), "assembled snapshot includes vendored payloads")
					quotaRefusal(t, w, used+1, used, refused.WriteBytes, refused.WriteBytes)
				} else {
					quotaRefusal(t, w, used+1, used, 8, 8)
				}
				assert.Equal(t, before, quotaBlobContents(t, dir), "a refusal must not even stage blobs")
				assert.Equal(t, used, storageUsed(t, r))
				assert.Contains(t, artifactBody(t, r, id), "old body")
				a, err := r.cfg.Store.GetArtifact(context.Background(), 1, id)
				require.NoError(t, err)
				assert.Equal(t, "Original", a.Title)
				versions, err := r.cfg.Store.ListVersions(context.Background(), 1, id)
				require.NoError(t, err)
				assert.Len(t, versions, 1)
			})
		}
	}
}

func TestStorageQuotaCountsRetainedVersionsAndAllowsTheExactLimit(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Entitlements = enforcing(10)
	id := createArtifact(t, r, map[string]any{"body": "12345678"})
	// A shorter replacement still consumes new bytes; the first version stays.
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "abc"})
	quotaRefusal(t, w, 10, 8, 3, 3)
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "ab"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(10), storageUsed(t, r))
	quotaRefusal(t, putWidgetReq(t, r, id, "x"), 10, 10, 1, 1)
	// A byte-identical save creates no blob or version and needs no space.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "ab"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(10), storageUsed(t, r))
}

func TestStorageQuotaChecksAssembledSnapshotsAndDeduplicatesOwnedAssets(t *testing.T) {
	withUnguardedFetcher(t)
	r, dir := newTestRouterWithBlobDir(t)
	srv := wasmFixture(t)
	request := map[string]any{"url": srv.URL + "/page.html", "snapshot": true}
	_, first := postArtifact(t, r, request)
	require.NotNil(t, first.Artifact)
	used := storageUsed(t, r)
	bodyBytes := int64(len(artifactBody(t, r, first.Artifact.ID)))
	assert.Equal(t, bodyBytes+int64(len(wasmFixtureBody)), used)
	r.cfg.Entitlements = enforcing(used + bodyBytes - 1)
	before := quotaBlobContents(t, dir)
	w, _ := postArtifact(t, r, request)
	quotaRefusal(t, w, used+bodyBytes-1, used, bodyBytes+int64(len(wasmFixtureBody)), bodyBytes)
	assert.Equal(t, before, quotaBlobContents(t, dir))
	// At the exact projected total the second snapshot fits: its wasm is
	// already charged to this owner, while its document is a fresh blob.
	r.cfg.Entitlements = enforcing(used + bodyBytes)
	w, _ = postArtifact(t, r, request)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, used+bodyBytes, storageUsed(t, r))

	// A new owner cannot exclude the payload just because somebody else holds it.
	other := newTestRouter(t)
	other.cfg.Entitlements = enforcing(bodyBytes)
	w, _ = postArtifact(t, other, request)
	quotaRefusal(t, w, bodyBytes, 0, used, used)
}

func TestLoweringStorageEntitlementPreservesTheLibraryAndAllowsDeletion(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.Entitlements = enforcing(100)
	w := in.do(t, "POST", "/api/artifacts", in.memberCookie, map[string]any{"body": "<html>kept forever</html>", "title": "Kept"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created createArtifactResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := created.Artifact.ID
	path := "/api/artifacts/" + id
	// The admin may lower a ceiling below current usage, without any reclaim.
	w = in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{"storage_limit_bytes": 0})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, read := range []string{"/api/artifacts", path + "?body=true", path + "/export", path + "/versions"} {
		w = in.do(t, "GET", read, in.memberCookie, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	body, err := in.ro.readBlobString(context.Background(), created.Artifact.SourceBlobID)
	require.NoError(t, err)
	assert.Equal(t, "<html>kept forever</html>", body)
	urls := testRenderURLs(in.ro.cfg.RenderOrigin)
	urls.signer = in.ro.tokens
	urls.ownerID, urls.viewerID = in.member.ID, in.member.ID
	w = renderGet(t, in.ro, urls.artifact(id))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = in.do(t, "PATCH", path, in.memberCookie, map[string]any{"body": "x"})
	quotaRefusal(t, w, 0, int64(len(body)), 1, 1)
	w = in.do(t, "PATCH", path, in.memberCookie, map[string]any{"title": "Still editable"})
	require.Equal(t, http.StatusOK, w.Code)
	w = in.do(t, "DELETE", path, in.memberCookie, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	n, err := in.st.StorageUsage(context.Background(), in.member.ID)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestStorageQuotaDefaultsAndOwnerOverrides(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.Entitlements = enforcing(0)
	request := map[string]any{"body": "12345"}
	w := in.do(t, "POST", "/api/artifacts", in.memberCookie, request)
	quotaRefusal(t, w, 0, 0, 5, 5)
	w = in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{"storage_limit_bytes": 5})
	require.Equal(t, http.StatusOK, w.Code)
	w = in.do(t, "POST", "/api/artifacts", in.memberCookie, request)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	// That owner's override does not change the admin's default zero limit.
	w = in.do(t, "POST", "/api/artifacts", in.adminCookie, request)
	quotaRefusal(t, w, 0, 0, 5, 5)
}

func TestConcurrentStorageWritesCannotSpendTheSameSpace(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Entitlements = enforcing(8)
	const writers = 12
	start := make(chan struct{})
	results := make(chan int, writers)
	for range writers {
		go func() {
			<-start
			req := httptest.NewRequest("POST", "/api/artifacts", strings.NewReader(`{"body":"12345678"}`))
			req.Header.Set("Authorization", authHeader())
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			results <- w.Code
		}()
	}
	close(start)
	counts := map[int]int{}
	for range writers {
		counts[<-results]++
	}
	assert.Equal(t, map[int]int{http.StatusCreated: 1, http.StatusRequestEntityTooLarge: writers - 1}, counts)
	assert.Equal(t, int64(8), storageUsed(t, r))
}

func TestStorageQuotaFailsClosedOnUnknownAccounting(t *testing.T) {
	for _, mode := range []string{"missing size", "record failure", "entitlement failure"} {
		t.Run(mode, func(t *testing.T) {
			r, dir := newTestRouterWithBlobDir(t)
			r.cfg.Entitlements = enforcing(100)
			switch mode {
			case "missing size":
				require.NoError(t, r.cfg.Store.PutArtifact(context.Background(), &store.Artifact{ID: "legacy", OwnerID: 1, SourceBlobID: "unmeasured", Tier: store.Tier1}))
			case "record failure":
				r.cfg.Store = brokenSizeStore{Store: r.cfg.Store}
			case "entitlement failure":
				r.cfg.Store = failingQuotaEntitlement{Store: r.cfg.Store}
			}
			w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{"body": "new"})
			require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
			assert.Empty(t, quotaBlobContents(t, dir), "failed accounting cannot leak uncharged bytes")
			arts, err := r.cfg.Store.ListArtifacts(context.Background(), store.ListOptions{OwnerID: 1})
			require.NoError(t, err)
			for _, art := range arts {
				assert.Equal(t, "legacy", art.ID, "the failed create cannot publish a reference")
			}
		})
	}
}

type failingQuotaEntitlement struct{ store.Store }

func (failingQuotaEntitlement) GetUser(context.Context, int64) (*store.User, error) {
	return nil, assert.AnError
}

func TestStorageQuotaOffIgnoresEntitlementAndAccountingFailures(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Store = failingQuotaEntitlement{Store: brokenSizeStore{Store: r.cfg.Store}}
	id := createArtifact(t, r, map[string]any{"body": "initial"})
	patchArtifact(t, r, id, map[string]any{"body": "more bytes"})
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "tile").Code)
}

func TestStorageQuotaRefusesAnOverflowingProjection(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"body": "initial"})
	a, err := r.cfg.Store.GetArtifact(context.Background(), 1, id)
	require.NoError(t, err)
	require.NoError(t, r.cfg.Store.RecordBlobSize(context.Background(), a.SourceBlobID, math.MaxInt64))
	r.cfg.Entitlements = enforcing(math.MaxInt64)
	quotaRefusal(t, doJSON(t, r, "POST", "/api/artifacts", map[string]any{"body": "x"}), math.MaxInt64, math.MaxInt64, 1, 1)
}
