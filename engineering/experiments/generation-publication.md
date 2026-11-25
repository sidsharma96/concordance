# Bounded generation publication

`internal/generationpublish` now connects prospective Git/HTML manifest validation,
immutable object confirmation and guarded PostgreSQL publication. It is a low-level
internal publisher, not the complete import operation. It consumes the existing
trusted `catalog.Request`; it does not construct the complete captured-input
fingerprint, acquire source, establish producer completeness or authorize callers.
Never expose it as a public digest-only equality or import API.

## Sequence and invariants

The owner supplies configured catalog/object clients, an already-authorized captured
namespace and positive logical limits. Client lifetime, primary database durability,
physical routing and authorization stay with that owner. No credentials are discovered.

1. Admit every supplied body, including the root manifest, against aggregate byte,
   object-count and individual batch-byte ceilings before hashing. Compute references
   through the configured object's size cap. Open the prospective root with actual
   bytes and the expected collection/candidate ID. This self-computed digest validates
   output bytes; it does not establish publisher authority or trusted input identity.
2. Require exactly one supplied body for each distinct manifest artifact. Reject
   missing, extra or duplicate bodies; ordering of supplied bodies is immaterial to
   membership. Open every edition, including known-empty ones, and verify every
   logical article slot's exact bytes, HTML/profile and complete image associations.
   Shared physical bytes keep their separate logical bindings. Source/recipe/media
   references check declared bytes, not derivation or acquisition completeness.
3. Bind the trusted request with `catalog.Bind` before any S3 operation. On a completed
   binding, return the original generation with `Duplicate=true` and no S3 access.
   This remains possible after quarantine; there is no activation or new publication.
   Local preflight still runs and can be expensive; this is not a cheap full-import
   retry path or a substitute for a complete input-construction/recovery protocol.
4. Confirm all required artifacts through `objectreuse.Ensure` in bounded batches,
   comparing results to preflight references. Confirm the root manifest last. Receipt
   hits retain conditional HEAD semantics; misses retain strict creation/full reads.
   Remote operations happen outside the catalog publication transaction.
5. Call `catalog.PublishInNamespace`, never strict `Publish`, to fence publication
   against the captured namespace's quarantine/epoch. Return only a confirmed result.
   Concurrent instances may upload candidates, but the key's catalog row chooses one
   published result. An already-completed winner remains recoverable.

Every error returns zero publication metadata, while bindings, receipts or remote
objects may already exist. Wrapped errors preserve `errors.Is`, including uncertain
commit outcomes. Do not infer rollback, invent another key, rebind changed inputs,
automatically retry/resume or delete. A previous success does not promise perpetual
R2 availability; later reads still verify bytes and report missing/corrupt objects.

## Admission, work and ownership

One caller per publisher instance is admitted with a nonwaiting permit. Others get
`ErrBusy` before local validation, binding or object access. This is not distributed
ownership or a guarantee that another process has stopped. Separate instances can
race; the catalog owns the result. No goroutine, waiting queue or hidden retry runs.

The caller keeps the outer slices, all bodies and aliases unchanged through the
synchronous return. The publisher borrows clients and bodies and retains no payload
buffers after the call. Returned generation metadata is an independently mutable
value. Copying a publisher value shares its permit and clients.

`MaxObjects` includes the root. `MaxInputBytes` sums all supplied payload occurrences
plus root bytes; every body must fit `MaxBatchBytes`. All nested codec ceilings remain
positive and independent. Limits are not exact heap/RSS/CPU budgets. The caller's
whole payload, parsed root, one diagnostic deep metadata copy, reference maps,
selected index and decoded article work coexist. Collection-sized metadata and the
current edition's index remain memory-resident. Shared article verification repeats
for each logical slot. Context checks are cooperative; hashing/codec work has the
existing bounded synchronous cancellation limits.

Expected local work includes hashing all admitted payload bytes, manifest checks,
every index open and per-slot article verification. Hash maps have expected linear
aggregate work, not a worst-case real-time bound. Codec temporaries, Go allocation
capacity and garbage collection add overhead. Receipt lookup scratch is batch-bounded.
This composes existing algorithms without a cache or measured throughput gain.

The simplest baseline retains admitted payloads in RAM. Temporary files would add
local writes/reads, file/disk quotas, close/cleanup and crash-leftover work. They reduce
retained payload memory only if acquisition and index consumers can release their
byte slices. They do not remove the current prepared-field/count-index or flat
manifest memory. Revisit staging when measured retained payload/peak heap or concrete
admission failures justify it, together with streaming consumers and disk budgets.
Persistent indexing, paged metadata and safe retention remain necessary scaling work.

Relevant researched examples support the limits rather than dictating implementation:
[containerd's pinned local writer](https://github.com/containerd/containerd/blob/fcd43222d6b07379a4be9786bda52438f0dd16a1/plugins/content/local/writer.go)
checks an ingest file's size/digest before moving it into its local content store.
Its local persistence/resume contract and rename do not implement our remote catalog
publication. [Lucene's pinned RAM-buffer contract](https://github.com/apache/lucene/blob/51190f35a16d2ce433139abfe0fd8365791b352a/lucene/core/src/java/org/apache/lucene/index/LiveIndexWriterConfig.java)
warns that a flush threshold is not a whole-process memory cap. Lucene's incremental
disk segments are not implemented by our edition index. No new dependency was added.

## Observed cross-store tests

Tests use synthetic input, a dedicated local PostgreSQL database with isolated schemas,
and the actual S3 SDK against an HTTP fixture. Ordinary checks skip database cases
unless `CONCORDANCE_TEST_DATABASE_URL` is set; `make verify` requires it. This is not
real R2/provider or hosted database recovery evidence.

The fixture has two logical paths sharing an article, a retained figure, a nonempty
edition and a known-empty edition. Seven distinct artifacts plus root produce eight
confirmed objects. The cold path makes 16 GETs (miss plus verified read per object)
and eight conditional PUTs. A new key/candidate sharing all seven artifacts makes
seven HEADs, two root GETs and one root PUT. A completed-key retry makes no S3 calls,
while still repeating local validation. These are exact fixture operation counts,
not latency or production scale measurements.

The successful path clears writer buffers and closes its database pool. Fresh SQL
and object clients resolve the published generation, check its physical key/root
reference, open a selected edition, search and verify a quote with correct logical
source metadata. That reader path fetches root, selected index and one article: three
GETs. The local database and HTTP service survive; this is client reconnection, not
loss of the storage hosts or proof that the production preparation pipeline exists.

Failure tests check no binding/upload after invalid bundle admission, no upload
when catalog binding is unavailable/conflicting, failed conditional writes leaving
unpublished bindings, stored-but-error PUTs reconciled by strict reads, SQL rejection
after all objects exist, explicit same-input retries, missing known objects causing
quarantine, cancellation, bounded caller admission and competing publisher instances.
A targeted race holds the import row, observes the publication query blocked in
PostgreSQL after every remote confirmation, quarantines the namespace, then releases
the row. Guarded publication fails and leaves the binding unpublished. Synchronization
uses channels and observed database lock state, not timing sleeps.

Network-level loss of COMMIT acknowledgment, real R2 faults, restored catalog/receipt
state, hosting/host-loss recovery, production source preparation, complete input
fingerprints, full import/query APIs and large-workload capacity remain open. No WAL,
automatic resume, object deletion or documentation deployment is introduced.
