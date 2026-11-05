# Replay a real documentation update without moving an open reader

Status: the native hosted lifecycle passed: publish old/new snapshots, explicitly
switch a separate test target, preserve an old reader and roll back. Both saved jobs
recover their original results. The existing `kubernetes` default is unchanged.

## Workload and existing contract

The experiment replays a real Kubernetes 1.36 documentation edit. Before is
`e647c21e7e07de1b84861e137294957facdd5ed6`; after is
`088fd22324cef3fd5a2025b4c762936d956447b7`. The entire Git-tree difference is a
16-line addition to `content/en/docs/concepts/workloads/pods/user-namespaces.md`.
It warns about draining a node before changing user-namespace mappings, including
DaemonSet/toleration caveats. Both snapshots' configuration declares version 1.36.
These are historical documentation revisions, not different Kubernetes editions
or observations of a running cluster.

Each intended import selects the complete 15-document Pods scope for edition 1.36.
This isolates the changed edition rather than rebuilding the unchanged 1.34/1.35
captures. The previously published three-edition corpus remains retained. This
single-edition workload does not test whole-document additions/removals, multiple
changed editions or a source connector's permissions/deletion protocol.

Use the existing [complete snapshot](../decisions/import-snapshots.md),
[saved-job command](../decisions/import-command.md) and
[conditional target update](../decisions/target-updates.md). No new incremental
indexer, background importer, mutable generation or storage service is needed to
exercise this workflow. Keeping the frozen corpus is sufficient for old queries;
it cannot establish changed-source behavior. Synthetic fixtures already cover the
local mechanism; the historical replay checks preparation and native-provider
composition without inventing a documentation edit.

Meilisearch's [pinned index-swap specification](https://github.com/meilisearch/specifications/blob/9eb937f1113c2f11bffd87929001aa68497bfc65/text/0191-swap-indexes-api.md)
builds a replacement before switching search clients. It swaps documents, settings
and task history. Concordance changes one catalog target tuple to another retained
immutable generation; this is a mechanism comparison, not adoption of that protocol
or its availability claims. Existing target contracts preserve exact-next-revision
retry recognition and reject superseded or different stale requests.

## Revalidate the preparer at both source pins

The source-specific offline profile was rechecked rather than carrying the original
38-page equality assertion to new revisions. It uses the existing pinned Hugo image,
fixed clock, complete declared English-content/template/data/asset/configuration
inputs and retained original figure. One broad-input and one restricted-input build
at each pin produced identical selected HTML by length and SHA-256. Between the
before/after captures, exactly the user-namespace page changes; the other 14 selected
HTML files are identical.

| Preparation observation | Before | After |
| --- | ---: | ---: |
| Selected Markdown and HTML pages | 15 | 15 |
| Broad declared input files | 4,469 | 4,469 |
| Restricted declared input files | 3,655 | 3,655 |
| Restricted raw occurrence bytes | 91,925,963 | 91,926,853 |
| Referenced original figures | 1 | 1 |
| Selected broad/restricted HTML equal | 15 | 15 |

The importer consumes the restricted handoff. The broad reference's roughly 392 MB
raw input needed a separate 512 MiB disposable staging ceiling; it does not raise
product import limits. Reads were staged sequentially with the existing
`gitsource.CaptureFiles`, in batches of at most 32 files and 32 MiB. It verifies the
exact pin, path, blob ID and byte count against the admitted inventory before
writing those owned buffers. Component/work limits are not whole-process heap bounds.

The first staging attempt used `git archive --no-lazy-fetch`. Although every
admitted blob was local, it failed on an unrelated Chinese-language blog object.
The source snapshot is a partial clone. Rather than acquire unrelated content,
the next attempt reused the existing exact-path reader. Git 2.54.0's
[`parse_treeish_arg`](https://github.com/git/git/blob/v2.54.0/archive.c#L485-L548)
prepares an index before pathspec parsing. That primary source helps explain why
export setup matters; the precise internal cause in Apple Git 2.54.0 was not
independently traced. The observed failure and successful exact-list staging are
the evidence for this workflow change, not a Git-wide performance claim.

A later broad render succeeded, but the probe initially rejected `en/index.html`.
Inspection showed an English home redirect, outside the selected Pods scope. The
final recipe records this outside-scope output and the known generated print page;
it still requires all selected articles and rejects other unexpected HTML. Failed
attempts remain recorded and are not counted as successful preparation trials.

All four successful builds used read-only source/root mounts, no network, two CPUs,
2 GiB memory, a 180-second per-build deadline, 256 MiB output tmpfs and a 64 MiB
output-archive ceiling. Owned containers closed afterward. The image still has the
original reference experiment's toolchain/cold-rebuild limitations. This verifies
these two source-specific handoffs, not universal dependency completeness or
production-render equivalence.

## Functional expectations before native publication

Existing extraction, article JSON, edition building, query preparation, BM25 and
quote resolution reconstructed exact local expectations before the hosted writes.
This is a functional source/selection check, not a relevance evaluation, reserved-set
run or timing benchmark. The query `reconfigure` has one old lifecycle passage
before the edit and four matches afterward: three new user-namespace passages plus
the original lifecycle passage. Every expected quote has an exact retained hash,
source path and Git revision. The new warning includes the rendered glossary term
`drain`; the template instruction itself is not presented as answer text.

A stable `kubelet` quote supplies an old-reader reference. Its bytes are identical
in both snapshots, while the complete candidate count changes from 116 to 118.
That does not imply scores or global ranking stay fixed across generations: edition
statistics legitimately change. An already-open reader retains its earlier state.

## Native publication, selection and rollback

The independently reviewed operator uses two new saved jobs/keys in the existing
authorized collection and namespace. It never reuses an old operation key for
changed input. A separate, newly named test target is created explicitly; the
original `kubernetes` name and its retained selection are not test fixtures.

Actual PostgreSQL 16.15/R2 observations, using the current native command:

| Step | Separate test target | `reconfigure` matches |
| --- | --- | ---: |
| Activate before generation | Before, revision 1 | 1 |
| Publish after generation, no activation | Before, revision 1 | 1 |
| Explicitly activate after generation | After, revision 2 | 4 |
| Explicit rollback | Before, revision 3 | 1 |

Every displayed quote's path, exact Git revision and SHA-256 matches its retained
local expectation. The three new passages include the drain warning; the original
lifecycle passage remains. A separate read-only helper held the same before reader
and passage reference across publication/activation. While a catalog lookup showed
the new generation at revision 2, that reader still returned one control match and
strictly recovered its original `kubelet` quote with the before revision. This is
one exercised reader/query/reference sequence, not a load or concurrency benchmark.

A stale attempt to restore before using expected revision 1 failed with zero stdout;
the target remained after/revision 2. Requesting absent edition 1.35 also failed
with zero stdout even though other retained generations contain that edition. The
explicit rollback used expected revision 2. Both native same-job retries returned
their original generations; saved record hashes stayed unchanged. No per-request
transport observer was installed, so no retry/object request totals are claimed.

The original `kubernetes` tuple stayed at its original generation, default 1.36
and revision 1. Namespace trust remained active at epoch 1. Both new generations,
saved jobs/keys, captures and the separate test target at revision 3 are retained.
Every owned helper, forward and container closed. No automatic reset, deletion,
reactivation, namespace/schema setup or administrator write was used.

## Required object references

A separate read-only inspector strictly opened both catalog-pinned roots, checked
their physical keys/hashes and compared exact non-root hash/length references.
The after generation contains 15 documents and one edition. Its two figure
references point to the one retained original figure file; both reads were strict,
without image interpretation. Required-reference observations are:

| Observation | Result |
| --- | ---: |
| Before non-root references | 3672 |
| After non-root references | 3672 |
| Shared non-root references | 3667 |
| After references absent from before | 5 |
| Before manifest bytes | 854620 |
| After manifest bytes | 854620 |

Each generation has a different root. This is a comparison of required references,
not a bucket-occupancy or billing delta: a reference absent from before can already
exist in the collection from another retained generation. Both roots remain retained.
The first read-only inspection incorrectly expected one figure-reference occurrence
rather than one distinct original file and stopped after reads. Its failed output
is retained separately; the corrected inspection reports both occurrences.

## Evidence and limits

Private material is under `.tools/reviews/documentation-update/`, with captures
located through the protected operator context. It includes exact source inventories,
acquired-blob hashes, failed/successful preparation logs, selected output comparisons,
functional expected quotes, read-only hosted preflight and independent baseline
review bindings. Baseline approvals cover the actual existing runtime/operator;
normal source-change approvals remain separate. Live identifiers, routes, credentials
and private teaching material are excluded from this record.

No source algorithm, runtime dependency, schema or default changed. No golden-set
query, model inference, agent evaluation, latency/billing benchmark, provider outage,
ambiguous COMMIT, host-loss recovery, retention or deletion test is implied.
