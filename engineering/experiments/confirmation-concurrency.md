# Confirm objects concurrently without moving the publication boundary

The accepted [decision](../decisions/confirmation-concurrency.md) permits at most
eight active object chains per admitted `objectreuse.Ensure` call. The publication
limit passes this setting through. Zero preserves sequential operation. This initial
implementation changes scheduling, not object formats, captured-input identity or
the required remote operations. Live retry results are recorded separately.

## Representation, ownership and failures

The caller admits count/aggregate bytes, hashes every input, preserves duplicate
output positions and loads receipts before starting workers. Distinct input indexes
travel over an unbuffered channel to at most min(configured workers, distinct objects)
goroutines. Each worker writes its own indexed proof/error positions. Inputs and all
aliases remain borrowed unchanged through return. Channel dispatch has no retained
background queue. `sync.WaitGroup.Wait` joins all workers before the caller reads
their slots or releases inputs.

An object error cancels the work context. Dispatch stops cooperatively, closes the
channel and joins every worker. Errors containing only cancellation leaves are
omitted while the caller remains live only if another material worker error survives.
A regression test supplies a transport-originated cancellation for warm objects;
the error must remain rather than bypassing skipped HEAD checks. Other errors stay in distinct-input order;
uncertain-write/cancellation combinations remain intact. Error classification must
check material causes rather than treating any cancellation marker as benign.
Quarantine runs after joining using the original caller context. Its failure remains
observable, and caller cancellation can still prevent that database write.

No failed batch records partial successful proofs or returns partial references.
After complete success, valid new opaque proofs are compacted in place, recorded
under the original namespace epoch and followed by a namespace check. Missing
validators remain strict-only. Artifact batches still finish before the root, and
guarded catalog publication still follows root confirmation. More simultaneous
requests can leave more uncertain writes after failure; nothing deletes them.

Hashing remains O(B) for B input bytes; indexed results and dispatch state require
O(K) scratch for K admitted objects plus at most eight workers. Expected map-based
deduplication is O(K), while proof insertion retains its O(K log K) sort. No claim
about adversarial map worst-case CPU or exact heap follows. Remote body buffers,
SDK/HTTP state and GC are additional memory; input/batch bounds still apply.
Concurrency is per call, not process-wide. Publisher instance admission is unchanged.

## Local evidence

Real SDK HTTP fixtures and a dedicated local PostgreSQL 14.17 database exercise
zero/one/two/eight workers. A barrier holds the first wave of HTTP calls and observes
the selected limit before release; final active/maximum counts and exact output
references check bounded overlap and order. Seventeen distinct bodies among nineteen
input positions require 34 GETs/17 PUTs cold and 17 HEADs warm at every setting,
including duplicate and empty-body positions. This is operation-count and scheduling
evidence, not a throughput benchmark.

Fault tests hold a peer inside response-body Close: integrity failure stops it, but
Ensure cannot return or quarantine until Close is released. After joining, quarantine
succeeds with the live caller and the error does not report cancellation-only peers.
Another test reaches a PUT effect before a peer integrity failure cancels its reply;
both integrity and uncertain-write causes survive. Caller cancellation after a good
proof slot exists still saves no partial receipt and waits for response Close.

Publisher tests block all seven artifact chains before releasing them, observe an
unpublished binding and no root request, then confirm successful root-last publication.
A transient artifact failure skips the root, and an explicit same-key retry succeeds.
A completed-key retry returns its original generation without further S3 calls.
Existing sequential scan/reuse/publication tests remain the oracle.

The implementation review found an actual failure in the first cancellation filter:
a transport-originated cancellation with a live caller could erase the only failure
and let receipt-backed references escape without completed HEAD checks. The focused
regression failed at both zero/sequential and eight-worker settings before the fix.
Suppression now requires another surviving material error. The same regression
passes after the repair. The before/after logs preserve that distinction rather than
treating the earlier passing suite as evidence for the missing case.

`make verify` passed after that repair against dedicated local PostgreSQL 14.17,
including uncached repository-wide race tests, build and the final 203-file staged
artifact check. Required code and architecture reviews inspected the corrected
implementation and operator plan before the deliberate retry. This source change
merged in PR 34 at `16fc0dc796d4abc353a0ba8a10e0bb312ff15a50`.

Raw local records are ignored under `.tools/reviews/confirmation-*`, notably
`confirmation-cancel-regression-before.txt`, `confirmation-cancel-regression-after.txt`,
`confirmation-verify-3.txt`, and the final code/architecture reports. Live retry
measurements belong to the [corpus record](kubernetes-publication.md).
No provider outage, live cold-write benchmark, peak-memory bound or production
capacity follows from these local fixtures.
