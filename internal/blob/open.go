package blob

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// s3EnvVars are every variable this package reads, bucket first. The list is
// what lets Open tell "no bucket wanted" apart from "a bucket was wanted and
// the name is missing"; see below.
var s3EnvVars = []string{
	"BLOB_S3_BUCKET",
	"BLOB_S3_ENDPOINT",
	"BLOB_S3_REGION",
	"BLOB_S3_ACCESS_KEY_ID",
	"BLOB_S3_SECRET_ACCESS_KEY",
	"BLOB_S3_PREFIX",
}

// Open selects the blob backend from the environment (av-52ll).
//
// Absent means filesystem. With none of BLOB_S3_* set there is no bucket, no
// credential to supply and no behavioural difference from before this existed —
// a self-hoster gets no new required configuration, which is the shape every
// optional feature in this service takes (OIDC_ISSUER, PUBLIC_MODE_ENABLED).
//
// But absent has to mean *entirely* absent. BLOB_S3_BUCKET alone is the
// selector, and reading a missing bucket as "filesystem, then" would let an
// operator who set the endpoint and both keys and then typo'd the bucket name
// boot happily onto local disk — on exactly the deployment this exists for, a
// container whose disk does not survive a restart, while they believe their
// artifacts are in the bucket. So any other BLOB_S3_* without a bucket is an
// error rather than a fallback: fail closed on ambiguity, never on absence.
//
// The reading lives here rather than in main so that selecting a backend is one
// call at the wiring site and every question about how a bucket is addressed
// stays inside the package that answers it.
func Open(ctx context.Context, fsDir string) (Store, error) {
	bucket := os.Getenv("BLOB_S3_BUCKET")
	if bucket == "" {
		if set := s3EnvSet(); len(set) > 0 {
			return nil, fmt.Errorf(
				"blob: %s set without BLOB_S3_BUCKET; name the bucket, or unset %s to store artifact bodies on disk",
				strings.Join(set, ", "), strings.Join(set, "/"))
		}
		slog.Info("blob store", slog.String("backend", "filesystem"), slog.String("dir", fsDir))
		return NewFSStore(fsDir)
	}
	cfg := S3Config{
		Bucket:    bucket,
		Endpoint:  os.Getenv("BLOB_S3_ENDPOINT"),
		Region:    os.Getenv("BLOB_S3_REGION"),
		AccessKey: os.Getenv("BLOB_S3_ACCESS_KEY_ID"),
		SecretKey: os.Getenv("BLOB_S3_SECRET_ACCESS_KEY"),
		Prefix:    os.Getenv("BLOB_S3_PREFIX"),
	}
	s, err := NewS3Store(ctx, cfg)
	if err != nil {
		return nil, err
	}
	slog.Info("blob store",
		slog.String("backend", "s3"),
		slog.String("bucket", cfg.Bucket),
		slog.String("endpoint", cfg.Endpoint),
		slog.String("prefix", cfg.Prefix),
	)
	return s, nil
}

// s3EnvSet names which of the bucket's companion variables carry a value.
func s3EnvSet() []string {
	var set []string
	for _, name := range s3EnvVars[1:] {
		if os.Getenv(name) != "" {
			set = append(set, name)
		}
	}
	return set
}
