# Concordance architecture direction

This describes the architecture direction. The command-line runtime provides help
and version only. PostgreSQL import-catalog operations now exist as an internal
component, and bounded Git object capture exists in `internal/gitsource`. An internal
S3 adapter implements verified immutable-object reuse, tested against HTTP fixtures
and a live R2 creation/reuse/fresh-client-read smoke probe. Bounded in-memory HTML
structure extraction now exists in `internal/htmlarticle`; the Kubernetes reference
profile extracts all 38 selected articles with image references. The Git adapter also captures explicit file dependencies;
the Kubernetes audit tool can acquire referenced original images locally at an
exact commit. `internal/htmlpassage` now forms bounded structure-first ranges in
these articles, with heading and group references. `internal/termsearch` supplies
bounded match-all candidate selection over caller-prepared terms within one exact
edition. Neither component supplies durable asset retention or publication.
Provider fault and full
recovery experiments remain open. The complete importer, Markdown processor, index,
crawler, query API and shared library are not implemented.

## Scope and boundaries

P0 retrieves version-aware technical documentation and identifies the exact source
revision behind every passage. Code search is a possible P1. Begin with a bounded,
explicit documentation collection; broad web crawling is not a prerequisite.

One Go process owns acquisition, indexing and retrieval initially. Separate these
responsibilities as real operations arrive, without a tree of empty packages:

- A source adapter acquires bytes and supplies stable source/document identity,
  revision information and deletion observations. Fetch failure is not deletion.
- A content processor extracts document structure, text and passage locations.
  Documentation headings and code symbols need different processors.
- Retrieval consumes normalized passages and metadata. Its ranking code should not
  know how Git, HTTP or a local fixture supplied them.
- Storage owns immutable objects and the catalog's publication/recovery protocol.
  CLI and future HTTP handlers translate requests; they do not define storage formats.

The accepted [first-source decision](decisions/first-source.md) starts with Markdown
at a specific Git commit. Acquisition and content format are independent: the same
repository can supply Markdown, code or PDFs; another adapter can supply provider
documents. Future processors must preserve headings, blocks, pages, slides or code
symbols rather than flattening everything into anonymous text. Retain captured
representations alongside derived passages, with source-specific provenance.

The [representative-corpus decision](decisions/representative-corpus.md) starts
evaluation with Kubernetes Pods docs across three pinned release snapshots, then
full English documentation. The raw-source inventory exposes shortcode and external
data dependencies; it does not establish complete extraction, retrieval quality or
production capacity. Settle processing and dependency capture before publication.

The accepted [source-preparation boundary](decisions/source-preparation.md) keeps
Kubernetes-specific rendering outside core retrieval. A bounded offline reference
render verifies selected notices/examples across the three pins, not complete
extraction or a production ingestion pipeline. Other sources may supply Markdown,
captured HTML, provider blocks or exports through different acquisition/preparation
paths. Format processors and site profiles preserve content and citations; ranking
does not need to know which builder or provider produced them.

Do not build a dynamic plugin system yet. First demonstrate the boundary with Git
and a fixture source. Introduce small interfaces through concrete consumers rather
than empty future adapters or a universal schema. Provider revisions, product-version
applicability and changing permissions need their own explicit contracts before a
private connector is enabled. Acquisition permission alone cannot authorize later
search results or citations. Shared lexical retrieval need not know the transport;
code navigation, visual search and other new capabilities may need new algorithms.

## Identity and query correctness

Keep source, logical document, product version, captured source revision, passage
location, content hash, processing version and index generation distinct. A corrected
page may retain its product version while acquiring a new revision. The same bytes
can legitimately occur in several documents or editions. A hash permits integrity
checks or scoped deduplication; it does not identify or authorize a logical document.

An explicitly version-scoped query considers eligible passages from that version.
Do not retrieve one global winner and drop it afterward, leaving an artificial miss.
A missing edition returns no eligible results, with an explanation; broader fallback
must be explicit. Resolve mutable labels such as `latest` to a concrete edition for
the request. Define version-label ordering per source rather than assuming SemVer.

A query pins a published generation so text, postings, statistics, tombstones and
citations agree. It must not mix an old passage with a newly overwritten source page.
Define replacement/deletion visibility and reader lifetimes before adding compaction.
Captured revisions still need a retention contract; citations cannot promise bytes
forever if garbage collection removes them.

The accepted [citation decision](decisions/passage-citations.md) makes the retained
article the quote authority for rendered HTML. Verified original Git file/line ranges
are optional additional provenance and may name several inputs. Missing mappings
are valid; invented mappings are not. Keep article identity, block locations, source
revision and processing version separate. The bounded HTML processor now returns
owned blocks and nested container intervals. The persisted article schema and
citation API are not implemented. See the [extraction experiment](experiments/html-extraction.md)
for supported structures and parser limits. The accepted
[media scope](decisions/media-coverage.md) retains original figures and searches
authored text first; visual contents remain uninterpreted. Later descriptions of
informative diagrams require separately labeled derived content in a new
processing version and generation. Fingerprint/manifest coverage fields and durable
asset retention remain unimplemented.

## Storage from the first milestone

Use external object storage from the first write/read/recovery slice, not as a final
port of a local-only engine. The accepted [catalog/publication split](decisions/catalog-publication.md)
uses PostgreSQL as the publication authority and R2 for immutable source, index and
manifest objects. The private R2 bucket `concordance-data` is provisioned; its live
creation/reuse/fresh-client-read probe passes. Hosted PostgreSQL is not
provisioned. Neither backend is required to build the scaffold.
Verify the operations used through the S3 client adapter on the actual R2 service.

The accepted [object-sharing decision](decisions/object-sharing.md) uses immutable
objects named by exact-byte SHA-256 within one authorized collection. Generations
can share their bytes while retaining separate identities and provenance. Verify
existing objects and uncertain writes; never overwrite unexpected bytes. Automatic
deletion remains disabled until shared references, readers and in-flight work are
protected. The [adapter experiment](experiments/object-storage.md) describes the
first bounded implementation and the limited live R2 evidence.

The accepted [manifest layout](decisions/manifest-layout.md) is one bounded,
versioned manifest per generation. It keeps the first recovery path and validation
simple. Encoded bytes and decoded metadata need explicit bounds. The full schema
and content representation remain open; source capture alone is not searchable
publication. Measure metadata before introducing paged layouts and preserve readers
for retained formats when another layout is introduced.

Write and confirm the required immutable files and manifest before publishing their
catalog reference in a short PostgreSQL transaction. Uploading an R2 manifest is not
publication. Queries open one published generation. A failed catalog commit can
leave unreferenced objects; a lost commit reply can hide a successful publication.
A PostgreSQL transaction cannot roll back remote writes. Keep automatic deletion
disabled in the first storage slice until reader/in-flight-work protection and safe
orphan cleanup are specified. The decision record defines the cross-store failure
matrix and required experiments.

The accepted [import completion decision](decisions/import-completion.md) is synchronous
publication: success returns a complete generation ready for queries selecting its ID.
The first recovery drill must reconstruct that generation and fetch its cited bytes
without the writer's local files. The accepted [retry contract](decisions/import-retries.md)
uses a saved operation key: identical retries recover the original result; changed
inputs under that key conflict. The [catalog experiment](experiments/import-catalog.md)
specifies the first concrete metadata operations. Capture/comparison rules,
cancellation, hosting and the full cross-store path still need implementation/design.
Do not borrow Rewind's acknowledgment contract implicitly. Bound input bytes, pending
work, retries, remote reads and cache memory. Start with one publishing writer until
measurements justify coordinating more.

## Retrieval and algorithm progression

The accepted [passage direction](decisions/passage-grouping.md) follows authored
structure. The first in-memory HTML builder partitions blocks without overlap,
keeps outer containers intact, and preserves group labels and common enclosing
headings by reference. It rejects malformed ranges, crossing groups, excessive
metadata and oversized bodies with no partial plan. Admission ceilings are caller
configured; no automatic splitting or permanent product-size limit is implied.
The caller must retain the exact unchanged article for all use of the returned
indices. The [experiment](experiments/structured-passages.md) records the current
38-page partition and limits. Context scoring, a tokenizer, persisted passage
formats and ranking remain open; preserving context does not establish relevance.

The accepted [keyword matching](decisions/keyword-matching.md) baseline requires
all distinct query terms within one passage's supplied body/heading/label union.
`termsearch.MatchAll` validates bounded prepared inputs, selects an exact edition,
and scans its passages. It distinguishes missing and explicitly empty editions,
rejects incompatible term-version tags and returns no partial result on failure.
Returned positions belong to this exact input, not a persisted passage identity.
The accepted preparation policy keeps CamelCase technical names whole without adding
their component words. The tokenizer and raw-text adapter remain unimplemented;
case, Unicode and punctuation rules remain open. Private experiment rules are not
an adopted production normalizer. Field scoring and ranked retrieval are separate.
See the [matching component](experiments/keyword-matching.md) for invariants and limits.

Begin with a small judged documentation corpus and an exact scan baseline. Then
build sorted inverted postings and a lexical ranker, with an explicit tokenizer and
version-filter semantics. Compare intersections and bounded top-k selection against
simple scans/full sorts on the same inputs. Use uncompressed postings before deciding
whether gap/varint encoding or skip data materially helps.

Embedding acquisition and vector search follow a stable citation/publication path.
Use exact vector search as the oracle before evaluating HNSW. Keep embedding model,
dimensions, distance metric and processing version compatible within a generation.
Measure approximate recall against exact neighbors separately from relevance on judged
queries. Hybrid fusion and reranking need evidence of a quality improvement over the
lexical baseline, alongside request cost, latency and failure behavior.

Future crawl scheduling, tries, Bloom filters and sharding are candidates, not a
checklist. For each structure state its invariant, cost variables, workload, simplest
credible alternative, measured outcome and revisit condition. Count object requests,
bytes, cold-cache behavior and recovery cost as well as CPU complexity.

## Agreement with Rewind

Share only a small in-process Go object-storage library when concrete consumers
establish its contract: immutable writes, bounded full/range reads, integrity checks,
explicit provider errors and reusable fault tests. Specify and exercise one
write/read/recovery sequence from each project before extraction. Start locally;
repository and versioning arrangements come later. No cross-checkout module imports.

Keep logs, catalogs, publication, compaction, retention, tenant isolation/deduplication
policy and acknowledgment/read-visibility semantics application-specific. Rewind
requires remote durability, including recovery metadata, before acknowledgment.
Shared primitives must support that without prescribing Concordance's policy.
Shared caching, a storage service, a shared schema and a universal engine are deferred.

## Changes from the roadmap

The roadmap proposes broad crawling, hybrid retrieval, a Rewind engine reused unchanged
and documents keyed by content hash. Our narrower initial corpus, separated identity,
and small shared object library replace those assumptions. Immutable segments still
need publication and reader-lifetime coordination. The roadmap's compressed postings,
skip pointers and HNSW are experiments to compare against baselines, not pre-proven
winners. Its timings and performance claims are not measurements of Concordance.

Before completing storage implementation, settle captured-input comparison, activation,
cancellation and retention contracts with concrete examples. Preserve
the accepted import completion promise while choosing those mechanisms. The public
query API is not fixed to explicit generation IDs, and the initial small corpus is
not a permanent size limit.
