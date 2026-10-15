package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A conversation is stored as Pi wrote it, tied to the artifact version it was
// last working against. What these pin is that tie, and that a list never has
// to read the stored files.

func saveConversation(t *testing.T, s *SQLiteStore, id, session, file string) {
	t.Helper()
	require.NoError(t, s.SaveTranscript(context.Background(), 1, Transcript{
		ArtifactID: id, SessionID: session, Title: "title of " + session, SessionFile: file,
	}))
}

// The version is read in the statement that stores the file, so the two always
// describe the same moment — and storing again, after the artifact moved on,
// moves the conversation's version with it.
func TestAConversationIsStampedWithTheHeadVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")

	saveConversation(t, s, "a1", "s1", "line1\n")
	ts, err := s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 1, ts[0].VersionSeq, "the artifact was at its first version")

	blob := "a1-body-2"
	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: &blob})
	require.NoError(t, err)
	head, err := s.HeadVersionSeq(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, 2, head)

	// Not stamped until it is saved again: what it records is what it last saw.
	ts, err = s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, 1, ts[0].VersionSeq)

	saveConversation(t, s, "a1", "s1", "line1\nline2\n")
	ts, err = s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, ts, 1, "the same conversation, kept in place")
	assert.Equal(t, 2, ts[0].VersionSeq)
}

func TestAListHoldsSummariesAndAGetHoldsTheFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	big := strings.Repeat("x", 1<<20) + "\n"
	saveConversation(t, s, "a1", "s1", big)

	ts, err := s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, "title of s1", ts[0].Title)
	assert.True(t, ts[0].Resumable)
	assert.Empty(t, ts[0].SessionFile, "a list never carries the file")

	got, err := s.GetTranscript(ctx, 1, "a1", "s1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, big, got.SessionFile)
	assert.True(t, got.Resumable)

	missing, err := s.GetTranscript(ctx, 1, "a1", "nope")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

// A conversation kept before session files existed holds a message dump and no
// file: it can be read, and cannot be resumed.
func TestAConversationKeptBeforeSessionFilesCanBeReadNotResumed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	_, err := s.db.Exec(`INSERT INTO agent_transcripts (artifact_id, session_id, messages)
	                     VALUES ('a1', 'old', '[{"role":"user","content":"hi"}]')`)
	require.NoError(t, err)

	ts, err := s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.False(t, ts[0].Resumable)
	assert.Equal(t, 0, ts[0].VersionSeq, "unknown")

	got, err := s.GetTranscript(ctx, 1, "a1", "old")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Resumable)
	assert.Empty(t, got.SessionFile)
	assert.Contains(t, got.Messages, `"hi"`)
}

func TestConversationsListMostRecentFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	saveConversation(t, s, "a1", "first", "a\n")
	saveConversation(t, s, "a1", "second", "b\n")
	_, err := s.db.Exec(`UPDATE agent_transcripts SET updated_at = datetime('now', '-1 day') WHERE session_id = 'first'`)
	require.NoError(t, err)

	ts, err := s.ListTranscripts(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, ts, 2)
	assert.Equal(t, "second", ts[0].SessionID)
	assert.Equal(t, "first", ts[1].SessionID)
}

// The history goes with the artifact, like everything else kept for it.
func TestConversationsGoWithTheirArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	saveConversation(t, s, "a1", "s1", "a\n")

	_, err := s.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	var n int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM agent_transcripts`).Scan(&n))
	assert.Equal(t, 0, n)
}
