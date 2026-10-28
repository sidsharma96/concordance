# Counted posting-list baseline

The accepted [count decision](../decisions/posting-counts.md) now has an immutable
in-memory `termsearch.Index`. The original `MatchAll` and `RankBM25` scans remain
unchanged as correctness oracles. This does not replace the borrowed `htmlsearch`
snapshot, persist an index or implement a complete searchable generation.

## Build and ownership

`NewIndex` admits and validates every edition, passage and prepared term before
building retained state. `BuildLimits` bounds original editions/passages/occurrences/
string bytes and retained vocabulary entries, word/passage postings and cloned key
bytes. Vocabulary is counted separately in each edition. Temporary maps, pointer
objects, slice capacity and allocator overhead are not exact heap quotas.

The builder visits passages in original order. A new word gets one list; a new
word/passage pair appends a row; later occurrences in that passage increment its
body, heading or label count. Lists are therefore sorted and duplicate-free without
sorting postings. At least one count is positive. Per-passage combined lengths,
including zero lengths, and each edition's total length are retained for BM25.

The index owns all state. It clones the term-version string, edition IDs and each
distinct per-edition word key using `strings.Clone`, avoiding retention of larger
caller string allocations. Map values point to list objects; later updates mutate
those lists rather than reassigning caller strings as map keys. No caller arrays
or query aliases survive a call. Copies share immutable state; concurrent queries
use local scratch. Zero/nil indexes are invalid. There is no Close, goroutine or I/O.

## Query and ranking

Query `Limits` still admit the original all-edition input plus query counts/bytes,
using cached totals rather than rescanning input. Constructor limits separately
bound the index's storage. Query duplicates count toward admission, then collapse
to distinct terms in lexical order. Terms remain exact caller-prepared strings;
unknown editions, known empty editions and unknown words preserve scan behavior.

For matching, sort list references by length, with lexical index as a deterministic
tie. Copy the shortest list's passage numbers into scratch. Intersect the remaining
lists by advancing the smaller of two sorted values. Filter the candidate slice in
place: the written prefix never overtakes unread candidates. No index list changes.
Check `MaxMatches` only on the final intersection. Rejecting an oversized intermediate
list could discard a valid small final answer. Complete results either fit or fail.
`MatchAll` copies a pruned final list to avoid retaining its larger scratch backing
allocation; clipping capacity alone would not free that allocation.

For ranking, document frequency is a word's posting-list length. N and average length
include every passage in the selected edition, including empty passages. A monotonic
cursor in each query list finds counts for ascending candidate positions. Sum the
same combined-field BM25 formula in lexical term order, then fully sort decreasing
score and ascending input position for exact ties. Intersection order does not
change floating-point addition order. No field weights, top-k or phrase matching.

All workful calls require deadlines and check cancellation cooperatively, including
after bounded standard-library sorts. Failure returns nil/zero output, without
captured text. Original terms, local positions and hashes establish no authorization
or durable citation. Future index/table/generation bindings remain separate work.
Count records cannot recreate the ordered fields checked by `htmlpassagemap.Open`;
this comparison keeps those canonical fields available and changes no reader contract.

## Cost model

For original string bytes A, occurrences T, passages P, per-edition vocabulary V
and postings U, expected construction is O(A + T + P + V + U) under ordinary map
assumptions. Owned state is O(P + V + U + cloned key bytes). String hashing/comparison,
map worst cases and Go allocation overhead prevent a strict constant-time lookup or
exact retained-byte claim. Source-independent lexical terms do not make code or PDFs
HTML; those formats still need deliberate processors and locations.

For Q distinct query terms, total queried posting lengths L and M final matches,
work includes query-byte validation, O(Q log Q) ordering, O(L) intersections/cursor
walks, O(M*Q) scoring and O(M log M) result sorting. Scratch is O(Q + shortest-list
length + M), not merely `MaxMatches`. Long common-word lists may still be scanned
when another word is rare. The uncompressed two-pointer baseline makes that work
explicit; binary/galloping/skip strategies need a separate comparison and decision.

## Evidence and limits

Public tests compare matching, exact scores and order with the unchanged scans over
constructed, generated and fuzzed small corpora. An independently counted formula
also checks scores by passage with numerical tolerance. Its different `Log` and
arithmetic parentheses can reorder almost tied scores, so it is not the authority
for bit-identical canonical ordering. The first new fuzz run exposed this test
comparison issue; the case is retained as a code seed. Canonical scan/index equality
was already exact. No production arithmetic was changed to accommodate the oracle.

Tests also check per-field counts against independent nested loops, original input
and output ownership, all-edition cached admission, exact/excessive storage/query
bounds, final-only intersection limits, missing/empty editions, query permutations,
Unicode/exact terms, duplicate fields, concurrent readers and cancellation at every
observed cooperative boundary. Allocation benchmarks use identical synthetic inputs
and verify equal results before timing; allocated B/op is not retained heap.

The private Kubernetes comparison preserved all 430 ranked references, exact scores
and quotes across 28 frozen cases, including matches through the saved-location
reader. Its 38 article records contain 1,889 passages and 84,053 term occurrences.
Independent input counting found 58,362 word/passage pairs, 7,566 distinct per-edition
vocabulary entries and 62,227 logical cloned key bytes (version/edition/word keys).
These counts are not allocated or retained heap, encoded index bytes or relevance.

A synthetic Go benchmark used 4,096 passages, 94,250 occurrences and identical query
outputs checked before timing. On Go 1.27.2 darwin/arm64, Apple M1, default GOMAXPROCS
8, five samples at nominal 100 ms per sub-benchmark gave these observed ranges:

| Query | Matches | Scan µs/op | Index µs/op | Scan / index B/op |
| --- | ---: | ---: | ---: | ---: |
| Rare + common word | 41 | 3,674–3,689 | 24.48–24.78 | 1,720 / 1,072 |
| Common word | 4,096 | 3,948–3,976 | 304.55–306.07 | 193,784 / 98,312 |
| Last-passage word + common | 1 | 3,657–3,787 | 22.77–22.99 | 24 / 40 |
| Absent + common word | 0 | 2,533–2,545 | 0.167–0.175 | 0 / 16 |

Construction was measured separately: 3.173–4.177 ms/op, approximately 8.75 MB
allocated/op and 577 allocations/op. Query index allocations were three/op except
one/op for the miss; scan allocations were respectively eight, seventeen, two and
zero. The index does not always allocate less per query. B/op counts allocations,
not retained heap. Query timings exclude construction and original validation, which
move into the constructor; scans repeat input validation. These are warm synthetic
microbenchmarks, not cold-open, real-corpus latency, relevance or enterprise results.
The controlled run followed completion of tests; an earlier concurrent run is kept
separately and excluded. Reproduce with `go test ./internal/termsearch -run '^$'
-bench '^BenchmarkCountedPostings$' -benchmem -benchtime=100ms -count=5`.

The final bounded two-worker fuzz run completed 227,730 executions in 5.411 seconds
without a mismatch; this is a test observation, not throughput evidence. Raw
measurements stay under `.tools/reviews/counted-postings-*` and disposable helpers
under `.tools/probes/counted-postings-audit/`; ordinary builds need neither.
No network calls, provider writes, persisted index format, publication or host-loss
recovery is established by an in-memory index. Persistent indexing, paged metadata,
bounded readers and safe deletion remain necessary for million-document claims.
