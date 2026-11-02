# Concordance counterpart to Rewind's storage handoff

This is the counterpart assessment requested in Rewind's delivered handoff for
[PR 45](https://github.com/sidsharma96/rewind/pull/45), not an agreed shared API or
approval to extract code. Rewind read and acknowledged this maintained note directly. Its acknowledgment
checked the pinned contracts/evidence references, not this ongoing source change
or fresh provider experiments. No cross-chat reply or other-project edit is required. Both projects still
own their application catalogs, identity/authority, retention and success semantics.

## Source and evidence pins

Concordance object/publication contracts were checked at
`68205d25ce53c62489538fd6a639acbf1722e0f9`; this retrieval-evaluation change alters no
runtime storage contract. Rewind's tested head
`aac325e46fd90fecc0ede269896b2688a4602d4e` and squash merge
`b0b53e7566d95af121808303955074b91138f319` have the same tracked tree
`7825f8f5752c43699ed14a2e98d69d47f452b308`, verified locally. Its merged
[extraction checkpoint](https://github.com/sidsharma96/rewind/blob/b0b53e7566d95af121808303955074b91138f319/engineering/OBJECTS.md)
and [recovery record](https://github.com/sidsharma96/rewind/blob/b0b53e7566d95af121808303955074b91138f319/engineering/INGEST.md)
state their narrower process-replacement scope. Relevant source is the two
`internal/objectstore/store.go` adapters, Concordance `internal/catalog/catalog_test.go`
and `internal/generationpublish/storage_test.go`, and Rewind's
`internal/catalog/recovery_{test,worker_test,proxy_test,objects_test}.go`.
Both use the same locked AWS SDK/S3 and pgx versions; that alone proves no semantic fit.

## What Concordance has actually exercised

`TestPublishedGenerationReopensWithoutWriterBuffers` confirms every artifact/root
before catalog publication, clears the original buffers, closes the pool and opens
fresh catalog/object clients to recover the catalog-pinned root and search data.
`TestPublishedRetrySurvivesReconnection` discards a successful caller result and
recovers the original result through another pool. Its comment explicitly says
this is not loss of PostgreSQL's COMMIT acknowledgment. The process is not killed.

`TestStoredPutErrorIsReconciledBeforePublication` uses the real SDK against a
controlled peer that stores the PUT but returns 503. Strict GET verification must
resolve it before publication. `TestLostCreateReplyIsResolvedByVerifiedRead` and
`TestUncertainWriteIsNotObjectAbsence` cover the strict object's corresponding
response/uncertainty classification. None is represented as an actual provider outage.

The separate live [droplet experiment](droplet-catalog.md) publishes through real
PostgreSQL and R2 and reads from a fresh process. Its synthetic encrypted catalog
restore quarantines old receipt namespaces and recovers a quote through strict
reads of surviving R2 bodies. The [Kubernetes experiment](kubernetes-publication.md)
interrupts an import, retains the same operation/capture, deliberately retries it,
then reads the completed published generation after the writer exits. The later
[public CLI check](hosted-search-target.md) uses the same corpus, exact source pins
and quotes. Normal writer exit/fresh processes and restore are useful evidence,
but do not replace a killed writer and a real lost wire-COMMIT response.

| Observed sequence | GET | HEAD | PUT | Scope |
| --- | ---: | ---: | ---: | --- |
| Live synthetic first publication | 16 | 0 | 8 | Actual provider SDK transport requests |
| Completed saved-key retry | 0 | 0 | 0 | Local preflight still reconstructs/checks input |
| Synthetic generation sharing seven artifacts | 2 | 7 | 1 | Receipt HEAD reuse plus new root |
| Fresh selected synthetic quote | 3 | 0 | 0 | Root, index, one article |
| Same quote from isolated restored catalog | 3 | 0 | 0 | Surviving objects, strict reads |
| Deliberate real-corpus retry | 9,691 | 160 | 4,834 | Observed attempts, not newly created object counts |
| Fresh real-corpus verification | 16 | 0 | 0 | Root, three indexes, six figures and six article reads |

Per-phase request/response body-byte counters were not retained for these live
publication paths. The manifest's 132,106,453 declared bytes are not wire traffic;
104/285-byte quotes are assembled text sizes, not downloaded article-body sizes.
Do not manufacture a byte comparison with Rewind's instrumented controlled peer.
The counterpart drill must record body reads/writes separately from headers,
SDK buffers, retries and guaranteed delivery, as Rewind's record does.

The separately authorized [counterpart drill](writer-process-recovery.md) now
kills and reaps the writer, removes its working directory and starts a replacement.
It withholds the real final publication COMMIT response after observing completion
and idle state. Four cases pass against native local PostgreSQL and a controlled
HTTP peer with per-phase request and actual body-byte counts. This closes the
local process/wire-fault evidence gap, not host-loss or provider-durability coverage.
The [design checkpoint](../decisions/shared-object-recovery-checkpoint.md) records
source-inspected tracer placement and the separate pre-implementation review.
An earlier returned error is never permission to delete uncertain objects.

## Differences that a shared primitive must preserve

### Namespace and authorization

Rewind accepts an opaque tenant string per object call and hex-encodes its bytes;
it does not require UTF-8. Concordance binds a globally qualified UTF-8 collection
to the Store and derives collection-scoped keys from that stable binding. Casting
one scope representation into the other could reject or collapse valid identities.
Authorization, qualification and routing ownership remain application-specific.

A credible common mechanism could operate on an opaque, already resolved physical
address while wrappers preserve each app's scope mapping and lifetime. That does
not authorize arbitrary endpoints/keys or make an address an authorization token.
The address/binding shape and which layer constructs prefixes are not selected.
A concrete counterexample supplied and checked by Rewind is tenant byte `0xff`,
which maps to hex `ff`: UTF-8-only common validation would reject valid existing
input. Preserve both `v1/tenants` and `v1/collections` mappings unless a migration
is explicitly designed.

### Exact length versus a maximum

Rewind's object Read returns owned, hash-verified bytes under the caller/adapter
maximum. Its ingest Store separately compares the result with the accepted
catalog size. Concordance's Ref carries exact expected size; the adapter checks
ContentLength when present, reads at most exact-size-plus-one, and checks actual
length and SHA-256 before exposing bytes. Both reject oversized/corrupt reads.

Bounded body reads, closure and hashing are credible common mechanics. Whether a
shared call always takes exact size or supports a maximum-only mode is a genuine
interface choice still to discuss. For example, a three-byte object read with Rewind maximum 1024 succeeds;
Concordance exact Ref.Bytes 1024 must reject that body. Rewind's accepted-record
caller still checks its actual catalog size. Wrappers must preserve end-to-end
exact-size guarantees; a digest or ETag cannot substitute for those checks.

### Admission, timeout and client lifetime

Rewind's adapter uses a nonwaiting active-call permit and an operation timeout,
releasing both on every return. Concordance's adapter requires a deadline-bearing
context and leaves admission to its owner; receipt confirmation adds its own
bounded overlap above the strict store. Transport/header/body buffers remain
separate from logical object bounds in both projects.

Context-propagating I/O is a credible shared mechanism. App wrappers can retain
their present admission/deadline ownership without a hidden queue, background
worker or silently changed timeout. A new shared guard, if proposed, needs explicit
waiting, overload and cancellation semantics. A live context with no pre-existing deadline works in Rewind because its
adapter installs one; Concordance rejects it. This difference must survive any
wrapper or reviewed common guard. Both owners still supply trusted
clients/routes/credentials and keep them valid through all dependent work.

### Write references and input ownership

Rewind's `Create` takes an `ObjectID` containing the caller's digest and verifies
that digest against the body before remote access. Concordance's `EnsureVerified`
computes and returns its exact reference. Neither method makes a caller's digest
an identity or permission. A common write/reference shape remains undecided.
Rewind also clones its input in the ingest coordinator and has separate
nonblocking ingest and object-adapter permits. Concordance's strict adapter borrows
the body through return and leaves admission to callers. These are source-derived
ownership and operation limits, not measured peak-heap results or reasons to copy
an extra buffer into Concordance.

### Conditional writes, errors and verification reuse

Both strict paths reconcile ambiguous conditional creation through a verified
read, preserve existing different bytes and disable SDK retries at this layer.
Their object-level missing/integrity/uncertain outcomes can be compared as common
facts, while application publication/acceptance errors remain separate. Generic
404, permission failure or a read after an uncertain PUT must not become proof of
catalog absence or a safe-to-delete object.

Concordance's receipt protocol stays above the strict common path. Its proof is
scoped to a stable physical binding and catalog namespace epoch; guarded publication
and quarantine are part of its application trust protocol. Rewind's current
permission/expiry and acceptance rules cannot be bypassed by importing that cache.
A common HEAD operation would not itself make receipt-based reuse safe. Range
reads, caching and deletion remain deferred.

## Smallest credible next step

The narrow candidate is immutable conditional-create/reconciliation and bounded
verified full-read mechanics, with honest provider/object error classification.
It excludes codecs, logical identity, catalogs, receipts, publication, compaction,
retention, acknowledgment, shared services and cross-checkout imports. This names
candidate responsibilities, not function signatures or an approved package API.

Complete the counterpart implementation reviews and compare the pinned evidence together and discuss the
namespace, write-reference, exact-length and admission alternatives before extracting a library.
Rewind's received handoff plus this assessment establishes delivered coordination;
it does not automatically satisfy the joint design or extraction gate. Actual
machine loss, provider-service loss, failover and disaster recovery remain distinct
claims even after both process drills pass.
