# Borrowed search snapshot and on-demand quotes

## Scope and representation

`internal/htmlsearch` composes canonical HTML field preparation, complete-match BM25
ranking and bounded quotation. The accepted [result decision](../decisions/search-results.md)
returns snapshot-bound handles and independent strings only on request. This is an
in-memory component, not a raw-text query command, persisted citation/index schema,
complete import or published generation. No dependencies or network work are added.

The constructor borrows ordered editions/articles and owns computed term arrays,
strings and compact body locations. A handle retains the whole state, including
original backing allocations. Wrapper copies share one state; independent construction
creates another. Source/logical-document/revision/profile/generation associations
remain with the caller and exact input ordering. A quote is retained extracted block
text, not raw HTML, normalized search terms or guessed original Git locations.

## Bounds, cost and failures

Global admission counts editions, articles, blocks, groups and every logical input
string occurrence before preparation. Nested field/passage limits apply per article.
Aggregate passages, term occurrences and normalized bytes are checked after each
bounded article preparation. Peak temporary memory can include prior admitted output
plus one article's prepared fields and complete plan. Only body ranges survive plan
flattening. Per-article projected-view ceilings times the article-count bound limit
projection work; there is no separate global premeasuring projection pass. Short
borrowed slices may retain larger arrays; logical byte ceilings are not heap quotas.

Construction adds counting/flattening to existing builder/projection/term costs,
including repeated context and group/reference processing. Search retains the public
ranker's all-input validation, exact-edition scans and full sort; it does not cache
statistics. Mapping M hits adds O(M) time/storage and an edition scan. Location lookup
is O(1). Quote traverses selected blocks twice and copies text/LF separators into a
new builder, O(blocks + bytes) work and O(quote bytes) output. Builder capacity may
exceed logical size. No remote requests occur. There is no measured latency, heap,
allocation, relevance or storage-size improvement claim.

A required deadline has cooperative checks; any constructor/search/quote error returns
nil snapshot, zero result or empty string respectively. Explicit quote-byte admission
includes separators and never truncates. Zero and cross-state references fail. Missing
and known empty editions remain distinct. All mutation/reuse of borrowed input is a
caller contract violation; cancellation does not prove other readers have finished.
Read-only calls own scratch and may run concurrently. Owned quote text can outlive
the state, but its provenance still belongs to the exact captured article.

## Checks and observed composition

Tests exercise Extract -> New -> prepared Query -> Search -> Location/Quote with
code whitespace, figure alt/caption text and notice-label separation. They compare
exact scores/order and canonical article-local locations with direct public component
composition and quotes with a simple slice-and-join oracle. One hundred fifty
fixed-seed generated inputs cover multiple articles, empty/missing editions and
matching across their independent passage orders. Limits test exact/excessive global
counts/bytes across editions, nested failures, all positive nested limits even for
empty input, late malformed articles, and quote LF/empty-text cases. Tests also cover
nil/zero snapshots, independent states, wrapper copies, result mutation isolation,
input immutability and interruption at every cooperative context check.

Six concurrent readers each repeat ten searches and quote every match using independent
scratch; race testing passes. A nominal five-second fuzz run with two workers completed
58,818 executions in 5.499 seconds without a composition disagreement. Inputs are
bounded to 24 bytes and map to small structural fixtures; this is not arbitrary
hostile HTML or a performance benchmark.

A private audit retains the exact existing 38 articles, 1,889 passages and 84,053
prepared term occurrences across three pinned Kubernetes editions. All 28 existing
query/edition cases and 430 ranked handles agree exactly with the frozen BM25 audit's
IDs, scores and order. Every handle resolves to the canonical article-local range;
all 430 requested quotes agree with independent original-block joins. Summed quote
bytes across those case hits are 138,032, including repeated passages across queries,
not stored size or a proposed display limit. Known-empty/missing controls pass and
input remains unchanged. Source hashes and the frozen prior report are checked.
No new reader judgments or published generation are involved.

Full `make verify` passed on Go 1.27.2/macOS arm64 with dedicated local PostgreSQL
14.17, including check/vet/tests, uncached race tests and build. Public package tests
need no private corpus, credentials or database. Live R2 remains opt-in and unchanged. Revisit retained memory,
copy/reuse needs and corpus growth before adding caches, persisted IDs or reader
release protocols. The snapshot remains an exact reference baseline for future
postings/top-k work and durable publication/recovery integration.
