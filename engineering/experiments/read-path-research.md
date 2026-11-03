# Apply production read-path research to Concordance

Status: research and a derived operation-count probe, not an implemented storage
optimization. The accepted [worker failure policy](../decisions/local-reranking.md)
is recorded separately. The uncached candidate API remains the oracle; the accepted
[grouped-read follow-up](../decisions/grouped-article-reads.md) now reuses encoded
downloads within a request, keeping per-passage verification/decoding. Worker and
opt-in command implementation remain pending. No schema, codec or relevance policy
change follows from the comparisons.

## What the primary reports establish

[Linear's delta-sync report](https://linear.app/now/rebuilding-delta-sync-read-path)
describes immutable workspace action logs with a secondary metadata index. It
filters IDs before fetching full payloads. Recent authoritative PostgreSQL data
overlaps indexed history; the server deduplicates IDs and can fall back to
PostgreSQL. Shadow requests checked action-ID equivalence. Its workload is
permission-filtered synchronization, not documentation relevance. These are reported
production mechanisms, not independently inspected implementation or reproduced
performance measurements.

[Turbopuffer's postings report](https://turbopuffer.com/blog/fts-v2-postings)
describes logical key/value posting blocks targeting roughly 256 entries, independent
of vector-cluster boundaries, inside an object-backed storage engine. Blocks can
share physical storage objects. It reports index size falling from 51.6 GiB to
5.22 GiB on 40 million MSMARCO documents. Its query-speed headline also includes
a query-execution rewrite. These before/after workloads do not measure Concordance.

The reports are primary production accounts, not public source for the proprietary
engines. Exact fetched-page hashes and source snapshots remain in the ignored
research artifacts. The public compression library linked by the postings post is
inspectable: [Quickwit bitpacking at bb6341a3e160f3566351b47e4b0dee6a17f80a39](https://github.com/quickwit-oss/bitpacking/blob/bb6341a3e160f3566351b47e4b0dee6a17f80a39/src/lib.rs)
defines fixed-block sorted-integer compression/decompression; its
[4x implementation](https://github.com/quickwit-oss/bitpacking/blob/bb6341a3e160f3566351b47e4b0dee6a17f80a39/src/bitpacker4x.rs)
uses 128-integer frames. That inspected revision is not asserted to be the one in
turbopuffer's deployment. No Rust dependency or format is adopted.

## Immediate investigation: read each selected article once

Concordance already ranks local index references before reading article bodies.
SearchAnyEvidenceFirst, however, reads and decodes the same article separately for
each selected passage. We counted existing development inputs without executing
new queries or inference. Controls and the reserved split are excluded.

| Development phrasing | Cases | Candidate occurrences | Sum of distinct article hashes per query |
| --- | ---: | ---: | ---: |
| Keywords | 60 | 3,596 | 373 |
| Ordinary questions | 60 | 6,000 | 667 |

For the ordinary questions, a hypothetical one-body-read-per-article-per-request
plan would reduce logical candidate-body GETs from 6,000 to 667, about 88.9%.
One existing 1.35 development case selects 100 passages from 11 article hashes.
Opening manifest/index reads are unchanged. No cross-query cache is assumed.
These are counts derived from exported metadata, not measured R2 requests, bytes,
latency, billing or model speed. Sixty cases repeat twenty needs across editions;
they are not independent production users. Model pair counts remain unchanged.

The implemented follow-up groups selected passages by exact physical article
reference within one authorized reader/request, fetches an article strictly once,
validates every logical-slot association and fills owned candidate texts back into
their original lexical positions. It can process one article group at a time to
avoid retaining all decoded articles. Preserve each logical citation even when
bytes are shared. Grouping by a digest alone is not source identity or permission.

Sharing decoded state would additionally require owned immutable checked-article state,
per-read and distinct-body work limits, aggregate extracted-text/plan bounds,
failure/cancellation behavior and the exact uncached oracle. Compare refs, scores,
quotes, headings, labels, ordering, limits, body GETs and decode work. A failed group
still yields no successful batch. The current per-hit article-byte admission cannot
simply be left unchanged and claimed to describe a new grouped read plan. The new
operation therefore has separate distinct-byte limits; the old API is unchanged.

The model needs every candidate's text to choose the final display prefix. We can
avoid repeated article work; we cannot fetch only the final ten texts before the
model has chosen them. Batching within a request adds a different ownership contract
from a persistent cache with eviction and invalidation. The follow-up implements
encoded-body grouping only; decoded-state reuse and persistent caching remain deferred.

## Protect completeness and authority

Our catalog determines which generation is published; a reader pins that generation
and edition through all matching, quoting and scoring. We publish only after the
required index/articles/root are confirmed. There is no asynchronously replicated
search index whose recent and historical ranges need merging. Retain that simpler
contract until actual live incremental-index requirements justify changing it.

If such a secondary index is introduced, a largest-seen ID is not proof of a fully
indexed prefix: an illustrative index containing 101 and 103 may still lack 102.
We would need an explicit complete-through boundary and recovery/replay rules.
The Linear report does not provide every detail of that protocol; this is an
unanswered design requirement, not a claim that its implementation drops actions.

For future private connectors, current authorization must precede candidate-body
fetches and model access, not just the displayed ten results. Indexed access fields
alone do not establish fresh permission. The existing collection-scoped trusted
operator command is not document-level permission enforcement.

An equivalent fallback can preserve one exact operation. A lexical fallback cannot
promise the requested model order, so the accepted scorer-error policy is unchanged.
No PostgreSQL replacement, CDC pipeline, client change log or shared Rewind engine
is implied by either report.

## Persistent index work needs its own evidence

The current edition format is one whole object with absolute unsigned-varint
posting positions/counts. Open reads and verifies that whole object and builds
an immutable in-memory index. A future directory of compressed posting blocks
could permit selective reads and bounded working sets; it is a new layout/reader,
not an encoding flag on the existing format.

Logical posting blocks need not each become a separate R2 object. Physical object
grouping, range request sizes and cold reads belong in the cost model. Partial
reads need an independently trusted directory and per-block integrity references;
our current whole-body SHA-256 check cannot authenticate an isolated byte range.
Keep existing strict Store.Read/Ensure guarantees intact. Specify short lists,
tail blocks, corruption and version compatibility before choosing block sizes.
Our immutable generation builds do not currently need a mutable posting-list
split/merge or LSM-compaction engine.

Block score bounds and exact top-k selection are useful future comparisons, but
skipping score work does not by itself provide our exact TotalMatches or complete
MaxMatches admission. Preserve those through a separate exact path or discuss an
explicit contract change. Stable ties and the frozen lexical first-100 pool remain
the oracle; a lexical score bound is not a bound on model scores.

First measure article duplication and repeated decode work. Revisit persistent
blocks when edition cold opens, retained index memory or skewed posting lengths
become demonstrated constraints; then compare sizes, metadata/request overhead,
decoded bytes, cold/warm behavior, exact candidates and recovery. Do not copy a
block size or infer a speed multiplier from a different engine's headline.

## Reproducibility

Source inspected at Concordance `bfef6b07c09289ad55c6f11a10205bea0b49c458`:
internal/publishedsearch/{reader,evidence,candidates}.go,
internal/termsearch/{index,index_query,index_bm25,index_any}.go and
internal/htmledition/codec.go. No new timing benchmark or live provider call ran.

The count probe uses the unchanged development bundle SHA-256
`d115432eed23a5a28bc16022ac4432335cc9804441c34c83b0cb0692e71ebc60`.
For each non-control case it counts candidates and distinct article_sha256 values,
then sums within each phrasing. Saved rows are under ignored
.tools/reviews/readpath-research/article-reuse-counts.json. Actual grouping must
use complete authorized object references and independently checked slot bindings,
not treat this hash-counting probe as a ready-to-use storage verifier.
