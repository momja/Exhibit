package blob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-52ll. The pure configuration logic: how an operator's endpoint string
// becomes a host and a TLS decision, and how the absence of a bucket keeps the
// filesystem backend. Neither needs a bucket to test, and both are where a
// misreading would be silent — a scheme guessed wrong is plaintext credentials
// on the wire, and a selector read wrong is bodies written somewhere nobody
// looks.

func TestParseS3Endpoint(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		secure bool
	}{
		{"", "s3.amazonaws.com", true},
		{"https://minio.example.com", "minio.example.com", true},
		{"http://localhost:9000", "localhost:9000", false},
		{"https://minio.example.com/", "minio.example.com", true},
		// No scheme written means none is assumed away: TLS is the default, so
		// plaintext is something an operator has to ask for by name.
		{"minio.example.com", "minio.example.com", true},
		{"minio.example.com:9000", "minio.example.com:9000", true},
		{"  minio.example.com  ", "minio.example.com", true},
	}
	for _, c := range cases {
		host, secure, err := parseS3Endpoint(c.in)
		require.NoError(t, err, "endpoint %q", c.in)
		assert.Equal(t, c.host, host, "endpoint %q", c.in)
		assert.Equal(t, c.secure, secure, "endpoint %q", c.in)
	}
}

func TestParseS3EndpointRejectsNonsense(t *testing.T) {
	for _, in := range []string{
		"ftp://files.example.com",
		"s3://bucket",
		"https://",
		// A path cannot be honoured — the SDK addresses buckets from the host —
		// so it must be refused rather than dropped, or an operator's requests
		// go somewhere they did not write and nothing says so.
		"https://gateway.example.com/s3",
		"http://localhost:9000/exhibit",
	} {
		_, _, err := parseS3Endpoint(in)
		assert.Error(t, err, "endpoint %q", in)
	}
}

func TestNormalizeS3Prefix(t *testing.T) {
	// A prefix is a key namespace, so it ends in exactly one separator however
	// the operator wrote it — and an unset one adds no separator at all, which
	// keeps keys identical to a bucket used without a prefix.
	assert.Equal(t, "", normalizeS3Prefix(""))
	assert.Equal(t, "", normalizeS3Prefix("/"))
	assert.Equal(t, "blobs/", normalizeS3Prefix("blobs"))
	assert.Equal(t, "blobs/", normalizeS3Prefix("/blobs/"))
	assert.Equal(t, "a/b/", normalizeS3Prefix("a/b"))
}

func TestS3StoreKeyIsPrefixPlusID(t *testing.T) {
	s := &S3Store{prefix: normalizeS3Prefix("exhibit")}
	assert.Equal(t, "exhibit/abc", s.key("abc"))

	unprefixed := &S3Store{}
	assert.Equal(t, "abc", unprefixed.key("abc"))
}

func TestNewS3StoreRequiresABucket(t *testing.T) {
	_, err := NewS3Store(context.Background(), S3Config{})
	assert.Error(t, err)
}

func TestOpenWithoutABucketIsTheFilesystem(t *testing.T) {
	// The acceptance criterion the self-hosted instance depends on: with the
	// variable unset, blob storage is FSStore and nothing else has changed.
	t.Setenv("BLOB_S3_BUCKET", "")
	dir := t.TempDir()

	s, err := Open(context.Background(), dir)
	require.NoError(t, err)
	assert.IsType(t, &FSStore{}, s)
}

// The other half of the selector's contract: absent means filesystem, but only
// when it is absent. A companion variable set without the bucket is the shape a
// typo'd bucket name takes, and reading it as "filesystem, then" would put a
// hosted instance's artifact bodies on a disk that does not survive a restart
// while its operator believed they were in a bucket.
func TestOpenRefusesAHalfConfiguredBucket(t *testing.T) {
	for _, name := range []string{
		"BLOB_S3_ENDPOINT",
		"BLOB_S3_REGION",
		"BLOB_S3_ACCESS_KEY_ID",
		"BLOB_S3_SECRET_ACCESS_KEY",
		"BLOB_S3_PREFIX",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BLOB_S3_BUCKET", "")
			t.Setenv(name, "something")

			_, err := Open(context.Background(), t.TempDir())
			require.Error(t, err, "%s without a bucket must not silently fall back to disk", name)
			assert.Contains(t, err.Error(), "BLOB_S3_BUCKET")
		})
	}
}

func TestKnownSize(t *testing.T) {
	// -1 is the honest answer for a reader that cannot say, and it is the one
	// that costs a bounded multipart upload rather than a wrong Content-Length.
	assert.Equal(t, int64(-1), knownSize(readerOnly{}))
}

type readerOnly struct{}

func (readerOnly) Read([]byte) (int, error) { return 0, nil }
