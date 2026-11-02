# Reorder a bounded lexical pool with a local model

Status: the user selected local BGE reranker v2-m3 for an offline development trial.
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

Add mutually exclusive development-only modes to tools/evalsearch:
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
