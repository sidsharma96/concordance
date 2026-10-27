# Borrow unchanged snapshot input

## Decision and scope

The user chose borrowing caller data (option B) over copying the entire input for
the in-memory search snapshot. `internal/htmlsearch.New` now implements that
direction; it is not a persistent schema or a publication guarantee.
Current `MatchAll` and `RankBM25` borrow only until their calls return; they retain
no input afterward. The snapshot extends borrowing across calls; its API documents
that difference.

The caller supplies authorized editions/articles, controls all aliases, and keeps
borrowed block/group arrays and source associations unchanged for every dependent
use. The constructor retains its edition/article input. It owns newly computed term
arrays/strings and compact location tables. Borrowing does not mean every allocation
disappears or that all outputs are borrowed.

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

Keep shared contents unchanged through the last dependent use of the snapshot or
its references. Discarding the snapshot variable alone is not enough if a reference
still needs that state. The accepted [result contract](search-results.md) returns
owned hit slices with opaque references retaining the state, and independent quote
strings on demand. No mutable block views escape through these operations.
Do not recycle input storage or mutate another alias while readers still use it.
Cancellation requests do not establish completion; wait before reuse.

`htmlsearch` supports concurrent read-only operations with unchanged input and
independent per-call scratch. This does not make arbitrary future mutable caches
safe. Concurrent writes can cause data races; even a sequential write between
preparation and quotation can silently mismatch terms, locations and cited text.
The race detector cannot enforce the entire contract.

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

Tests exercise unchanged input, cross-state reference rejection, owned result
storage, quote limits and concurrent read-only search/quotation under the race
detector. The [component reference](../experiments/search-snapshot.md) records their
scope; tests do not enforce the caller's lifetime discipline. A small
private executable used for the book demonstrates shared and shallow-copied slices;
it is an illustrative language experiment, not snapshot runtime coverage.
