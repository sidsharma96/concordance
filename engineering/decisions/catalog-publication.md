# Catalog and publication

Status: accepted design. Use PostgreSQL for the authoritative publication catalog
and R2 for immutable source, search-index and manifest objects. The user accepted
this split with the requirement that failures between the stores are addressed.
The first PostgreSQL catalog component is implemented and tested locally. The full
cross-store protocol is not implemented. The S3 object adapter has HTTP fixture
tests and a passing live creation/reuse/fresh-client-read probe against the private
`concordance-data` R2 bucket. Hosted PostgreSQL and the combined recovery path remain pending.

## Responsibilities

R2 holds the bulk data. A manifest describes one generation's objects, their integrity
information and compatible format/processing versions. Manifests are immutable data;
their presence in R2 does not make a generation published.

PostgreSQL is the publication authority. Its records identify published generations
and their manifest references. Keep coordinated catalog changes in one transaction:
the generation record, any applicable collection pointer and the import's completion
record must not disagree. The subsequent [import retry decision](import-retries.md)
selects saved operation keys. The first schema is described in the
[catalog experiment](../experiments/import-catalog.md); it is not the full importer.

Keep posting lists, vectors and individual ranking comparisons out of the catalog.
The catalog can grow to support collections, versions and import status without
turning PostgreSQL into the retrieval algorithm. This does not mandate one database
row per passage or a single unbounded manifest for a large collection.

## Publication order

1. Capture and process a source revision; construct the generation's files.
2. Write the required immutable files and manifest to R2. Confirm required writes
   and integrity according to the storage contract before catalog publication.
3. In a short PostgreSQL transaction, publish the complete generation and its related
   catalog changes. Perform remote uploads outside the database transaction.
4. Return success after the publication commit is confirmed. New queries can resolve
   the published generation and read its referenced data.

Catalog reads that promise the just-published generation must observe at least that
commit. A lagging replica or stale cache must not turn acknowledged publication into
a false missing-generation result. This requirement does not choose a cache strategy
or require a database lookup for every posting or vector comparison.

There is no transaction spanning PostgreSQL and R2. Database rollback cannot undo
remote writes. Ordering makes incomplete imports stay unpublished; it does not remove
the need for recovery, retry identity, retention and cleanup protocols.

## Failures between the stores

| Interruption | Required behavior |
| --- | --- |
| R2 write fails or has an unresolved outcome before publication | Do not publish the generation or return success. Resolve uncertain writes with bounded retries and identity/integrity checks under the storage contract. |
| R2 writes finish; PostgreSQL definitely rejects or rolls back publication | The generation stays unpublished. Unreferenced objects may remain. Do not promise that every timeout means rollback. |
| PostgreSQL commits; its reply or the caller's final reply is lost | Treat completion as uncertain to that caller. Discover the original outcome with the saved import key; do not blindly create a new publication. |
| PostgreSQL is unavailable | New publication and catalog-dependent resolution/outcome discovery fail explicitly. A previously pinned query can only proceed under its valid authorization, data-availability and lifetime contract; offline serving is not promised. |
| PostgreSQL says published; R2 reads fail or return missing/corrupt data | Report an availability or integrity failure. Do not invent empty results, combine generations, or silently substitute an older edition. Database commit cannot guarantee perpetual object availability. |
| Publication metadata is lost or restored to an earlier state | Restore/reconcile the catalog under an explicit recovery policy. An R2 manifest alone is not proof that publication committed; bucket listing must not silently republish it. |

Keep automatic object deletion disabled in the first storage slice. Before cleanup
is implemented, define reader protection, in-flight upload protection, ambiguous
publication handling and a safe orphan-identification rule. A missing catalog row
observed during an outage, in-flight commit or old backup is not sufficient proof
that an object can be deleted. Disabling cleanup retains extra data; it is not a
complete retention policy or a promise to store every generation forever.

## Recovery and evidence required

Publication metadata must survive loss of the import machine as well as the R2
objects. A PostgreSQL database whose only durable copy lives on that same machine
does not satisfy the accepted import contract. Database hosting, durability settings,
backups and disaster-recovery objectives still need a concrete deployment choice.

The first storage implementation must exercise partial uploads, complete uploads
followed by catalog failure, lost publication replies, duplicate retries, catalog
unavailability, object-read failures and recovery without local writer files.
Use fault injection for each boundary and a real R2/PostgreSQL recovery experiment.
Mocked successes do not establish provider equivalence or remote durability.

## Why this split

An R2-only catalog would remove a service and suit isolated immutable snapshots.
We chose PostgreSQL for the larger documentation system: related publication metadata
can change together, and collection/version/import queries can use a conventional
database instead of a growing custom metadata protocol. R2 still carries the bulk
storage and the object-storage learning work.

This adds database operation, migrations, connections, backup and availability work.
It does not prove a throughput or latency advantage. Metadata contention and catalog
availability need measurement as workloads grow. Preserve bounded manifests/reads and
generation consistency rather than assuming either provider eliminates scaling limits.

Synchronous import completion is independent of this choice; a future background
worker can use the same publication protocol. Generation pinning is an internal
consistency mechanism, not a requirement that users supply generation IDs. Starting
with a small known corpus is an experiment strategy, not a permanent product limit.

The catalog and failure policy belong to Concordance. Rewind currently documents the
same PostgreSQL/R2 split as a candidate. Shared object primitives do not require a
shared schema, catalog, acknowledgment contract or deployment.

Sources: [PostgreSQL transactions](https://www.postgresql.org/docs/current/tutorial-transactions.html)
and [R2 consistency and ambiguous writes](https://developers.cloudflare.com/r2/reference/consistency/).
