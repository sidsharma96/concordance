# First known-reference retrieval baseline

The reviewed set contains 30 authored question intents, two phrasings and three
pinned Kubernetes documentation editions, plus six scope controls. The first run
uses only the 20 development intents: 120 query runs and 18 controls. The reserved
10 intents account for 60 planned runs and were not queried. Their source bindings
were checked during full preflight. This is an offline reconstructed-index audit,
not a hosted CLI, R2, throughput or representative-user benchmark.

## Frozen inputs and method

Set SHA-256: `12debe31676faddc2ed38d33228ce5df0d3c2179b268eb1a19596772ba7d342f`.
Source question inventory is [retrieval-questions.md](../corpora/retrieval-questions.md).
The machine set remains outside source history under the repository artifact policy.
The retained descriptor, source Git pins, 38 canonical article hashes, processors
and 120 known-useful passage references are bound by that set. Quotes and ranges
for every reference were verified before any query, including reserved cases.

The independent label review read all 30 intents, 90 edition mappings and 120
actual retained passage excerpts. Three unsupported rationale clauses were removed;
query texts, reference addresses, pins and split membership stayed unchanged.
A suggestion to narrow the long preStop reference was rejected: its 17 blocks form
one canonical outer-container passage, so a shorter fixture range would invent a
passage unavailable in the index. A known useful hit can still be verbose.

A private hash-projection helper initially mutated nested data and dropped rationale
fields. The first audit failed at set validation with empty output before querying.
That rejected serialization and its failure log are retained. The corrected helper
copies before projecting and preserves the reviewed query/reference projection.
The valid frozen set above predates the first actual search results.

`tools/evalsearch` reconstructs the exact articles and existing canonical
`htmledition` counts/locations, then uses the unchanged `textterms.Prepare` and
complete match-all/BM25 search. Preflight builds one edition at a time and discards
it; the query phase rebuilds it rather than retaining three loaded indexes.
Reference validation decodes a whole article for each reference occurrence.
These are explicit CPU/memory costs, not measured performance conclusions.

## Observed development results

Known-at-one/five means at least one frozen useful reference appears within that
rank. The reference pool is incomplete; unknown hits are unjudged, not irrelevant.
These are known-reference discovery counts, not exhaustive answer accuracy,
precision, recall, confidence or a promise of a complete answer.

| Edition | Phrasing | Runs | Known at one | Known at five | Zero matches |
| --- | --- | ---: | ---: | ---: | ---: |
| 1.34 | Keywords | 20 | 4 | 13 | 1 |
| 1.35 | Keywords | 20 | 3 | 13 | 1 |
| 1.36 | Keywords | 20 | 2 | 12 | 2 |
| 1.34 | Ordinary question | 20 | 0 | 0 | 20 |
| 1.35 | Ordinary question | 20 | 0 | 0 | 20 |
| 1.36 | Ordinary question | 20 | 0 | 0 | 20 |

Descriptively, keyword variants found known evidence within five in 38 of 60 runs;
all 60 ordinary-question variants returned no matches. Repeated editions/phrasings
are correlated and no statistical generalization is made. All 18 scope controls
also returned no lexical matches; this does not add an abstention policy to search.
No ranking, query preparation or reference label changed in response to the result.

For example, `Pending` in edition 1.34 returns six matches and its first frozen
reference is at rank six. The ordinary question `Why is my Pod in the Pending phase?`
returns zero matches. The first five keyword hits are unjudged by this set; they
may contain useful evidence. The corpus keeps the source passage answering the
question, but the current all-term eligibility rule need not admit it for a
longer query. BM25 only orders eligible passages and cannot restore an excluded
candidate. This locates the next investigation without establishing which query
policy or semantic model would improve results.

## Reproduction and limits

With the retained local capture and reviewed set available, from the checkout:

```sh
go run ./tools/evalsearch -capture /path/to/capture -set /path/to/golden-v1.json
```

Development is the default split. Do not run reserved or all while tuning on these
cases. The diagnostic report includes the set/capture hashes, processing/ranking
tags, full-match counts, first-known ranks and first-five addresses with explicit
`known_useful` or `unjudged` judgments. All selected operations must succeed before
reporting; malformed/misbound fixtures, query errors and limits are not misses.
No PostgreSQL, cloud credentials or remote writes are involved.

Primary private records: `.tools/evaluation/golden-v1.json`,
`development-v1.json`, `.tools/reviews/eval-freeze.txt`,
`eval-labels-baseline-1.json`, `eval-first-run.log`, `eval-development-run.log`
and `eval-verify-1.log`. The local verification uses Go 1.27.2 on darwin/arm64 and
dedicated PostgreSQL 14.17 for ordinary repository tests; the audit itself uses no
SQL or provider access. No timings were rerun or quoted. Broader label coverage,
actual user questions, held-out validation and deployment/scale faults remain open.
See the [evaluation contract](../decisions/retrieval-evaluation.md).
