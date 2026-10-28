// Package catalog stores import bindings and published generation references in
// PostgreSQL. It does not authorize callers, upload objects, build indexes, select
// an active generation or implement the complete import operation.
package catalog

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid            = errors.New("invalid catalog input")
	ErrNotFound           = errors.New("catalog record not found")
	ErrConflict           = errors.New("import key is bound to different inputs")
	ErrFingerprintVersion = errors.New("input comparison requires the original fingerprint encoding")
	ErrGenerationExists   = errors.New("generation belongs to another import key")
	ErrOutcomeUnknown     = errors.New("publication outcome unknown; reuse the import key")
	ErrCorrupt            = errors.New("catalog contains malformed metadata")
)

// Request is internal trusted metadata, not a public caller's assertion of
// equality. The coordinator must authorize Collection and fingerprint the complete
// captured input using FingerprintVersion's rules. Preserve old encoders for outstanding keys.
// Values are copied; the catalog neither owns nor retains caller buffers.
type Request struct {
	Collection         string
	Key                string
	FingerprintVersion string
	Fingerprint        [sha256.Size]byte
}

// Generation references already-written immutable objects. The caller must confirm
// all required objects and their integrity before Publish; this package cannot
// establish object existence or continued availability from metadata alone.
type Generation struct {
	ID             string
	ManifestKey    string
	ManifestSHA256 [sha256.Size]byte
	ManifestBytes  int64
}

// Record has no attempt-owner or worker status. An unpublished binding may have
// work in flight. A returned Generation is owned by the caller and may be modified.
type Record struct {
	Request    Request
	Generation *Generation
}

type Publication struct {
	Generation Generation
	Duplicate  bool
}

// Store borrows a pool with a trusted, application-owned schema/search_path. Its
// caller owns configuration, authorization, admission limits and Close. Pool size
// limits connections, not the number of waiting callers. Use an authoritative
// primary with durability appropriate to the catalog contract; this client cannot
// make a local database survive host loss. Methods are concurrency-safe.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, ErrInvalid
	}
	return &Store{pool: pool}, nil
}

//go:embed schema.sql
var schema string

// CreateSchema installs the experiment's first table in an empty owned schema.
// It is explicit setup, not automatic migration: an existing table returns an
// error and is never dropped, altered or assumed compatible.
func (s *Store) CreateSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return wrap("create catalog schema", err)
}

// Bind durably fixes the key's input identity before remote writes. If its reply
// is lost, retry this same binding or Lookup. Failure does not prove no binding
// exists. A successful unpublished result does not claim this caller owns the work.
func (s *Store) Bind(ctx context.Context, request Request) (Record, error) {
	if err := validateRequest(request); err != nil {
		return Record{}, err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO concordance_imports
		(collection_id, import_key, fingerprint_version, input_fingerprint)
		VALUES ($1, $2, $3, $4) ON CONFLICT (collection_id, import_key) DO NOTHING`,
		request.Collection, request.Key, request.FingerprintVersion, request.Fingerprint[:])
	if err != nil {
		return Record{}, wrap("bind import key (outcome may be unknown)", err)
	}
	record, err := s.Lookup(ctx, request.Collection, request.Key)
	if err != nil {
		return Record{}, err
	}
	if err := matches(record.Request, request); err != nil {
		return Record{}, err
	}
	return record, nil
}

const columns = `collection_id, import_key, fingerprint_version, input_fingerprint,
	generation_id, manifest_key, manifest_sha256, manifest_bytes`

func (s *Store) Lookup(ctx context.Context, collection, key string) (Record, error) {
	if !identifier(collection, 256) || !identifier(key, 256) {
		return Record{}, ErrInvalid
	}
	return scanRecord(s.pool.QueryRow(ctx, `SELECT `+columns+`
		FROM concordance_imports WHERE collection_id=$1 AND import_key=$2`, collection, key))
}

// LookupGeneration reads a published reference from authoritative catalog state.
// Do not use a lagging replica when promising read-after-publication visibility.
func (s *Store) LookupGeneration(ctx context.Context, collection, id string) (Generation, error) {
	if !identifier(collection, 256) || !identifier(id, 256) {
		return Generation{}, ErrInvalid
	}
	record, err := scanRecord(s.pool.QueryRow(ctx, `SELECT `+columns+`
		FROM concordance_imports WHERE collection_id=$1 AND generation_id=$2`, collection, id))
	if err != nil {
		return Generation{}, err
	}
	if record.Generation == nil {
		return Generation{}, ErrCorrupt
	}
	return *record.Generation, nil
}

// Publish fixes one complete result for an existing binding. Concurrent identical
// attempts return the same winning result, even if their candidate IDs differ.
// Returning a prior result never reactivates it. The candidate must be valid even
// on this low-level duplicate path; a coordinator should Lookup before rebuilding.
// Receipt-based callers must use PublishInNamespace instead of this strict path.
// Any commit error is conservatively uncertain. Callers must reuse the key.
func (s *Store) Publish(ctx context.Context, request Request, candidate Generation) (Publication, error) {
	return s.publish(ctx, request, candidate, nil)
}

// PublishInNamespace fences new receipt-based publication against quarantine.
// The caller still confirms EVERY required object and manifest before calling.
// Existing completed imports remain recoverable even after quarantine.
func (s *Store) PublishInNamespace(ctx context.Context, request Request, candidate Generation, n Namespace) (Publication, error) {
	if !s.validNamespace(n) || n.binding.Collection != request.Collection {
		return Publication{}, ErrInvalid
	}
	return s.publish(ctx, request, candidate, &n)
}
func (s *Store) publish(ctx context.Context, request Request, candidate Generation, namespace *Namespace) (result Publication, resultErr error) {
	if err := validateRequest(request); err != nil {
		return result, err
	}
	if err := validateGeneration(candidate); err != nil {
		return result, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return result, wrap("begin publication", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = Publication{}
			resultErr = errors.Join(resultErr, wrap("release publication transaction", err))
		}
	}()
	record, err := scanRecord(tx.QueryRow(ctx, `SELECT `+columns+`
		FROM concordance_imports WHERE collection_id=$1 AND import_key=$2 FOR UPDATE`,
		request.Collection, request.Key))
	if err != nil {
		return result, err
	}
	if err := matches(record.Request, request); err != nil {
		return result, err
	}
	if record.Generation != nil {
		return Publication{Generation: *record.Generation, Duplicate: true}, nil
	}
	if namespace != nil {
		if err := s.checkNamespace(ctx, tx, *namespace, true); err != nil {
			return result, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE concordance_imports SET generation_id=$3,
		manifest_key=$4, manifest_sha256=$5, manifest_bytes=$6
		WHERE collection_id=$1 AND import_key=$2`, request.Collection, request.Key,
		candidate.ID, candidate.ManifestKey, candidate.ManifestSHA256[:], candidate.ManifestBytes)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" &&
			postgresError.ConstraintName == "concordance_imports_generation_unique" {
			return result, ErrGenerationExists
		}
		return result, wrap("write publication", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, wrap("commit publication", errors.Join(ErrOutcomeUnknown, err))
	}
	return Publication{Generation: candidate}, nil
}

func scanRecord(row pgx.Row) (Record, error) {
	var record Record
	var fingerprint, digest []byte
	var id, key *string
	var size *int64
	err := row.Scan(&record.Request.Collection, &record.Request.Key, &record.Request.FingerprintVersion,
		&fingerprint, &id, &key, &digest, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return record, ErrNotFound
	}
	if err != nil {
		return Record{}, wrap("read catalog record", err)
	}
	if len(fingerprint) != sha256.Size {
		return Record{}, ErrCorrupt
	}
	copy(record.Request.Fingerprint[:], fingerprint)
	if validateRequest(record.Request) != nil {
		return Record{}, ErrCorrupt
	}
	if id == nil {
		if key != nil || digest != nil || size != nil {
			return Record{}, ErrCorrupt
		}
		return record, nil
	}
	if key == nil || size == nil || len(digest) != sha256.Size {
		return Record{}, ErrCorrupt
	}
	generation := Generation{ID: *id, ManifestKey: *key, ManifestBytes: *size}
	copy(generation.ManifestSHA256[:], digest)
	if validateGeneration(generation) != nil {
		return Record{}, ErrCorrupt
	}
	record.Generation = &generation
	return record, nil
}

func matches(bound, request Request) error {
	if bound.FingerprintVersion != request.FingerprintVersion {
		return ErrFingerprintVersion
	}
	if bound.Fingerprint != request.Fingerprint {
		return ErrConflict
	}
	return nil
}

func validateRequest(request Request) error {
	if !identifier(request.Collection, 256) || !identifier(request.Key, 256) || !identifier(request.FingerprintVersion, 64) {
		return ErrInvalid
	}
	return nil
}

func validateGeneration(generation Generation) error {
	if !identifier(generation.ID, 256) || !identifier(generation.ManifestKey, 1024) || generation.ManifestBytes <= 0 {
		return ErrInvalid
	}
	return nil
}

func identifier(value string, limit int) bool {
	return len(value) > 0 && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
