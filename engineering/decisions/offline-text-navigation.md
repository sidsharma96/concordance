# Read exact captured text after discovery

Status: the maintainer approved a shared Go lookup/find/read layer for the offline agent
comparison. This is experiment tooling, not native RFC ingestion, a production
read API or a universal document schema. No model evaluation or reserved-question
run is part of this implementation.

## Workload and recommendation before the check

The captured RFC corpus has 1,000 files and 56,412,884 raw bytes. The earlier common
searchable subset admits 996, explicitly excluding three unsupported UTF-8 files
and one NUL-containing file. A question about weak entity tags received RFC 9110's
If-Match section but not the comparison definition it references. The exact capture
contains the missing definition. Changing ranking cannot supply context that a
fixed-input answerer is never allowed to read.

Keep source discovery separate from document navigation. Build a small tooling-only
package, `tools/internal/textnav`, with a concrete immutable corpus and sequential
per-question sessions. The actual consumer, `tools/rfcnav`, reads the existing RFC
pin format and exercises the operations without a model, database or renderer.
It must not parse RFC sections or special-case particular documents. Other retrieval
arms can later return the same exact document references and byte locations.

The simplest alternative is existing fixed evidence: no new code or I/O, but no
follow-up reading. Reopening and hashing a file on each read avoids retaining the
corpus, while repeating verification and exposing later calls to local changes.
A lazy verified-document cache needs eviction, lifetime and memory policy. For this
bounded 56 MB experiment, verify the complete selected corpus once and retain owned
immutable text. Reject a larger input rather than claim this scales to millions of
documents. Lazy loading or a production artifact reader needs a separate measured
workload; this package does not replace `publishedsearch`.

The smallest check is deterministic: open the selected captured bodies under their
pins, find the referenced definition, read its exact bytes and verify its address
and SHA-256 against the retained RFC. Also replace or remove a file after opening:
the existing corpus must still return its verified original bytes, while a new
open must reject changed bytes. Failure to find the retained definition would
challenge navigation or admission; a successful read alone is not evidence that
an agent will choose it or that answer quality improves.

## Primary comparisons and limits

BrowseComp-Plus at
[046949032b0328319cc9a02663a759ec601d9402](https://github.com/texttron/BrowseComp-Plus/blob/046949032b0328319cc9a02663a759ec601d9402/searcher/searchers/base.py)
separates `BaseSearcher.search` from `get_document(docid)`. A search result can be a
snippet while a subsequent operation retrieves the document. Its interface neither
implements our byte/hash checks nor establishes version or authorization semantics.
We borrow that separation, not its abstract inheritance hierarchy or whole-document
response size.

Exa's benchmark
[README at a6a9590b1634fd4bf6f6ea7fd80b0084489c8e2b](https://github.com/exa-labs/benchmarks/blob/a6a9590b1634fd4bf6f6ea7fd80b0084489c8e2b/README.md)
distinguishes supplied-URL extraction from discovery and fixes agent/model/budgets
when swapping search providers in Scout. That motivates common read capabilities
across later retrieval arms, not copying vendor scores or a hosted-web system.
Neither project proves that our proposed API improves answers or latency.

## Concrete data and ownership

`Pin` contains a logical ID, root-relative file path and expected SHA-256. They are
trusted caller configuration for an already-authorized corpus. The ID and path
remain separate from the digest. This is not a signed manifest or permission check.
The RFC consumer alone maps the existing `rfcN digest` records to `rfcN.txt` files.
Unsupported inputs must be explicitly excluded by the operator before opening; the
shared loader never guesses encodings, normalizes bytes or skips bad documents.

`Open` validates all metadata and count/byte ceilings before reading. It reads only
root-relative regular files through an `os.Root`, caps every body and aggregate
logical bytes, verifies SHA-256, and rejects invalid UTF-8 or NUL. Success owns
independent immutable strings and metadata; no caller slice, file handle, root or
context is retained. Errors return no partial corpus. Preflight regular-file checks
avoid knowingly opening special files; they are not a hostile-filesystem sandbox,
a hard I/O deadline or a filesystem snapshot. Hashes establish the accepted bytes.

A reference contains logical ID plus expected SHA-256. Lookup is exact and returns
that reference and body length. Find/read reject an absent ID or changed digest;
there is no fallback to another document or capture. Identical references from
another corpus are acceptable only when that exact ID/digest is also a member of
the current authorized corpus. These are explicit source values, not the temporary
reader-bound handles used by the production HTML searcher.

Read returns an exact half-open raw byte interval `[start,end)` and its text. It
rejects invalid, empty or UTF-8-splitting intervals rather than modifying them.
Returned strings may retain that document's complete immutable body; they do not
retain the whole corpus through a back-pointer. Do not claim returned-byte budgets
bound retained heap. Find returns owned byte-range slices, so caller mutation cannot
alter the corpus. It performs case-sensitive, literal, non-overlapping matching:
no regex, folding, stemming, source-specific section parser or semantic search.
Pagination returns a positive bounded prefix with a next byte position and an
explicit more flag, never an apparently complete truncated result.

## Session budgets and costs

A session retains the immutable corpus and owns mutable counters. A caller uses
one session sequentially for one question; the corpus can be shared by independently
owned sessions. No goroutines, locks, queues, model workers or caches are introduced.
Every admitted operation attempt consumes a call, including invalid input, absent
IDs and cancellation. At exhaustion no further work is admitted. Reads reserve
actual returned source bytes before returning text, including repeated reads.
Failures return a zero operation result. Metadata/find ranges carry no additional
source text; the eventual transport must separately cap encoded metadata, errors,
search snippets and model tokens. This layer is not a complete agent budget manager.

The caller chooses positive open and session limits. The RFC diagnostic uses at
most 1,000 selected pins, 8 MiB per document, 64 MiB aggregate text and 1 MiB metadata.
A session admits at most 32 attempts, 16 KiB returned text, a 256-byte literal and
16 matches per find page. These are diagnostic ceilings, not production targets or
the final agent-comparison budget. The CLI starts one new corpus/session per
invocation and prints human diagnostics; it is not a persistent machine protocol.
A later in-process harness can retain the same verified corpus across questions.

Open costs O(total source bytes plus metadata), with O(total retained text plus
metadata) space and ordinary decoding/read scratch. Lookup is expected O(1) by
logical ID, followed by a digest comparison. Find uses the standard literal search
on the remaining document; a conservative worst-case comparison bound is O(B*Q)
for B scanned bytes and Q literal bytes, with Q explicitly capped. Read validates
bounds in O(1) and returns an immutable substring; serializing it costs O(returned
bytes). Find results use O(page size) additional space. Context cancellation is
cooperative around bounded work; standard string search and local reads are not
preempted mid-call. All operations require a deadline. No fresh timing comparison
on the shared machine belongs in this checkpoint.

## Validation and next boundary

Synthetic tests cover exact identity, complete admission, altered hashes, malformed
text, regular-file bounds, lifetime after file replacement, UTF-8 boundaries,
non-overlap/pagination and attempted/returned-byte accounting. A simple direct scan
is the find oracle. Exercise independent sessions under the race detector. The
manual RFC check uses no questions, labels, models or reserved intents.

Only after this groundwork should a separately frozen agent pilot choose queries
and follow-up reads. Filesystem/grep, BM25 and exact-vector arms must receive the
same operations, source pins and complete workflow budgets. Keep ranking failures,
insufficient reading context, answer errors and judge uncertainty separate. This
implementation neither changes the earlier scores nor establishes a search winner.
