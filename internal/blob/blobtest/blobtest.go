// Package blobtest points tests at a real S3-compatible bucket when one is
// configured, and skips them when it is not (av-52ll).
//
// It exists so that "how do I run the suite against MinIO?" has one answer
// rather than one per package. The environment variables are deliberately *not*
// the production BLOB_S3_* names: a developer who has the real bucket
// configured in their shell must not have a test run reach into it.
//
//	EXHIBIT_TEST_S3_ENDPOINT=http://127.0.0.1:9000 \
//	EXHIBIT_TEST_S3_ACCESS_KEY_ID=minioadmin \
//	EXHIBIT_TEST_S3_SECRET_ACCESS_KEY=minioadmin \
//	go test ./...
package blobtest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/momja/Exhibit/internal/blob"
)

// Configured reports whether an S3-compatible endpoint is available to test
// against, for callers that want to branch rather than skip.
func Configured() bool { return os.Getenv("EXHIBIT_TEST_S3_ENDPOINT") != "" }

// S3OrSkip returns a blob.Store backed by the configured bucket, namespaced to
// a key prefix unique to this test, or skips t when no endpoint is configured.
//
// Every test gets its own prefix and its own cleanup, so tests neither see each
// other's objects nor leave any behind — and the namespacing exercises
// S3Config.Prefix as a side effect, which is the field a shared bucket depends
// on.
func S3OrSkip(t *testing.T) blob.Store {
	t.Helper()
	endpoint := os.Getenv("EXHIBIT_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("EXHIBIT_TEST_S3_ENDPOINT not set; skipping S3 blob backend test")
	}
	ctx := context.Background()

	cfg := blob.S3Config{
		Bucket:    envOr("EXHIBIT_TEST_S3_BUCKET", "exhibit-test"),
		Endpoint:  endpoint,
		Region:    os.Getenv("EXHIBIT_TEST_S3_REGION"),
		AccessKey: os.Getenv("EXHIBIT_TEST_S3_ACCESS_KEY_ID"),
		SecretKey: os.Getenv("EXHIBIT_TEST_S3_SECRET_ACCESS_KEY"),
		Prefix:    "test/" + sanitize(t.Name()),
	}

	raw := rawClient(t, cfg)
	if exists, err := raw.BucketExists(ctx, cfg.Bucket); err != nil {
		t.Fatalf("reach test bucket %q: %v", cfg.Bucket, err)
	} else if !exists {
		// `go test ./...` runs the packages that call this concurrently, so on
		// a fresh endpoint several of them can see "no bucket" and all try to
		// create it. Losing that race is the bucket existing, which is what was
		// wanted; only a different failure is worth stopping for.
		err := raw.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region})
		if err != nil && !bucketAlreadyThere(err) {
			t.Fatalf("create test bucket %q: %v", cfg.Bucket, err)
		}
	}

	s, err := blob.NewS3Store(ctx, cfg)
	if err != nil {
		t.Fatalf("open S3 blob store: %v", err)
	}
	t.Cleanup(func() { removePrefix(t, raw, cfg) })
	return s
}

// rawClient is a second, plain SDK client for the things a blob.Store
// deliberately cannot do: create the bucket, and sweep the prefix afterwards.
func rawClient(t *testing.T, cfg blob.S3Config) *minio.Client {
	t.Helper()
	host := cfg.Endpoint
	secure := true
	if rest, ok := strings.CutPrefix(host, "http://"); ok {
		host, secure = rest, false
	} else if rest, ok := strings.CutPrefix(host, "https://"); ok {
		host = rest
	}
	c, err := minio.New(strings.TrimSuffix(host, "/"), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		t.Fatalf("dial test S3 endpoint %q: %v", cfg.Endpoint, err)
	}
	return c
}

func removePrefix(t *testing.T, c *minio.Client, cfg blob.S3Config) {
	t.Helper()
	ctx := context.Background()
	objects := c.ListObjects(ctx, cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    cfg.Prefix + "/",
		Recursive: true,
	})
	for err := range c.RemoveObjects(ctx, cfg.Bucket, objects, minio.RemoveObjectsOptions{}) {
		t.Logf("test blob cleanup: %v", err.Err)
	}
}

// sanitize turns a Go test name into something usable as a key segment.
func sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func bucketAlreadyThere(err error) bool {
	switch minio.ToErrorResponse(err).Code {
	case minio.BucketAlreadyOwnedByYou, minio.BucketAlreadyExists:
		return true
	}
	return false
}
