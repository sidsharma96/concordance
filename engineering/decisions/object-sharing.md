# Collection-scoped immutable object sharing

Status: accepted design. Use content-addressed immutable objects shared between
generations within one authorized collection (option B). An internal S3 adapter now
implements verified reuse/conditional creation. The dedicated private R2
bucket is provisioned. Its live creation/reuse/fresh-client-read probe passes;
provider fault and complete recovery experiments remain open.

## Identity and scope

Name stored objects by the SHA-256 of their exact serialized bytes inside a
collection-specific namespace. A collection identifier must be globally unique
across tenants; it is selected after authorization, not supplied as proof of access.
The first internal key shape is versioned, encodes the opaque collection ID without
path ambiguity, and includes the digest. There is no global cross-collection reuse.

Share bytes, not logical documents. Source/document identity, product version,
captured revision, extraction rules and citation locations remain in application
metadata. The same bytes can legitimately describe different documents. Do not
attach source-specific metadata or content types to shared blobs; record those in
the referring manifest. Empty stored bytes are valid; an empty manifest may still
be invalid to its application. Compression/encryption changes serialized bytes and
therefore the object hash; semantic similarity is not byte deduplication.

An object reference includes SHA-256 and byte length. The adapter derives its key
from the authorized store scope instead of accepting an arbitrary cross-scope key.
These references are not authorization capabilities. Existing objects are immutable:
unexpected bytes, size or hash cause an integrity error, never an overwrite.

## Write and retry contract

The first bounded baseline reads/verifies an expected object before deciding whether
to create it. If found and correct, reuse it. If the read reports explicit object
absence, attempt a conditional create with `If-None-Match: *`. A missing bucket,
denied permission or unavailable service is not object absence. A negative read
does not prove no other write is in flight; conditional creation is still necessary.

After a create attempt, read back with a byte bound and verify exact length and
raw SHA-256. This resolves a lost create reply when the expected bytes are readable,
including when another identical writer created them. A checksum or size mismatch
must remain an integrity error. If verification cannot finish after the create
attempt, report an unresolved outcome and retain the same content identity for retry.
Do not publish or delete in response to that uncertainty. Context cancellation cannot
undo a remote write that may already have completed.

This defines object availability/integrity at verification time, not perpetual
availability or catalog publication. R2 manifest presence alone is not publication.
The saved import key continues to identify the requested operation; an object digest
does not replace it. PostgreSQL still commits the complete generation's publication.

No automatic deletion is enabled. Shared objects require future cleanup to protect
every published reference, active reader, in-flight upload and ambiguous publication.
Deleting one old generation cannot authorize deleting objects another still needs.
Retaining unused objects costs storage; it is a deliberate initial safety boundary,
not a permanent unlimited-retention policy or an implemented garbage collector.

## Alternative and why share now

Separate per-generation objects simplify ownership, but repeat unchanged stored
bytes. For illustration, ten separate 1 MiB objects use 10 MiB of object payload;
one shared object uses 1 MiB, excluding manifests/metadata and provider overhead.
Actual savings depend on identical serialized objects, not similarity of documents.

The earlier recommendation favored separate objects for the first experiment.
The maintainer pointed out the storage benefit. Because automatic deletion is already
deferred, sharing does not require implementing a complete cleanup system now.
The chosen extension is collection-scoped naming and verified reuse. There is no
measured storage-cost, throughput or latency advantage yet. Fresh small objects may
cost more requests than separate blind writes; reuse savings must be measured.

## Evidence and revisit

Before calling the first storage milestone complete, exercise concurrent conditional
creation, wrong existing bytes, lost write responses, missing objects/buckets,
permission failure, cancellation, bounded full reads and recovery without local
writer files on actual R2. HTTP fixtures can test adapter behavior but not provider
durability/equivalence. Keep the library local to Concordance until Rewind also has
its concrete write/read/recovery sequence; collection policy is not a shared primitive.

Revisit sharing granularity and read-before-write verification after measuring
unchanged bytes, request counts, transferred bytes and recovery work on a real corpus.
The accepted [article layout](article-storage.md) now chooses separate bounded
article-data objects. It also records necessary scaling work. The maintainer selected
the direction to design [verification receipts](verification-receipts.md); the
PostgreSQL-backed reuse component does not change strict Ensure/Read guarantees.
Define safe retention and reader lifetimes before deleting anything. Later streaming,
multipart or range reads require their own bounds/integrity contracts; a full-object
hash cannot validate an arbitrary isolated range by itself.

Sources: [R2 conditional S3 operations](https://developers.cloudflare.com/r2/api/s3/api/)
and [R2 consistency, including in-flight writes](https://developers.cloudflare.com/r2/reference/consistency/).
