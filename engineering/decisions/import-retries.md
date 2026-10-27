# Import identity and retries

Status: accepted design. Use a saved key for each requested import. The CLI will
create and persist the key before the first request and reuse it after a timeout;
ordinary users need not invent keys manually. The full CLI workflow is not built.

## Contract

An import key identifies one requested operation within an authorized collection.
It is separate from product version, source revision, content hash and generation ID.
After authorization, the same key with the same immutable source revision and import
settings identifies the same operation. Once published, an identical retry returns
the original generation without publishing it again or reactivating an older result.

The same key with different inputs returns a conflict and preserves the original
binding/result. A deliberately new import uses a new key even if its source and
settings match an earlier import. Byte deduplication is a separate storage policy.

Bind the key to its inputs durably before remote writes. Otherwise an interrupted
attempt could leave the key free to mean something different. The catalog's input
comparison must use a trusted, versioned fingerprint of the complete captured-input
description. A public caller cannot establish equality merely by supplying a digest.
The source adapter and processing contract must define that description; a mutable
URL or branch name alone cannot identify the captured revision.

The following is a sketch of caller intent, not a finalized API:

```go
request.Key = savedImportKey
generation, err := engine.Import(ctx, request)
// Retry the same request and key if its outcome is unknown.
```

## Publication and uncertainty

Store the published generation reference and the import's completion together in
PostgreSQL. If a publication or final caller reply is lost, look up the existing key
against authoritative catalog state. An outage or uncertain result is not permission
to invent a fresh key or treat the original publication as absent.

Identical concurrent attempts must resolve to at most one published result for the
key. This does not promise that no physical work is repeated. An unpublished binding
does not establish whether another attempt is still running. Concurrency admission,
retry work reuse and recovery must preserve this distinction; they must not rely on
an arbitrary sleep or assume timeout killed the earlier attempt.

Bindings and completed results must not expire silently in the first slice. A later
retention policy must specify the retry guarantee after expiry and protect referenced
data. Permanent retention for an unlimited workload is not promised.

## Alternative considered

Deriving operation identity from the source revision and settings automatically
coalesces identical inputs, including a deliberately new import. We chose a saved
operation key so retrying and starting again remain explicit separate actions. We
can still fingerprint inputs for conflicts and deduplicate immutable bytes.

This choice has no performance evidence yet. The first catalog experiment must test
restart lookup, conflicting retries, lost caller replies and concurrent publication.
It is not a claim that the complete import/storage path is implemented.
