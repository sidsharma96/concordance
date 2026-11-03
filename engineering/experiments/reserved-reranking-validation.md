# Validate the frozen reranker on the reserved intents

Status: the user authorized release of the ten reserved information needs for one
fixed validation. The fixed round uses both phrasings across all three pinned editions, giving 60
cases. The fixture's stored split label remains reserved and its hash is unchanged;
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
