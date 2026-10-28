# Saved HTML passage locations

The accepted [option A](../decisions/saved-passage-locations.md) now has a bounded
internal component, `htmlpassagemap`. It saves article-local body ranges bound to
exact encoded article bytes and ordered prepared fields. It does not implement a
complete index, generation manifest, source registry, citation API or importer.
The record can later be embedded or referenced by search artifacts; this does not
mandate one object per passage, article or table in the final deployment layout.

## Writer and reader

`Capture` decodes bounded article JSON, calls `htmlterms.Build` once, and returns an
immutable table and owned prepared `Fields`. Those fields and ranges come from the
same canonical preparation. The table copies scalar ranges; changing returned fields
cannot change it. The caller must store the exact fields or a compatible index and
bind their positions to this table in the future published generation metadata.

`Encode` returns owned compact JSON in explicit field order. The required fields
are `format`, `article_sha256`, `article_bytes`, `passage_processor`, `field_processor`,
`terms_version`, `fields_sha256`, `block_count`, `locations`. The format identifier is
`concordance.html-locations/v1`. Locations are required `first`/`after` integer pairs.
All blocks are covered once, in contiguous nonempty ranges. Unknown/missing/null/
duplicate fields, including escaped duplicates, wrong capitalization, unsupported
versions, invalid UTF-8 and non-integer spellings fail. All header strings are fixed
ASCII tags or lowercase hex digests; replacement Unicode cannot pass these checks.
The codec does not accept arbitrary descriptive strings requiring Unicode repair.

The v1 field fingerprint starts with `concordance.html-fields/v1` and NUL. It writes
three length-prefixed strings (passage, field and term versions), then passage count.
For each passage, it writes body, headings and labels in that order: a term count,
then length-prefixed exact term bytes. Every length/count is an unsigned 64-bit
big-endian integer. The streaming SHA-256 covers empty fields, duplicates and order
without building another concatenated copy. This grammar belongs to v1, not current
Go struct layout. It is not the complete import fingerprint or an index codec.

`Open` checks bounded location JSON, article length/SHA-256, article schema and block
count, field versions/count/content/order, and ranges before returning an owned
reader. It does not run a passage builder or prepare terms. Supported persisted v1
tags are independent literals, so changing current builders does not redefine them.
Future query preparation still needs compatible term rules; this component does not
solve historical query-tokenizer dispatch. Unknown versions fail without fallback.

The reader owns its decoded article and ranges and retains no caller fields or byte
buffers. `Location` returns scalar bounds; `Quote` counts then joins block text with
LF into an owned string, excluding external headings, labels and image URLs. Concurrent
read-only calls use local scratch. Calls with work require deadlines; cancellation
is cooperative, with no goroutines or I/O in this package. Errors return no partial
results or captured payloads. A zero reader/table is invalid.

Bindings detect changed contents, not logical generation/document identity. Identical
fields may legitimately occur in different preparations. A future manifest/index
reader must pin the intended table object, article and exact edition/document/source
associations; possession of matching hashes is not authorization. A writer that
forges a mutually consistent table and fields is not authenticated by these checks.
Range validation alone cannot certify the claimed builder's semantic work.

## Bounds and cost

Positive limits cover encoded table bytes, blocks, passages, term occurrences,
individual term bytes and all field string bytes (including version tags). Capture
also takes bounded article-codec and preparation limits; Open takes article limits.
Capture may temporarily hold one complete preparation admitted by its separate
limits before applying smaller final field/table budgets. Counts are checked before
appending decoded rows. One token/record may allocate
before its smaller limit is checked; admitted bytes/record fields bound that unit,
not exact heap usage. Standard-library calls are not preempted mid-call.

For P passages and E table bytes, range copying/validation is O(P) and encoding/
decoding is O(E + P). Field hashing visits all occurrences and bytes. Capture also
pays article decoding and the existing complete projection/grouping work. Open pays
article hashing/decoding and field hashing, not just location loading. The reader
retains the decoded article plus O(P) ranges; it retains no terms. Loaded location
lookup is O(1); a quote costs O(blocks + bytes) work and O(quote bytes) output.
No allocation, heap or latency benchmark was run.

## Evidence and limits

Public tests compare exact wire bytes and an independent JSON view; every truncated
prefix of a sample; malformed/duplicate/null/version/integer/range failures; exact
and excessive limits; framing ambiguities; and a fixed SHA-256 vector from an
independent Python hashlib/struct calculation. They exercise ownership, concurrent
reads, and cancellation at observed cooperative checks. A synthetic non-current
partition verifies that the reader obeys saved ranges; it is not claimed historical
production output or proof that arbitrary input was prepared canonically.

A real S3 SDK HTTP fixture stores article, location and synthetic field-transport
bytes, reads through fresh clients, then compares ranking/quotes with the existing
snapshot. The field JSON is a test fixture, not a chosen product index format.
No live R2 write or complete publication/host-loss recovery is claimed.

The private Kubernetes audit saved and reopened 38 article records with 1,889 rows.
Location JSON totaled 60,209 bytes; the largest record was 4,848 bytes. All 28 frozen
cases retained the same 430 ranked references/scores and exact quotes. This excludes
articles, search fields/indexes, manifests, media, receipts and provider overhead;
it is not enterprise capacity or a compression/speed result. Selected raw evidence
and frozen source/baseline identifiers remain under `.tools/reviews/saved-locations-*`
and `.tools/probes/saved-locations-audit/`, outside normal builds and source history.

Persistent indexing, paged metadata, bounded selective readers and safe deletion
remain necessary scaling work. Revisit table placement/compression after measuring
encoded metadata, decoded memory and cold-open reads on a larger corpus with the
same integrity and citation guarantees.
