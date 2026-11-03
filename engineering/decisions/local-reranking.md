# Opt-in reranking through a persistent local worker

Status: accepted option A. Keep the tested model loaded in a separately started
local worker. Ordinary search remains every-word/BM25; an explicit reranking mode
will use the evaluated first-100 any-word pool. The framed worker and opt-in command now implement this direction; read the
[protocol](worker-protocol.md) and [functional experiment](../experiments/local-worker.md).

## Ownership and lifecycle

Go owns authorized target resolution, the exact generation/edition reader,
candidate selection, strict retained-article verification and displayed citations.
The worker owns the existing pinned BGE model/runtime and receives only the raw
question, bounded canonical candidate text and opaque request/pair bindings. It
gets no catalog or R2 credentials. Keep the trial's full heading/notice/body input,
raw logits, no truncation and lexical-rank ties. Do not replace the model/runtime
or tune against the now-observed reserved questions.

Start the worker explicitly and keep it loaded between commands. Use a Unix-domain
socket in an operator-owned private directory for the initial macOS/Linux path.
This is a trusted local process boundary, not a public search API or cryptographic
proof that inference ran. One whole query owns the scorer at a time; reject busy
requests rather than accumulate waiting inference work. Bound parsing, connections,
request/response bytes, candidate count and actual token work as well as inference.

The protocol implementation supplies readiness, bounded receipt, cooperative
cancellation and joined draining; the functional experiment records its checks.
Client cancellation cannot instantly interrupt an active tensor operation. Keep
ownership until computation has actually stopped; no overlapping admission after
the client leaves. Explicit reranking errors must not silently become lexical or
hosted results. A successful batch needs complete finite scores bound to that exact
request and reader. Keep final output buffered, with the existing output-device
partial-write caveat.

## An unavailable scorer fails the requested operation

Status: accepted option A. If scoring is needed and the local worker is absent,
busy, crashes or cannot complete, return a clear error and no successful result.
Do not automatically return lexical results, use another model or contact a hosted
provider. Ordinary search remains a separate explicit invocation. Successful empty
candidate queries retain their empty result; there are no pairs to score.

A labelled lexical fallback could preserve availability, but it changes the
requested ranking and adds a successful degraded state. We keep failures visible
for this first trusted-operator command. Revisit an explicitly enabled fallback
only with a concrete user-facing availability requirement. Authorization, article
integrity, binding, budget and caller-cancellation failures always remain errors.
The worker/protocol/command implementation now exists under the explicit
[worker contract](worker-protocol.md).

## Candidate work comes before the model

Add explicit any-word APIs without changing existing Search/RankBM25 semantics.
Keep distinct query terms in lexical sum order, exact-edition statistics, positive
BM25 contributions, complete union admission and input-position ties. Compare exact
scores and order to the existing single-term composition oracle. No tokenizer,
artifact codec, index generation or relevance policy changes are needed.

Candidate text must come from the same strictly verified article as its saved
body range. Canonical headings and notice labels stay separate from the verbatim
body and must reproduce the offline serialization exactly. Keep actual reader-bound
references beside owned text; a worker-returned path or position cannot redirect
a citation. Explicit component and aggregate text budgets reject excess, rather
than cutting an input. No persistent article cache is selected.

Before any candidate article GET, admit the complete selected read plan against a
cumulative declared article-byte budget. With the current uncached per-hit path,
charge repeated article reads repeatedly. This bounds successful declared body work,
not all headers, retries or failed/overlong network responses. Existing strict
per-read limits remain necessary. At 100 candidates the simple path can require
102 logical GETs including manifest/index, versus 12 for ten ordinary quotes.
At an 8 MiB article cap that is up to 800 MiB of candidate article bodies, a derived
ceiling, not observed normal traffic. The accepted [grouped-read operation](grouped-article-reads.md)
now charges distinct article bytes with separate limits and reuses one encoded body
within a candidate batch. Per-passage verification/decoding remains. The worker composition now uses that grouped operation; the uncached API keeps its
original budget meaning.

## Alternatives, evidence and revisit conditions

A fresh worker per query releases model memory when it exits and avoids a resident
service, but repeats model initialization and can multiply replicas across commands.
A hosted provider removes local model resources but adds text transfer, billing,
outages and model/input compatibility decisions. Choose the resident local worker
to reuse the runtime already exercised and avoid repeated initialization; accept
resident model resources and a second process to operate. No cold-start or serving
latency benchmark has been run. The validation memory figures overlap and are not
a production concurrency reservation. Revisit with query frequency, target hardware,
resource/admission measurements and demand for remote users.

TEI v1.9.0 at `5699247f57e46aa09eb4f8c4cf74114099372fe7` creates, health-checks and
warms a backend before serving in
[router startup](https://github.com/huggingface/text-embeddings-inference/blob/5699247f57e46aa09eb4f8c4cf74114099372fe7/router/src/lib.rs).
Its [rerank handler](https://github.com/huggingface/text-embeddings-inference/blob/5699247f57e46aa09eb4f8c4cf74114099372fe7/router/src/http/server.rs)
uses shared inference state, validates batch size and obtains per-pair permits.
That is a model-lifetime example, not our proposed whole-query gate or evidence of
runtime/numerical parity. Sentence Transformers v5.1.2 separates loading from
repeated prediction in
[CrossEncoder](https://github.com/UKPLab/sentence-transformers/blob/dfcbbd93056c6292ee6a245c0740683a270df0b1/sentence_transformers/cross_encoder/CrossEncoder.py);
its batching and activation defaults differ from our trial. Neither library is
selected as a replacement runtime. The
[Cohere API](https://docs.cohere.com/reference/rerank) illustrates hosted query/text
scoring; its model and automatic truncation are not inherited.

The [development trial](../experiments/local-reranking.md) and
[frozen validation](../experiments/reserved-reranking-validation.md) support the opt-in implementation, not production latency, relevance confidence, abstention or a claim
that a single local worker handles enterprise traffic. Embeddings/vector retrieval
remain separate work.
