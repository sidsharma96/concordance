package catalog

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

var (
	ErrNamespace             = errors.New("storage namespace is inactive, stale or incompatible")
	ErrReceiptConflict       = errors.New("stored verification evidence conflicts")
	ErrReceiptOutcomeUnknown = errors.New("receipt outcome unknown; look up the same namespace and object evidence")
)

const receiptPolicy = "sha256-get-etag/v1"

//go:embed receipts.sql
var receiptSchema string

// Namespace binds a pool, physical target and verification epoch. It grants no
// authorization. A caller must authorize its collection on every operation.
type Namespace struct {
	pool    *pgxpool.Pool
	id      string
	binding objectstore.Binding
	epoch   int64
}

func (n Namespace) Binding() objectstore.Binding { return n.binding }
func (n Namespace) ID() string                   { return n.id }
func (n Namespace) Epoch() int64                 { return n.epoch }
func (s *Store) validNamespace(n Namespace) bool {
	return n.pool == s.pool && identifier(n.id, 256) && n.epoch > 0 && n.binding.Valid()
}
func (s *Store) CreateReceiptSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, receiptSchema)
	return wrap("create receipt schema", err)
}

// BindNamespace declares an already-managed immutable namespace. Existing targets
// cannot be retargeted, aliased or automatically reactivated. Setup is explicit.
func (s *Store) BindNamespace(ctx context.Context, id string, b objectstore.Binding) (Namespace, error) {
	if !identifier(id, 256) || !b.Valid() {
		return Namespace{}, ErrInvalid
	}
	_, e := s.pool.Exec(ctx, `INSERT INTO concordance_object_namespaces VALUES($1,$2,$3,$4,$5,1,true) ON CONFLICT(namespace_id) DO NOTHING`, id, b.Endpoint, b.Bucket, b.Collection, b.KeyVersion)
	if e != nil {
		return Namespace{}, wrap("bind storage namespace", e)
	}
	n := Namespace{pool: s.pool, id: id, binding: b}
	var actual objectstore.Binding
	var active bool
	e = s.pool.QueryRow(ctx, `SELECT endpoint,bucket,collection_id,key_version,epoch,active FROM concordance_object_namespaces WHERE namespace_id=$1`, id).Scan(&actual.Endpoint, &actual.Bucket, &actual.Collection, &actual.KeyVersion, &n.epoch, &active)
	if e != nil {
		return Namespace{}, wrap("read storage namespace", e)
	}
	if actual != b {
		return Namespace{}, ErrReceiptConflict
	}
	if !active || n.epoch <= 0 {
		return Namespace{}, ErrNamespace
	}
	return n, nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) checkNamespace(ctx context.Context, q rowQuerier, n Namespace, lock bool) error {
	if !s.validNamespace(n) {
		return ErrInvalid
	}
	sql := `SELECT endpoint,bucket,collection_id,key_version,epoch,active FROM concordance_object_namespaces WHERE namespace_id=$1`
	if lock {
		sql += " FOR SHARE"
	}
	var b objectstore.Binding
	var epoch int64
	var active bool
	e := q.QueryRow(ctx, sql, n.id).Scan(&b.Endpoint, &b.Bucket, &b.Collection, &b.KeyVersion, &epoch, &active)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNamespace
	}
	if e != nil {
		return wrap("check storage namespace", e)
	}
	if !active || epoch != n.epoch || b != n.binding {
		return ErrNamespace
	}
	return ctx.Err()
}
func (s *Store) CheckNamespace(ctx context.Context, n Namespace) error {
	return s.checkNamespace(ctx, s.pool, n, false)
}

// Quarantine disables future reuse/publication. It cannot fence remote writes or
// retract published generations. An error does not prove this update rolled back.
func (s *Store) Quarantine(ctx context.Context, n Namespace) error {
	if !s.validNamespace(n) {
		return ErrInvalid
	}
	command, e := s.pool.Exec(ctx, `UPDATE concordance_object_namespaces SET active=false,
 epoch=CASE WHEN epoch<9223372036854775807 THEN epoch+1 ELSE epoch END
 WHERE namespace_id=$1 AND endpoint=$2 AND bucket=$3 AND collection_id=$4 AND key_version=$5 AND active AND epoch=$6`, n.id, n.binding.Endpoint, n.binding.Bucket, n.binding.Collection, n.binding.KeyVersion, n.epoch)
	if e != nil {
		return wrap("quarantine namespace (outcome may be unknown)", e)
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	var active bool
	var b objectstore.Binding
	e = s.pool.QueryRow(ctx, `SELECT active,endpoint,bucket,collection_id,key_version FROM concordance_object_namespaces WHERE namespace_id=$1`, n.id).Scan(&active, &b.Endpoint, &b.Bucket, &b.Collection, &b.KeyVersion)
	if e != nil {
		return wrap("confirm namespace quarantine", e)
	}
	if active || b != n.binding {
		return ErrNamespace
	}
	return nil
}

// Receipt is an owned validated row, not a capability or current body checksum.
// Zero values represent a missing receipt; only the decoder makes positive ones.
type Receipt struct {
	found bool
	ref   objectstore.Ref
	tag   string
}

func (r Receipt) Found() bool  { return r.found }
func (r Receipt) ETag() string { return r.tag }

// LookupReceipts returns one owned position per input, including duplicates. A
// bounded batch uses one query; all errors return nil, never partial evidence.
// Keep refs and its aliases unchanged until return; no input is retained.
func (s *Store) LookupReceipts(ctx context.Context, n Namespace, refs []objectstore.Ref, maxBatch int) ([]Receipt, error) {
	if !s.validNamespace(n) || maxBatch <= 0 || len(refs) > maxBatch {
		return nil, ErrInvalid
	}
	for _, r := range refs {
		if r.Bytes < 0 {
			return nil, ErrInvalid
		}
	}
	if len(refs) == 0 {
		if e := s.CheckNamespace(ctx, n); e != nil {
			return nil, e
		}
		return []Receipt{}, nil
	}
	hashes := make([][]byte, len(refs))
	for i := range refs {
		hashes[i] = refs[i].SHA256[:]
	}
	rows, e := s.pool.Query(ctx, `SELECT n.endpoint,n.bucket,n.collection_id,n.key_version,n.epoch,n.active,
 i.pos,r.bytes,r.etag,r.policy FROM concordance_object_namespaces n
 CROSS JOIN unnest($2::bytea[]) WITH ORDINALITY i(hash,pos)
 LEFT JOIN concordance_object_receipts r ON r.namespace_id=n.namespace_id AND r.epoch=n.epoch AND r.sha256=i.hash
 WHERE n.namespace_id=$1 ORDER BY i.pos`, n.id, hashes)
	if e != nil {
		return nil, wrap("lookup verification receipts", e)
	}
	defer rows.Close()
	out := make([]Receipt, 0, len(refs))
	for rows.Next() {
		var b objectstore.Binding
		var epoch, pos int64
		var active bool
		var size *int64
		var tag, policy *string
		if e = rows.Scan(&b.Endpoint, &b.Bucket, &b.Collection, &b.KeyVersion, &epoch, &active, &pos, &size, &tag, &policy); e != nil {
			return nil, wrap("decode receipt", e)
		}
		if !active || epoch != n.epoch || b != n.binding {
			return nil, ErrNamespace
		}
		if pos != int64(len(out)+1) || len(out) >= len(refs) {
			return nil, ErrCorrupt
		}
		r := Receipt{ref: refs[len(out)]}
		if size != nil {
			if *size != r.ref.Bytes || tag == nil || policy == nil || *policy != receiptPolicy || !objectstore.ValidETag(*tag) {
				return nil, ErrReceiptConflict
			}
			r.found = true
			r.tag = *tag
		} else if tag != nil || policy != nil {
			return nil, ErrCorrupt
		}
		out = append(out, r)
	}
	if e = rows.Err(); e != nil {
		return nil, wrap("read verification receipts", e)
	}
	if len(out) != len(refs) {
		return nil, ErrNamespace
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}

// RecordVerified accepts only adapter-produced full-verification evidence. It
// commits against the original namespace epoch, never retagging old work. Remote
// reads have already completed; no object I/O occurs inside this transaction.
func (s *Store) RecordVerified(ctx context.Context, n Namespace, proofs []objectstore.Verified, maxBatch int) (resultErr error) {
	if !s.validNamespace(n) || maxBatch <= 0 || len(proofs) > maxBatch {
		return ErrInvalid
	}
	unique := make(map[[32]byte]objectstore.Evidence, len(proofs))
	for _, p := range proofs {
		e, err := p.Evidence()
		if err != nil {
			return err
		}
		if e.Binding != n.binding {
			return ErrNamespace
		}
		if old, ok := unique[e.Ref.SHA256]; ok && old != e {
			return ErrReceiptConflict
		}
		unique[e.Ref.SHA256] = e
	}
	values := make([]objectstore.Evidence, 0, len(unique))
	for _, e := range unique {
		values = append(values, e)
	}
	sort.Slice(values, func(i, j int) bool { return bytes.Compare(values[i].Ref.SHA256[:], values[j].Ref.SHA256[:]) < 0 })
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return wrap("begin receipt transaction", e)
	}
	defer func() {
		if e := tx.Rollback(ctx); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			resultErr = errors.Join(resultErr, wrap("release receipt transaction", e))
		}
	}()
	if e = s.checkNamespace(ctx, tx, n, true); e != nil {
		return e
	}
	hashes := make([][]byte, len(values))
	sizes := make([]int64, len(values))
	tags := make([]string, len(values))
	for i := range values {
		hashes[i] = values[i].Ref.SHA256[:]
		sizes[i] = values[i].Ref.Bytes
		tags[i] = values[i].ETag
	}
	if len(values) > 0 {
		_, e = tx.Exec(ctx, `INSERT INTO concordance_object_receipts(namespace_id,epoch,sha256,bytes,etag,policy)
 SELECT $1,$2,i.hash,i.bytes,i.etag,$6 FROM unnest($3::bytea[],$4::bigint[],$5::text[]) i(hash,bytes,etag)
 ORDER BY i.hash ON CONFLICT(namespace_id,epoch,sha256) DO NOTHING`, n.id, n.epoch, hashes, sizes, tags, receiptPolicy)
		if e != nil {
			return wrap("record verified objects", e)
		}
		rows, e := tx.Query(ctx, `SELECT r.bytes,r.etag,r.policy,i.pos FROM unnest($3::bytea[]) WITH ORDINALITY i(hash,pos)
 JOIN concordance_object_receipts r ON r.namespace_id=$1 AND r.epoch=$2 AND r.sha256=i.hash ORDER BY i.pos`, n.id, n.epoch, hashes)
		if e != nil {
			return wrap("compare receipt evidence", e)
		}
		count := 0
		for rows.Next() {
			var size, pos int64
			var tag, policy string
			if e = rows.Scan(&size, &tag, &policy, &pos); e != nil {
				rows.Close()
				return wrap("decode saved receipt", e)
			}
			if count >= len(values) || pos != int64(count+1) || size != sizes[count] || tag != tags[count] || policy != receiptPolicy {
				rows.Close()
				return ErrReceiptConflict
			}
			count++
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return wrap("read saved receipts", e)
		}
		if count != len(values) {
			return ErrCorrupt
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return wrap("commit receipts", errors.Join(ErrReceiptOutcomeUnknown, e))
	}
	return nil
}
