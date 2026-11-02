# Compare question-word removal and minimum overlap offline

Status: completed authorized offline experiment; product search remains unchanged.
Neither new rule is selected for production. The development set and controls are the workload; reserved questions
are not executed. The predeclared rules below are fixed before running either new
method. Do not sweep thresholds or edit the word list after inspecting results.

## Two rules, unchanged corpus and scorer

The current every-word baseline can require question filler such as why and my;
unrestricted any-word matching can admit passages sharing only incidental words.
Compare both existing baselines with exactly these two diagnostic methods:

- `question-words/all-retained-v1`: remove exact prepared query terms in the fixed
  list `a an are can do does how i is it my of on the to what when where which why`.
  Require every distinct remaining term with the existing Search and BM25. Remove
  these terms from scoring too; keep corpus term counts, lengths and statistics
  unchanged. An originally valid query that loses every term explicitly reports
  `all_terms_removed`, zero matches and no fallback. An invalid original query or
  failed search remains an error, never a relevance miss.
- `minimum-half/raw-terms-v1`: keep the original terms, require at least
  `max(1, (Q+1)/2)` distinct terms for Q distinct prepared terms, and sum the existing
  single-term BM25 contributions in lexical order. Integer division rounds this
  half threshold upward. A duplicated word neither increases the denominator nor
  its contribution. Missing words still count in Q; they contribute no match or
  score. No corpus-dependent threshold or common-word removal applies here.

For the illustrative prepared terms why/is/my/pod/pending, the first method searches
pod/pending; the second requires three of the five original words. Neither understands
negation, intent or synonyms. The small removal list preserves not/no/nor/without,
before/after and technical identifiers such as is.ready; preserving these tokens
is not a promise to interpret their meaning. The list can still remove meaningful
words such as can or when. These are experimental hypotheses, not recommended
production defaults or PostgreSQL-equivalent analysis.

Add explicit `-compare-noise` to tools/evalsearch, restricted to the development
split. It includes the unchanged every/any-word baselines and two tagged methods;
`-compare-any` alone and ordinary output retain their existing behavior. Keep the
16 MiB encoded-report bound and complete local fixture/reference preflight. Add
retained/removed distinct terms, required count and query status per new case so
failed eligibility can be explained without guessing from ranks. Every method's
first five hits retain exact article/body/heading/notice evidence through existing
public APIs; unjudged passages remain unknown. No schema/index/source processing,
rank formula, field weight, product flag or remote storage operation changes.

## Cost, ownership and failure boundary

Generalize the diagnostic union's private loop to accept a positive minimum while
keeping rankAny as its minimum-one wrapper. The complete every-word projection
must still reproduce the original multi-term result exactly. A threshold changes
eligibility only: eligible scores equal the original any-word scores, in identical
relative order. The removal method instead uses the same index Search with its
retained query and naturally changes the contributing score terms.

For P passages, Q distinct query terms, H summed single-term hits and U eligible
results, threshold composition uses O(P+Q) scratch and O(P+H+U log U) combination
work above Q existing single-term searches/sorts. Running the two comparisons
separately repeats those searches deliberately; share no result cache or new
production algorithm. The filter sorts T query occurrences in O(T log T), then applies a constant
twenty-word membership check in a linear pass; its returned slices are owned.
The new pair also repeats original query preparation and Search for its own validation. Borrow raw
queries, index state and article buffers unchanged through the synchronous call.
All capture buffers, edition index, per-hit decode/plan work and reports add memory;
this is small-corpus admission, not a whole-process heap or scalability claim.
No queues, goroutines, provider requests or timing reruns are introduced.

Cancellation, bad bindings, incompatible preparation, limits and evidence failures
return no report before the final write. The final output device can still accept a
prefix then fail. An intentionally empty filtered query has explicit status and is
not confused with such an error. Test independent scan/count eligibility, duplicate
and permutation invariance, missing words in the denominator, threshold boundaries,
owned filtering, all-removed status, stable score order, forbidden splits, reserved
preflight and checked output.

## Reading sample and comparison

Reproduce the existing baseline and any-word case fields exactly before interpreting
new results. Run 120 development cases and 18 controls per method on the unchanged
frozen set SHA-256
`12debe31676faddc2ed38d33228ce5df0d3c2179b268eb1a19596772ba7d342f`.
Report known-at-one/five, zero results and candidate counts by edition and phrasing.
These incomplete known-positive counts are not exhaustive recall or precision.

Use the previous fixed sample: edition 1.35, intents pod-pending, termination-default,
init-probes, qos-guaranteed and pod-hostname, both phrasings. Pool the first five from
all four methods by intent, edition, exact location and article binding. Hide method,
rank and score; use a deterministic hash order. Carry a previous reviewed grade only
for the same need and exact body/context evidence; grade every new pooled passage
2 decisive, 1 related-only or 0 unrelated/misleading, then independently review all
new grades and the carry-forward bindings. This is a small author-read sample with
an automated independent check, not blinded human judgment or representative traffic.
Keep frozen positives unchanged even if the new pool reveals additional useful text.

## Primary mechanisms and next boundary

PostgreSQL 16's [dictionary documentation](https://www.postgresql.org/docs/16/textsearch-dictionaries.html)
explains common-word removal and its effects. The pinned REL_16_0
[English word list](https://github.com/postgres/postgres/blob/c372fbbd8e911f2412b80a8c39d7079366565d67/src/backend/snowball/stopwords/english.stop)
contains our twenty words but also meaningful qualifiers such as not and before.
Our much smaller query-only subset deliberately avoids adopting its entire analysis
pipeline; it does not change indexed length/statistics or stem technical terms.

Elasticsearch 8.19's [minimum-match documentation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/query-dsl-minimum-should-match.html)
and pinned [Queries.calculateMinShouldMatch](https://github.com/elastic/elasticsearch/blob/93788a8c2882eb5b606510680fac214cff1c7a22/server/src/main/java/org/elasticsearch/common/lucene/search/Queries.java)
show a separate clause-eligibility requirement. Its positive percentages round down;
our explicit at-least-half rule rounds up. Its analyzer, optional clauses and Lucene
execution differ from distinct technical terms and our additive combined-field score.
Neither upstream establishes our list, threshold, relevance or latency.

The simplest alternative is to retain the existing two baselines and do no tuning.
Compare whether either fixed control preserves useful first-page evidence while
reducing weak matches. Discuss the observed failures with the user before changing
product behavior, adding new thresholds, a learned query rewriter or semantic search.


## Recorded outcomes

The complete original every/any-word report, excluding only the new noise_controls
section, reproduces the prior report exactly. The same 120 development cases and
18 controls ran per method; no reserved query ran. No fixed rule changed after the
run, and no timing was measured.

| Rule and phrasing | Known at one | Known within five | Zero matches | Median candidates |
| --- | ---: | ---: | ---: | ---: |
| Every word, keywords | 9/60 | 38/60 | 4/60 | 4.5 |
| Any word, keywords | 7/60 | 40/60 | 0/60 | 66.5 |
| Remove question words, keywords | 9/60 | 38/60 | 4/60 | 4.5 |
| At least half, keywords | 7/60 | 40/60 | 0/60 | 22 |
| Every word, ordinary questions | 0/60 | 0/60 | 60/60 | 0 |
| Any word, ordinary questions | 10/60 | 41/60 | 0/60 | 566 |
| Remove question words, ordinary questions | 0/60 | 3/60 | 57/60 | 0 |
| At least half, ordinary questions | 7/60 | 26/60 | 0/60 | 7 |

Question-word removal leaves every keyword case unchanged in this set. The small
list does not solve vocabulary mismatch: for the ordinary init-probe question in
1.35 it leaves container/have/init/liveness/or/ordinary/probes/readiness, requires
all eight, and finds nothing. The hostname question retains `give` and `and` alongside
hostname/pod/subdomain; no passage contains all five. In the Pending question it
retains in/pending/phase/pod, finds two passages and puts the frozen answer second.
No real evaluation query loses all terms; that outcome is exercised synthetically.

The half threshold's ordinary-question candidate range is 1..119, versus 354..710
for any-word. It reports zero matching controls out of 18, versus 15 for any-word.
Those controls do not establish an abstention guarantee: matching half of incidental
words can still exclude an answer or admit another topic. In 1.35 the shutdown
question requires eight of sixteen words and returns six candidates, but none is
one of its frozen useful references. The ordinary init-probe answer falls from
rank 20 with any-word to rank 14 with half, still outside the first five; the QoS
answer moves from rank five to four. These are query-specific outcomes, not an
estimate of expected production gains.

The fixed sample pools 45 distinct need/location/article passages with 140 displayed
occurrences. Full need/body/context objects exactly reproduce all 41 prior pooled
passages before carrying their reviewed grades. Four new passages were read and
independently approved: unrelated resource-killing and kubelet-restart defaults for
the shutdown need, related BestEffort background for Guaranteed criteria, and a
decisive hostname-field setup for the hostname part of that need. Scores and method
membership were hidden during initial reading; unblinding produced these counts:

| Sampled rule and phrasing | Decisive (2) | Related only (1) | Unrelated (0) | Displayed hits |
| --- | ---: | ---: | ---: | ---: |
| Every word, keywords | 8 | 10 | 2 | 20 |
| Any word, keywords | 8 | 13 | 3 | 24 |
| Remove question words, keywords | 8 | 10 | 2 | 20 |
| At least half, keywords | 8 | 13 | 3 | 24 |
| Every word, ordinary questions | 0 | 0 | 0 | 0 |
| Any word, ordinary questions | 3 | 17 | 5 | 25 |
| Remove question words, ordinary questions | 1 | 1 | 0 | 2 |
| At least half, ordinary questions | 4 | 16 | 5 | 25 |

The sample provides no clear relevance win for half overlap: four decisive displayed
occurrences versus three for any-word is one small correlated sample, while the
full known-positive comparison loses fifteen successes within five. For example,
half overlap's shutdown results still contain resource-killing and kubelet-restart
readiness text, which supply no default graceful-shutdown duration. Zero matching
scope controls therefore does not mean the returned first page has no noise.
Question-word removal is mostly silent rather than broadly useful.

Keep product matching unchanged. These fixed controls demonstrate a trade-off,
not a production default. Discuss which ordinary-question failures to address next
before trying other lists, thresholds, combined policies or semantic retrieval.

## Reproduction and evidence

Run from a checkout with the retained trusted capture and frozen set available:

```sh
go run ./tools/evalsearch -capture /path/to/capture -set /path/to/golden-v1.json -compare-noise
```

The two new tagged methods include retained/removed terms and required counts;
-compare-noise also includes both prior baselines. It rejects reserved/all splits.
The result is bounded diagnostic output, not the product's machine interface.
The frozen private plan SHA-256 is
`81f4bc4c59084363f222c04a861e304702dfbff0e1192debb97db155e73700cc`;
the complete comparison SHA-256 is
`bc528437f8c268d1f0d71481c29ebf6bf903891af885109284dd03ed18f288ed`.
Primary records are under ignored `.tools/evaluation/query-noise/`: plan and hash,
comparison, run-hashes and exact run-source, pool and memberships, carried/new author
grades, reviewed-grades and sample-summary. The reviewed-grades SHA-256 is
`138bdb6fedb44618fd99adf201b59c4d0b366f78d0fdd375447449424ba64740`.

The run used Go 1.27.2 on darwin/arm64 without database/provider access. Source tests
also exercise a real local retained-capture/edition pipeline, independent scan BM25
scores plus flat-field distinct membership across every threshold, duplicate terms,
missing terms in the denominator, exact technical/qualifier retention, slice ownership,
all-removed status, explicit invalid minima, forbidden splits, bad reserved reference
preflight, cancellation, evidence identity and checked output. The full local verify
uses a dedicated native PostgreSQL 14.17 database for the existing repository tests;
it is separate from the database-free retrieval experiment. No hosted tests or
provider latency claims follow.
