# First import-catalog experiment

This is the first implemented slice under the accepted retry and publication
decisions. It implements PostgreSQL catalog operations in `internal/catalog`, not the complete importer,
R2 adapter, search engine or public API. External object storage remains required
before calling the first storage milestone complete.

The concrete Go catalog uses the pinned pgx/v5 PostgreSQL client and pool instead of
a custom SQL driver or generic repository framework. Tests use an isolated schema
in an explicitly selected local test database. Ordinary source tests can run without
a database; CI is configured to supply PostgreSQL18 and run the integration cases.
The local experiment uses PostgreSQL14.17; the configured remote CI has not run.

## Operations and representation

One table maps `(collection, import key)` to a versioned input fingerprint and an
optional completed generation reference. A completed reference contains a generation
ID, immutable R2 manifest key, SHA-256 digest and positive byte length. A unique
constraint on `(collection, generation ID)` prevents giving one generation to two
different import keys. Opaque identifiers are bounded, valid UTF-8 without NUL;
they are SQL parameters, not interpolated paths or statements.

- Bind inserts a key/input fingerprint if absent and returns its current record.
  An existing different fingerprint is a conflict, even before publication. A different
  encoding version is explicitly incomparable, not proof the inputs changed. Preserve
  the original encoding/encoder for retries before changing it in a later deployment.
- Lookup returns an existing binding/result. Absence and database failure are distinct.
- Publish requires a binding. It locks that row in a short transaction, checks the
  inputs, returns an already-published original result, or sets the complete reference
  together in one update. Only confirmed commit returns the new result. Commit errors
  are conservatively uncertain and instruct the caller to reuse the key.
- Generation lookup resolves an already-published reference by generation ID inside
  the collection. No collection activation/latest-pointer behavior is introduced.

The catalog accepts an internal trusted fingerprint value and encoding version; it
does not yet construct the complete input description. That belongs to the source
capture and processing contract. Do not expose a public digest-only equality check.
Authorization and collection selection occur in the caller before catalog access;
this package is not a tenant-authentication layer.

There is no attempt owner, background job or lease in this slice. Multiple identical
attempts may stage different candidate files, but row locking and uniqueness allow
only one published result. A later retry of an unpublished binding can try again;
it cannot overwrite the binding. Concurrency/resource limits belong to the importing
coordinator. The catalog methods honor caller cancellation and the pool bounds active
connections, but that does not by itself bound waiting application callers.

## Validation and remaining experiments

Use a real disposable PostgreSQL cluster without TCP listeners, with synthetic data:
bind/rebind, changed-fingerprint/version conflict, new key for identical inputs,
lookup after a new pool is opened, duplicate publication after a discarded caller
reply, many concurrent publications of one key, generation-ID collision, cancellation
before work, and explicit not-found results. Test invalid values before SQL access.
Do not claim that discarding a completed method result simulates loss of the database's
COMMIT acknowledgment. That network-level ambiguous-commit fault remains a separate
experiment, as do R2 writes and recovery from loss of the import host.

Use one metadata row as the simplest baseline rather than introducing a job table,
generation table, lease service and migration framework before they are needed.
The first schema is an experiment; version/migrate it deliberately as contracts grow.
Its one-row representation does not set document-corpus size or search-index layout.
