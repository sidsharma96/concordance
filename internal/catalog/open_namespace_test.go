package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

func TestOpenNamespaceIsReadOnlyAndReturnsInactiveObservation(t *testing.T) {
	f := newFixture(t)
	if err := f.store.CreateReceiptSchema(f.ctx); err != nil {
		t.Fatal(err)
	}
	b := objectstore.Binding{Endpoint: "https://example.invalid", Bucket: "fixture", Collection: "guide", KeyVersion: "v1"}
	original, err := f.store.BindNamespace(f.ctx, "fixture", b)
	if err != nil {
		t.Fatal(err)
	}
	config := f.pool.Config()
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	readOnly, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	n, err := readOnly.OpenNamespace(f.ctx, b)
	if err != nil || n.ID() != original.ID() || n.Epoch() != original.Epoch() || n.Binding() != b {
		t.Fatal(n, err)
	}
	if err := readOnly.CheckNamespace(f.ctx, n); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []objectstore.Binding{
		{Endpoint: "https://other.invalid", Bucket: b.Bucket, Collection: b.Collection, KeyVersion: b.KeyVersion},
		{Endpoint: b.Endpoint, Bucket: "other", Collection: b.Collection, KeyVersion: b.KeyVersion},
		{Endpoint: b.Endpoint, Bucket: b.Bucket, Collection: "other", KeyVersion: b.KeyVersion},
	} {
		if got, err := readOnly.OpenNamespace(f.ctx, missing); !errors.Is(err, ErrNotFound) || got.ID() != "" {
			t.Fatal(got, err)
		}
	}
	if got, err := readOnly.OpenNamespace(f.ctx, objectstore.Binding{}); !errors.Is(err, ErrInvalid) || got.ID() != "" {
		t.Fatal(got, err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM concordance_object_namespaces").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := f.store.Quarantine(f.ctx, original); err != nil {
		t.Fatal(err)
	}
	inactive, err := readOnly.OpenNamespace(f.ctx, b)
	if err != nil || inactive.ID() != original.ID() || inactive.Epoch() != original.Epoch()+1 {
		t.Fatal(inactive, err)
	}
	if err := readOnly.CheckNamespace(f.ctx, inactive); !errors.Is(err, ErrNamespace) {
		t.Fatal(err)
	}
	if _, err := readOnly.LookupReceipts(f.ctx, inactive, nil, 1); !errors.Is(err, ErrNamespace) {
		t.Fatal(err)
	}
	writerNamespace, err := f.store.OpenNamespace(f.ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Bind(f.ctx, request()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.PublishInNamespace(f.ctx, request(), generation("new"), writerNamespace); !errors.Is(err, ErrNamespace) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if got, err := readOnly.OpenNamespace(ctx, b); !errors.Is(err, context.Canceled) || got.ID() != "" {
		t.Fatal(got, err)
	}
	t.Log("read-only open: no namespace creation; inactive observation returned; receipt and publication guards reject it")
}
