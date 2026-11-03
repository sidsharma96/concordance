# Validate the frozen reranker on the reserved intents

Status: completed the user-authorized frozen validation on the ten reserved
information needs, both phrasings and all three pinned editions: 60 cases. The fixture's stored split label remains reserved and its hash is unchanged;
after release it is observed validation evidence, never an untouched tuning set.
The same queries may be recomputed for exact artifact verification and compatible
recovery, not to compare altered settings or select a new policy.

## Freeze before release

Keep the complete development configuration from
[the local trial](local-reranking.md): BGE revision
`953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e`, weights/tokenizer, runtime and dependency
versions, float32/MPS/SDPA, batch one, two intra-op/one inter-op threads, no truncation,
same Heading/Notice/Body serialization, first 100 diagnostic any-word candidates,
raw logits and lexical-rank ties. No model, window, threshold, query processing,
passage/index/statistics, label or score-fusion changes based on validation outcomes.

The local producer changes only artifact-directory and expected split/count checks.
Preserve its original source and a minimal diff; verify model loading, tokenization,
scoring, pilot/resource gates and checkpoint code are unchanged. Its source hash
therefore changes honestly while the model configuration does not. Record both
producer fingerprints, model files, runtime and dependency-lock fingerprints.
Use a separate validation directory/checkpoint to prevent mixing development work.

## Explicit validation route

The implemented `-validate-reserved` switch is allowed only with `-split reserved`
and exactly one rerank mode. Ordinary rerank modes remain development-only.
The existing compare-any and compare-noise modes continue to reject reserved/all;
no rerank mode admits all. This explicit trusted-operator switch expresses intent,
not single-use enforcement or authorization of future tuning.

Keep the development bundle/report bytes unchanged. A reserved bundle adds
`validation_split: reserved`, so its fingerprint and result identify the scope.
Candidate acquisition uses only the selected reserved intents; no development
queries or controls are executed in this mode. Full capture/reference preflight,
including both splits' bindings, is retained. Score validation, model input, rank
formula, window and raw-score interpretation are unchanged.

Export and score every selected first-100 pair; pair count is determined by those
fixed candidates, not used to choose a new threshold. Freeze available device
before inference, matching the completed development device. Tokenize all pairs
without truncation. Pilot the five longest, pair-ID ties, with unchanged 8,192-token
and separate 6 GiB RSS/driver gates. Stop on an oversized input, resource failure
or incompatible runtime rather than altering the configuration. Compatible missing
work may resume, but a partial checkpoint is never a completed validation.

## Compare without tuning

Report every-word, any-word and reranked known-reference counts at one/five, first
known ranks, zero matches and candidate-window coverage separately for each phrasing
and edition. A known reference excluded before reranking is a candidate failure,
not a model ranking failure. Include gains and regressions. Unknown hits remain
unknown; errors are errors, never relevance misses. These ten needs repeated across
versions are not 60 independent users or an enterprise sample.

For qualitative reading, select every other reserved intent in frozen fixture
order, starting with the first: five needs, edition 1.35, both phrasings. Pool the
three methods' first five by exact need, location and article. Hide method/rank/score
and use fixed hash order. Grade full body/context 2 correct decisive part with
material conditions, 1 related-only or 0 unrelated/misleading, then independently
review every grade. Do not modify the frozen positive list. The author knows the
project and evaluation hypothesis; this is automated-review-checked author judgment,
not blinded human validation.

Keep original plans, inputs, scores, reports and rejected/amended label judgments.
Do not rerun modified settings against the released set and still call it unseen
validation. Any subsequent model/configuration work needs a fresh evaluation plan
and new untouched questions for an independent validation claim.

## Cost, failure and primary examples

Algorithm/ownership/byte limits remain those of the local trial: full lexical
ranking, bounded window and repeated exact quote/context work, trusted score-file
producer, 128 MiB export, 8 MiB scores, 16 MiB report. The extra split marker changes
metadata and its fingerprint only; it changes no model input or scoring. No cache,
queue, ML Go dependency, timing benchmark, hosted call, deployment or storage write
is introduced. Context/file I/O remains cooperative, and final writer errors can
leave bytes even though logical failures produce no report.

The pinned [BEIR loader](https://github.com/beir-cellar/beir/blob/ef83d29307061c65d04b035b4f4e7c18bd8374af/beir/datasets/data_loader.py)
keeps corpus, queries and judgments separate and selects a split. Scikit-learn's
[validation guide](https://scikit-learn.org/1.9/modules/cross_validation.html)
explains why test data must remain outside model selection. Our model is pretrained;
this split limits our own policy selection, not proof that pretraining never saw
the public material or authored questions. Neither source establishes our corpus's
representativeness or supplies the incomplete judgments.

Record the outcome before proposing production integration. Product search remains
unchanged; release of this validation set does not approve a product policy,
new model service or subsequent tuning.

## Recorded validation results

Implementation source `8a00fd034b7927285b04ccf4598983c4f661623d` was independently
reviewed and merged in PR 50, merge `5287a7e2ffe1057a04f7037a2fff2b8a40360b76`.
Before first reserved export, the original development input bundle and complete
report reproduced byte-for-byte using the original scores. The plan below was
frozen before the reserved export. Full validation completed all 4,169 pairs:
1,169 keyword and 3,000 ordinary-question pairs, plus five longest-pair pilot calls.
No development query or control was executed by the reserved route. No model,
input, window, runtime, threshold or label policy was selected from these outcomes.

Known-reference counts below use the unchanged incomplete frozen positives. Each
cell is first-result / within-five cases, out of 30 per phrasing:

| Phrasing | Every-word | Any-word | Reranked |
| --- | ---: | ---: | ---: |
| Keywords | 15 / 29 | 15 / 29 | 29 / 29 |
| Ordinary questions | 0 / 0 | 2 / 23 | 23 / 30 |

The per-edition counts use ten needs per row, again first-result / within-five:

| Edition and phrasing | Every-word | Any-word | Reranked |
| --- | ---: | ---: | ---: |
| 1.34 keywords | 5 / 10 | 5 / 10 | 10 / 10 |
| 1.34 ordinary | 0 / 0 | 1 / 8 | 8 / 10 |
| 1.35 keywords | 5 / 10 | 5 / 10 | 10 / 10 |
| 1.35 ordinary | 0 / 0 | 1 / 7 | 8 / 10 |
| 1.36 keywords | 5 / 9 | 5 / 9 | 9 / 9 |
| 1.36 ordinary | 0 / 0 | 0 / 8 | 7 / 10 |

Every-word returns zero matches for all 30 ordinary-question cases. Any-word and
reranking have no zero-match cases. All 30 ordinary cases have a frozen reference
inside the hundred-candidate window; keywords have 29. The remaining keyword case,
startup-probe in 1.36, has nine matches and no frozen reference anywhere in them.
It is a lexical-candidate failure against the positive list, not a window cutoff
or evidence that every other candidate is irrelevant. No known-reference first-five
success regressed. Seven ordinary cases gained one: startup-probe and fqdn-length
in all three editions, and sandbox-ready in 1.35. At 1.35, their first-known ranks
move 55 to 1, 17 to 1 and 6 to 5, respectively. These are ten correlated needs with
edition/phrasing variants, not 60 independent users, accuracy or exhaustive recall.

The prespecified five-intent 1.35 pool contains 57 distinct need/location/article
items and 108 displayed occurrences. All judgments are new; none carried from
development. Method, rank and score were hidden during author reading, then joined
back. Independent all-item review approved the 57 grades. Counts are decisive /
related-only / unrelated-or-misleading occurrences:

| Method | Keywords | Ordinary questions |
| --- | ---: | ---: |
| Every-word | 6 / 8 / 0 of 14 | No hits |
| Any-word | 6 / 15 / 1 of 22 | 5 / 15 / 5 of 25 |
| Reranked | 6 / 15 / 1 of 22 | 9 / 14 / 2 of 25 |

A decisive passage can supply one correct part with material conditions; it need
not answer the whole question. Many reranked hits still supply only background.
The reviewer identified seven resource-background items as defensible but borderline
0 versus 1: specifying, enforcing or classifying resource values does not explain
reading them. Retain their original grades and the sensitivity record. Counting
all seven as related would move ordinary any-word from 5 / 15 / 5 to 5 / 19 / 1 and
reranked from 9 / 14 / 2 to 9 / 16 / 0; each keyword method's sole zero becomes a one.
Every-word is unchanged. The decisive improvement remains 5 to 9, while a strong
claim about the magnitude of unrelated-hit reduction depends on that boundary.
This is automated-review-checked author judgment, not blinded human validation.

## Resources, checks and limits

The unchanged local MPS/float32/SDPA/batch-one runtime completed without fallback or
truncation. Token counts include both texts and special tokens: median 98, 95th
percentile 241, maximum 850; 25 exceed 512, none exceed 1,024 or 8,192. The original
model-card 512-token example still limits claims about trained long-input quality.
Recorded process peak RSS is 2,625,273,856 bytes. Sampled MPS driver allocation
reaches 3,274,309,632 bytes, though the final snapshot is lower. Separate 6 GiB gates
passed after the pilot and every pair. Unified-memory figures can overlap; do not
sum them or infer a hard whole-machine reservation. Model weights remain
2,271,071,852 bytes, independent of runtime overhead. No timings were benchmarked.

Full local `make verify` passed on Go 1.27.2/darwin-arm64 with dedicated PostgreSQL
14.17, loopback port 55453. GitHub Actions remains disabled. No live R2, hosted DB,
other platform/version test or provider inference ran. The Go importer reconstructed
and checked the exact complete bundle and finite score coverage before reporting.
Full capture/reference preflight still checks both fixture splits. Operational
errors are not relevance misses. Private JSON/model/runtime artifacts remain ignored;
no product search, publication, storage or deployment change is included.

The validation supports investigating product integration, not enabling it. Inference
latency, concurrent admission, cancellation, model readiness, query limits, missing
model behavior, candidate-retrieval gaps and abstention remain unresolved. Controls
were not released or rerun in this validation; the development trial already showed
that sorting alone does not reject out-of-scope queries. The pretrained model may
have seen public source text. Subsequent policy selection needs new untouched
questions before another independent validation claim.

## Reproducibility fingerprints

Private artifacts are under `.tools/evaluation/reranking-validation/`. Preserve the
completed files, original producer, minimal producer diff, original/amended review
records and runtime/dependency/model manifests. The helper changed only artifact
routing, reserved scope/count assertions and a compatibility check against the
previous runtime excluding its own source hash. Loading, tokenization, scoring and
resource/checkpoint functions are unchanged; recorded function ASTs match.

- Plan: `a78a9bc35f55e395e39b946204da9c0d9a19cb6505da64a38275e696e67cc3c1`.
- Unchanged golden set: `12debe31676faddc2ed38d33228ce5df0d3c2179b268eb1a19596772ba7d342f`.
- Reserved input bundle: `d540a9d03837b87adc611757da3c270a98c73097b68042f0cfcc2006594882fd`.
- Complete scores: `92856515bad5bf0f058ae8d209dad789370b06c4026bb773e70030f89cc198a5`.
- Complete comparison: `9942d3f104e9d87bd4144cc3c89d31a42f0223f7daadcfc8988bef6f13ef5691`.
- Full pooled items: `d3ef4ed2388af2b976424c8138faeff39cae16938e570a87d692ed439a6015c5`.
- Reviewed grades: `607bf1054d8630b7a7ec80cff77b7035d1480253c13f15850c3ed16ee26a0dd2`.
- Original producer: `9055dd7ec90a66eb4998ee6374d02cf99941813fd4ba36a8d632d474710b7d76`.
- Validation producer: `2ad51f5577c46ad34d60890325837a981bc76421ba8423ba855795e871ba86f7`.
- Dependency lock: `46b2ddcb769f005da50bc9b8854060a656e3ef03503bc69324707c9bd15bea98`.
- Weights: `d9e3e081faff1eefb84019509b2f5558fd74c1a05a2c7db22f74174fcedb5286`.
- Tokenizer: `69564b696052886ed0ac63fa393e928384e0f8caada38c1f4864a9bfbf379c15`.
