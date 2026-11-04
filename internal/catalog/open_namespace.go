package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

// OpenNamespace observes an existing exact physical binding without creating or
// reactivating anything. The caller authorizes the collection and trusted routing.
// Inactive namespaces are intentionally returned: a completed saved-key retry can
// recover its catalog result before object confirmation or guarded publication.
// This handle is NOT proof of current trust. Receipt operations and publication
// still check active state and the captured epoch, including concurrent quarantine.
// Missing bindings return ErrNotFound; errors return a zero handle.
func (s *Store) OpenNamespace(ctx context.Context, b objectstore.Binding) (Namespace, error) {
	if !b.Valid() {
		return Namespace{}, ErrInvalid
	}
	n := Namespace{pool: s.pool, binding: b}
	err := s.pool.QueryRow(ctx, `SELECT namespace_id,epoch FROM concordance_object_namespaces
 WHERE endpoint=$1 AND bucket=$2 AND collection_id=$3 AND key_version=$4`, b.Endpoint, b.Bucket, b.Collection, b.KeyVersion).Scan(&n.id, &n.epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return Namespace{}, ErrNotFound
	}
	if err != nil {
		return Namespace{}, wrap("open storage namespace", err)
	}
	if !s.validNamespace(n) {
		return Namespace{}, ErrCorrupt
	}
	if err := ctx.Err(); err != nil {
		return Namespace{}, err
	}
	return n, nil
}
