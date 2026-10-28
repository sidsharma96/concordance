package objectstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	ErrChanged     = errors.New("known object metadata no longer matches")
	ErrNoValidator = errors.New("verified object has no compatible reuse validator")
)

// Binding describes trusted, stable client routing. Custom middleware and provider
// configuration remain caller-owned; this is not a sandbox or bucket incarnation ID.
type Binding struct{ Endpoint, Bucket, Collection, KeyVersion string }

func (b Binding) Valid() bool {
	u, e := url.Parse(b.Endpoint)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && opaque(b.Endpoint, 2048) && opaque(b.Bucket, 63) && opaque(b.Collection, 256) && b.KeyVersion == "v1"
}
func (s *Store) Binding() Binding { return s.binding }

// Verified can only be produced by successful full verification, including close.
// It retains its Store, but no object bytes. It is evidence, not authorization.
type Verified struct {
	store *Store
	ref   Ref
	tag   string
}
type Evidence struct {
	Binding Binding
	Ref     Ref
	ETag    string
}

func (v Verified) Evidence() (Evidence, error) {
	if v.store == nil {
		return Evidence{}, ErrInvalid
	}
	if !v.store.binding.Valid() || !ValidETag(v.tag) {
		return Evidence{}, ErrNoValidator
	}
	return Evidence{v.store.binding, v.ref, v.tag}, nil
}
func (v Verified) Reference() (Ref, error) {
	if v.store == nil {
		return Ref{}, ErrInvalid
	}
	return v.ref, nil
}

// ValidETag admits bounded strong ASCII entity tags; tags are not SHA-256.
func ValidETag(tag string) bool {
	if len(tag) < 2 || len(tag) > 1024 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for _, b := range []byte(tag[1 : len(tag)-1]) {
		if b < 0x21 || b > 0x7e || b == '"' {
			return false
		}
	}
	return true
}

// Reference checks the byte ceiling before hashing. Keep body stable until return.
func (s *Store) Reference(body []byte) (Ref, error) {
	if int64(len(body)) > s.maxBytes {
		return Ref{}, ErrInvalid
	}
	return referenceBytes(body), nil
}

// Check confirms metadata under a prior-verification trust contract, not current
// body integrity. Errors never authorize creation. It performs one HEAD, no PUT.
func (s *Store) Check(ctx context.Context, ref Ref, tag string) error {
	if err := boundedContext(ctx); err != nil {
		return err
	}
	key, e := s.Key(ref)
	if e != nil || !ValidETag(tag) {
		return ErrInvalid
	}
	out, e := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), IfMatch: aws.String(tag)}, attemptOptions(controlBodyLimit))
	if e != nil {
		var status interface{ HTTPStatusCode() int }
		if errors.As(e, &status) && (status.HTTPStatusCode() == 404 || status.HTTPStatusCode() == 412) {
			return errors.Join(ErrChanged, safeError("check", e))
		}
		return safeError("check", e)
	}
	if out == nil || out.ContentLength == nil || *out.ContentLength != ref.Bytes || out.ETag == nil || *out.ETag != tag {
		return ErrChanged
	}
	return ctx.Err()
}

func referenceBytes(body []byte) Ref {
	return Ref{SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
}
