package catalog

import (
	"context"
	_ "embed"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrTargetConflict       = errors.New("search target no longer matches the expected revision")
	ErrTargetOutcomeUnknown = errors.New("search target update outcome unknown; retain the update or resolve the target")
)

// TargetValue is one complete selection. DefaultEdition is an exact opaque label,
// not a latest/version-sorting rule. All strings are copied values, not credentials.
type TargetValue struct{ Collection, Generation, DefaultEdition string }

// Target is an owned catalog-schema-local named selection. Revision identifies
// one change, not publication identity or authorization. No deletion/reset API is
// supplied; supported updates never reuse an old revision.
type Target struct {
	Name     string
	Value    TargetValue
	Revision int64
}

// TargetUpdate describes a conditional state change. Zero expects absence and
// creates revision one; otherwise success advances ExpectedRevision by one.
// Keep these exact values for retry. This is not a durable operation/history ID.
type TargetUpdate struct {
	Name             string
	Value            TargetValue
	ExpectedRevision int64
	// Nonempty restricts the replaced row's collection atomically. Empty keeps
	// the trusted unscoped API. The caller still authorizes the desired value.
	ExpectedCollection string
}

// Validate admits bounded metadata before an application performs remote preflight.
// It cannot establish publication, edition existence or caller authorization.
func (u TargetUpdate) Validate() error {
	if !identifier(u.Name, 256) || !validTargetValue(u.Value) || u.ExpectedRevision < 0 || u.ExpectedRevision == math.MaxInt64 {
		return ErrInvalid
	}
	if u.ExpectedCollection != "" && !identifier(u.ExpectedCollection, 256) {
		return ErrInvalid
	}
	return nil
}

//go:embed targets.sql
var targetSchema string

// CreateTargetSchema explicitly installs targets after CreateSchema. This is
// additive setup, not an automatic migration or a check of an existing table.
func (s *Store) CreateTargetSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, targetSchema)
	return wrap("create search target schema", err)
}

const targetColumns = `name,collection_id,generation_id,default_edition,revision`

// ResolveTarget copies one exact tuple and revision from the authoritative catalog.
// The caller authorizes its use and validates any chosen edition through the
// published reader before search. A missing target is not database unavailability.
func (s *Store) ResolveTarget(ctx context.Context, name string) (Target, error) {
	if err := ctx.Err(); err != nil {
		return Target{}, err
	}
	if !identifier(name, 256) {
		return Target{}, ErrInvalid
	}
	return scanTarget(s.pool.QueryRow(ctx, `SELECT `+targetColumns+` FROM concordance_search_targets WHERE name=$1`, name))
}

// SetTarget conditionally changes trusted metadata. The caller must first check
// the exact published generation/default edition against immutable manifest/index
// bytes, as searchtarget.Activate does. The foreign key establishes publication,
// not edition existence or object availability. This is not an end-user API.
//
// An exact-next-revision replay with the same tuple returns that observed state,
// including when an identical competing caller wrote it. It does not identify the
// writer. Later revisions conflict, even if their tuple happens to be equal.
// An in-flight write error can conceal success; errors return no partial target.
func (s *Store) SetTarget(ctx context.Context, update TargetUpdate) (Target, error) {
	if err := ctx.Err(); err != nil {
		return Target{}, err
	}
	if err := update.Validate(); err != nil {
		return Target{}, err
	}
	v := update.Value
	var row pgx.Row
	if update.ExpectedRevision == 0 {
		row = s.pool.QueryRow(ctx, `INSERT INTO concordance_search_targets
 (name,collection_id,generation_id,default_edition,revision) VALUES ($1,$2,$3,$4,1)
 ON CONFLICT (name) DO NOTHING RETURNING `+targetColumns, update.Name, v.Collection, v.Generation, v.DefaultEdition)
	} else {
		row = s.pool.QueryRow(ctx, `UPDATE concordance_search_targets
 SET collection_id=$2,generation_id=$3,default_edition=$4,revision=revision+1
 WHERE name=$1 AND revision=$5 AND ($6='' OR collection_id=$6) RETURNING `+targetColumns,
			update.Name, v.Collection, v.Generation, v.DefaultEdition, update.ExpectedRevision, update.ExpectedCollection)
	}
	result, err := scanTarget(row)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, ErrNotFound) {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23503" && postgresError.ConstraintName == "concordance_search_targets_generation" {
			return Target{}, wrap("target generation is not published", ErrNotFound)
		}
		return Target{}, wrap("set search target", errors.Join(ErrTargetOutcomeUnknown, err))
	}

	// The conditional statement completed without changing a row. Inspect fresh
	// state to distinguish a narrowly idempotent retry from a stale operator.
	current, err := s.ResolveTarget(ctx, update.Name)
	if errors.Is(err, ErrNotFound) {
		return Target{}, ErrTargetConflict
	}
	if err != nil {
		return Target{}, err
	}
	if current.Revision != update.ExpectedRevision+1 || current.Value != update.Value {
		return Target{}, ErrTargetConflict
	}
	return current, nil
}

func scanTarget(row pgx.Row) (Target, error) {
	var t Target
	err := row.Scan(&t.Name, &t.Value.Collection, &t.Value.Generation, &t.Value.DefaultEdition, &t.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Target{}, ErrNotFound
	}
	if err != nil {
		return Target{}, wrap("read search target", err)
	}
	if !identifier(t.Name, 256) || !validTargetValue(t.Value) || t.Revision <= 0 {
		return Target{}, ErrCorrupt
	}
	return t, nil
}

func validTargetValue(v TargetValue) bool {
	return identifier(v.Collection, 256) && identifier(v.Generation, 256) && identifier(v.DefaultEdition, 256)
}
