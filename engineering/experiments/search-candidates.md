# Retrieve verified candidates before model inference

Status: implemented library prerequisites for the accepted
[local worker direction](../decisions/local-reranking.md). The model worker,
wire protocol and opt-in command remain unimplemented; ordinary search is unchanged.
No model, corpus, artifact codec, query preparation or relevance setting changed.

## Same evaluated ranking, explicit eligibility

Index.RankBM25Any admits the original input/query through the same private lookup
used by match-all. It visits distinct query terms in lexical order, accumulates
strictly positive combined-field BM25 contributions from their posting rows, then
sorts the complete union by descending score and ascending input position. Missing
terms contribute nothing; duplicates contribute once. AnyBM25Version identifies
the evaluated any-word/single-term-sum-v1 policy separately from match-all.
The existing RankBM25 and reader Search preserve their contracts.

For P selected passages, Q distinct query terms, L queried posting rows and M union
matches, additional work is O(Q log Q + L + P + M log M), plus byte validation and
string lookup/comparison, with O(P + Q + M) scratch. Scores use dense per-passage
scratch; immutable postings are borrowed and unchanged. Query aliases are borrowed
only through return; results own their storage. A deadline is required, sorts
complete cooperatively, and every public error returns zero output. MaxMatches
admits the complete union before any prefix or article work.

The independent oracle sums the existing scan's single-term BM25 results in the
same lexical order. Two hundred generated corpora check exact floating-point
scores and ties across selected, other, empty and absent editions. Additional
checks cover original query limits before deduplication, union overflow, unchanged
match-all, cancellation and concurrent result ownership. The offline any-word
composition now checks the new index operation against its old composition on
every ordinary comparison, keeping report/model input bytes unchanged.

## Text and citations stay with one reader

Published SearchAny produces actual reader-bound references, never a new persistent
passage identifier. Evidence performs the existing strict object read, manifest
article/profile/image validation and independent edition quote verification. Its
private readArticle helper is shared with Quote. The existing two article decodes
remain; the already manifest-verified decoded article supplies a bounded canonical
passage plan. Exactly one canonical body range must equal the saved range before
headings and nonempty notice labels can be returned.

Evidence owns its LF-joined quote string and context slices. The context strings
are immutable decoded values, independent of input JSON; copying Evidence shares
its mutable slices. No raw article or model serializer escapes. Context remains
separate from verbatim body. Full canonical-plan limits and selected quote/context
byte limits are distinct and positive. A late plan/context/cancellation failure
returns zero Evidence. Existing Quote still returns an empty string on error and
performs one article GET, three digest checks and two decodes.

SearchAnyEvidenceFirst uses one complete ranking and a positive prefix count.
Before the first candidate GET it sums the selected per-hit manifest article sizes
using remaining-budget subtraction. Each article also must fit its per-read cap.
Shared bytes are charged repeatedly because the initial path reads once per hit.
It then collects owned Evidence and admits the aggregate quote/heading/label byte
count. Any later error discards earlier candidate results; no failed hit is skipped.
The result reports complete TotalMatches separately from selected Candidates.

K selected hits need K logical article GETs after the two opening reads, O(K+B)
returned state for B text bytes, the complete ranking and one article/plan scratch.
The sum bounds successful declared body work, not HTTP headers, SDK retries,
failed/overlong responses, model-token work, serialized output or process heap.
Owners still bound simultaneous calls/readers. No persistent or request cache,
queue, goroutine, remote write or fallback is added.

## One encoded body can serve several candidates

The accepted [grouped-read operation](../decisions/grouped-article-reads.md) adds
SearchAnyEvidenceGroupedFirst alongside the uncached API. It groups complete
expected object refs within the reader's stable Store, admits distinct article
bytes before GET and processes groups in first-occurrence order. Output is filled
at original ranked positions. Each logical passage still receives all manifest,
edition, decoding and canonical-context checks. No decoded state or byte buffer
is kept between calls. Returned text still counts every occurrence.

The synthetic three-path A,B,A fixture returns exactly the uncached refs, scores,
body and context with two body GETs instead of three. A later request makes two
again. A deliberately wrong profile on a second slot sharing bytes fails after
one GET and returns zero result, preserving logical provenance independently of
physical reuse. Separate budgets preserve the old per-hit API's meaning.

## Evidence and limits

Synthetic local PostgreSQL/HTTP publisher-reader checks cover exact body/context,
notice labels, stable source references, independently owned result slices,
cross-reader rejection before GET, inclusive article/text budgets, repeated shared
body charging, preflight rejection with zero candidate GETs, complete union overflow
before I/O even for a one-hit prefix, empty output, invalid budgets, later missing
reads and deterministic cancellation after an earlier success. The context fixture
returns body "Delay startup." followed by "startupProbe: enabled", headings
Pods/Checks and notice SlowStart; the label is not inserted into its body quote.

The real development input export is compared byte-for-byte with the original
frozen reranking bundle. That probe verifies the new posting union reproduces the
old candidate scores/order, while the unchanged offline context composition still
serializes the same model text. It is not an end-to-end real-corpus published-reader
context or worker test. No reserved queries are used for tuning or fresh relevance
claims. Full local checks require a dedicated PostgreSQL test database; no hosted
DB, live R2, inference or timing benchmark is part of this slice.

Private test logs and exact comparison artifacts stay under .tools. Revisit dense
scratch on measured query pressure, and repeated article decoding/planning if
its remaining cost becomes material after grouped downloads. This foundation does not establish production
model latency, readiness/cancellation behavior, private multi-user authorization
or million-document support.
