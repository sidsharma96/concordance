// Package objectstore verifies and shares immutable serialized bytes within one
// authorized collection through an S3 client. It does not authorize callers,
// publish generations, define document identity or delete objects.
package objectstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
)

var (
	ErrInvalid        = errors.New("invalid object storage input")
	ErrNotFound       = errors.New("object not found")
	ErrIntegrity      = errors.New("object integrity check failed")
	ErrResponseLimit  = errors.New("object storage response limit exceeded")
	ErrOutcomeUnknown = errors.New("object write outcome unresolved; retry the same bytes and scope")
)

// Ref describes exact stored bytes, not a document, generation or permission.
// Its key is derived within the receiving Store's authorized scope. Zero-length
// objects are valid; applications decide which formats permit empty content.
type Ref struct {
	SHA256 [sha256.Size]byte
	Bytes  int64
}

// Store borrows a concrete S3 client. Its owner configures the trusted endpoint,
// credentials, transport and SDK middleware, and owns their lifecycle. No credential
// discovery occurs here. Store methods are concurrency-safe; callers own admission.
// Byte limits bound each response, not all concurrent calls or transport buffers.
type Store struct {
	client   *s3.Client
	bucket   string
	prefix   string
	maxBytes int64
}

// New binds one already-authorized, globally unique collection ID to one bucket.
// The caller must qualify collection identity across tenants before construction
// and authorize every later access. The namespace encoding grants no permission.
func New(client *s3.Client, bucket, collection string, maxBytes int64) (*Store, error) {
	maxInt := int64(int(^uint(0) >> 1))
	if client == nil || !opaque(bucket, 63) || !opaque(collection, 256) || maxBytes <= 0 || maxBytes >= maxInt {
		return nil, ErrInvalid
	}
	return &Store{client: client, bucket: bucket, maxBytes: maxBytes,
		prefix: "v1/collections/" + hex.EncodeToString([]byte(collection)) + "/sha256/"}, nil
}

// Key returns the versioned storage key for application catalog/manifest references.
// It does not establish object existence or authority to read it.
func (s *Store) Key(ref Ref) (string, error) {
	if ref.Bytes < 0 || ref.Bytes > s.maxBytes {
		return "", ErrInvalid
	}
	return s.prefix + hex.EncodeToString(ref.SHA256[:]), nil
}

// Ensure verifies existing content or conditionally creates and reads it back.
// The caller owns body and must not mutate it until return. A successful result
// proves a verified read at that time, not catalog publication or perpetual access.
// Each call uses at most three SDK operations with SDK retries disabled. A caller
// may retry the same bytes/scope after an unresolved outcome; it must not delete.
func (s *Store) Ensure(ctx context.Context, body []byte) (Ref, error) {
	if err := boundedContext(ctx); err != nil {
		return Ref{}, err
	}
	if int64(len(body)) > s.maxBytes {
		return Ref{}, ErrInvalid
	}
	ref := Ref{SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
	if stored, err := s.Read(ctx, ref); err == nil {
		if !bytes.Equal(stored, body) {
			return Ref{}, ErrIntegrity
		}
		return ref, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Ref{}, err
	}
	key, _ := s.Key(ref) // ref was constructed within this store's size bound.
	_, putErr := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(body),
		ContentLength: aws.Int64(ref.Bytes), ContentType: aws.String("application/octet-stream"),
		IfNoneMatch: aws.String("*"),
	}, attemptOptions(controlBodyLimit))
	// A negative read is not proof that no other write is in flight. Even a lost
	// or rejected create reply is reconciled by reading the expected immutable key.
	stored, err := s.Read(ctx, ref)
	if err != nil {
		if errors.Is(err, ErrIntegrity) {
			return Ref{}, errors.Join(err, safeError("create", putErr))
		}
		// Absence observed after a create attempt is not a no-write guarantee.
		// Preserve the provider cause without returning Read's absence sentinel.
		var failure *providerFailure
		if errors.Is(err, ErrNotFound) && errors.As(err, &failure) {
			err = failure
		}
		return Ref{}, errors.Join(ErrOutcomeUnknown, safeError("create", putErr), err)
	}
	if !bytes.Equal(stored, body) {
		return Ref{}, ErrIntegrity
	}
	return ref, nil
}

// Read returns caller-owned bytes after bounded length/hash verification. It never
// substitutes another object or trusts ETag/custom metadata as a raw SHA-256 value.
// Every successful GetObject body is closed, including malformed/error paths.
func (s *Store) Read(ctx context.Context, ref Ref) (body []byte, resultErr error) {
	if err := boundedContext(ctx); err != nil {
		return nil, err
	}
	key, err := s.Key(ref)
	if err != nil {
		return nil, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, attemptOptions(ref.Bytes+1))
	if err != nil {
		return nil, readError(err)
	}
	if output == nil || output.Body == nil {
		return nil, ErrIntegrity
	}
	defer func() {
		if err := output.Body.Close(); err != nil {
			body = nil
			resultErr = errors.Join(resultErr, safeError("close read", err))
		}
	}()
	if output.ContentLength != nil && *output.ContentLength != ref.Bytes {
		return nil, ErrIntegrity
	}
	data, err := io.ReadAll(io.LimitReader(output.Body, ref.Bytes+1))
	if err != nil {
		return nil, safeError("read body", err)
	}
	if int64(len(data)) != ref.Bytes || sha256.Sum256(data) != ref.SHA256 {
		return nil, ErrIntegrity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func attemptOptions(successBodyLimit int64) func(*s3.Options) {
	return func(options *s3.Options) {
		options.Retryer = aws.NopRetryer{}
		options.RetryMaxAttempts = 1
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		options.ClientLogMode = 0
		options.Logger = logging.Nop{}
		options.HTTPClient = limitedClient{next: options.HTTPClient, successLimit: successBodyLimit}
	}
}

func boundedContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	return nil
}

func opaque(value string, max int) bool {
	return len(value) > 0 && len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// providerFailure preserves error inspection without formatting the remote error
// message, which may contain captured data. Callers must not log unwrapped causes.
type providerFailure struct {
	operation string
	cause     error
}

func (e *providerFailure) Error() string {
	return fmt.Sprintf("object storage %s failed", e.operation)
}

func (e *providerFailure) Unwrap() error { return e.cause }

func safeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &providerFailure{operation: operation, cause: err}
}
