# Complete snapshot imports

Status: accepted design. The user chose option A: an import describes the complete
intended scope, rather than a patch to a previous generation. The bounded prepared
importer and generation manifest now implement this contract; the
[historical update replay](../experiments/documentation-update.md) exercises changed
source and explicit selection on the hosted stores.

## Contract

The request selects the full source, pinned revision, edition/path scope and
processing settings intended for the new generation. The source adapter enumerates
and captures the complete inventory for that selection. The caller does not supply
a second, independently authoritative claim of inventory completeness.

For example, generation G1 contains Pods and Deployments. A new request whose full
selected scope contains only Pods produces G2 with only Pods. G1 and its objects
remain retained. Removing membership from a new generation does not delete stored
bytes, change an older generation or establish a mutable current-generation pointer.

Failed or incomplete enumeration, dependency capture or document acquisition fails
the import. Never publish the successfully fetched subset or infer deletion from a
fetch failure. The pinned Git adapter can enumerate a tree at an exact commit;
future providers need their own revision, completeness, permission-change and
deletion contracts before their snapshots can be published.

These Go calls illustrate request meanings, not working APIs or settled types:

```go
// A: select everything intended for the new generation.
generation, err := engine.Import(ctx, Snapshot{Scope: selection})

// B, deferred: change an explicitly pinned previous generation.
generation, err := engine.Apply(ctx, Patch{
    Base: oldID, Upsert: changes, Remove: removals,
})
```

The coordinator must compute a trusted, versioned fingerprint of the complete
captured-input description, including actual inventory, pinned selection and
settings. Bind that description to the saved operation key before remote writes,
as required by the [retry contract](import-retries.md). A caller-asserted digest or
a mutable branch name alone cannot establish equality. The
[captured-input identity](captured-input-identity.md) specifies the implemented
Git/HTML fingerprint; [document identity](document-identity.md) specifies source/path
bindings. Other provider formats still need deliberate contracts.

Success still means [synchronous publication](import-completion.md) of a complete,
queryable generation. Confirm all required immutable artifacts before catalog
publication. This request contract does not settle manifest encoding, admission
budgets, activation, cancellation, resume or physical retention/deletion.

## Why start here

The concrete starting workload is 38 Kubernetes article-version entries across
three pinned release snapshots. Complete selection keeps membership explicit and
avoids requiring a prior generation, stale-base policy or removal protocol just to
describe the first import. No end-to-end cost comparison between the request forms
has been measured.

A complete snapshot does not require re-uploading or reprocessing all bytes. Reuse
unchanged objects or whole edition files only when exact input and processing
bindings establish compatibility, with the accepted verification rules. The current
`htmledition.Build` rebuilds a changed edition from all its retained articles;
neither request form provides incremental posting updates.

For N selected documents, inventory and the accepted flat manifest require O(N)
entries. Acquisition and processing additionally depend on total captured bytes and
content; request semantics alone do not bound these costs. The importer still needs
explicit byte, entry, memory and remote-work budgets. Large collections need measured
admission and eventually paged metadata, persistent indexing and safe retention.

## Deferred alternative and revisit condition

A patch names a pinned base generation, explicit upserts and removals. In the same
example, upserting only Pods leaves Deployments present unless it is removed.
The coordinator would materialize and validate the resulting complete generation
before publication. A patch is not permission to publish a partial generation.

With C changes, a patch can carry O(C) changed entries, but needs base lookup,
compatibility and stale/concurrent-base rules, explicit deletion semantics and the
same complete O(N) flat manifest. Its fingerprint must include the pinned base;
retrying the same key against another base must conflict. Less request metadata
does not prove less network acquisition, processing or indexing work.

Revisit patches when a concrete connector provides reliable deltas or measurements
show repeated complete enumeration/acquisition dominates imports. Preserve complete
publication, saved-key conflicts and explicit failure behavior if patches are added.
