# Combined-field BM25 baseline

## Workload and invariants

`internal/termsearch.RankBM25` implements the accepted
[combined-field policy](../decisions/lexical-ranking.md). It reuses `MatchAll` to
admit/validate all caller input and select complete matches in one exact edition.
No stored index, raw-text query command, network work or publication is introduced.

The score combines body, ancestor headings and labels without weights. Statistics
cover all selected-edition passages, including empty ones. Other editions affect
input admission but not scores. Query duplicates and permutations produce identical
scores; lexical summation order fixes rounding order. Exact ties use input position.
Output owns scored positions, retains no input, and needs the caller's unchanged
source associations. A missing edition differs from a present empty edition.

Every failure returns a zero result, including excessive complete matches and
cancellation after scoring/sorting. Logical ceilings and a required deadline bound
admitted work, not heap usage or instant interruption. Standard-library sorts finish
before a cancellation is observed. No implicit top-k or partial-match fallback.

## Representation and cost

A dense query-term map and reusable integer array count frequencies for one passage
at a time. The first selected-edition scan collects document frequency and total
length; the next revisits only matches for scoring. No full-vocabulary map or P by Q
frequency matrix is retained. `math.Log1p` computes positive IDF accurately near zero.
Full sort orders the complete score slice and is the baseline for later top-k work.

For A admitted bytes/items, T selected term occurrences, P selected passages, U
provided query occurrences, Q distinct query terms and M matches, expected work is
O(A + T + P*Q + M*Q + Q log Q + M log M) under ordinary hash assumptions, with string
comparison/validation costs additionally dependent on admitted byte lengths. Working
storage is O(editions + U + Q + M), including MatchAll's transient map reservation
for supplied query occurrences. Ranking's own map grows only with distinct terms.
It creates term-name, count, frequency, IDF and scored-output slices, plus match
positions already returned by MatchAll. This is a logical model, not measured heap
or allocation totals. No remote requests or encoded storage are involved.

## Checks and scoped observations

Tests check independently evaluated four-passage scores, selected-only statistics,
field equivalence, exact ties, frequency saturation, length adjustment, unchanged
input/independent output, failures and cancellation at every Err visit, including
the final check after sorting. A deliberately slow oracle independently rescans
passages for each distinct query term and insertion-sorts matches. Two hundred
fixed-seed small generated corpora agree within a stated numerical tolerance.

A nominal five-second, two-worker fuzz run completed 286,288 executions in 5.367
seconds without disagreement. Inputs are bounded to 48 bytes and mapped to four
small passages; this is not arbitrary hostile-input coverage, relevance evidence
or a performance benchmark.

A private composition audit uses the unchanged retained Kubernetes source hashes:
38 articles, 1,889 passages and 84,053 prepared term occurrences across three pinned
editions. It passes extracted articles through the current public HTML adapter and
technical-term grammar. All 28 existing query/edition cases and 430 scored case hits
agree with a separate oracle that materializes every passage's full vocabulary and
computes its statistics independently. Source associations and score order are checked,
not just hit counts. No reader judgments, latency, allocations or stored index size
are measured. No objects/catalog writes or generation publication occur.

Local full verification uses Go 1.27.2/macOS arm64 and dedicated PostgreSQL 14.17;
record the actual final outcome in the PR. Live R2 remains opt-in and is not needed
for this in-memory change. Revisit field weights with current-grammar queries and
reader judgments; revisit scans/full sort using measured work and a correctness
comparison on identical inputs. Earlier private BM25F results used an older grammar
and do not establish quality for this implementation.
