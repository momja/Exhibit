package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store keeps artifact bodies in an S3-compatible bucket (av-52ll).
//
// It is the second implementation of Store, and it is what the interface was
// drawn for (architecture §3.3): nothing above Store knows which of the two is
// behind it, and if a caller ever needs to, the backend is wrong. The target is
// S3-*compatible* rather than AWS — MinIO is the reference — which is why the
// endpoint is configuration and no bucket layout, lifecycle rule, or
// vendor-specific feature appears here.
//
// Why a bucket at all: on one machine's disk the bodies pin the service to that
// machine and leave backup half-solved, since the Litestream profile streams
// the SQLite WAL and nothing else. A restore that recovers every row and none
// of the bytes those rows point at is not a recovered library.
type S3Store struct {
	client *minio.Client
	bucket string
	prefix string
}

// S3Config is the whole configuration surface. Bucket is the only required
// field: it is also the selector, since an unset bucket means this backend does
// not exist and Open keeps FSStore (see Open).
type S3Config struct {
	Bucket string
	// Endpoint is the S3 API host, optionally carrying a scheme:
	// "minio.example.com", "https://minio.example.com", or
	// "http://localhost:9000". Empty means AWS S3 itself. Without a scheme TLS
	// is assumed, so the insecure case has to be asked for by name.
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	// Prefix optionally namespaces every key, so a bucket can be shared with
	// something else — the Litestream backups, most obviously.
	Prefix string
}

// partSize is the multipart part size, pinned to the smallest S3 accepts. It is
// the bound on what any upload may allocate; see Put.
const partSize = 5 * 1024 * 1024

// reachTimeout bounds the startup reachability check. It exists because the
// ambient credential chain's last resort is the instance-metadata endpoint,
// which on a machine that has no such thing does not refuse so much as go
// quiet — and a service that hangs on boot is a worse way to learn the keys
// were forgotten than an error is.
const reachTimeout = 15 * time.Second

// NewS3Store dials the endpoint and confirms the bucket is reachable.
//
// The confirmation is deliberate: a misconfigured bucket that first surfaces on
// an ingest costs the user the artifact they were uploading, where the same
// mistake at startup costs a restart. The check is a HEAD on the bucket, so a
// credential scoped tightly enough to deny that will fail here — with an error
// naming the bucket and endpoint — rather than mysteriously later.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("blob: S3 bucket is required")
	}
	endpoint, secure, err := parseS3Endpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  s3Credentials(cfg),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("blob: dial %s: %w", endpoint, err)
	}

	reachCtx, cancel := context.WithTimeout(ctx, reachTimeout)
	defer cancel()
	exists, err := client.BucketExists(reachCtx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("blob: reach bucket %q at %s: %w", cfg.Bucket, endpoint, err)
	}
	if !exists {
		return nil, fmt.Errorf("blob: bucket %q does not exist at %s", cfg.Bucket, endpoint)
	}

	return &S3Store{client: client, bucket: cfg.Bucket, prefix: normalizeS3Prefix(cfg.Prefix)}, nil
}

// s3Credentials prefers the explicitly configured key pair and otherwise falls
// back to the SDK's ambient chain — the AWS/MinIO environment variables, the
// shared credentials file, then the instance role. Configuring nothing is
// therefore how a deployment that already has a role attached says so, and the
// static pair is how everyone else does.
func s3Credentials(cfg S3Config) *credentials.Credentials {
	if cfg.AccessKey != "" {
		return credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}
	return credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.EnvMinio{},
		&credentials.FileAWSCredentials{},
		&credentials.IAM{},
	})
}

// parseS3Endpoint splits an operator-written endpoint into the host:port the
// SDK wants and whether to speak TLS. An empty endpoint is AWS S3.
func parseS3Endpoint(raw string) (host string, secure bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "s3.amazonaws.com", true, nil
	}
	if !strings.Contains(raw, "://") {
		// No scheme written, so none is assumed away: TLS is the default and
		// plain HTTP has to be spelled out.
		return strings.TrimSuffix(raw, "/"), true, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("blob: parse S3 endpoint %q: %w", raw, err)
	}
	switch u.Scheme {
	case "https":
		secure = true
	case "http":
		secure = false
	default:
		return "", false, fmt.Errorf("blob: S3 endpoint %q has scheme %q; want http or https", raw, u.Scheme)
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("blob: S3 endpoint %q names no host", raw)
	}
	// A path would be silently dropped — the SDK addresses buckets from the
	// host — and an endpoint written with one is an operator expecting requests
	// to go somewhere they will not. Say so, rather than serve a whole
	// deployment out of the wrong place. (The schemeless branch above refuses
	// the same input via minio.New, so both spellings disagree with the
	// operator rather than with each other.)
	if p := strings.Trim(u.Path, "/"); p != "" {
		return "", false, fmt.Errorf("blob: S3 endpoint %q has a path (%q); give the host only", raw, u.Path)
	}
	return u.Host, secure, nil
}

func normalizeS3Prefix(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

// key is the object name a blob id maps to. Ids are validated to be flat names
// (validateBlobID), so the prefix is the only structure in the key and an id
// can never reach outside it.
func (s *S3Store) key(id string) string { return s.prefix + id }

// Put streams the blob to the bucket.
//
// Nothing here holds the whole body, whichever of the SDK's three upload paths
// the arguments select. A body that fits in one part goes up as a single PUT
// with a real Content-Length, the reader drained straight onto the wire. A
// larger one of known length is read part-by-part at offsets, which allocates
// nothing. Only an unknown length has to be *discovered*, by filling a buffer —
// and pinning the part size to the protocol minimum is what bounds that at
// 5 MiB rather than the SDK's much larger default. A snapshot that vendored a
// wasm payload is tens of megabytes, and on a shared host it must not become an
// allocation the size of itself once per request.
//
// The part size is set unconditionally rather than only on the path that
// allocates, because it is also what keeps that path *reachable in a test*: it
// is where the size a backend infers has to be right, and a threshold of 16 MiB
// would make the case cost 16 MiB to cover. The price is that a body between
// 5 and 16 MiB is a multipart upload rather than one request, which costs two
// extra round trips and no memory.
func (s *S3Store) Put(ctx context.Context, id string, r io.Reader) error {
	if err := validateBlobID(id); err != nil {
		return err
	}
	info, err := s.client.PutObject(ctx, s.bucket, s.key(id), r, knownSize(r),
		minio.PutObjectOptions{PartSize: partSize})
	if err != nil {
		return fmt.Errorf("blob: put %s: %w", id, err)
	}
	slog.DebugContext(ctx, "blob stored", slog.String("id", id), slog.Int64("bytes", info.Size))
	return nil
}

// knownSize reports how many bytes r will yield, or -1 when it cannot be said
// safely. Every caller above this interface hands Put a reader over a body it
// already holds, so the answer is normally known.
//
// Two things make this narrower than it looks, and both are correctness rather
// than caution:
//
// The types are named concretely rather than matched by an anonymous
// `interface{ Len() int }`. Len happens to mean "bytes remaining" on these
// three and means something else elsewhere, and a wrong Content-Length is a
// corrupted upload where an unknown one is merely a slower path.
//
// And a *bytes.Reader or *strings.Reader that has already been read from is
// reported as unknown even though it can state its remainder exactly. Both are
// io.ReaderAt, and the SDK's fast path for a large known-size ReaderAt reads
// parts at absolute offsets from zero — ignoring the read position entirely.
// Handed a consumed reader it would upload the object's *first* n bytes while
// claiming they were the last n: silently wrong content, at the right length.
// Declining to name a size puts such a reader on the sequential path, which
// starts where the reader actually is.
func knownSize(r io.Reader) int64 {
	switch v := r.(type) {
	case *bytes.Reader:
		return unreadSize(int64(v.Len()), v.Size())
	case *strings.Reader:
		return unreadSize(int64(v.Len()), v.Size())
	case *bytes.Buffer:
		// Not an io.ReaderAt, so the offset-addressed path is unreachable and
		// a partly-drained buffer is still safe to size: Len is what is left.
		return int64(v.Len())
	}
	return -1
}

func unreadSize(remaining, total int64) int64 {
	if remaining != total {
		return -1 // already read from; see knownSize
	}
	return total
}

// Get opens the object for streaming.
//
// The SDK's handle is lazy: GetObject has issued no request when it returns, so
// a blob that is not there would first be noticed by a Read — after the render
// surface had already begun writing a 200. FSStore fails at Get, callers are
// written for that (the render surface turns it into a 404), so this one must
// too, which means forcing the request before returning.
//
// It forces it by reading a byte rather than by calling the handle's Stat.
// Stat is the obvious way and the wrong one: it issues a *separate* HEAD and
// keeps no response, so every blob read would cost two round trips — a gallery
// of forty widget tiles paying forty extra ones — and it would leave a window
// where an object deleted between the HEAD and the GET still failed mid-200.
// A one-byte read is the same GET the caller was going to make; the byte is
// simply put back in front of it.
func (s *S3Store) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := validateBlobID(id); err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(id), minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("blob: get %s: %w", id, err)
	}
	var first [1]byte
	// EOF is not a failure: it is what an empty blob reads, and an artifact
	// with an empty body is a state the API permits.
	n, err := obj.Read(first[:])
	if err != nil && !errors.Is(err, io.EOF) {
		_ = obj.Close()
		return nil, fmt.Errorf("blob: get %s: %w", id, err)
	}
	slog.DebugContext(ctx, "blob opened", slog.String("id", id))
	return readCloser{Reader: io.MultiReader(bytes.NewReader(first[:n]), obj), Closer: obj}, nil
}

// readCloser rejoins the byte Get read to force the request with the rest of
// the object, while Close still reaches the SDK handle that owns the response.
type readCloser struct {
	io.Reader
	io.Closer
}

// Delete removes the object, honouring Store.Delete's idempotent contract.
//
// There is no existence check in front of this call and there must not be: S3's
// DeleteObject already answers success for a key that was never there, which is
// the whole reason the interface defines a missing id as success. Adding a HEAD
// would pay a round trip on every delete to manufacture a failure no caller
// wants.
func (s *S3Store) Delete(ctx context.Context, id string) error {
	if err := validateBlobID(id); err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, s.key(id), minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("blob: delete %s: %w", id, err)
	}
	slog.DebugContext(ctx, "blob deleted", slog.String("id", id))
	return nil
}
