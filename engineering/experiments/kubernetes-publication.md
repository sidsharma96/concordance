# A first corpus can wait on thousands of remote objects

Status: interrupted live experiment with an unpublished saved binding, not a
successful Kubernetes publication. The accepted restricted input profile is unchanged.
The operation key, original capture and pinned Git objects remain available for a
deliberate same-input retry. No object or catalog record was deleted.

## Workload and operator boundary

The [local construction](prepared-import.md#local-construction-of-the-selected-kubernetes-corpus)
contains 38 documents in three editions, six image references and three indexes.
Its cost fixture has 5,017 distinct staged objects including root and 132,106,434
bytes. Live collection/candidate identifiers change root metadata; this staging size
is not measured live bucket occupancy.

The disposable operator initialized a fresh globally unique collection, namespace,
key and candidate. Its protected operation file saves capture path/digest and Git
route without credentials. File, operation-directory and new-directory-parent entries
are synced before remote work. Original HTML/recipe files and Git objects remain
available. This is retained manual operator state, not a product journal, import
command or locked host-loss rebuild plan.

The probe uses the existing restricted droplet runtime login through an owned SSH
forward, pool two, catalog schema and private R2 bucket. No migrations, test schemas,
grants or Receipt changes ran. Initial strict SSH authentication rejected an unknown
domain-name entry before import effects. The sole resolved address's existing pinned
key matched the presented key; using that address as HostKeyAlias succeeded with
strict checking. No new key was accepted or global SSH configuration changed.
Credentials and routing identifiers remain private.

`BindNamespace` registers configuration before importer construction. The actual
`ImportPrepared` builds all artifacts and runs publisher preflight before binding
the saved operation key or making S3 requests. Confirmation stays sequential within
32-object/16 MiB batches. Artifacts precede root, and guarded PostgreSQL publication
follows every confirmation. Caller ceilings match the local probe, with a 20-minute
context and 20-second HTTP timeout. These are experimental settings, not defaults.

## Remote progress exposed waiting

The log counts attempted HTTP round trips, not confirmed object totals:

```text
object_requests=250 elapsed=2m27s
object_requests=500 elapsed=4m47s
```

The operator stopped the attempt after observing that pace, before its deadline.
This is intentional cancellation, not a provider outage or timeout measurement.
The final error preserved an unresolved object-write outcome and instructed reuse
of the same bytes/scope. Successful uploads without receipts and an uncertain final
PUT may remain. This log does not count those precisely.

A fresh reader returned `catalog.ErrNotFound`. Read-only catalog queries found:

| Observation | Result |
| --- | --- |
| Saved operation binding | Present |
| Published generation | Absent |
| Namespace | Active, epoch 1 |
| Current verified receipts | 160 |
| Receipt byte total | 1,975,176 |

Receipt totals are prior verification evidence, not a full current R2 inventory.
No partial generation is exposed. A later retry must keep the original key/inputs,
check receipts through conditional HEAD and strictly reconcile objects lacking
receipts. No retry or cleanup was launched.

## Separate catalog and object waits

With the writer stopped, five `SELECT 1` calls took 15.646, 7.446, 10.034, 9.513 and
9.154 ms through the same runtime/SSH route. This is a small-query round-trip sample,
not a benchmark of receipt transactions or PostgreSQL capacity.

The read-only object experiment selected the first 32 current-epoch receipt hashes
in byte order with positive bodies at most 64 KiB, totaling 222,802 bytes. Every read
used the strict object reader's length/SHA-256/close checks. It rotated 1/4/8-worker
order across three samples over the exact same objects:

| Sample | One worker | Four workers | Eight workers |
| --- | ---: | ---: | ---: |
| 1 | 10.117389 s | 2.576276 s | 1.262854 s |
| 2 | 7.442824 s | 2.433001 s | 1.262211 s |
| 3 | 8.200683 s | 2.504599 s | 1.281979 s |

All 288 reads passed. The diagnostic made no uploads, binding/receipt writes or
quarantine changes. These samples support investigating object-path waiting rather
than changing the catalog or extraction; they do not assign all import time to one
cause.

Environment: Go 1.27.2, macOS 26.6.2 arm64, eight logical CPUs/16 GiB host, existing
droplet catalog and R2 target, 20-second HTTP timeout. Caches, host load, connection
reuse and provider conditions were uncontrolled. Successful small GETs do not model
missing GETs, conditional PUTs, larger objects, throttling or complete publication.
No full-import speedup, peak-memory or production capacity is measured.

## Next checkpoint: bounded confirmation overlap

The existing sequential path is the simplest alternative and a useful oracle. It
has one active object confirmation but serializes remote waits. A proposed alternative
permits at most eight simultaneous object confirmations **inside the existing bounded
batch**, keeping each object's strict creation/reuse protocol. Write concurrency and
failure work change, so this remains a user decision. No concurrency implementation
or accepted default follows from this record.

The proposed path must retain batch count/byte admission, independent result positions
and unchanged borrowed buffers until all workers join. On failure, cancel remaining
work, wait for every worker, preserve material integrity/uncertain-write causes and
handle quarantine without losing it to peer cancellation. Return no partial refs,
save no unsupported proof, confirm root last and preserve guarded publication. More
in-flight operations may leave more uncertain writes. Conditional creation and the
saved key remain necessary. Overlapping waits reduces neither request count nor
storage semantics. Transport concurrency does not change captured input meaning:
keep the same key for this intended operation if that transport is implemented.

As inspected in October 2026, restic 0.18.0's
[`connectionLimitedBackend`](https://github.com/restic/restic/blob/d401ad6c1ec93a8fbcfcd9f6855a7a4468878ec2/internal/backend/sema/backend.go)
limits Save/Load/Stat with tokens. Lock-file exceptions belong to its backup workload,
not Concordance. Rclone 1.70.0's
[`startTransfers`/`stopTransfers`](https://github.com/rclone/rclone/blob/9d464e8e9a56a988bbb5f701006173c9bbefeb6a/fs/sync/sync.go)
starts configured workers and closes/joins their queue. Its many-file copy workload
and [transfer/checker controls](https://rclone.org/docs/#transfers-int) illustrate bounded
overlap, not Concordance's cross-store publication/integrity guarantees. Borrow the
mechanism, not their defaults, deletion policies or architecture.

Revisit if bounded concurrency fails to improve actual confirmation, raises memory
or provider errors materially, or complicates failure recovery. Keep bundling, new
storage formats and background jobs separate until evidence needs them.

## Selected evidence

Ignored operator sources are `.tools/probes/corpus-live/`. Reviewed pre-write source
is captured in `.tools/reviews/corpus-live-plan-2.txt`; its original publication binary
stayed unchanged when read-only diagnostics were added. Raw records are
`corpus-live-publish.txt`, `corpus-live-after-stop-read.txt` and
`corpus-live-status-latency.txt` in that private directory. A successful fresh reader
for this corpus remains open. The earlier synthetic publication/restore result is
separate and still valid.
