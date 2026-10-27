# Rank complete matches with BM25

## Decision

The user chose combined-field BM25 as the first ranking baseline. Keep the accepted
`MatchAll` eligibility rule: every distinct prepared query term must occur in a
passage's supplied body, ancestor headings or labels. Ranking orders these complete
matches; it never admits a partial match or substitutes another edition.

Count each supplied term occurrence once across the three fields, with no field
weights or concatenated text copy. Repeated authored terms and repeated context
count toward both frequency and length. Retain the original separate fields for a
later BM25F comparison. The earlier private field-weight experiment did not add
selected direct answers in its top five, but used an older text grammar and a small,
author-judged sample; it does not establish general equivalence or ranking quality.

## Exact scoring policy

For each distinct query term, add:

```
idf = ln(1 + (N - df + 0.5) / (df + 0.5))
score contribution = idf * tf * (k1 + 1) /
    (tf + k1 * (1 - b + b * length / averageLength))
```

Use fixed `k1 = 1.2` and `b = 0.75` initially. `tf` counts that term's occurrences in
this passage; `df` counts selected-edition passages containing it. `N` includes all
passages in that exact edition, including empty passages. Average length is total
supplied term occurrences divided by `N`. Compute these statistics over the whole
selected edition, not just matches and never other editions. An edition without
matches requires no score calculation, avoiding zero-length division.

This positive IDF variant and initial parameters are documented in Apache Lucene
9.12.3's [BM25Similarity](https://lucene.apache.org/core/9_12_3/core/org/apache/lucene/search/similarities/BM25Similarity.html).
The saturation factor includes `k1 + 1`; scores are not claimed byte-for-byte Lucene
scores (no encoded norms, analyzer or overlap-token policy). The conceptual source is
Robertson and Zaragoza's [BM25 review, sections 3.4–3.6](https://www.staff.city.ac.uk/~sbrp622/papers/foundations_bm25_review.pdf).
Scores are relative ordering values, not probabilities or comparable confidence
across queries/editions. Rare terms contribute more, repeated occurrences have
diminishing returns, and longer-than-average passages need more evidence.

Expose this policy with `BM25Version = "bm25/combined-v1"`. Process distinct query
terms in lexical order, ignoring duplicate query occurrences, to fix floating-point
summation order. Sort by decreasing score, breaking exact ties by ascending input
passage position. Return the complete bounded match set, not an implicit top-k.
Version changed formulas, parameters, statistics or tie semantics deliberately.

## Component and limits

Add `termsearch.RankBM25(ctx, Corpus, Query, Limits) (RankedResult, error)` beside the
existing scan. Reuse `MatchAll` for admission, validation and eligibility instead of
duplicating that contract. `RankedResult` carries edition presence, ranking version
and owned scored passage positions. Positions still refer to this exact caller input;
no durable IDs, authorization, citation lookup, persisted index or CLI is introduced.

The caller keeps authorized input immutable until return. Require the same positive
limits and deadline. `MaxMatches` bounds the complete output, not truncation. Any
limit, malformed/version-incompatible input, or cancellation returns a zero result.
No I/O, goroutines, shared cache or new dependency. Cancellation is cooperative:
check term visits and phase boundaries; standard-library bounded in-memory sorts
finish before cancellation is returned. No hard wall-clock or heap guarantee.

First enumerate matches. Then collect document frequencies and total length from
all selected passages using a query-term map and reusable count array. Rescan only
matching passages for scoring, avoiding a passage-by-query matrix. All counts fit
admitted integer occurrence limits before conversion to float64. Scores must remain
finite and positive for qualifying passages. Empty/missing/no-match results preserve
edition presence with no scores. Output does not retain input slices or strings.

For A admitted input bytes/occurrences, T selected occurrences, P selected passages,
Q distinct query terms, M matches: expected work is O(A + T + P*Q + M*Q +
Q log Q + M log M), including map operations under ordinary hash assumptions.
Scoring revisits at most T occurrences. Additional memory O(editions + U + Q + M) for U supplied query occurrences,
including MatchAll's transient admission/match buffers (its map reserves U entries); no persisted statistics or
remote requests. Map worst cases and string validation/comparison costs mean this
is not a strict worst-case constant-time lookup claim. Full sort is the oracle for
future bounded top-k; scans are the oracle for future postings.

## Alternatives and revisit

Raw occurrence counts let repeated words and long passages dominate. BM25 adjusts
these effects with a small explicit formula. BM25F instead normalizes each field and
weights body/headings/labels before saturation; that adds useful control but needs
judged examples to tune. Keep the simple combined baseline and compare on the same
eligible candidates if generic headings dominate or independent reader judgments
show missing useful answers. Neither formula understands truth or answer completeness.

Test formula values independently, saturation/length effects, rare-term statistics,
all-edition validation but selected-only statistics, duplicate/permuted queries,
empty/missing editions, exact ties, input ownership, failures and cancellation.
Compare generated small corpora against a deliberately simple independently counted
full-sort oracle. Any real-corpus audit must use current technical terms, preserve
source identities and distinguish mathematical agreement from relevance evaluation.
