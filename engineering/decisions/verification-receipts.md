# Verified receipts and HEAD checks

Status: accepted PostgreSQL receipt direction, now implemented as a bounded internal
component. Read the [implementation and limits](../experiments/postgres-receipts.md).
The full importer, persisted content formats, audit and recovery/retention operations
remain open. Existing `objectstore.Ensure` and `Read` retain strict full verification;
`objectreuse` explicitly uses the accepted prior-verification contract on receipt hits.
No comparative benchmark establishes a speedup over R2 markers.

## One object through the selected path

An import needs the same saved article bytes as an earlier import. Its authorized
collection and exact-byte hash select the same immutable object. Rather than read
the entire body again, the coordinator could use evidence of a prior verified read:

```text
No trusted receipt
  existing Ensure protocol
  -> verified body + metadata from that same GET
  -> save receipt

Trusted receipt
  HEAD with saved ETag condition
  -> check length and returned ETag
  -> confirm this object for the current import

Any later body read
  bounded GET -> exact length + raw SHA-256
```

The extra ETag check can detect some replacements which length alone
misses. It is an opaque provider validator, not our SHA-256, an unforgeable object
incarnation or protection against every same-tag/same-size corruption. This path
trusts prior verification and continued immutability. It deliberately does not
provide a fresh full-body integrity check during every import.

## Receipt identity and trust

A receipt binds a trusted storage-namespace identity, globally qualified
collection, key-layout version, raw byte digest, byte length, verification-policy
version, current trust epoch and the ETag returned by the verified GET. Namespace
identity must bind the actual account/endpoint, bucket and physical key namespace;
recreating a bucket or changing an endpoint must not inherit old evidence. An epoch
is a catalog-controlled verification era, not a clock or automatic expiry timer.
Diagnostic timestamps do not prove current integrity.

Only the trusted coordinator may record positive evidence after a successful bounded
read, exact hash/length check and successful body close. Capture the ETag from that
same response; a separate later HEAD cannot establish which body was verified.
Missing/malformed ETags prevent recording a fast-reuse receipt, without invalidating
an otherwise successful strict verified read. Such an object must continue to use
the strict path; publication may rely on that current full verification, not on
invented receipt evidence. The existing Ensure API returns only a reference; EnsureVerified now exposes
opaque verified evidence from the same response. A plain reference, caller-provided digest, object metadata,
published manifest or untrusted serialized struct must not construct a receipt.

Authorize the collection before receipt lookup and object access. Receipts are
neither authorization capabilities nor proof of publication. Do not share them
across collections or let a caller choose a storage target by submitting a receipt.
Keep proof persistence and trust policy with Concordance, not a shared Rewind library.

## Selected persistence

PostgreSQL is the selected receipt store. It already owns durable coordination;
indexed records can identify prior verified objects and a separate namespace guard
can disable reuse. These are separate from import bindings: one receipt per
distinct scoped object, not per passage or generation reference. This adds database
rows, indexes, migrations, authoritative lookups and backup/recovery work. Exact SQL, field limits and Go operations are described in the implementation reference.

Use bounded batches of receipt lookups rather than a separate database round trip
for every article. Validate every returned association and retain only a bounded
batch. Do not build an unbounded positive cache or use a lagging replica for trust
invalidation. Missing rows allow strict verification; a database error or malformed
row is not an absent receipt and must fail explicitly.

The alternative is small R2 receipt objects written after verification. They keep
proof near the bytes and avoid a receipt table, but add separate marker writes,
reads, integrity/compatibility checks, orphan recovery and later cleanup. A marker
can outlive a body, so it still requires an object HEAD and trusted write paths.
It does not remove PostgreSQL publication or the need to coordinate trust revocation.
Neither alternative has measured throughput or storage overhead yet.

## Failure and concurrency rules

Remote operations run outside short database transactions. Reuse needs a supported
receipt policy, active namespace/epoch, a positive conditional HEAD and matching
length/ETag. The planned first caller owns deadlines, bounded batches and total
remote work; a receipt does not remove those limits.

| Observation or interruption | Proposed action |
| --- | --- |
| No receipt, or a deliberately invalidated older epoch | Use strict Ensure/full verification; create a current receipt only if the namespace permits it. |
| Receipt matches, HEAD succeeds and metadata matches | Confirm this object under the accepted prior-verification trust contract; do not download its body. |
| Receipt exists but HEAD returns 404, 412, missing metadata or mismatching length/ETag | Fail this import. Do not silently refresh the receipt, recreate the body or overwrite anything. Diagnose and explicitly reverify under an authorized recovery plan. |
| HEAD returns 403, 5xx, cancellation or another unresolved error | Fail; neither successful reuse nor object absence. Bound any explicit retry. |
| Strict object creation has an uncertain outcome | Keep the existing same-bytes/scope reconciliation. A receipt/HEAD alone cannot resolve an unverified first write. |
| Object verifies but process stops before saving a receipt | No durable proof exists; a retry verifies again. The object may remain unused; deletion stays off. |
| Receipt write reply is lost | Look up the same receipt identity and compare the complete evidence. Do not infer rollback or invent proof. Unresolved receipt state blocks this proposed coordinator's completion. |
| Receipt commits but generation publication fails | The object may be reused later while the namespace remains trusted. A receipt never makes this generation searchable. |
| Catalog publication reply is lost | Discover the original result with the saved import key, as before. |
| A later GET finds corrupt or missing bytes | Return integrity/availability failure; no partial quote, empty search substitute or edition fallback. Stop new reuse/publication in the affected namespace pending diagnosis. |

HEAD errors can be generic. In particular, a bare 404 does not establish the
modeled `NoSuchKey` outcome which strict Ensure requires before conditional creation.
The existing GET classification must remain intact; never authorize PUT merely
because HEAD failed. A negative read also cannot rule out an earlier in-flight write.

For PostgreSQL placement, capture the active namespace/epoch before remote
verification. Namespace state and receipt insertion must then be checked together
in a short transaction against that exact original epoch; never retag old work
with a newer epoch. This prevents an old verifier from committing usable proof
after a concurrent invalidation. Compatible duplicate inserts are idempotent;
conflicting evidence must not be replaced by an unconditional upsert. A receipt-based
publication coordinator must recheck the active namespace/epoch in its publication
transaction, using a lock shared with trust invalidation. This fences old proof
from new publication; it does not lock R2 or guarantee perpetual availability.
The guard operations now exist; the complete importer does not.

## Immutability, incidents and deletion

Enable the optimized path only for a managed namespace: all normal writers use
conditional creation; no overwrite, object expiration, automatic deletion or
uncoordinated repair is allowed. Privileged changes and any newly admitted writer
need explicit review. The current R2 read/write credential and content-addressed
names alone do not enforce write-once storage. A managed-namespace declaration
is an application trust assumption, not an adversarial-writer security boundary.

Use a durable namespace guard to quarantine fast reuse and new publication after
suspected mutation or a restore/migration which invalidates the proof. Do not simply
change a TTL or mark a failed object verified. Full revalidation can establish new
evidence only after trusted write behavior is restored. Advancing an epoch invalidates
old receipts but does not fence outstanding remote writes. If such writes cannot
be ruled out, remain quarantined or migrate to a fresh physical namespace; metadata
epoch changes alone are insufficient. Database restores must not silently revive
old positive evidence for a different storage namespace.

No automatic deletion is introduced. Future deletion/repair must coordinate receipts,
published generations, active readers, in-flight work and uncertain commits. Merely
deleting a receipt first is not enough: an old verifier could still be running.
A matching HEAD can become unavailable immediately afterward, just as a verified
GET can; neither is a reader-lifetime or retention protocol. An audit strategy and
incident/recovery exercise remain required; no audit schedule is selected here.

## Costs and evidence

For N previously verified distinct objects totaling B bytes, the proposed warm
path uses N HEAD operations instead of downloading B object-body bytes. Headers,
latency, local input hashing and processing still cost work. With lookup batches
of at most K records, database round trips can be O(ceil(N/K)); indexed lookup work
still depends on receipt-table size, and scratch space must stay O(K). Receipt/index
storage grows with distinct scoped objects, not just live document count; retention
is unfinished. This is a cost model, not a benchmark or promise of bounded whole-
import memory. New objects still pay the existing full-verification cost plus
receipt persistence. Exact operation counts need implementation evidence.

The [read-only provider experiment](../experiments/verification-receipts.md) checked
conditional HEAD against one existing synthetic R2 object. It did not exercise
receipt persistence, invalidation, concurrent writers, publication or scale. The implementation reference distinguishes tested APIs/bounds from remaining work,
including:
same-response ETag binding, forged/wrong-scope/old-epoch receipts, generic HEAD errors,
size/tag changes, failed close, receipt-write ambiguity, invalidation races, restored
catalogs and fresh-client recovery. Keep strict full reads as the integrity oracle.

Sources: [R2 S3 API compatibility](https://developers.cloudflare.com/r2/api/s3/api/),
[R2 consistency and conditional writes](https://developers.cloudflare.com/r2/reference/consistency/)
and [S3 HEAD semantics](https://docs.aws.amazon.com/AmazonS3/latest/API/API_HeadObject.html).
Provider documentation is context; it does not establish our unimplemented protocol.
