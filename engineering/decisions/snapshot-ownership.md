# Borrow unchanged snapshot input

## Decision and scope

The user chose borrowing caller data (option B) over copying the entire input for
the proposed in-memory search snapshot. This records ownership direction, not an
implemented snapshot constructor, a persistent schema or a publication guarantee.
Current `MatchAll` and `RankBM25` borrow only until their calls return; they retain
no input afterward. The proposed snapshot would extend the borrowing period across
calls, so that difference must be visible in its API documentation.

The caller supplies authorized data, controls all aliases and keeps borrowed
articles, plans, term fields and source associations unchanged for every use that
relies on them. The exact set retained by the eventual constructor must be stated;
newly computed internal buffers may still be owned by the snapshot. Borrowing does
not mean every allocation disappears or that all outputs are borrowed.

## What Go copies

Assigning a slice or passing it to a function copies a descriptor referring to an
underlying array, not the array elements. Changing an element through one alias can
therefore affect another. A copied struct containing slices still shares their
arrays. `slices.Clone` copies the outer elements by assignment; nested slices in
those elements still refer to shared storage. A complete independent copy must
account for every mutable level that the owner intends to reuse.

Strings themselves are immutable in ordinary safe Go. Replacing a string field in
a shared article block or term slice is still mutation of that shared element.
An append can reuse capacity; it must not be treated as a reliable copy operation.
A full slice expression limiting capacity can constrain append reuse but cannot
make existing elements or nested slices read-only.

The Go language/specification and standard-library documentation establish these
semantics: [slice types](https://go.dev/ref/spec#Slice_types),
[append](https://go.dev/ref/spec#Appending_and_copying_slices), and
[slices.Clone](https://pkg.go.dev/slices#Clone). No unsafe aliasing is proposed.

## Lifetime and ownership

Ownership here means responsibility for controlling mutation and reuse. Go does
not enforce this borrowing discipline in its type system. Reachable slice references
keep their backing storage reachable for garbage collection; an original variable
may leave scope or be reassigned without erasing the storage referenced elsewhere.
Lifetime is not a requirement to keep that variable in scope or manually free memory.

Keep shared contents unchanged from construction through the last use of the
snapshot and any returned borrowed views. Discarding the snapshot variable alone
is not enough if a result still depends on shared data. Owned result copies can
have independent lifetimes; the eventual API must explicitly distinguish them.
Do not recycle an input buffer, mutate an alias, or return borrowed storage to a
pool while readers or borrowed results can still use it. Cancellation requests do
not establish that a reader has finished; wait for completion before reuse.

Read-only concurrent use can be safe when all shared contents are unchanged and
each operation owns its mutable scratch. This decision does not certify a future
snapshot's concurrency safety. Concurrent writes can cause data races; even a
sequential write between preparation and quotation can silently mismatch terms,
locations and cited text. The race detector cannot enforce the entire contract.

## Why borrow first

The intended pipeline controls its prepared inputs and can leave them unchanged.
Borrowing avoids a whole-input defensive copy without requiring a second mutable
representation. Copying is useful if callers need to reuse or edit their buffers
while readers continue, or if an API must provide independence from those callers.
The earlier recommendation for copies was defensive, not supported by an observed
caller need or measured performance comparison.

Borrowing still retains reachable allocations. A short slice can retain a large
backing array; substring references can retain larger string storage. Logical
input limits do not by themselves bound the size of all retained backing allocations.
A constructor can own lookup tables/scratch while borrowing bulk input; count both
its additional allocations and the original data it keeps reachable. There are no
measured memory savings or latency claims yet.

For the current bounded pipeline, choose the simpler ownership contract rather
than introduce pooling, reference counting or explicit release methods without a
consumer that needs them. Revisit copies, ownership transfer or bounded on-demand
readers when real callers require mutation/reuse or retained memory measurements
show a problem. Define persisted reader lifetimes and deletion safety separately
before relying on this direction for R2/catalog retention.

Tests for the eventual implementation must check that construction/search leave
input unchanged, returned views/copies follow their documented lifetimes, and
concurrent readers own independent scratch if concurrency is supported. A small
private executable used for the book demonstrates shared and shallow-copied slices;
it is an illustrative language experiment, not snapshot runtime coverage.
