package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteShareRevokesOnlyTheAnonymousLink(t *testing.T) {
	in := newGrantedInstance(t)
	ctx := context.Background()
	third, err := in.ro.cfg.Store.UpsertUser(ctx, "sub-third", "third@example.test")
	require.NoError(t, err)
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, in.granted, third.ID)
	require.NoError(t, err)
	link, err := in.ro.cfg.Store.CreateAnonymousLink(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	before, err := in.ro.cfg.Store.ListArtifactShares(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	require.Len(t, before, 3)

	for _, id := range []string{before[1].ID, before[2].ID, "never-issued"} {
		w := deleteGrantAs(in.ro, "/api/shares/"+id, in.cookieOne)
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	}
	after, err := in.ro.cfg.Store.ListArtifactShares(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	assert.Equal(t, before, after, "grant ids cannot revoke grants or the link")

	w := deleteGrantAs(in.ro, "/api/shares/"+link.ID, in.cookieOne)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	remaining, err := in.ro.cfg.Store.ListArtifactShares(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	assert.Equal(t, before[1:], remaining, "link revocation preserves every grant unchanged")
	deleted, err := in.ro.cfg.Store.GetShare(ctx, in.ownerOne, link.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
	for _, recipientID := range []int64{in.ownerTwo, third.ID} {
		readable, err := in.ro.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(recipientID), in.granted)
		require.NoError(t, err)
		assert.NotNil(t, readable, "each recipient retains access after link revocation")
	}
}

type invalidRecipientStore struct{ store.Store }

func (s invalidRecipientStore) CreateGrant(context.Context, int64, string, int64) (*store.Share, error) {
	return nil, fmt.Errorf("create grant: %w", store.ErrInvalidRecipient)
}

func TestCreateGrantInvalidRecipientIsABadRequest(t *testing.T) {
	ro := newTestRouter(t)
	seedLocalAccount(t, ro, "alice")
	artifactID := seedShareableArtifact(t, ro, "Tool")
	ro.cfg.Store = invalidRecipientStore{ro.cfg.Store}
	w := do(t, ro, http.MethodPost, "/api/shares",
		[]byte(fmt.Sprintf(`{"artifact_id":%q,"recipients":["alice"]}`, artifactID)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), store.ErrInvalidRecipient.Error())
	assert.Empty(t, listShares(t, ro, artifactID).Grants)
}

func TestOwnerRevokesGrantByRecipient(t *testing.T) {
	in := newGrantedInstance(t)
	ctx := context.Background()
	link, err := in.ro.cfg.Store.CreateAnonymousLink(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	third, err := in.ro.cfg.Store.UpsertUser(ctx, "sub-third", "third@example.test")
	require.NoError(t, err)
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, in.granted, third.ID)
	require.NoError(t, err)

	path := fmt.Sprintf("/api/artifacts/%s/grants?recipient=%d", in.granted, in.ownerTwo)
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound} {
		w := deleteGrantAs(in.ro, path, in.cookieOne)
		require.Equal(t, status, w.Code, w.Body.String())
	}

	readable, err := in.ro.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(in.ownerTwo), in.granted)
	require.NoError(t, err)
	assert.Nil(t, readable, "revoking the grant removes the recipient's access")
	shares, err := in.ro.cfg.Store.ListArtifactShares(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	require.Len(t, shares, 2, "the anonymous link and the other grant survive")
	assert.Equal(t, link.ID, shares[0].ID)
	require.NotNil(t, shares[1].RecipientID)
	assert.Equal(t, third.ID, *shares[1].RecipientID)

	// Missing grants use the same repeat-delete response as anonymous links.
	linkPath := "/api/shares/" + link.ID
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound} {
		w := deleteGrantAs(in.ro, linkPath, in.cookieOne)
		assert.Equal(t, status, w.Code, w.Body.String())
	}
}

func TestGrantRevokeIsOwnerScoped(t *testing.T) {
	in := newGrantedInstance(t)
	path := fmt.Sprintf("/api/artifacts/%s/grants?recipient=%d", in.granted, in.ownerTwo)
	for _, cookie := range []*http.Cookie{in.cookieTwo, newStrangerSession(t, in)} {
		w := deleteGrantAs(in.ro, path, cookie)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	}
	unknown := fmt.Sprintf("/api/artifacts/missing/grants?recipient=%d", in.ownerTwo)
	assert.Equal(t, http.StatusNotFound, deleteGrantAs(in.ro, unknown, in.cookieOne).Code)

	readable, err := in.ro.cfg.Store.GetArtifactReadableBy(context.Background(), store.ViewerID(in.ownerTwo), in.granted)
	require.NoError(t, err)
	assert.NotNil(t, readable, "a recipient or stranger cannot revoke the grant")
}

func TestGrantRevokeRequiresOnePositiveRecipient(t *testing.T) {
	in := newGrantedInstance(t)
	for _, query := range []string{
		"", "?recipient=", "?recipient=0", "?recipient=-1", "?recipient=alice",
		"?recipient=1.5", "?recipient=9223372036854775808", "?recipient=..",
		"?recipient=2&recipient=3",
	} {
		t.Run(query, func(t *testing.T) {
			w := deleteGrantAs(in.ro, "/api/artifacts/"+in.granted+"/grants"+query, in.cookieOne)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
	shares, err := in.ro.cfg.Store.ListArtifactShares(context.Background(), in.ownerOne, in.granted)
	require.NoError(t, err)
	assert.Len(t, shares, 1, "invalid revokes leave the artifact and its grant intact")
}

func TestGrantViewsExposeRecipientInsteadOfRowID(t *testing.T) {
	ro := newTestRouter(t)
	recipientID := seedLocalAccount(t, ro, "alice")
	artifactID := seedShareableArtifact(t, ro, "Tool")
	result := do(t, ro, http.MethodPost, "/api/shares",
		[]byte(fmt.Sprintf(`{"artifact_id":%q,"recipients":["alice"]}`, artifactID)))
	require.Equal(t, http.StatusOK, result.Code)
	assert.NotContains(t, result.Body.String(), `"share_id"`)

	listing := do(t, ro, http.MethodGet, "/api/artifacts/"+artifactID+"/shares", nil)
	require.Equal(t, http.StatusOK, listing.Code)
	var view struct {
		Grants []map[string]json.RawMessage `json:"grants"`
	}
	require.NoError(t, json.Unmarshal(listing.Body.Bytes(), &view))
	require.Len(t, view.Grants, 1)
	assert.NotContains(t, view.Grants[0], "id")
	assert.JSONEq(t, fmt.Sprint(recipientID), string(view.Grants[0]["recipient_id"]))

	fragment := getPage(t, ro, "/partials/share-panel?artifact="+artifactID)
	assert.Contains(t, fragment, fmt.Sprintf(`data-recipient-id="%d"`, recipientID))
	assert.NotContains(t, fragment, "data-share-id=", "grant controls need no grant id")
}

func deleteGrantAs(ro *Router, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}
