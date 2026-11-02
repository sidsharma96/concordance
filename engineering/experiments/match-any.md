# Compare every-word and any-word retrieval offline

Status: authorized experiment, not a product matching-policy change. Keep the
existing MatchAll APIs, published readers, CLI and immutable artifacts unchanged.
The frozen development set and scope controls are the workload; reserved intents
are never queried by this comparison. This record starts with the predeclared
method; results follow only after implementation checks.

## Mechanism and invariants

Add an explicit development-only `-compare-any` mode to `tools/evalsearch`. Run the
existing complete every-term search, then combine the existing single-term searches
for the distinct query terms in lexical order. Sum each returned term contribution
by passage position and sort the union by descending score, then input position.
BM25 is an additive term sum, so this produces an any-term ranking with the same
preparation, field counts, edition statistics, k1/b and tie rule. A missing term
contributes zero; it does not exclude the passage. No term removal, query rewriting,
weights, semantic model, passage regrouping or new corpus statistics are introduced.
The experiment gets its own policy tag; the production BM25 version stays intact.

Check the complete-intersection projection against the existing multi-term search:
same positions, exact scores and stable order. Test missing terms, empty/no-match
queries, duplicates/permutations, shared article bytes under separate logical
paths, ties and cancellation. Keep the simple scan BM25 implementation as a score
oracle for synthetic corpus cases. Any failed binding/query/check is an error with
no report, never a relevance miss. The comparison accepts only the development
split and retains the existing complete fixture preflight, including reserved
reference binding checks that do not execute reserved queries.

For Q distinct query terms and H summed single-term result lengths, the union
retains at most P admitted passage scores and sorts U union candidates. Additional
combination work is O(P + H + U log U) with O(P + Q) scratch, above Q ordinary single-term
searches and their own validation/sorts. This is a deliberately simple experiment,
not a scalable union-postings implementation or latency comparison. Candidate
counts and summed single-term hits describe structural work, not measured CPU.
Source buffers, decoded articles, indexes, quoting scratch and encoded reports add
memory. No remote I/O, queues or background workers are introduced.

## Evaluation and reading cost

Use the frozen set hash and capture from the first baseline. Reproduce its existing
120 development query cases and 18 controls exactly before interpreting the trial.
Report per edition/phrasing the same known-at-one/five and zero-match counts, plus
complete candidate counts. Unknown passages remain unjudged; a newly retrieved
useful passage must not silently change the frozen label set. Controls may now
match incidental words; that is diagnostic noise, not a broken abstention promise.

Expose the full retained body quotes for the first five results in comparison
mode, with exact logical locations and article bindings. Include ancestor headings
and nonempty group labels as separate context arrays, never part of the verbatim
body quote. To obtain them, decode the exact canonical article with
`htmlarticlejson.Decode`, rebuild its plan through the existing public
`htmlpassage.Build` using the same build limits, and require exactly one plan body
range equal to the saved index range. Resolve that body's heading indices and group
indices into the decoded article's text and nonempty labels. Fail on a missing or
ambiguous range. Thus the independent judgment evidence contains the complete body
and exact context, rather than depending on an inaccessible private index field.
This repeats decoding/plan construction per displayed hit, with the existing
article/plan bounds and the final report ceiling; report that extra local work
rather than claiming these diagnostic quotes are free. These remain diagnostic
results, not a product top-k or citation schema. Record summed body bytes of those
five quotes as reading volume, not remote transfer, attention or quality. Keep the
16 MiB encoded report ceiling and checked final write; errors return no report.

Before seeing rankings, select development intents at zero-based positions
0, 4, 8, 12 and 16 in authored fixture order, edition 1.35, both phrasings:
`pod-pending`, `termination-default`, `init-probes`, `qos-guaranteed`, `pod-hostname`.
Pool the union of both methods' first five results by intent and exact location,
using the complete retained body plus ancestor heading/notice context when present.
Hide method, rank and score and order the pool by a fixed content hash. This is a
small systematic sample, not representative traffic or true blinded human testing.
The author knows the corpus and hypothesis; an independent review checks the
judgments and conclusions against the supplied actual passages.

Use grades 2 (directly supplies a correct, decisive part of the information need),
1 (related background or incomplete evidence without the decisive answer), and
0 (unrelated or misleading). Correct edition and material conditions are required.
Grade every pooled passage; unjudged is not zero. Report grade counts per sampled
case and method and show supported examples, including failures. Exact duplicate
passages may be shared across phrasings of the same information need; do not
silently transfer labels across different needs or editions. Preserve the original
known-reference counts independently of these new sampled judgments.

## Primary comparison and limits

Elasticsearch 8.19's [match-query documentation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/query-dsl-match-query.html)
allows OR/AND eligibility and minimum-match controls; its
[similarity documentation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/index-modules-similarity.html)
describes BM25 independently. Those are version-pinned production search examples
of separating candidate eligibility from scoring. Its analyzer, per-field/distributed
statistics and Lucene execution differ from this small combined-field experiment.
The [pinned builder](https://github.com/elastic/elasticsearch/blob/93788a8c2882eb5b606510680fac214cff1c7a22/server/src/main/java/org/elasticsearch/index/query/MatchQueryBuilder.java)
sets DEFAULT_OPERATOR to OR and applies the configured occurrence/minimum-match
settings in doToQuery. They do not establish our relevance, preferred threshold
or performance. Our pinned
baseline `d6e7b19c29faf4816169067a545ba993b27ca5cd`,
`internal/termsearch/bm25.go:RankBM25` and `index_bm25.go:Index.RankBM25` supply the
actual additive formula used here; no Elasticsearch dependency is added.

The simplest alternative is to keep only the every-word baseline. It cannot answer
whether its zero-result ordinary questions contain useful lexical evidence. Any-word
matching can surface that evidence while admitting far more weak matches. Read the
sampled results before proposing a product policy. Thresholds, common-word removal,
semantic retrieval, top-k and optimized union postings require separate checkpoints.
No timing is rerun or claimed on a shared machine. Scope remains the 38 retained
Kubernetes document versions, local reconstruction and incomplete known positives.

## Recorded development results

The existing baseline's complete case fields, first-known ranks, first-five
addresses/scores/judgments and summaries reproduce the frozen earlier report
exactly. The comparison queries only 120 development cases plus 18 controls per
method. Every all-word projection check passed. The same frozen set is used:
`12debe31676faddc2ed38d33228ce5df0d3c2179b268eb1a19596772ba7d342f`.

| Phrasing and rule | Known at one | Known within five | Zero matches | Median candidates |
| --- | ---: | ---: | ---: | ---: |
| Keywords, every word | 9/60 | 38/60 | 4/60 | 4.5 |
| Keywords, any word | 7/60 | 40/60 | 0/60 | 66.5 |
| Ordinary questions, every word | 0/60 | 0/60 | 60/60 | 0 |
| Ordinary questions, any word | 10/60 | 41/60 | 0/60 | 566 |

For any-word keywords, known-within-five counts by edition 1.34/1.35/1.36 are
14/20, 13/20 and 13/20. For ordinary questions they are 14/20, 14/20 and 13/20.
Keyword candidate totals grow from 273 to 7,132; ordinary questions admit 32,798
candidates versus zero. Any-word candidate ranges are 3..512 for keywords and
354..710 for ordinary questions. These are correlated runs over three editions,
not independent user observations or exhaustive accuracy/recall measurements.

Fifteen of 18 scope controls now return lexical matches, compared with zero for
every-word matching. The deliberately absent identifier still returns zero in all
three editions. Matching `lifecycle` or `state` in Pod material cannot answer an S3
expiration or Terraform locking question. These controls expose incidental word
matches, not a violated product abstention contract or semantic interpretation.

The sum of displayed first-five body bytes across the 60 keyword cases is 75,610
for every-word and 75,478 for any-word. Ordinary-question any-word bodies total
95,545 bytes versus no displayed results before. These are retained body UTF-8
bytes, excluding headings, labels and report encoding, not measured attention or
remote traffic. Full-result work grows much more than these first-five views show.
The current product's SearchQuoted invokes a strict article read for every hit,
including repeated articles. For the 619 any-word candidates in the 1.35 Pending
question, porting this rule unchanged to that path would imply 619 article reads
plus root/index opening reads, if all succeeded. This is derived from code and
candidate count; no provider experiment or latency claim was run.

## The fixed reading sample shows both gains and noise

The predeclared five-intent, edition-1.35 pool has 41 distinct intent/location
passages and 69 displayed occurrences across both phrasings/methods. The author
read every full body with its actual heading/label context, assigned 2/1/0 grades,
and a separate read-only baseline review approved all 41 supplied judgments.
Method/rank/score were hidden in the hash-ordered pool; the author knew the corpus
and hypothesis. This is automated reviewer-checked author assessment, not blinded
human relevance evaluation or representative traffic.

| Sampled phrasing and rule | Decisive evidence (2) | Related only (1) | Unrelated/misleading (0) | Displayed hits |
| --- | ---: | ---: | ---: | ---: |
| Keywords, every word | 8 | 10 | 2 | 20 |
| Keywords, any word | 8 | 13 | 3 | 24 |
| Ordinary questions, every word | 0 | 0 | 0 | 0 |
| Ordinary questions, any word | 3 | 17 | 5 | 25 |

The Pending question now finds the general phase-definition table at rank three
in 1.35, explaining scheduling and image downloads. But the first two hits give
lifecycle background. The ordinary init-probe question returns 452 candidates and
ranks the frozen answer twentieth; its first five are related links/background,
while the short keyword variant has a direct answer first. The ordinary termination
question returns 608 candidates but its first five supply no decisive default
shutdown duration; its first frozen reference is at rank 65. More matches alone
are not a better first page.

The hostname question supplies a useful configuration example second under the
any-word ordinary phrasing. The QoS ordinary phrasing puts a decisive container
request/limit criterion fifth, behind other classes/background. Even the original
keyword termination results contain a decisive 30-second passage not in that
intent's frozen positive list. Keep that grade separate instead of modifying the
frozen fixture to improve its known-reference score. This illustrates why those
scores are not exhaustive relevance judgments.

## Reproduction, records and next boundary

From the source checkout with the existing trusted capture and set available:

```sh
go run ./tools/evalsearch -capture /path/to/capture -set /path/to/golden-v1.json -compare-any
```

The flag permits only the development split. It emits the unchanged baseline case
fields plus a tagged any-word section and complete top-five body/context evidence.
One invocation runs both methods; no reserved query, database, provider request,
import, target activation or automatic tuning occurs. A final writer error can
still leave partial output; logical errors fail before that final write.

Primary ignored artifacts are `.tools/evaluation/match-any/plan.json`,
`plan.sha256`, `comparison.json`, `run-hashes.json`, `pool.json`,
`pool-memberships.json`, `author-grades.json`, `reviewed-grades.json` and
`sample-summary.json`. Pool construction/judgment helpers are private disposable
probes, not required contributor workflows or production components. The actual
comparison SHA-256 is
`439ac7e25f9dab36f279f1b7dd486abfa40264c04ec7e3e888cf3b600888bf1d`;
reviewed-grades SHA-256 is
`6dbb227b520161daed89666c8207a9976a18079b4017e195fd3e6dd3a0f56904`.
The run used Go 1.27.2 on darwin/arm64 with no database or network. No timings were
measured or rerun. The experiment's source hashes identify the exact run code;
the exact run sources are retained under `run-source/` and their staged tree in
`run-source-tree.txt`. Later changes add tests, clarify a command comment and
remove an unused wrapper, without changing ranking or performing a tuned run.

The design review first identified missing context provenance for reproducible
judgments. The amended public Decode/Build path was independently approved before
implementation; a synthetic notice-label match now checks that context stays
outside the verbatim quote. Tests also compare union candidates/scores with the
existing scan oracle, check exact every-word projection, duplicates/permutations,
missing terms, ties, distinct logical paths sharing article bytes, cancellation,
reserved binding preflight, development-only admission and checked output.

Keep product search unchanged. The result supports testing broader eligibility,
but does not support adopting unrestricted any-word matching as the default.
Before a product change, discuss candidate eligibility together with complete
output/strict quote costs, and compare credible noise controls on development
queries without using the reserved set for tuning. This experiment selects no
threshold, word list, mode, top-k, semantic model or optimized union algorithm.
