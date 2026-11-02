# Reorder a bounded lexical pool with a local model

Status: completed user-authorized local BGE reranker v2-m3 development trial.
Product search, preparation, indexes, storage and frozen positives remain unchanged.

## Workload, model and local runtime

Use the existing 120 development cases and 18 controls, never the ten reserved
intents. The any-word diagnostic finds a frozen useful reference somewhere for all
60 ordinary-question cases, within five for 41 and within 100 for 57. Selecting only
100 therefore excludes all frozen references in three cases before model scoring.
These are incomplete known positives, not exhaustive recall or a relevance guarantee.
The first min(100, matches) produces 10,573 pairs: 3,596 keyword, 6,000 ordinary and
977 control pairs. Three empty controls require no inference.

Pin BAAI/bge-reranker-v2-m3 revision
`953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e` and verified safetensors/tokenizer files.
Its weights are 2,271,071,852 bytes, separately from runtime files and memory.
Python 3.14.2, torch 2.15.0, transformers 5.22.1, safetensors 0.8.0 and tokenizers
0.23.3 live under ignored .tools. A disposable local helper performs inference;
maintained export, score checking and evaluation stay Go, with no application ML
dependency or hosted inference API.

The tokenizer accepts up to 8,192 tokens per pair; the pinned model card's normal
example uses truncation at 512. Effective long-input training/relevance quality is
not established here. Record pair-length distribution, maximum and counts above
512/1,024/8,192. No silent truncation, passage splitting or replacement model:
oversize input stops the trial for discussion.

## Export checked inputs, then import checked scores

Two mutually exclusive development-only modes are implemented in tools/evalsearch:
`-rerank-export` exports candidates; `-rerank-scores FILE` reconstructs them and
checks scores before emitting existing all/any baselines plus tagged reranking.
Refuse existing comparison flags and reserved/all splits in these modes. Complete
capture/reference preflight, including reserved bindings without their queries,
still precedes any query.

Each case exports raw query, edition, form, total indexed matches and its lexical
window. Candidates retain path/article/block bindings, lexical scores, complete
saved body quotes and separate ancestor headings/notice labels. Versioned model
text joins `Heading: ` lines, `Notice: ` lines and `Body:\n` plus the full quote.
The model sees only raw query and this text, never paths, relevance labels,
rationales or expected answers. Pair IDs hash versioned fixed-field JSON of case,
query, exact binding and model text. A bundle fingerprint hashes canonical exported
JSON bytes. Scores must bind the exact bundle, input/model revisions, weights,
tokenizer, runtime, numeric mode and device. Metadata is a trusted producer
assertion, not proof the declared model ran.

Bound export JSON to 128 MiB, score input to 8 MiB and final report to the existing
16 MiB. These are encoded-byte bounds, not whole-process heap limits. Reject
unknown/duplicate fields, invalid Unicode/trailing data, duplicate/missing/unknown
pair IDs, nonfinite/missing scores and invalid token counts. Require exactly one
score for every exported pair. Any operational failure yields no completed report;
a final output-device error can still leave bytes. Partial checkpoints are recovery
material, not completed evaluation.

## Pilot and full run

Use local safetensors, no remote model code, eval/inference mode, float32, batch one,
two intra-op threads and one inter-op thread. Freeze available MPS or CPU before
inference; never fall back mid-run. Use raw classifier logits, no sigmoid, threshold,
query rewriting, text generation or fine-tuning.

Tokenize every complete pair without truncation first. Pilot five longest pairs,
tie by pair ID, chosen without relevance. Continue only if all fit and produce finite
scores, process peak RSS is at most 6 GiB, and MPS driver allocation, where applicable,
is at most 6 GiB. Check these gates after every full-run pair too. RSS and MPS driver
allocation can overlap in unified memory; they are not a combined hard reservation.
Failures stop without changing input/model/device. No shared-machine timing claim.

Checkpoint scores every 25 pairs. Resume only exact compatible missing work under
the frozen bundle/model/runtime identity. One worker owns its model/tensors/buffers;
no queue, concurrent inference or cache policy is introduced. Record work counts,
runtime versions, model file fingerprints and memory.

## Evaluation, ownership and cost

Sort only the selected window by descending model score, lexical-rank ties. Report
indexed matches and reranked candidates separately; first-known rank is explicitly
inside the window. A known reference outside it is excluded before scoring.
First-five evidence stays exact retained text, with context separate from body.
Compare both phrasings and controls; no score is answer confidence or abstention.

Extend the same predeclared five-intent, edition-1.35 reading pool with the new first
five. Carry a reviewed grade only after exact need/body/context equality. Read all
new hash-ordered items without method/rank/score, then independently review new
grades and carry bindings. Unknown remains unknown and frozen positives stay fixed.
This is author judgment checked by an automated reviewer, not human validation or
representative traffic.

Candidate acquisition repeats current complete lexical searches/sorts, full article
quote reads, decoding and canonical-plan work. Captures, one edition, candidate text,
model state and encoded buffers add memory. For K<=100, window sorting adds
O(K log K) work/O(K) order state; score validation uses expected O(N) lookup in N
pairs. Model work depends on pair lengths. A pair-count bound is not a token-work,
heap or provider-transfer bound. Inputs stay borrowed unchanged synchronously;
returned bundle/report storage is owned. No original source or remote object changes.

## Primary mechanisms and next decision

Elasticsearch 8.19's [retriever documentation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/retriever.html#text-similarity-reranker-retriever)
and [pinned builder](https://github.com/elastic/elasticsearch/blob/93788a8c2882eb5b606510680fac214cff1c7a22/x-pack/plugin/inference/src/main/java/org/elasticsearch/xpack/inference/rank/textsimilarity/TextSimilarityRankRetrieverBuilder.java)
separate candidate retrieval from bounded reranking. Our offline exact-edition
passage transport differs from its distributed model serving. The pinned
[BGE model card](https://huggingface.co/BAAI/bge-reranker-v2-m3/blob/953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e/README.md)
shows paired sequence classification and raw logits; its default truncation is not
inherited. Neither source establishes Concordance quality or speed.

The simplest alternative is unchanged lexical ordering. Record the trial before
discussing a new model, window, truncation policy, score fusion, semantic candidate
retrieval or production integration. No production default is selected here.

## Recorded development results

The reconstructed model input export is byte-for-byte identical at the final
reviewed implementation. The Go score importer accepted all 10,573 finite scores
with exact bundle/model/runtime coverage; the existing all/any report, after removing
only the new reranked section, reproduces the prior report exactly. No reserved
query, parameter sweep, alternative model or truncation was used.

| Phrasing and method | Known at one | Known within five | Known in selected/full ranking |
| --- | ---: | ---: | ---: |
| Keywords, every word | 9/60 | 38/60 | 41/60 |
| Keywords, any word | 7/60 | 40/60 | 56/60 |
| Keywords, reranked first 100 | 38/60 | 50/60 | 56/60 |
| Ordinary questions, every word | 0/60 | 0/60 | 0/60 |
| Ordinary questions, any word | 10/60 | 41/60 | 60/60 |
| Ordinary questions, reranked first 100 | 23/60 | 56/60 | 57/60 |

The last column for reranking is within the selected window; other rows use their
complete rankings. Frozen positives are incomplete and the three editions repeat
the same information needs, so these are not accuracy, exhaustive recall, answer
completeness or independent user observations.

Ordinary questions gain fifteen known-within-five successes and lose none relative
to any-word. Keywords gain thirteen and lose three: the CrashLoop keyword reference
moves from fifth to sixth in each edition. Its new first five were not part of this
five-intent reading sample, so that cutoff regression does not prove the other
passages unhelpful. No policy was changed to fix these cases.

Three ordinary restart-backoff cases have their first known references at 109, 155
and 165 in editions 1.34/1.35/1.36. All are excluded before scoring. The remaining
ordinary known-within-five miss is readiness-probe in 1.36: its first known reference
moves from fifteenth to twenty-fifth within the window. The model cannot recover
an excluded passage and can also worsen ordering inside its window.

In the predeclared 1.35 sample, the ordinary init-probe answer moves from twentieth
to first, default-shutdown from sixty-fifth to second, Pending from third to first,
Guaranteed QoS from fifth to fourth and hostname setup remains second. These are
concrete failures/gains of the fixed method, not a justification to tune it on the
reserved questions.

## Reading judgments and a source-content trap

The combined five-method pool has 55 distinct need/location/article items and 189
displayed occurrences. All 45 previous full need/body/context objects match exactly
before carrying their reviewed grades. Ten new items were read in fixed hash order,
without method/rank/score, then independently reviewed. The author already knew
the corpus and hypothesis; this is not blinded human validation.

| Sampled phrasing and method | Decisive (2) | Related only (1) | Unrelated/misleading (0) | Hits |
| --- | ---: | ---: | ---: | ---: |
| Keywords, any word | 8 | 13 | 3 | 24 |
| Keywords, reranked | 10 | 12 | 2 | 24 |
| Ordinary questions, any word | 3 | 17 | 5 | 25 |
| Ordinary questions, reranked | 10 | 11 | 4 | 25 |

One new initial grade was rejected by review and amended from 2 to 0. The retained
init-container paragraph at blocks [75,76) broadly claims app-container fields and
then only explicitly prohibits readinessProbe. Read alone, it can wrongly imply
liveness support. The same retained article's blocks [14,15) explicitly exclude
ordinary-init liveness, readiness and startup probes. The partial readiness fact
does not make the broad claim safe for the combined question. This is conflicting
source wording, not text invented by the model. The capture and frozen positives
were left unchanged; original/provisional grades and the rejected review remain
alongside the amended records.

The diagnostic-only Pending/init-container example retains grade 2 as a correct
specific initialization cause. The independent reviewer accepted it but noted grade
1 is a more cautious reading; that changes the ordinary decisive count by at most
one and does not change the conclusion. All grades are judgments by one author
with an automated independent check.

The sampled results improve, but still contain considerable background and some
misleading text. All 15 previously matching scope controls still return candidates;
there is no score threshold or abstention contract. Provenance binds exact text,
not factual correctness. These limits matter before production integration.

## Input lengths, memory and reproducibility

All complete inputs fit without truncation: tokenized pair median 93, 95th percentile
238, maximum 853, 62 above 512 and none above 1,024 or 8,192. The model's accepted
8,192-token capacity is not a claim about its long-input training quality.

The five longest pilot pairs passed before the 10,573-pair full run; those pilot
pairs were scored again in the full run, for 10,578 model evaluations altogether.
On this Apple M1 with 16 GiB, float32/MPS/SDPA/batch-one run, observed process peak RSS
was 2,836,545,536 bytes and observed MPS driver allocation reached 3,274,309,632
bytes. Both remained below their separate 6 GiB gates, checked after each pair.
These quantities can overlap in unified memory, are not a hard combined reservation,
and establish no production capacity or latency. No timing benchmark ran.

The ordinary Go build/tests require neither Python nor model weights. The complete
local make verify passed with Go 1.27.2/darwin-arm64, dedicated native PostgreSQL
14.17 and live R2 disabled. Source tests cover actual retained-capture export/import,
first 100 of 125 matches, exact evidence and ties, all-pair score coverage, mutated
input bindings, finite/null scores, fixed metadata, invalid Unicode/duplicate and
noncanonical JSON fields, split guards, reserved preflight, cancellation and checked
output. Independent code/architecture review approved the exact implementation;
the separate label review accepted the amended judgments.

Frozen plan SHA-256:
`3d2bc422ecd785f3a95210b12ad1fc6e86dd4249ff7f303388fd6f4099a4297b`.
Input bundle SHA-256:
`d115432eed23a5a28bc16022ac4432335cc9804441c34c83b0cb0692e71ebc60`.
Complete score-file SHA-256:
`74492108946775e3259eefc7ac3862a601fad4aa8cd0f505593f1383c66a92d0`.
Comparison SHA-256:
`50810828bf6899ad0ed1bd5090fe8c27e48cde0eb40c11bac731706dfe4530fc`.
Reviewed grades SHA-256:
`8c531df0bae78b94f9f16386bde89651d0dc0a2f302f8d336da1ef2f90d37521`.

Ignored .tools/evaluation/reranking contains the plan, inputs, token audit, pilot,
checkpoints and complete scores, resource record, comparison, pool/memberships,
original/amended/reviewed grades, summaries, initial development source snapshots
and final evaluation-source/evaluation-hashes. The final reviewed code reproduces
the exact original input bytes. The producer is .tools/probes/local_rerank.py with
SHA-256 `9055dd7ec90a66eb4998ee6374d02cf99941813fd4ba36a8d632d474710b7d76`;
the pinned installed dependency list is under .tools/reranking/requirements.txt.
Partial checkpoints are not valid input to completed evaluation.

Keep this setup frozen for a separately authorized reserved-set validation before
deciding on production behavior. Do not infer representativeness, calibrated scores
or a preferred fusion/threshold from this development run. Product search remains
unchanged.
