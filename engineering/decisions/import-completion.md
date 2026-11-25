# Import completion

Status: accepted design. The maintainer chose synchronous publication after comparing
completion on publication with durable acceptance of a background job. The bounded
prepared importer and trusted-operator CLI now implement it.

## Decision

The core import operation returns success only after a complete generation has been
published and is ready for new queries that explicitly select its generation ID.
Required remote objects and the publication metadata must survive loss of the
writer's local files. Existing queries may continue reading the generation they
already opened.

The sequence is capture, processing/index construction, complete remote writes,
catalog publication, then successful return. The exact interleaving of construction
and writes is an implementation detail; incomplete generations must stay unpublished.
A successful return must not depend on uncompleted background indexing or a local
cache that is the only copy of required data.

The [snapshot decision](import-snapshots.md) defines the request: select the complete
intended scope and let the source adapter capture its inventory. A fetch failure
fails the import; omission under a deliberately narrower scope changes only the new
generation's membership. Patch requests are deferred.

This illustrates the intended caller contract; names and types are API sketches:

```go
generation, err := engine.Import(ctx, request)
if err != nil {
    return err
}
results, err := engine.Search(ctx, generation.ID, query)
```

Synchronous describes the completion promise. It does not require an HTTP request,
holding a global lock throughout indexing, or serializing every internal operation.
The generation-ID query above is an internal/API sketch, not a decision that users
must provide generation IDs. Beginning with a small known corpus is a first experiment,
not a permanent limit on documentation size.

## Why start here

The initial workload is a bounded documentation collection with one publishing
writer. A CLI or scheduled importer can wait for completion. This contract makes
the first experiment follow one operation through capture, external storage,
publication, querying and recovery.

It keeps the initial system small while exposing the storage problems we need to
solve: incomplete uploads, orphan objects, ambiguous replies, consistent generations
and recovery without local files. It does not make those problems disappear.

## Alternative considered

A durable background-job API could return a job ID once the source data and job
metadata are durably accepted. Workers would index and publish later:

```go
job, err := engine.SubmitImport(ctx, request)
if err != nil {
    return err
}
status, err := engine.ImportStatus(ctx, job.ID)
```

That fits long imports and interactive clients that should disconnect while work
continues. It adds durable job states, worker recovery, duplicate-execution handling,
admission/backlog limits and progress reporting. A submission response does not mean
the content is searchable, and queuing does not by itself make indexing faster.

We deferred this orchestration for the first implementation. A later worker can call
the synchronous import operation and record its result; the job system still needs
its own durability and recovery design.

## Consequences and unresolved contracts

Callers wait through the import, so large collections can cause long waits. A timeout
or lost reply can occur after successful publication. An error therefore cannot
universally mean that nothing happened. The [retry decision](import-retries.md)
selects saved import keys and conflict behavior. Detailed outcome discovery and
cancellation must preserve that contract. Synchronous completion does not automatically require durable resume
of every unsuccessful attempt.

The subsequent [catalog/publication decision](catalog-publication.md) selects
PostgreSQL as the publication authority and R2 for immutable files/manifests. The
bounded publisher confirms required objects before guarded catalog publication;
[the native CLI exercise](../experiments/hosted-import-command.md) checks this path
and completed-result recovery on the hosted providers.

A completed saved-key retry returns the record of an earlier publication before
object access; it is not a fresh availability check. The native blocked-route
experiment returns that original result while a query fails to reach object storage.
Opening and quoting still require their stores to be reachable. Existing readers
remain pinned, and no global `latest` cache is implied. Complete provider faults,
production recovery targets, generation retention and reader-safe cleanup remain open.

Revisit background submission when concrete imports exceed acceptable caller wait
times, need to outlive client connections, or need scheduled multi-tenant processing.
No latency benchmark or production result motivated this decision. Validate the
contract through interruption/recovery tests and a real remote-storage experiment.
