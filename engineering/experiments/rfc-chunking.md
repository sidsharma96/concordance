# Compare RFC passage boundaries under a shared reading budget

Paragraph-first has a small known-evidence lead under the shared text budget,
with 16% more vectors and padded model work. The predeclared rule finds no
quality-and-cost winner. Dense budget scores are 38.19% for paragraph-first and
37.08% for fixed windows; these are marked-source coverage scores, not answer
accuracy.

Status: completed private development experiment. No application search default,
source contract, encoder or splitter is adopted for production. All raw captures,
labels, model files and score arrays remain ignored.

## Question and recommendation before the run

Does preferring paragraph boundaries find more known source evidence than filling
fixed-size windows? We initially preferred paragraph-first ranges as a small,
diagnosable baseline without individual-RFC parsing rules. Fixed windows are a
credible simpler competitor. The earlier layout audit showed that blank lines do
not reliably identify authored units: page furniture interrupts prose, and large
units include tables and schema trees. The tested alternative is bounded splitting,
not a claim to have reconstructed document structure.

Whole-file/file-search workflows, faithful preparation for the existing HTML
pipeline, existing document processors, parent-child context reads and contextual
embeddings remain credible alternatives. This run isolates two splitters; it does
not evaluate an agent choosing files or writing queries. Prior complete modern and
legacy RFC extraction/converter failures remain in the private source-boundary
records. They do not prove that faithful preparation is impossible.

## Frozen workload and controls

All 1,000 captured body hashes were checked. Both methods admit the same 996 files,
56,255,149 raw bytes. RFCs 101, 178 and 237 are not valid UTF-8. RFC 542 contains NUL,
which the existing lexical processor rejects. These four originals remain retained;
exclusions were explicit before any queries and are not relevance misses. Every
known evidence source is admitted. This is not a full-1,000-file processing result.

The [question inventory](../corpora/rfc-retrieval-questions.md) remains unchanged.
Only the six development families run: 54 evidence-seeking intents and six scope
controls. The 40 reserved questions are not queried or scored. Broad source-section
references were refined into 82 concise known-useful fragments, with independent
source-evidence review and a freeze before retrieval. Fragments are an incomplete
positive pool, not exhaustive judgments or complete answers. Other returned sources
remain unknown, including potentially useful alternatives.

Both methods retain contiguous, exact raw byte ranges and cover each admitted file
once without overlap. Each final passage is independently tokenized and limited to
256 encoded tokens (including prefix and special tokens) and 4,096 source bytes:

- A prefers blank-line units, greedily packs them and splits oversized units at LF
  line boundaries. An oversized individual line errors; nothing is truncated.
- B fills tokenizer windows up to the same limits, verifies tokenization on the
  actual standalone window and preserves all whitespace. It is not a sentence-aware
  or whole-word-only window. Boundaries can divide related text in either method.

There is no inferred title, header removal, overlap, added context or reranking.
The actual limits split 3,909 physical units across 765 admitted files for A. A byte
ruler alone would not establish model admission. These are experimental settings,
not a claim that 256 tokens is optimal.

## Model and ranking

Both use [E5-small-v2 at a fixed revision](https://huggingface.co/intfloat/e5-small-v2/tree/ffb93f3bd4047442299a41ebb6fa998a38507c52):
`query: ` and `passage: ` prefixes, masked mean pooling and L2 normalization per its
instructions, with no truncation. The 384-dimensional model is an economical
standard retrieval baseline, not a production selection or evidence that it is the
best encoder for technical standards. The existing cross-encoder reranker cannot
substitute for a passage embedding model.

The local runtime uses Python 3.14.2, torch 2.15.0, transformers 5.22.1, float32 MPS,
SDPA attention and sequential source-order batches of 32. Dependencies were already
installed; the separately pinned download is 134,478,053 bytes of model/configuration
files. There is no remote code or cloud embedding call. Query vectors are shared
across the two dense conditions. Exact cosine scores every candidate, with stable
input-position ties, and uses no approximate index. Floating-point/kernel variation
was not investigated with repeated runs.

The lexical control reuses the existing Go `textterms.Prepare` and
`termsearch.Index.RankBM25Any`, body only, on those same ranges. Its complete union
and edition-local statistics remain intact; the private harness retains the first
64 after complete ranking. This is not ordinary match-all command behavior.

## Compare evidence against bytes, not only result count

Five longer passages can cover more evidence merely by returning more text. Before
search, independent design review changed the primary metric from a first-five
count to a shared reading-budget curve. At each budget from zero to 16,384 bytes,
return the ranked prefix of whole passages that fits, up to 64 passages. Stop before
the next passage would exceed the budget; do not skip it or credit a partial read.

Coverage is the fraction of marked non-whitespace source bytes present in the union
of returned ranges. Evidence can cross passage boundaries. The normalized staircase
area averages that coverage across the budget range. The table calls it the mean
budget score; it is not answer accuracy, precision or exhaustive recall. The 8 KiB
and 16 KiB columns count intents whose entire marked fragment set was covered.

| Retrieval | Split | Mean budget score | Complete marked evidence at 8 KiB | At 16 KiB |
| --- | --- | ---: | ---: | ---: |
| vector | A | 38.19% | 17/54 | 22/54 |
| vector | B | 37.08% | 12/54 | 21/54 |
| lexical | A | 35.63% | 15/54 | 22/54 |
| lexical | B | 35.30% | 16/54 | 19/54 |

For vector, paired budget scores favor A in 21 intents and B in 19, with 14 ties. These are repeated conditions of the same 54 authored intents, not independent production observations.

For lexical, paired budget scores favor A in 19 intents and B in 17, with 18 ties. These are repeated conditions of the same 54 authored intents, not independent production observations.

## Preparation and storage costs

| Split | Passages | Actual model tokens | Padded model tokens | Raw vector bytes |
| --- | ---: | ---: | ---: | ---: |
| A | 60,098 | 13,166,699 | 15,338,744 | 92,310,528 |
| B | 51,786 | 13,133,388 | 13,257,216 | 79,543,296 |

Raw vector bytes count uncompressed float32 values only, excluding file headers,
metadata and any future index. Padded tokens depend on the frozen batching order;
actual unpadded tokens are shown alongside them. Neither count is a measured latency
or dollar cost. Both methods cover identical raw bytes; their actual returned body
bytes vary and are retained per case. The 16 KiB budget does not include response
metadata or establish an agent tokenizer/context or all-wire-traffic limit.

## Decision and limits

The frozen Pareto rule favors a method only if its mean budget score is strictly
higher, its complete-marked-evidence count at 16 KiB is not lower, and both its
padded passage-model tokens and raw vector bytes are no greater. Otherwise the
outcome is a trade-off, not a unique winner. This deliberately leaves product
quality-versus-cost preferences unresolved.

Dense result under that rule: **trade-off; no Pareto winner**. The analogous coverage comparison with shared embedding-preparation costs gives **trade-off; no Pareto winner** for the lexical control. A lexical-only deployment has no embedding cost; its allocation/index costs were not measured, so this is not a lexical runtime cost verdict.

Both methods share caps, but A has shorter passages on average. This compares two complete splitter configurations; it does not isolate semantic coherence from passage length, context or embedding-batch effects.

Exploratory inspection shows why incomplete judgments matter. For the DNSSEC
privacy question, A has zero coverage of the marked RFC 4033 fragment, but its
second result from RFC 7858 directly explains that DNSSEC does not protect request
and response privacy. That useful alternative stays outside the frozen metric;
no references were added or scores changed. A marked-source miss is not proof
that the returned evidence cannot answer the question. This example is a factual
post-run observation, not a new scored label or a full review of unjudged hits.

The dense paired result is 21 A wins, 19 B wins and 14 ties. Family means are mixed:
A leads in HTTP semantics, TLS, DNS and formats; B leads in caching and TCP. Neither
the small mean difference nor the five extra complete cases at 8 KiB establishes
production or answer-quality superiority. The recommendation remains to keep a
simple controlled baseline and improve evidence/task judgments before paying for
richer preparation; this experiment does not justify more chunker complexity.

The rule applies to this single model/size/development run. It does not select an
agent workflow, justify an approximate vector index, establish production latency
or prove supported complete answers. Scope controls produce related passages; they
are unscored for relevance and cannot establish actual private deployment state.
No answer-generation or abstention model ran. A held-out comparison remains later
work; it was not used to tune this run.

## Verification and retained records

The complete workflow has 240 retrieval conditions: two splitters, two retrieval
methods and 60 development queries. There are 216 evidence-seeking conditions and
24 scope-control conditions, not 240 independent questions. All inputs are pinned
before inference. Independent checks cover all 1,000 original body hashes, both
sets of passage ranges, all 120 dense full-sort results, 1,200 scalar dot-product
comparisons and 216 independent discrete-budget metric calculations. The synthetic
metric fixture covers split evidence, whole-passage cost, controls and paired ties.
No retrieval/query failure is reclassified as a relevance miss.

Private reproduction material is retained under `.tools/evaluation/rfc-chunking-1/`:
`plan.json`, `before-run.json`, `prepared.json`, `labels.json`, `model-pin.json`, the
Go lexical probe, Python preparation/scoring/metric probes, vectors, scores and
`outcome.json`. It requires provisioned local inputs/runtime and is not an ordinary
build dependency. Independent role reviews stay under `.tools/reviews/rfc-chunking-1/`.
No timing rerun, hosted publication, target activation or native RFC import
is part of the experiment. Ordinary local checks use a separate dedicated test
database and synthetic fixtures, not the hosted catalog.

Frozen SHA-256 identities:

- `before-run.json`: `4ce1a290f600cfac773b4becc7e305771c2e8a47323331b678c1beee6885b01a`.
- `labels.json`: `db7495497848e55cb3d6f1cb45026d2719398a4f52a0a933fbecea5e4feb9e64`.
- `prepared.json`: `8131a4e9cf49b0ce4e2b074cfed41a111f772c2afb6f9517331809fb32c77c3e`.
- `model-pin.json`: `702c02d4cc029107cec5d3bff74e8a9db6f2fac99b073686427873fb1428d821`.
- `outcome.json`: `22dc0671f241de487c65e320cbc0135d404babeb43d568de43fa7afb8594877a`.
- `dense-completed.json`: `edaa2f370e40345c68463511794ff90f198fdd451c3be993d6dddae2472f6ca0`.
- `verified.json`: `5a2a20e482975d9ec3b2f69a5d7414b7a8898c196bb6fc4c7d847c36ef0182d5`.

The original 100-question set remains `d90b3ae5cb66acd4c34dddc6d5ac3dec91866451a1ddb38fc0d745807e41d994`.
