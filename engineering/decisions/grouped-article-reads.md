# Read each candidate article once within a batch

Status: accepted option A, implemented by
publishedsearch.Reader.SearchAnyEvidenceGroupedFirst. Reuse encoded downloads only;
keep per-passage manifest, edition, decoding and context checks. The model worker
and opt-in command are implemented. The ordinary JSON command now uses
SearchEvidenceGroupedFirst with all-term eligibility. The text command and
standalone quote APIs remain unchanged.

## Workload and invariant

Several ranked passages can share the same physical article. The existing
SearchAnyEvidenceFirst is the uncached oracle: one strict article read per hit,
with a per-hit byte budget. Keep that API and its budget meaning intact. The new
operation groups an admitted ranked prefix by complete object Ref (SHA-256 and
expected byte length), under one exact already-authorized reader and stable Store.
A digest is neither logical document identity nor permission.

For selected ranks A1, B1, A2, process A's group and fill positions 0 and 2, then
B into position 1. Groups use first-selected-occurrence order; map iteration never
defines processing or result order. Return the exact original refs, scores, quote,
headings and labels. Complete any-word match admission and TotalMatches still
precede the positive requested prefix. No changes to eligibility or BM25.

## Bound work before reading

GroupedCandidateLimits has positive Evidence component limits,
MaxDistinctArticleBytes and MaxTextBytes. Before any candidate GET, resolve every
selected handle, check its per-read article ceiling and admit the sum of distinct
expected body sizes using remaining-budget subtraction. A huge requested prefix
cannot allocate more slots than the complete admitted ranking contains.

Strict Store.Read downloads and verifies each group's exact body once. Borrow that
owned byte buffer unchanged through per-passage processing. For every occurrence,
verify its logical manifest slot, original HTML/profile/image bindings, independent
edition article/block/range checks, and bounded canonical context. No exported
already-verified article, caller-asserted proof or mutable decoded cache is added.

MaxTextBytes counts every returned quote/heading/label occurrence, even when bytes
are shared. Unique-download admission and logical output admission are different
budgets. They do not bound HTTP headers, failed/overlong responses, model tokens,
serialized output or whole-process heap. Owners still bound simultaneous calls.

A missing/corrupt group, mismatched logical association, limit failure or
cancellation returns zero CandidateResult, including after earlier groups succeeded.
No skip, truncation, partial success or fallback. A group observes the body from its
one verified read; it need not observe later storage deletion/corruption during
that same request. New requests read afresh. First encountered errors may differ
from the uncached path because fetch order changes.

## Ownership and costs

Let K be selected passages, U distinct physical articles and B returned text bytes.
Expected map work is O(K), excluding existing metadata lookup and per-hit checks.
Grouping metadata and result state use O(K+U+B), alongside complete ranking and one
encoded article with per-hit decode/plan scratch. No article buffer or decoded
state is retained on the reader or references; results own text and mutable slices.
Application references bound live scratch, not garbage-collector timing or allocator
residency. Concurrent calls multiply these costs.

Successful candidate body operations, derived from code:

| Work | Uncached | Grouped A |
| --- | ---: | ---: |
| Body GETs | K | U |
| Full article SHA-256 checks | 3K | U + 2K |
| Article decodes | 2K | 2K |
| Canonical passage builds | K | K |

Opening a reader separately needs two more GETs. This reduces downloads, not all
per-passage CPU. The earlier development count of 6,000 occurrences and 667
within-query article hashes remains a modeled GET reduction, not measured R2
latency, transferred bytes or billing; see [read-path research](../experiments/read-path-research.md).

## Alternatives and primary examples

Option B also shares decoded articles and passage plans. That needs an immutable,
unforgeable verified-value contract across manifest and edition checks, including
all logical slots. We choose A to remove demonstrated repeated downloads while
preserving the simpler independent-check baseline. Revisit B on measured decode,
hashing or allocation pressure, or a deliberate design goal. A cross-query cache
could avoid more reads but adds retained-byte admission, eviction, concurrent miss
coordination and permission/lifetime policy; no such cache is selected.

[OpenSearch 3.3.0 FetchPhase](https://github.com/opensearch-project/OpenSearch/blob/e972d15408320e6be84c64bb0fdc076ca7696ea2/server/src/main/java/org/opensearch/search/fetch/FetchPhase.java#L138-L232)
sorts document IDs for stored-field access while carrying each original result
position, then fills hits at those original positions. It is a local Lucene
storage traversal, not an R2 integrity or provenance protocol. We use the same
separation of processing order from result order with exact object groups.

[DataLoader v2.2.3](https://github.com/graphql/dataloader/blob/5e851ce7d3ad4e4473fb4782ebfecb6b1290bd81/README.md#caching-per-request)
scopes reuse to a request and requires output positions to correspond to input
keys. Its dynamic resolver batching, promise cache and default unbounded Map are
not our mechanism: the complete selected list is known synchronously, its size
and bytes are admitted, and each encoded body is processed one group at a time.
No dependency or general cache framework is introduced.

## Verification

The synthetic local PostgreSQL/HTTP fixture publishes three logical paths with
ranked articles A,B,A. Compare the grouped result exactly to the uncached result:
three GETs become two, and a second request performs two again. Check independent
context/result slices, cross-reader rejection, exact and one-byte-below distinct
and output budgets, complete-union overflow before GET, empty/invalid queries,
corrupt/missing later distinct bodies, bad shared-slot provenance, per-read and
context/plan/quote limits, cancellation and concurrent calls. These checks exercise
real client/catalog composition against local fixtures; they are not live R2,
model inference or a production performance benchmark.
