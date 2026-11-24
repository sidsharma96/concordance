# Ask a question through the published reranking command

Status: implemented opt-in CLI composition using the accepted
[framed worker contract](../decisions/worker-protocol.md). The actual fresh-process
command searched the existing hosted Kubernetes corpus in editions 1.35 and 1.36,
scored 100 candidates in each and displayed ten verified passages. Ordinary
search remains every-word/BM25. This is a functional milestone, not a production
capacity or latency benchmark.

## One question reaches retained evidence

The question was `Why is my Pod stuck in Pending?`. The normal command resolved
`kubernetes`, pinned an exact published edition, admitted its complete any-word
ranking, selected the first 100, grouped strict article downloads, constructed the
frozen heading/notice/body inputs, received complete model scores and rendered a
requested prefix using the original reader-bound citations.

| Requested edition | Complete indexed matches | Scored candidates | Displayed passages |
| --- | ---: | ---: | ---: |
| 1.35 | 580 | 100 | 10 |
| 1.36 | 624 | 100 | 10 |

The hosted target's before/after display was identical. A subsequent command with
a missing worker socket failed with zero stdout, preserving no-fallback behavior.
The run used the existing native service/password files, an owned strict-host-key
SSH forward and existing R2 settings passed as child environment data. No setup,
activation, import, catalog/object/receipt write or deletion ran. The forward closed
afterward. These outputs establish executable composition and exact selection,
not newly judged relevance or complete answers.

## Preserve the frozen experiment's inputs and numbers

The pinned local model remains BGE revision
`953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e`, Python 3.14.2, float32/MPS/SDPA, batch one,
two intra-op/one inter-op threads and complete untruncated inputs. Startup checks
all 34 dependency-lock entries and streams all five pinned assets through SHA-256
before loading. The optional Go fetcher verified the already-present files without
network downloads. A two-pair authored synthetic batch returned complete finite
scores and token counts through the actual protocol.

The byte-compatibility probe read the existing development export without executing
new retrieval queries or inference. Across all 138 development/control cases and
10,573 pairs, the real Go input builder sent exactly the recorded raw questions and
formatted model texts through the Python codec. The receiver checked those values
and the recorded token lengths. This is not a new reserved-set run or relevance
comparison.

A separate real-model check selected the existing 1.35 ordinary-question
`pod-pending` batch. All 100 token counts and raw scores equaled the saved values
exactly; maximum absolute score difference was zero, and the full order including
ties was identical. This is one numeric sample, not a claim that every future
hardware/kernel version reproduces every score. No model/configuration was tuned.
The earlier [frozen validation](reserved-reranking-validation.md) remains the
relevance evidence; these checks establish input/implementation compatibility.

## Exercise failures before relying on model serving

The Python stdlib suite uses a fake scorer and deterministic held-compute events.
A disconnected client still leaves new callers busy until computation drains.
Graceful shutdown waits for that work before removing the owned socket, and a
replacement path is preserved. Stalled/partial headers and bodies expire under
the server's own receipt deadline. Whole-batch token overflow starts no inference.
Tokenizer/model exceptions fail the worker closed. Strict JSON, wrong bindings,
Unicode and truncated/oversized messages are rejected.

Go tests verify exact input ownership/serialization, bounded frame reads, strict
JSON, score cardinality, nonce/request/input/model bindings, unique scores, finite
numbers and token limits. The client restores original pair order even when the
peer returns records in another order. Cross-language tests need Python's standard
library, not ML packages or model files.

The local PostgreSQL/HTTP CLI fixture scores twelve candidates while displaying
one, with two opening GETs and one shared article GET. Ties preserve the original
source order; an explicitly test-only counter scorer forces reversed model order
and verifies the final citation follows it. Candidate overflow, missing worker,
format limits and invalid flags return no partial body. Empty candidates need no
worker. The production worker has no fake-scoring flag. The launcher environment
allowlist excludes inherited database/cloud credentials and Python import settings.

## Costs, reproduction and limits

The new Go binary does not link an ML runtime. Model assets total about 2.3 GB,
plus the separately installed Python dependencies. The persistent process retains
the model between queries. Article reads are grouped within a query; the same
article can still be decoded twice and have its passage plan rebuilt for every
selected hit. Strict local model-file verification also reads full files at startup,
using streaming buffers; there is no persistent local verification receipt.

No query latency, throughput or new peak-memory benchmark ran on the shared
machine. Passing per-pair resource gates is not an admission reservation. The
hosted run did not instrument provider request counts or transferred bytes; request
counts above come from the synthetic fixture. The real model proof is on the
existing Apple M1/MPS environment, not Linux/CUDA parity, multi-user authorization,
public hosting, statistical accuracy or million-document support.

README contains the venv, pinned asset fetch, worker start and search commands.
Ordinary local verification requires only Go, Python stdlib and a dedicated local
PostgreSQL database, with live R2 disabled. GitHub Actions remains disabled. Private
teaching notes and task artifacts remain local and excluded from source review.

## Private records

`.tools/reviews/local-worker/` retains full local check/review logs, exact input
parity and score-comparison records, hosted command stdout/stderr and before/after
target output. Original development input SHA-256 is
`d115432eed23a5a28bc16022ac4432335cc9804441c34c83b0cb0692e71ebc60`.
The unchanged dependency-lock SHA-256 is
`46b2ddcb769f005da50bc9b8854060a656e3ef03503bc69324707c9bd15bea98`.
Runtime assets, operator routes/credentials and dataset JSON remain ignored.
