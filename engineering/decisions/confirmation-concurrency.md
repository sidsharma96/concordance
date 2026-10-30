# Bound object confirmation overlap inside each admitted batch

Status: accepted. The bounded internal implementation exists; local and live
evidence are recorded separately in the experiments.

## Problem and evidence

The first Kubernetes corpus has 5,017 staged objects, including its root. Its
interrupted sequential publication reached 500 attempted HTTP requests in 4m47s;
the generation remains unpublished. A separate fixed 32-object read experiment
took 7.44–10.12 seconds with one worker and 1.26–1.28 seconds with eight. Those
successful small GETs do not measure cold confirmation or full-import speedup;
see [the experiment](../experiments/kubernetes-publication.md).

We choose at most eight concurrent object-confirmation chains within an already
admitted batch. Each chain keeps the existing conditional HEAD or strict
GET/conditional PUT/verified GET protocol. Eight is an initial experimental ceiling,
not a provider capacity claim. Keep one-worker operation available as the simplest
alternative and oracle. Combining objects into a new storage format is separate work.

## Contract and implementation

Add `MaxConcurrentObjects` to reuse and publication limits. Zero retains sequential
behavior for existing callers; values 1 through 8 explicitly select the limit;
negative values and values above 8 fail construction. Actual worker count is the
smaller of that limit and distinct objects in this call. This is a per-call bound,
not process-wide admission: callers still bound simultaneous Store calls, and a
Publisher still admits only one nonwaiting caller per instance.

Hash and deduplicate all admitted inputs and perform the existing bounded receipt
lookup before dispatch. Use fixed workers and an unbuffered index channel; no
per-object goroutine or retained background queue. Each index has one writer for
its proof/error slot. Do not copy input bodies. The caller owns every borrowed
alias unchanged until return, including error and cancellation paths.

On an object error, cancel peer work, close dispatch and join every worker. Suppress
errors whose entire unwrap tree consists only of `context.Canceled` only when the
caller context remains live and another material worker error survives. A transport
can itself return cancellation; never erase the batch's only failure or infer that
every cancellation error originated from stopping peers.
Preserve other worker errors in stable input order, especially integrity and uncertain
write causes. A mixed uncertain-write/cancellation error remains intact; callers
must inspect material causes before treating cancellation as an ordinary stop.
If the caller is done, also preserve its error. Only after joining, attempt any
required quarantine using the original
caller context, not the canceled peer context. Caller cancellation can still prevent
quarantine; preserve that failure. Cancellation is a request, not proof that an R2
write did not happen. A worker already admitted at cancellation may reach its next
context check; no claim of an atomic stop is made.

If any object failed, return no references and save no proof from that batch. Some
objects may already exist remotely. Otherwise collect valid opaque proofs, record
them through the original namespace epoch guard and recheck that namespace. No
publication contract changes: all artifact batches finish before root confirmation,
and root finishes before `PublishInNamespace`. A receipt conflict, failed namespace
check or canceled caller returns no partial result.

## Cost, failures and recovery

For K admitted inputs and B input bytes, hashing remains O(B), scratch O(K), and
worker count W <= min(8, distinct K). Remote request counts and stored bytes are
unchanged on a successful path: warm objects need HEAD; cold objects may need three
SDK operations each. At most W object chains overlap per call. Verification buffers,
HTTP/SDK state, goroutine stacks and GC add memory beyond admitted input bytes;
the worker limit is not a whole-process heap limit. No new dependency is required.

More in-flight writes can produce more uncertain outcomes on interruption. Retain
the original saved operation key and exact capture for the intended live retry;
transport scheduling is excluded from captured-input identity. Do not rekey, delete
objects or automatically retry. Conditional creation and strict reconciliation of
objects without receipts remain necessary.

## Established mechanisms and their limits

As inspected in October 2026, restic 0.18.0's
[`connectionLimitedBackend`](https://github.com/restic/restic/blob/d401ad6c1ec93a8fbcfcd9f6855a7a4468878ec2/internal/backend/sema/backend.go)
acquires/releases tokens around Save/Load/Stat operations. Its lock-file exception
serves backup lock refresh and is not part of our proposal. Rclone 1.70.0's
[`startTransfers` and `stopTransfers`](https://github.com/rclone/rclone/blob/9d464e8e9a56a988bbb5f701006173c9bbefeb6a/fs/sync/sync.go)
start configured transfer workers and close/join their queue. Both workloads move
many remote files; neither establishes our receipt, quarantine or cross-store
publication guarantees. Borrow bounded overlap and explicit joining, not defaults
or application policies.

## Validation and revisit

Exercise real SDK/local PostgreSQL cold and warm paths, duplicate/order behavior,
exact concurrency ceilings, one-worker equivalence, cancellation while a peer owns
a response body, suppression of cancellation-only peers with a live caller,
simultaneous integrity/uncertain failures, quarantine after peer
cancellation, failed-batch proof suppression and publication ordering. Run race tests.
After independent review, deliberately retry the retained operation and use a fresh
process to verify published root/index/article/quote bindings. Report actual request
counts, elapsed time and limits; do not extrapolate read-only samples into upload
throughput. Revisit the limit if memory, throttling, failure recovery or measured
confirmation performance make eight unsuitable.
