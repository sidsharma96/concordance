# One bounded manifest per generation

Status: accepted design. The user chose option A: one bounded, versioned immutable
manifest containing the generation's complete document metadata and object references.
This is a layout decision, not an implemented codec or a finalized persisted schema.

## Layout and invariants

PostgreSQL publishes one manifest reference with its SHA-256 and byte length. R2
stores the manifest and separately stored source, passage and index objects. Source
bodies are not embedded in the manifest. The later [index-layout decision](edition-index-layout.md)
chooses one bounded index object per exact edition under this same manifest.
Object presence alone does not publish a generation; all required remote objects must be confirmed before catalog publication.

Preserve logical identity, product version, exact captured revision, processing and
format versions, passage locations and content references as separate concepts. The
schema must describe what the reader needs to reconstruct the generation, including
retained representations and dependencies. It must not treat a byte digest as document
identity or silently discard content whose extraction is unsupported.

Bound serialized bytes and decoded entries/metadata, validate compatible versions
and cross-field relationships, and reject excessive or incomplete input explicitly.
A byte cap alone is not a decoded-memory budget. These bounds are application input
limits; the repository's 1 MiB source-file policy does not set the runtime format.
The exact codec, fields and numeric limits remain to be specified against concrete
representations before implementation.

One manifest describes a complete generation, not a promise to rebuild everything
on every query. Opening/caching lifetimes and per-query reads are not fixed by this
decision. Metadata which merely archives captured source must not be published as
the complete searchable generation promised by the import contract.

## Alternative and costs

A paged layout would use a bounded root referring to bounded metadata pages. It can
consume one page at a time and potentially read selected pages when routing exists.
It also requires page routing, cross-page validation and confirmation of every page
before publication. Stable page boundaries may permit reuse; neither cheap lookup
nor metadata deduplication follows automatically from splitting a list.

For M serialized metadata bytes and N entries, the flat baseline reads one object
and materializes O(M + N) metadata under explicit limits, excluding source/index
reads. A full paged metadata read uses a root GET plus P page GETs. It can retain
O(root + largest page) metadata only if it releases consumed pages; retaining all
entries loses that benefit. The root/page counts and total work still need bounds.
The object adapter adds its existing hashing, verification and write request costs.

Wrong/missing metadata is an integrity/availability error, not empty search results.
A lost manifest write reply must follow immutable-object reconciliation. Confirmed
uploads followed by database rejection may leave unused objects. An uncertain commit
still requires saved-key outcome discovery. No automatic deletion is enabled.

## Why start here and when to revisit

The first measured corpus has 38 document-version entries. Its growth inventory has
4,773 raw Markdown entries across three snapshots, including hidden data resources.
These are not measurements of manifest size, decoded heap, search capacity or an
enterprise workload. A flat list gives the first publication/recovery experiment a
simple baseline without cross-page failure and validation machinery.

Measure actual serialized metadata and decoded memory once the schema exists. Revisit
paging when those approach agreed limits or generation-opening cost is unacceptable
on the representative corpus. Introduce an explicit new format and retain readers
for retained older generations; a version field does not perform migration for us.
The catalog's opaque manifest reference need not change just to support another layout.

The Kubernetes inventory found template-expanded content and external examples.
The subsequent [source-preparation decision](source-preparation.md) accepts a bounded
Kubernetes-only rendered reference experiment. Production representation/citation
semantics and the complete captured-input fingerprint remain open; the experiment
does not implement the complete import/recovery protocol or finalize the schema.
