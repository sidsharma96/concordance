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
`.tools/reviews/prepared-import-*`. No new live R2 writes, Kubernetes import,
performance benchmark or Cloudflare book deployment is claimed.
