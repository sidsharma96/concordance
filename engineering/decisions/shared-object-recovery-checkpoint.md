# Compare recovery before extracting shared object I/O

Status: implemented local recovery drill.
This is not a shared API or approval to extract code. The user renewed this comparison
while approving the retrieval evaluation set. Existing project acknowledgment,
publication, authorization and retention contracts remain separate.

## Current evidence and differences

Rewind's tested `writer-recovery` revision
`aac325e46fd90fecc0ede269896b2688a4602d4e` exercises writer-process replacement
with native PostgreSQL and controlled surviving HTTP objects. It kills/reaps the
writer, removes its working directory and starts replacement clients. Cases cover
acknowledged acceptance, a stored PUT with a disconnected reply, a lost actual
PostgreSQL COMMIT response, and a crash after upload before publication. This is
local process recovery, not ingest-machine/storage-service loss or provider HA.
It is now merged in PR45 as `b0b53e7566d95af121808303955074b91138f319`,
with the same tracked tree. Its delivered handoff and our
[counterpart assessment](../experiments/shared-object-assessment.md) are recorded;
no common API is selected.
Its extraction record compares Concordance `10a92299c32e7995e74cfd9cbc43a11a1dcc869b`.
Concordance's later target-setup change does not alter object contracts.

Concordance has actual R2/droplet generation publication, saved-key recovery,
fresh-process strict reads and exact quotes, plus an isolated synthetic catalog
restore with surviving R2 bodies. That is different evidence from killing a writer
or dropping the real PostgreSQL commit reply. The separately authorized [local counterpart drill](../experiments/writer-process-recovery.md)
now exercises those two fault placements with surviving local services.

| Boundary | Concordance | Rewind |
| --- | --- | --- |
| Scope | Store-bound authorized UTF-8 collection | Opaque tenant bytes per call |
| Read length | Adapter verifies exact reference length | Adapter bounds read; ingest Store checks catalog length |
| Admission | Caller bounds overlap and supplies deadline | Adapter bounds active calls and elapsed time |
| Publication | Complete generation becomes visible | Independent records become accepted and discoverable |
| Verification reuse | Receipts plus conditional HEAD above strict object path | No agreed equivalent reuse |

Both strict adapters use conditional creation and verified complete reads. Neither
supplies range reads or safe deletion. Keep receipts, catalog schemas, tenant
policy, retention, compaction and success semantics with their applications. No
cross-checkout import, shared service, cache or shared storage engine is approved.

## Proposed Concordance fixture

Use a synthetic actual generation publisher and published reader with a dedicated
local PostgreSQL schema and an owned controlled S3 HTTP peer. Parent/services
survive separately from the writer child. Four cases mirror the useful failure
boundaries: successful publication followed by writer termination; conditional PUT
stored but reply lost; final publication COMMIT applied but its reply withheld;
and writer termination after object confirmation but before final publication.

Replacement receives trusted deployment configuration and the original deterministic
producer inputs/operation identity for an explicit retry. It receives no accepted
manifest reference, cached catalog result, object map or writer-local file. Resolve
published state through a fresh catalog, then strictly open index/articles and
check exact search/quote bytes. An uploaded but unpublished root must not become
readable as a published generation. Keep the original saved key for a deliberate
retry after the old child is known dead; never infer that a timeout stopped it.

A local test-only pgx query tracer can emit a bounded barrier immediately before
the final generation UPDATE, after object/receipt work. Parent arms a bounded
server-message proxy there and resumes the child. Observe actual
CommandComplete(COMMIT) and idle ReadyForQuery, withhold both and disconnect.
Do not accidentally fault an earlier receipt/namespace transaction or merely
throw away a successful Go result. The exact tracer/proxy placement needs source
inspection and architecture approval before implementation; production APIs
receive no test hook or new interface just for this drill.

Every child/pipe/proxy connection has a deadline and bounded protocol/body sizes.
Kill and Wait/reap on all failures; close tracked connections and join goroutines.
Remove only the writer's owned temporary directory before replacement. Use barriers,
not sleeps, to locate interruptions. Count requests/body bytes and retained orphan
objects by phase; do not rerun timings or claim host-loss recovery. Keep remote
provider reads/writes and any hosted schema changes out of ordinary tests.

The smaller existing fresh-client tests remain the oracle. Reusing Rewind's test
ideas does not authorize a shared production interface. After the actual drill
passes and its limits are reviewed, refresh both source revisions and seek a joint
comparison of scope, read length, integrity, errors/retry and admission. A prepared
local note is not a delivered message or jointly accepted API. Explicitly authorize
any cross-chat message before sending one.

## Primary mechanism

PostgreSQL 16's [protocol flow](https://www.postgresql.org/docs/16/protocol-flow.html)
describes completion and ready messages; its
[commit durability settings](https://www.postgresql.org/docs/16/runtime-config-wal.html#GUC-SYNCHRONOUS-COMMIT)
distinguish local WAL flushing from wider recovery guarantees. The locked pgx
v5.11.0 supplies query tracing and bounded pgproto3 message decoding; inspect those
local APIs before implementation. These establish a controlled fault placement,
not provider durability or zero-loss disaster recovery.

## Inspected fault placement

The counterpart writer-process drill is now authorized separately from extraction.
The architecture design review approved the inspected placement before new Go. Keep all changes
in test files and evidence; do not change a production interface to inject faults.

At source baseline `1256d52583d1733603403b4e10df0ad837a21f2a`,
`generationpublish.(*Publisher).publish` confirms the artifact batches, confirms
its root, and only then calls `catalog.PublishInNamespace`.
`catalog.(*Store).publish` starts a transaction, locks the import row, checks the
namespace, executes `UPDATE concordance_imports SET generation_id=$3`, then commits.
A child-local pgx `QueryTracer` matches that exact UPDATE prefix. No other
transaction is in flight on the serial writer pool at this barrier. The parent
checks a still-unpublished binding and all expected stored objects, then arms the
wire proxy before resuming the UPDATE. Namespace binding and receipt commits have
already completed. The same barrier permits killing the writer before the UPDATE.

The pinned [pgx Exec implementation](https://github.com/jackc/pgx/blob/5e583fa7aabfa88b796292f849fc9d7d75ac159d/conn.go)
invokes `TraceQueryStart` before executing SQL. Its
[transaction Commit](https://github.com/jackc/pgx/blob/5e583fa7aabfa88b796292f849fc9d7d75ac159d/tx.go)
executes `commit` and closes the transaction on error. Our proxy withholds the
actual successful `CommandComplete(COMMIT)` and idle `ReadyForQuery`, then close
both connections. The publisher must return a zero publication with
`catalog.ErrOutcomeUnknown`; a direct parent lookup must find the committed row.
PostgreSQL's [message flow](https://www.postgresql.org/docs/14/protocol-flow.html)
explains these boundaries. Idle completion is not evidence of host-loss durability.

The test executable supplies deterministic `makeBundle` producer inputs afresh in
each child, not a serialized result or saved artifact. The replacement may choose
a different candidate generation ID while retaining the original operation key
and input fingerprint. A published retry must recover the original ID with zero
object requests; an unpublished retry must publish the new candidate while the
old uploaded root remains unreferenced and retained. Before retry, the strict
published reader must reject the unpublished candidate without S3 requests. A
changed fingerprint must conflict without changing publication or object state.

The parent owns one fresh disposable schema, a bounded controlled HTTP peer and
an optional plaintext-local-only protocol proxy per case. Children use explicit
configuration via bounded pipes, a cleared environment, an empty private working
directory, a deadline, bounded diagnostics, a bounded transport and a small pool.
Forced termination is checked and reaped, then the directory is removed before
replacement starts. Schema cleanup runs after children/proxy/HTTP work is joined.
Reuse the existing fixture and small fresh-client oracle, rather than building a
new fixture framework. No shared library dependency on Rewind is introduced.

Request counts and body bytes are sampled at quiescent child protocol barriers:
writer publication, replacement initial read, explicit retry, changed-input
conflict and final read. Count successful response-body writes at the controlled
HTTP peer and request bytes actually read there, including missing-object XML.
These are body-I/O observations, not network delivery, headers, TCP/TLS bytes,
provider billing, allocation or latency. Retained object counts and exact bodies
are separate assertions. Every server body is bounded and total retained state is
capped; tracked connections and forwarding goroutines are closed and joined.

The simplest existing alternative discards a successful Go result and reopens a
pool. It remains a useful oracle, but cannot locate a wire COMMIT failure or prove
independence from a killed process's files. This drill adds those boundaries with
local surviving services. Revisit extraction only after both implementations and
the three known contract differences have been jointly compared; no common API is
settled by a passing test.
