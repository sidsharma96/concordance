# Separate bounded article objects

Status: accepted layout (option B). Store derived article data in separate bounded
immutable objects rather than one whole-generation article bundle. Keep one bounded,
versioned generation manifest, as already chosen. The persisted article schema,
codec, provenance associations and complete searchable publication remain open.
The reuse optimization below is a proposal, not an accepted or implemented contract.

## Layout and alternative

The first concrete workload has 38 retained article-version entries and 1,889
passages across three Kubernetes snapshots. The growth inventory contains 4,773 raw
Markdown entries, not that many extracted/searchable articles. Larger examples below
are illustrative capacity arithmetic, not measured workloads.

Each generation manifest associates logical documents, editions, captured revisions,
processing versions and typed locations with the objects needed to read them. Article
data is separate from the manifest. Original sources, images and search-index objects
remain separate responsibilities; this choice does not put every artifact in one
article object or force future formats to use HTML.

Unchanged exact encoded bytes can reuse an object within the same authorized,
globally unique collection. Unchanged prose alone is insufficient: encoding,
processing or included provenance changes may change the saved bytes. Content hashes
are neither logical document IDs nor authorization. The current in-memory snapshot
references are not persisted identifiers.

The simplest alternative is one bounded article-data bundle for each generation.
It uses fewer object operations and makes the first complete read simpler. Changing
one article changes that bundle's bytes and prevents reuse of the aggregate object.
Separate article objects allow finer reuse and individual bounds, but add metadata,
requests and cold-open work. Different compression boundaries can affect size.
Neither layout alone implements incremental indexing, selective reads or lower peak
memory. No storage or latency advantage has been measured.

## Cost model

For N articles with B total encoded article bytes, separate objects require O(N)
references plus O(B) payload for the first copy. Each later generation adds its
manifest and any distinct objects; unchanged objects are shared. The exact overhead
depends on the still-open format and compression choices.

For illustration, one million articles averaging 100 decimal KB occupy 100 GB of
article payload. Ten retained snapshots with 1% of articles changing to distinct
same-sized bytes between snapshots occupy about 1,000 GB as full article bundles,
or 109 GB as shared article objects. This excludes manifests, raw sources, images,
indexes, encoding overhead and unused uploads. It assumes the other 99% remain
byte-identical. It is not a compression ratio, bill or complete capacity estimate.

The existing `objectstore.Store.Ensure` reads and verifies an entire existing object
before reuse. For N calls and B supplied bytes, reused objects still cost N logical
GET operations and B response bytes, plus hashing/comparison work. New objects use
an absence GET, conditional PUT and verification GET, at most three SDK operations
per call with SDK retries disabled. Per-article reuse saves stored bytes; it does
not currently avoid reading all reused bytes. See the
[adapter experiment](../experiments/object-storage.md) for exact bounds and limits.

## Necessary scaling work

Persistent indexing, paged metadata and safe retention/deletion are necessary
scaling work before claiming support for millions of documents. They are not
optional finishing touches, and this layout does not implement them.

- A persistent inverted index maps prepared words to passage locations. Preserve
  edition selection, matching/ranking semantics and citation associations while
  comparing against the current exact scan. Define update/statistics costs and
  recovery before claiming incremental indexing.
- Bounded metadata pages need routing and cross-page validation under a versioned
  root manifest. Measure encoded bytes, decoded memory and generation-opening work
  to choose a transition point. Keep readers for retained older formats. The accepted
  flat manifest remains the bounded first baseline, not the million-document layout.
- Reader/cache budgets and selective article reads must bound loaded data; splitting
  objects does not make the current all-input `htmlsearch` snapshot load lazily.
- Safe retention/deletion must protect published generations, active readers,
  in-flight writes and ambiguous publications before reclaiming any shared object.
  No automatic deletion is enabled. Unused uploads and old versions accumulate.
- Reuse verification must avoid unnecessary full-body transfers under an explicit
  integrity/trust contract, with request counts, recovery and provider tests.

## Proposed next checkpoint: verification receipts

A reference already acts as a pointer: collection scope, exact-byte digest and size
derive an object key. It describes expected bytes, not proof those bytes are there.
A second marker object or caller-written checksum can also outlive a missing or
replaced body. HEAD establishes current existence and metadata, not independently
verified raw SHA-256. An ETag is not our SHA-256.

The recommended direction to discuss is a durable trusted verification receipt:
after full verification, record the storage target/namespace, collection, digest,
length and verification-policy version. Later reuse would require that receipt and
a metadata/existence check, while normal body reads would still verify length and
SHA-256. Unknown objects, missing receipts and uncertain first writes would use
full verification/reconciliation. Receipt persistence and its recovery relationship
to publication are not designed; a receipt is not publication authority.

This trades a fresh full-body integrity check at import for prior verification plus
an immutability assumption. It cannot detect same-sized replaced/corrupt bytes merely
from existence and length. All write paths, privileged access, lifecycle rules and
future deletion must preserve or invalidate that assumption; content-addressed names
and conditional creates alone do not prevent out-of-band overwrite/deletion. Missing
objects must not become successful reuse; unavailable services are not absence.
Define invalidation/recovery and later read/audit failure behavior before adopting
this contract. A lost receipt must cause safe fallback, not invented verification.

Keep full verification as the current baseline and oracle. Provider-validated
checksums are another candidate only after checking supported API/algorithm semantics
and actual R2 behavior. Custom metadata and multipart/composite checksums are not
automatically our raw full-object SHA-256. No adapter API, receipt table or faster
verification path is implemented or authorized by the layout choice.

## Failure sequence and evidence needed

Prepare bounded artifacts, confirm every required object, confirm the manifest,
then publish through the short PostgreSQL catalog transaction. Partial uploads
remain unpublished. Resolve uncertain writes using the same bytes and authorized
scope; resolve uncertain catalog commits through the saved import operation key.
Neither object granularity nor a future receipt removes the two-store protocol.

Before completing this slice, measure artifact/manifest bytes, exact-byte reuse,
request and transfer counts, decoded memory and cold-open work. Recover with a fresh
reader from R2 and PostgreSQL without local writer files. Exercise malformed and
oversized objects, missing/replaced objects, provider failures, lost replies and
receipt invalidation if that proposal is chosen. Revisit granularity if measured
small-object/request overhead outweighs reuse, using the same workload and guarantees.

Provider references: [R2 S3 operation compatibility and checksum types](https://developers.cloudflare.com/r2/api/s3/api/)
and [R2 consistency and in-flight writes](https://developers.cloudflare.com/r2/reference/consistency/).
These document provider semantics, not evidence of a tested optimized adapter.
