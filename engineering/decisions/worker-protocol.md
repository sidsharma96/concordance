# One framed JSON batch for the local model worker

Status: accepted option A, including a narrow Python source exception. The new
client, worker and CLI composition implement this contract. Functional checks and
remaining limits are recorded in the [worker experiment](../experiments/local-worker.md).
The earlier [worker direction](local-reranking.md) still owns model policy and
no-fallback behavior; ordinary search remains every-word/BM25.

## One owner for one complete batch

Go resolves an authorized target once, pins its published generation/edition,
retrieves the evaluated first 100 any-word candidates, and verifies exact retained
body/context through the [grouped read](grouped-article-reads.md). Model input keeps
all Heading/Notice/Body text without truncation. The worker receives query/text
and opaque bindings, not reader handles or catalog/R2 credentials. Go retains
citations and sorts returned raw logits, keeping lexical order on exact ties.

The separately started worker loads the existing frozen Python/MPS runtime once.
A private Unix socket carries a 4-byte unsigned big-endian byte length followed by
one UTF-8 JSON object. The length is checked before allocating/receiving its body.
This uses Go net/encoding/binary/io/encoding/json and Python socket/struct/json,
not pickle, a generic RPC framework or an HTTP service.

```text
worker ready/reserved with model/input/runtime tags and fresh nonce
    -> Go request: nonce, random request ID, fingerprint, raw query, ordered pairs
    <- complete scores for those exact pairs, or one bounded error
close; a later request starts a fresh connection
```

Every message has the closed version `concordance.local-rerank/v1`. Duplicate keys,
nulls, invalid Unicode, excessive depth/value count, unknown fields, wrong scalar
types and incomplete frames fail. Compatibility and request/pair tags bind a
response to the expected inputs; they are not proof that inference happened.
One request per connection, no multiplexing, automatic retry or result cache.

## Admission, deadlines and cleanup

The server has one acceptor and at most one operation thread. It checks whether
that thread is alive before creating another, including after the previous caller
has disconnected. Busy rejection precedes large candidate uploads. The socket's
kernel backlog is separate from this application admission; multiple worker
processes and concurrent Go queries remain operator-admitted.

Directory mode 0700, socket mode 0600 and current-user ownership are checked.
Paths must be absolute and at most 103 encoded bytes; symlink directory aliases
are rejected. Startup never removes an existing socket. Shutdown removes only the
socket inode this instance created, leaving a replacement path untouched. The
operator keeps model files and routing stable; this is no hostile-filesystem or
adversarial-process sandbox.

The worker enforces a five-second overall greeting/request-receipt deadline,
starting when the slot is reserved. It is independent of the client-supplied
budget and is never renewed per chunk. Control errors have a 250 ms write budget.
Inference receives at most five minutes, further limited by the caller's remaining
budget. Go applies its context to connect/read/write and joins its cancellation
callback after closing the connection.

Before and after tokenization and between model pairs, the worker checks its
monotonic deadline, shutdown and peer disconnect. A tensor operation already in
progress is not instantly interruptible. The thread keeps the slot until that
operation synchronizes and cleanup completes; scores are discarded on cancellation.
Graceful stop closes admission and joins the active operation. A wedged operation
may require explicit process termination. Uncertain model/tokenizer/device failures
stop the worker rather than serve the next request on damaged state.

| Limit | Bound |
| --- | ---: |
| Greeting / request / response JSON bytes | 16 KiB / 16 MiB / 64 KiB |
| Query / one complete formatted text | 4 KiB / 128 KiB |
| Candidate pairs | 100 |
| Tokens in one pair / all pairs | 8,192 / 32,768 |
| JSON nesting / bounded token walk | 8 / 2,048 values and keys |

Complete token admission precedes the first inference. The aggregate token cap
admits the recorded development maximum of 13,331 pair tokens per request, but is
not an enterprise capacity claim. Text/frame/token limits are independent work
bounds, not whole-process heap or GPU reservations. The model keeps the trial's
separate 6 GiB process peak RSS and MPS driver gates after each synchronized pair;
these quantities can overlap and are not preallocation enforcement.

The CLI defaults to five minutes only in explicit rerank mode; ordinary search
keeps its existing timeout. `--all` conflicts with reranking, because the model pool
is a fixed first 100 rather than all matches. `--limit` at most 100 chooses display
only after every admitted candidate is scored. Candidate article bytes default to
64 MiB of distinct downloads and candidate evidence to 8 MiB; returned display
quotes keep their separate existing aggregate budget. Empty candidates need no
socket or worker. Any verification, compatibility, scoring, limit or cancellation
failure yields no successful display; there is no automatic lexical fallback.

## Exact inputs and compatible deployment

Pair IDs hash a versioned domain, original candidate position and length-prefixed
raw query/formatted text. The batch fingerprint hashes its domain, input version,
model revision, query, pair count, and each pair ID/text in order. Lengths/counts
use four-byte unsigned network order; IDs are 32 raw digest bytes inside the hash.
The worker recomputes these bindings rather than trusting caller digest fields.
Response score cardinality, finite numbers, token counts and unique pair IDs are
checked before Go restores candidate order. SHA-256 is not source identity,
authorization or an inference attestation.

Only worker/rerank.py and worker/test_rerank.py are maintained Python exceptions.
The dependency lock is plain text; all core retrieval, input assembly, validation,
rendering and repository tools remain Go. The launcher supplies a minimal explicit
environment, excluding inherited PostgreSQL/R2/Hugging Face credentials and Python
import customization. Weights, virtual environments, runtime caches and diagnostic
JSON stay ignored. Python bytecode writes are disabled for the worker/tests.

The real runtime requires the tested Python 3.14.2 dependency closure and MPS device:
float32, SDPA, batch one, two intra-op/one inter-op threads, raw logits, no truncation
or device/model fallback. Five pinned model assets are streamed through SHA-256
before loading; their directory stays unchanged through load. The opt-in Go asset
fetcher downloads only that pinned public revision, streams bounded bytes, fails
on local mismatches and conditionally installs verified files without overwrite.
Checksums do not make mutable local files a filesystem snapshot. Linux/CUDA runtime
parity, containers, general model selection and public hosting are separate work.

## Established mechanisms and alternatives

[CPython 3.14.2 Connection](https://github.com/python/cpython/blob/df793163d5821791d4e7caf88885a2c11a107986/Lib/multiprocessing/connection.py#L423-L456)
frames ordinary messages with a network-order length and checks a maximum before
reading the body. Its signed/extended format, pickle API, authentication and
long-lived connections are not inherited. Our unsigned capped frames are a
separate protocol.

[TEI v1.9.0's rerank handler](https://github.com/huggingface/text-embeddings-inference/blob/5699247f57e46aa09eb4f8c4cf74114099372fe7/router/src/http/server.rs#L307-L366)
uses HTTP JSON with inference permits. HTTP would bring standard tools/status
handling, with a server and explicit header/body/admission/connection policies.
Its broader router, per-pair admission and truncation behavior differ from this
single trusted local operation. We choose framed JSON for the smaller boundary,
not a measured latency advantage. Revisit HTTP for actual interoperability or
hosting requirements, or if maintaining the frame protocol costs more than it saves.
