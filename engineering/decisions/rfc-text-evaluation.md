# Freeze a larger offline corpus before comparing retrieval

Status: accepted official RFC plain text for an offline experiment. This prepares
source evidence and questions; it does not add RFC ingestion to the application.
Exact vector search remains an accepted experimental direction, with its encoder,
text preparation and comparison harness still to be decided.

## Workload and competing explanations

An agent asking whether a lost PUT can be retried needs the right rule, not just a
passage containing PUT. A miss could mean that useful text was lost during
preparation, no candidate was selected, a useful candidate ranked poorly, or the
cost of reading and checking evidence dominated the task. Keep these stages
observable before proposing another model, database, cache or concurrency rule.

The existing Kubernetes corpus gives a working, source-checked control, but repeats
20 development information needs across three editions. We chose 1,000 distinct
RFCs and 100 new questions to broaden topic and document variation. These are
chosen diagnostic cases, not a sample of production traffic or company size.

Keeping Kubernetes alone avoids preparation work but cannot answer the broader
question. Native RFC ingestion would exercise the second-source boundary, while
also adding source identity and processing contracts before the retrieval
comparison. An established benchmark such as BEIR provides ready judgments but
would change our exact-source task. Offline official TXT is the smallest next
step that isolates corpus preparation. It leaves native RFC ingestion separate.

The RFC Editor offers [text-only distribution](https://www.rfc-editor.org/series/rfc-download/).
TXT is a published representation, not the definitive RFCXML format described in
[RFC 9720](https://www.rfc-editor.org/rfc/rfc9720.html), as updated by
[RFC 9920](https://www.rfc-editor.org/rfc/rfc9920.html). Captured hashes identify
actual bytes, independently of the RFC number or current catalog status. Text
capture does not establish complete diagrams, XML structure or errata coverage.

## Recommendation before the check

Capture exact official TXT responses first. Retain historical and superseded
material deliberately. Draft information needs before observing any retrieval or
embedding scores, then check their expected facts against the captured sources.
If old and new text cannot be captured under the declared bounds, stop and diagnose
rather than replace a document or silently accept a smaller corpus. If a draft
claim lacks source evidence, amend it before freezing, not after seeing a miss.

The capture checks retained the recommendation and corrected an assumption:
archival TXT is not universally UTF-8. Preserve exact raw bytes and record encoding
validity. Do not guess an encoding, replace characters or add individual-RFC
conditions to shared retrieval code. Decoding unsupported bytes remains a text
preparation checkpoint before any search experiment consumes them. All question
sources must have validated encoding before source grounding can pass.

## Selection and capture boundary

Start from one retained official catalog. Select 56 explicit question-source seeds,
then their one-hop update and obsolescence relatives with TXT declarations, giving
201 distinct RFCs. Fill the remaining 799 slots from other TXT-declared entries by
ascending SHA-256 of the UTF-8 bytes of
`concordance-rfc-text-corpus/v1`, a zero byte and the unpadded decimal RFC number.
This fixes selection before body capture or retrieval scores. Catalog declarations
alone do not prove that a file can be downloaded.

The private capture runs sequentially, with at most two request starts per second,
a 60-second request deadline, an 8 MiB body cap and a 256 MiB aggregate body cap.
It checks successful HTTPS status, official host and exact path, text/plain type,
native RFC number, size and hash. Requests ignore user curl configuration and allow
at most two HTTPS redirects. Curl 8.4 or newer enforces the body limit during a
transfer even when the size was not known in advance; the installed client was
8.7.1. See its [documented limit](https://curl.se/docs/manpage.html#--max-filesize).

Accepted files are never overwritten. Failed attempts remain evidence; a missing-only
retry rechecks accepted bytes and identity before fetching missing files. The final
manifest rechecks every selected body. Individual response capture times do not make
this an atomic publisher-wide snapshot. No R2 or PostgreSQL write, target activation,
source-format coercion or application import is part of this work.

## Questions and judgments

The [question inventory](../corpora/rfc-retrieval-questions.md) contains ten topic
families, each with nine evidence questions and one deployment-state control.
Development uses six whole families, 60 questions. Four whole families, 40 questions,
remain reserved from retrieval and tuning. Protocol families still overlap; this
is procedural separation, not a claim of independent samples or unknown training
material. The earlier Kubernetes set remains unchanged and already observed.

A control asks for an actual private deployment fact absent from public RFCs. A
plausible default from a standard is insufficient evidence. Incidental keyword
matches are allowed; the control does not require zero search results or introduce
an answer-generation contract.

Known-positive evidence binds an RFC number, exact body SHA-256, half-open raw byte
span, LF-only line range and quote SHA-256. Form feed is retained and does not count
as a line break in those addresses. Source sections are grounding evidence, not
canonical search passages or a new citation schema. Passage-level judgments need
the separately selected preparation rules before retrieval can be scored.

Independent review checks every question against its supplied source spans before
freeze. It does not establish exhaustive relevance, errata coverage or current
protocol advice. Explicit historical requests require their named sources. Other
useful returned evidence remains unknown until judged; it is not automatically
irrelevant because it lacks a known-positive label.

The separation follows BEIR's pinned
[corpus/query/judgment loader](https://github.com/beir-cellar/beir/blob/ef83d29307061c65d04b035b4f4e7c18bd8374af/beir/datasets/data_loader.py)
and Elasticsearch 8.19's
[manually judged evaluation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/search-rank-eval.html).
We do not adopt their frameworks or claim the coverage of their benchmarks.

## Retained identities and next check

Capture completed for all 1,000 selected RFCs, totaling 56,412,884 raw bytes.
Three bodies, RFCs 101, 178 and 237, are not valid UTF-8; they remain retained
unchanged and require a separate decoding decision before text processing. All
56 question-source seed bodies are valid UTF-8. The 90 answerable questions have
124 checked source spans; ten controls have no public-source answer labels.
Independent per-question review covers all 100 intents. Aggregate counts come from
validated unique verdict rows, because the reviewers' prose summaries occasionally
miscounted their own rows. No retrieval, embedding or timing run preceded freeze.

The [body pins](../corpora/rfc-text.pins) give every captured RFC number and SHA-256.
The official catalog snapshot SHA-256 is
`891d504b74051f8091ec6ccfba30170357899f07ee1f81d7824ba1657416ffc1`.
The private selection SHA-256 is
`545b4970f4c28f87d89d3b70e2757dd994af803a61dff28b21698db5d9873221`.
The private completed corpus manifest SHA-256 is
`90c6d66503e3845e99e771d9f37927694d692f8fa96306fa2ef54fb864fad8bc`.
The private question-set SHA-256 is
`d90b3ae5cb66acd4c34dddc6d5ac3dec91866451a1ddb38fc0d745807e41d994`. The machine-readable corpus, labels, attempts and reports remain
ignored under `.tools/`. No model, normalizer, chunker or approximate index is
selected by this preparation record.

The [first controlled passage trial](../experiments/rfc-chunking.md) now compares
paragraph-first and fixed-window ranges with one experimental local encoder and
the existing lexical engine. It runs the 54 evidence-seeking development intents
and six controls on an explicit 996-file supported-input subset. The reserved
40 questions remain unqueried. Neither splitter has a quality-and-cost win under
the predeclared rule, and unjudged useful hits limit answer-quality conclusions.
No production model, native RFC processor or passage contract is selected.

An agent using file search and end-to-end answer/source checks remain untested.
Before changing retrieval, separate source availability, candidate coverage,
ordering, context and read/model cost; preserve errors as errors. Revisit the
corpus if practical tasks need live information, operational traces, richer media
or different topic coverage.
