# Separate index objects by exact edition

Status: accepted option B. Save one bounded index object for each exact edition,
referenced by the existing single generation manifest. This settles index granularity,
not a complete manifest schema or implemented publication. The bounded
[HTML binary codec/reader](edition-binary.md) now implements one edition artifact;
`termsearch.Index` remains its in-memory query component.

## Follow one version-scoped search

The representative corpus has Kubernetes documentation for 1.34, 1.35 and 1.36.
A query selecting 1.35 needs that edition's postings and statistics. Its generation
manifest will identify the exact index artifact to open. The reader must not combine
another generation's locations or silently fall back to a different version.

An edition key is opaque, not necessarily a SemVer string. Source, logical document,
captured revision, generation identity and authorization remain separate metadata.
An absent edition means no eligible edition, preserving the current found/not-found
distinction. A missing or corrupt object for a declared edition is an error, not an
empty edition. A known-empty edition remains distinguishable; the bounded HTML binary format
encodes it with empty tables.

## Alternative and costs

Option A puts all editions in one bounded index object. A simple whole-file reader
needs one index GET and all editions' bytes. It has fewer references and confirmation
targets and can suit comparisons across several versions. Changing one edition
changes the aggregate encoded object. A directory plus range reader could change
its read costs, but that requires another layout and integrity design.

Option B also needs one index GET for a cold single-edition lookup after opening the
manifest, but carries only the selected edition's index bytes. Comparing K uncached
editions needs K index reads rather than one aggregate read. The manifest contains
more references and the writer confirms more objects. Unchanged exact encoded edition
objects may be reused within the same authorized collection; codec/header choices
still determine byte identity. No storage or latency advantage has been measured.

For E editions with payload sizes I_e, a full aggregate read transfers roughly
sum(I_e) plus its header; separate reads transfer the chosen I_e plus its header.
This excludes manifests, article/location reads, provider retries and the existing
write/reuse verification protocol. It is a layout cost model, not a bill or benchmark.
The current 38-article sample has 1,889 passages and 58,362 word/passage pairs across
three real editions. Those counts do not establish encoded index size or capacity.

## Publication and validation

Keep one bounded, versioned manifest for the complete generation. Confirm every
required index, article, location, source and manifest artifact before the PostgreSQL
publication transaction. Splitting index objects does not allow partial edition
publication or change saved-operation-key, receipt, epoch or ambiguous-outcome rules.
Automatic deletion remains disabled.

Opening one edition must not pretend to validate every other object's body. Future
orchestration needs generation-metadata admission and selected-index validation,
with exact index/table/article/source bindings and compatible processing versions.
This choice does not change `NewIndex`'s validation of every edition in its supplied
Corpus or its cached original-input query limits.

The accepted [artifact-binding direction](edition-artifact-binding.md) keeps counts
and saved passage locations together without an additional ordered-word copy. The
existing `htmlpassagemap.Open` stays unchanged; counts cannot reconstruct its ordered
field fingerprint. The new reader must check actual artifact bytes against the
published generation reference and validate structure and bindings. The bounded [binary reader](edition-binary.md) now implements these checks; complete
published-generation recovery remains open. The [payload experiment](../experiments/edition-index-payload.md)
measures the illustrative ordered-field alternative.

A single edition can still be too large. Persistent index blocks, paged metadata,
bounded selective readers and safe retention/deletion remain necessary scaling work.
Revisit grouping or pages after measuring per-edition cold opens, small-object costs
and real multi-version workloads. This layout is not million-document support.
