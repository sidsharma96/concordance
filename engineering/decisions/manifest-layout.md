# One bounded manifest per generation

Status: accepted design. The user chose one bounded, versioned immutable manifest
containing the generation's complete document metadata and object references, then
selected explicit versioned JSON for its encoding. The codec and complete persisted
schema are not implemented.

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
The fields, canonical writer, strict codec and numeric limits remain to be specified
against concrete representations before implementation. JSON is the chosen encoding,
not permission to accept arbitrary metadata or a default decoder's permissive behavior.

One manifest describes a complete generation, not a promise to rebuild everything
on every query. Opening/caching lifetimes and per-query reads are not fixed by this
decision. Metadata which merely archives captured source must not be published as
the complete searchable generation promised by the import contract.

## Encoding and research

Use explicit versioned JSON for this manifest. Keep the selected retained-article
JSON and per-edition binary index formats independent. Readable metadata helps the
first recovery/debugging sequence and needs no new Go dependency or code generator.
It does not establish smaller files or faster opening; neither has been measured
for the complete schema, which does not exist yet.

Relevant precedents were researched before this choice:

- OCI image manifests use JSON and describe separate objects with type, digest and
  byte-size descriptors. Its [manifest](https://github.com/opencontainers/image-spec/blob/147f9c13cedb47a0c4d9a11a222961073d585877/manifest.md)
  and [descriptor](https://github.com/opencontainers/image-spec/blob/147f9c13cedb47a0c4d9a11a222961073d585877/descriptor.md)
  specifications support separating readable metadata from large payloads. Their
  image graph, unknown-field and extension rules are not Concordance's schema,
  publication, authorization or capacity guarantees.
- Iceberg stores table metadata and snapshots as JSON while its manifests and
  manifest lists use Avro. Its [specification](https://github.com/apache/iceberg/blob/e17f4824458948d1e53a6efea0000b0855c4d62a/format/spec.md)
  shows that metadata and larger file listings can use different encodings. Its
  hierarchical data-table layout does not prove our flat manifest scales indefinitely.
- Bazel's remote execution API uses Protocol Buffers for output paths and content
  digests/sizes; see `OutputFile`, `Digest` and `ActionResult` in its pinned
  [schema](https://github.com/bazelbuild/remote-apis/blob/c2c07e3d718d3d490a4958344d9c3e47de0bfaf3/build/bazel/remote/execution/v2/remote_execution.proto).
  This is a credible binary alternative, with a different build-cache/RPC workload.

The deferred alternative is Protocol Buffers with an explicit schema and generated
Go types. It offers schema tooling and potentially compact fields but adds a runtime,
pinned code-generation workflow and diagnostic tooling. No dependency was added.
Handwritten binary, as used by the index, would instead add our own metadata-format
and evolution machinery. Neither alternative has a measured advantage here.

Both JSON and binary need admission before allocation, required/default presence
rules, closed versions/kinds, cross-reference checks and explicit unknown/duplicate
handling. For JSON reject duplicate or case-folded field names, forbidden nulls,
unknown fields, malformed Unicode, number coercion and trailing values. Plain
`json.Unmarshal` or `DisallowUnknownFields` alone is insufficient. Admit encoded
bytes, decoded entries, individual and aggregate key/path bytes; these are logical
limits, not exact heap quotas.

Specify versioned writer/input-comparison rules rather than treating any encoder
as a canonical identity function. Protocol Buffers explicitly warns that
[deterministic serialization is not canonical](https://protobuf.dev/programming-guides/serialization-not-canonical/).
The catalog's independent exact-byte hash/length must still be checked before
parsing a fetched manifest. A manifest hash does not replace the separate trusted
captured-input fingerprint or grant authority to publish or read.

Revisit binary when measured encoded bytes, decoded allocations/admission or cold
opens justify its extra workflow, or a concrete shared-schema/language requirement
appears. Retain readers for retained formats. Changing encoding does not add paging,
bound all metadata, reduce the number of logical entries or solve cross-store failures.

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
