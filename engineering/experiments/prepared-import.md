# Prepared Git/HTML import composition

`internal/preparedimport` implements the accepted
[trusted handoff](../decisions/prepared-capture.md) as an internal synchronous Go
operation. It adds no transport format, builder, public endpoint or CLI command.
The existing retained article, edition, manifest, fingerprint and publication
formats/protocols are unchanged.

## Operation, trust and ownership

`New` takes a concrete `*generationpublish.Publisher`, authorized collection,
trusted source registrations and positive budgets. Each registration separates
source ID, observed repository identity and local Git routing. It copies the
registration slice's values into a private map; strings remain immutable values.
There is no dynamic source lookup or authorization discovery.

`ImportPrepared(ctx, key, candidateID, editions)` borrows the complete request,
HTML/recipe bytes and extraction-profile aliases unchanged through return. Each
capture supplies a source, exact commit/directory, preparation/toolchain labels,
nonempty recipe, explicit Git dependency paths, profile and source-path/HTML pairs.
The recipe must record producer and output-exclusion policy. It is opaque retained
configuration, never executed. HTML association and dependency completeness remain
trusted producer assertions; source bytes and hashes cannot prove how HTML arose.

The importer admits supplied metadata/counts/bytes before membership maps or Git
reads. It captures complete selected Markdown itself and requires exact page
membership. Missing, extra and duplicate mappings fail. Dependencies must be unique
and disjoint from selected document paths. `CaptureFiles` reads them at the same
pin, including originals for every extracted image. Only the existing manifest's
`/images/...` to `static/images/...` mapping is supported; no URL fetch or guessed
path normalization occurs. Undeclared or missing images fail.

Within each edition it orders captures by source ID and documents by full Git path,
using private ordinals or owned Git results rather than sorting caller slices.
It extracts/encodes articles and builds the count index and saved ranges using the
actual supported processors. Conflicting full profile policies under one ID fail;
membership-list order alone is normalized. Empty captures/editions are explicit.

One staging map retains one body per exact reference while preserving every logical
slot. Equal hashes/lengths also require equal local bytes before sharing. Acquired
Git and generated buffers are owned locally; HTML/recipe buffers remain borrowed.
Nothing from a call is retained by the importer afterward. A nonwaiting permit
admits one call per instance; it does not coordinate separate instances/processes.
No goroutines or website builders are added by the importer.

All local construction completes before `PublishCaptured`. That existing publisher
checks the full artifact inventory, index/article bindings and fingerprint before
binding the key. It confirms artifacts, then root, then guarded catalog publication.
Wrapped errors preserve catalog conflict/uncertainty and storage/context causes.
Errors return zero, but remote writes or publication may already have committed.
Exact retries require the original handoff; no journal, automatic retry/resume or
physical deletion is added.

## Admission and cost model

`MaxInputBytes` counts HTML/recipe occurrences plus actual captured raw-file
occurrences, including bytes shared across editions/documents. `MaxStagedBytes`
counts distinct staged bodies plus the root manifest. `MaxObjects` includes that
root. Manifest metadata/global/per-source quotas, Git/document/dependency limits,
HTML parser/output limits and edition/article/preparation limits also apply.
`MaxHashBytes` separately bounds the framed comparison preimage at publication.

One bounded component operation may allocate/read its admitted output before a
smaller aggregate ceiling rejects it. All HTML/recipes, staged bodies and metadata
coexist; edition preparation/index state, publication decoding, Git internals and GC
add memory beyond logical counters. This is bounded composition, not a measured
whole-process heap, latency or million-document capacity guarantee.

For P declared paths and B admitted body bytes, membership/staging maps have expected
O(P) metadata work plus O(B) hashing/equality work. Slot sorting adds O(P log P).
Map hashing is not a worst-case constant-time guarantee. Acquisition spawns bounded
Git operations through the existing adapter; extraction and index costs retain
their own structure/term/posting bounds. Duplicate bodies still need processing
and hashing; this slice adds no extraction cache or incremental index updates.

## Observed synthetic evidence

Tests create a real temporary Git repository with two Markdown paths sharing raw
and rendered bytes, one figure and one included YAML dependency. Uncommitted and
untracked files do not change the pinned selection. They check actual retained raw
and figure bytes, source/revision bindings, stable slot order, shared objects,
search and verified article associations. Multiple sources/editions and declared
or captured empty editions are exercised. Reordered page/dependency lists produce
the same manifest; request aliases remain unchanged.

Boundary tests cover exact and one-below aggregate input/staging/object ceilings,
global/per-source document limits, files, image occurrences, metadata and extraction.
Invalid memberships, duplicate/overlapping/missing dependencies, missing/external
images, malformed HTML/UTF-8, missing recipe, unknown source, mutable revision,
conflicting profiles, cancellation/deadline/nil input and nonwaiting admission fail
without a partial bundle.

With a dedicated local PostgreSQL 14.17 database and an HTTP fixture behind the real
AWS S3 client, the two-document import confirms seven distinct artifacts plus root:
16 GET, zero HEAD, eight PUT. A same-key retry with a different candidate ID returns
the original publication with no added object requests; changed recipe conflicts
without storage access. Invalid preparation and a too-small hash budget leave no
key binding and make no object requests. A rejected PUT leaves an unpublished
binding; an explicit same-input retry can complete it. A channel-controlled HTTP
boundary confirms a simultaneous second import returns `ErrBusy` and binds no key.

Fresh catalog/object clients reopen the generation after caller buffers are cleared,
the local Git repository is removed and the original database pool is closed. The
reader searches the stored edition, resolves `docs/a.md` at its captured revision,
verifies article bytes and quotes `Pods\nRetry uploads safely.`. This checks the
prepared-import composition, not a website build or real-corpus relevance.

Run the cheap membership reproduction with no database:

```sh
go test -count=1 -v ./internal/preparedimport -run 'TestPreparedMembership'
```

Use the dedicated test-database setup in CONTRIBUTING for the full package and
`make verify`. Never run schema-creating tests on the shared hosted catalog.
Selected local output/check/review evidence is ignored under
`.tools/reviews/prepared-import-*`. No new live R2 writes, Kubernetes publication
or Cloudflare book deployment is claimed.

## Complete replacements preserve earlier evidence

`TestCompleteSnapshotsChangeMembershipWithoutChangingOldEvidence` exercises the
existing importer, target coordinator and published reader together. It commits
three complete synthetic Git snapshots, with one authored HTML passage per document:

| Snapshot | Exact document paths under `docs/` |
| --- | --- |
| Before | `a.md`, `b.md`, `c.md` |
| After | `0-added.md`, `a.md`, `moved/c.md` |
| Reused path | `0-added.md`, `a.md`, `b.md`, `moved/c.md` |

The first two have equal document counts but different membership. Every hit for
`guidance` is checked against an explicit path-to-quote map, exact source ID,
captured Git revision, generation and edition. The added path sorts before the
stable document, so rank/slot positions cannot stand in for document identity.
The removed `legacy` text has no match after activation. The renamed file keeps
identical raw and article references while its new Git path is a different key.

Publishing After leaves the target selecting Before. Explicit activations advance
target revisions 1, 2 and 3; rollback selects Before at revision 4. Previously opened
readers keep their own sets. Recreating `docs/b.md` with replacement text does not
redirect an earlier reference: it still strictly quotes `Guide` followed by
`Legacy rollback guidance.` with unchanged old provenance. A new reader rejects
that old temporary reference rather than interpreting its position in the new set.

Before replacement publication, an old three-page handoff against the new three-file
tree, a missing page and a missing declared dependency all fail. Each returns zero,
leaves no operation binding, makes no object request and preserves the target.
Changed membership under the original completed key conflicts before object access.
All three completed keys later recover their original generations with no new
object requests. These are actual fixture request counters, not latency claims.

Run the test with `go test -count=1 -v ./internal/preparedimport -run
TestCompleteSnapshotsChangeMembershipWithoutChangingOldEvidence` and a dedicated
local `CONCORDANCE_TEST_DATABASE_URL`, following CONTRIBUTING. Never point it at the
hosted catalog: setup creates and drops an isolated schema and uses a local HTTP
object fixture behind the actual S3 client. The observed run used PostgreSQL 14.17
and Go 1.27.2 on darwin/arm64. No renderer, CLI, hosted provider, source-permission
revocation, physical deletion, ambiguous-commit fault or relevance evaluation is
exercised. The earlier [native update replay](documentation-update.md) remains the
separate one-page real-corpus/provider evidence. Runtime behavior is unchanged.

## Local construction of the selected Kubernetes corpus

The accepted [restricted build-input profile](../decisions/kubernetes-build-inputs.md)
now passes the actual local construction step with three editions, all 38 documents
and six image references. This is preparation and component-reader validation, not
an invocation of the publisher, its full preflight, catalog or object service.

A disposable Go test overlay calls the existing unexported build method through a
test-only wrapper, leaving the runtime API and checkout source unchanged. The normal
constructor checks registrations/limits using an inert publisher that is never called.
The probe opens the generated manifest and every edition, checks the full referenced
artifact inventory and all article bindings, derives the captured-input fingerprint,
then searches `ContainerRestartRules` and verifies quotes in each edition (two matches
per edition). It checks pipeline associations, not reader-judged retrieval quality.

| Constructed data | Bytes or count |
| --- | ---: |
| Distinct staged bodies including root | 5,017 objects |
| Distinct staged bytes including root | 132,106,434 |
| Largest staged body | 5,605,160 bytes |
| Generation manifest | 2,444,076 bytes |
| Raw file occurrences | 242,874,975 bytes |
| HTML occurrences | 20,693,182 bytes |
| Article JSON occurrences | 1,024,848 bytes |
| Three edition indexes | 368,252 bytes |

The single exploratory run used Go 1.27.2, macOS 26.6.2 arm64, Git 2.54.0
(Apple Git-157), eight logical CPUs and 16 GiB host memory. The time from starting
the file read through construction was 218.277 seconds. After a forced GC while
keeping request and bundle live, Go reported 173,255,360 heap bytes and 185,008,128
heap-in-use bytes; total allocation through that snapshot was 1,465,123,240 bytes
across 5,376,697 allocations and 22 GC cycles. One sample gives no spread estimate.
Caches and host load were uncontrolled. This is not peak RSS, peak live heap,
end-to-end import latency or a production capacity benchmark.

Caller ceilings for this experiment were 512 MiB input occurrences, 256 MiB distinct
staging, 10,000 objects including root, 8 MiB per object and a 16 MiB fingerprint
preimage. The Go runtime had a 768 MiB soft memory target; it is not an OS memory
limit. Exact component ceilings are retained in `.tools/probes/import-cost/` with
the overlay and test-only source. These settings admit this experiment, not automatic
production defaults. The original file handoff and pinned Git objects remain available.
Raw output is `.tools/reviews/restricted-import-cost-1.txt`.

The current Git reader starts one `cat-file blob` process per raw file occurrence:
38 selected Markdown plus 10,407 dependencies imply 10,445 blob subprocesses,
separately from commit/tree metadata commands. This is a derived operation count,
not a profile proving how much of the 218.277 seconds it caused. A separate disposable
transport comparison reads the first 128 path-ordered staged inputs of at most
64 KiB in edition 1.36, totaling 878,317 occurrence bytes. Both paths verify exact
length and framed Git blob SHA-1. Three alternating-order executions produced:

| Sample | One Git process per file | One `git cat-file --batch` process |
| --- | ---: | ---: |
| 1 | 2.630087250 s | 0.068889416 s |
| 2 | 2.571436833 s | 0.021415209 s |
| 3 | 2.308249875 s | 0.020930375 s |

Both methods read the same IDs and bytes; the batch prototype checks exact bounded
headers, body lengths/hashes, separators, final EOF and process success. It uses a
bounded prebuilt stdin request and drains stdout while Go's exec machinery owns the
input writer. Cancellation/cleanup is limited exploratory code, not an approved
runtime reader. The micro-comparison excludes listing, extraction, indexing,
publication and peak memory; it does not predict a complete-import speedup. Its
source is `.tools/probes/git-read-cost/main.go`; raw results are
`.tools/reviews/git-read-cost.txt`.

As inspected in October 2026, Git 2.54.0's
[`batch_objects`](https://github.com/git/git/blob/94f057755b7941b321fd11fec1b2e3ca5313a4e0/builtin/cat-file.c)
reads successive requested objects in one process, with header/body framing exercised
by its [cat-file tests](https://github.com/git/git/blob/94f057755b7941b321fd11fec1b2e3ca5313a4e0/t/t1006-cat-file.sh).
GitLab's documented [Gitaly cat-file cache](https://docs.gitlab.com/administration/gitaly/configure_gitaly/#cat-file-cache)
also reuses batch processes across repository RPCs to reduce repeated setup. That
service workload motivates reuse; a cross-request cache would add process lifetime,
resource and isolation policy beyond our single synchronous capture. These examples
support investigating batching, not adopting a long-lived process pool.

The [bounded sequential batch reader](git-capture.md#bounded-batch-framing-and-process-lifetime)
now implements this local transport without changing public contracts or storage
formats. It uses one blob process per nonempty selection: six for three Markdown
and three dependency selections, instead of 10,445. Metadata commands remain separate.
The per-file transport remains a real-Git test oracle; no cross-call cache or pool
is introduced.

Three sequential executions of the same local-construction probe with that reader
produced the same input fingerprint, document/image counts, staged object counts,
byte totals, index lengths and verified query results as the original run:

| Sample | Local construction seconds | Heap bytes after GC | Allocated bytes through snapshot |
| --- | ---: | ---: | ---: |
| 1 | 1.952906083 | 138,030,608 | 497,491,200 |
| 2 | 2.140234708 | 138,033,200 | 497,550,256 |
| 3 | 2.143338000 | 138,023,712 | 497,399,200 |

The environment, workload, probe counters and soft runtime target match the original
observation. Caches/host load were uncontrolled, including a concurrent race-test
process during the third execution; these are exploratory observations rather than
a controlled repeated A/B benchmark. The original path has one timing sample only.
Retained heap is a forced-GC snapshot, not peak memory. This does not measure remote
publication or predict larger-corpus latency. Raw logs are
`.tools/reviews/restricted-import-batch-cost-{2,3,4}.txt`; the first development run
is retained separately. The main cost result is eliminating per-object startup while
keeping the admitted snapshot and verification semantics.
