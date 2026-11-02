# Compare recovery before extracting shared object I/O

Status: prepared recovery checkpoint, not a shared API, implementation approval or
claim that a new Concordance crash drill passed. The user renewed this comparison
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
or dropping the real PostgreSQL commit reply. No counterpart to those last two
fault placements is claimed here.

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
