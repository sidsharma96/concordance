# Bounded PostgreSQL receipts and object reuse

This internal slice implements the selected PostgreSQL placement and the receipt/
HEAD protocol. It does not provide a complete importer, article codec, persisted
citation, search index, audit scheduler, deletion or hosted database deployment.

## Representation and operations

`objectstore.EnsureVerified` retains strict conditional creation and full length/
SHA-256/read-close verification, returning opaque proof without retaining the body.
The ETag comes from that same verified GET. `Ensure` and `Read` keep their strict
contracts. `Check` performs one conditional HEAD, comparing length and returned tag;
404/412 become failed known-object confirmation, never permission to create. Strong
ASCII quoted ETags are limited to 1,024 bytes. Missing/unsupported validators or an
implicit endpoint cannot produce a reusable receipt; strict verification still works.
Trusted client routing/configuration must remain stable. Metadata is not raw SHA-256.

The catalog explicitly installs two additional tables with `CreateReceiptSchema`;
this is owned-schema setup, not an automatic migration. Namespace IDs are bounded
opaque labels bound to an explicit endpoint, bucket, collection and key version.
Physical bindings are unique. Namespace tokens are tied to their pool and epoch;
reopening a pool requires loading a fresh token. Binding cannot retarget, alias or
reactivate a physical namespace. `Quarantine` disables it durably and advances its
positive epoch without overflow; there is deliberately no resume or delete method.

`LookupReceipts` uses one bounded array/ordinality query and returns caller-order
positions, including duplicates and explicit misses. Database errors and malformed
rows fail the whole call. `RecordVerified` admits only opaque adapter proof matching
that namespace, deduplicates/sorts compatible evidence, locks the original namespace
with `FOR SHARE`, inserts a bounded batch without overwriting conflicts and compares
complete saved evidence before commit. A commit error is conservatively uncertain;
look up the same namespace/objects. It does no remote work while holding SQL locks.

`objectreuse.Store` composes those concrete components. Its configured object-count
and aggregate-input-byte limits are checked before remote work, and each object has
the adapter's byte ceiling. Input aliases stay unchanged until return. A batch hashes
inputs, deduplicates identical references, looks up receipts, checks hits with HEAD,
fully verifies misses, records compatible proof, then rechecks the namespace. Output
references are owned and ordered like the input; errors return nil output. Missing
validators remain strict-only. Reads first look up the reference and then verify the full body. Only a known
receipt can authorize quarantine for a failed read; unknown references do not
disable a namespace. Known integrity/
metadata failures attempt quarantine using the original caller context; an unconfirmed
quarantine error remains observable. Transient failures do not invent absence.

A caller using receipt confirmations must use `PublishInNamespace` after confirming
EVERY generation object and manifest. Its short publication transaction rechecks and
locks the same namespace epoch before new publication. Existing committed import
results remain recoverable after quarantine. The old `Publish` is the strict-proof
path and cannot protect receipt-based callers. The catalog does not inspect complete
manifest coverage. No complete importer currently invokes either path.

## Cost and ownership

For at most K input objects and B admitted input bytes, hashing costs O(B), lookup
and output scratch cost O(K), and each distinct warm object uses one HEAD. Missing
receipts pay strict GET or GET/conditional PUT/GET plus receipt persistence. Sorted
proof insertion adds O(K log K) local comparisons. The batch uses one lookup query,
a final namespace query, and, for new compatible proof, a short transaction with
namespace check, batch insert and comparison. Bounded batches reduce round trips,
not total index work or the N HEAD requests over a large import. Repeated target
metadata is also returned by the lookup query. No speedup/heap measurement is claimed.
Proof retains its object Store, not object bytes. Namespace tokens retain their pool;
callers still own pool/client closure, authorization, waiting callers and total work.

## Evidence and remaining work

Real SDK HTTP fixtures and dedicated PostgreSQL 14.17 integration tests exercise
strict-then-warm reuse, duplicate/order/empty-object behavior, fresh-pool receipt
loading, bounds, forged proof, malformed metadata, closed database, compatible
concurrent inserts and conflicts without overwrite. A paused HEAD followed by
quarantine returns no batch. Another test observes an actual PostgreSQL lock wait:
invalidation commits before a waiting verifier proceeds, and that verifier cannot
save stale proof. Guarded new publication is rejected while the prior committed
result remains recoverable. Changed bytes fail verified reads and disable reuse;
known changed objects are not repaired. Lost receipts cause full verification again.

A separate read-only live R2/local PostgreSQL probe used the existing 45-byte
synthetic object, with a transport rejecting all methods except GET and HEAD. The
first confirmation used one GET and saved a receipt. Reopening the database pool
and confirming again added one HEAD, no GET. A later verified body read added a
second GET and matched the expected bytes. It wrote no R2 data or generation and
used only an isolated temporary local database schema. This is interoperability
and reconnection evidence, not host-loss recovery, remote PostgreSQL durability,
provider-fault coverage, latency or million-document capacity evidence.

Tests use explicit channels/observed database locks, not timing sleeps. Actual
network-level ambiguous receipt COMMIT and invalidation failures remain unexercised;
errors are handled conservatively, but branches alone are not fault evidence.
Further provider faults, audits, restored-database/recreated-bucket recovery,
production migrations/hosting and receipt/index size/throughput measurements remain
open. Namespace quarantine cannot fence in-flight R2 writes; repair/recovery may
require a fresh physical namespace. Automatic deletion remains disabled.

Local evidence is ignored under `.tools/reviews/postgres-receipts-*` and
`.tools/probes/postgres-receipts/`. Private sources and credentials are excluded.
