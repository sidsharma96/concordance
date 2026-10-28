# Versioned JSON for retained articles

Status: accepted option A, implemented in `internal/htmlarticlejson`. This is a
bounded HTML-article codec, not a generation manifest, citation registry, importer
or stored search index. Other content kinds need their own explicit schemas and
processors. The complete captured-input fingerprint and source association remain
open. The implementation/evidence reference is [article JSON](../experiments/article-json.md).

## Why this format

The first recovery experiment needs inspectable files and explicit compatibility.
JSON uses Go's standard library and makes individual fields readable. The credible
alternative was a custom versioned binary layout: potentially less field/number
syntax, but more codec and diagnostic work. Neither has a measured size or speed
advantage. Measure before optimizing. A later binary format requires its own format
identifier and retained readers; version tags do not migrate old objects.

Keep wire types separate from the current Go article structs. A new exported Go
field must not silently change an immutable stored format or exact-byte hash. Source,
document, edition and revision identities belong in referring application metadata.
The article's captured-HTML digest/length and processor/profile are preserved to
reconstruct this exact representation, not to establish provenance or authorization.
Unchanged prose alone does not imply identical encoded bytes: this metadata matters
for reuse. Retain original HTML/source/media separately under the existing decisions.

## Closed v1 schema

An object has these required, case-sensitive fields in canonical writer order:

| Field | Value |
| --- | --- |
| `format` | `concordance.article/v1` |
| `kind` | `html` |
| `processor` | `htmlarticle/v1` |
| `profile` | Exact bounded extraction profile identifier |
| `html_sha256` | 64 lowercase hexadecimal characters describing the captured input |
| `html_bytes` | Positive bounded captured-input byte count |
| `blocks` | Nonempty ordered array |
| `groups` | Ordered array, including empty containers |

Every block requires `kind`, `text`, `level`, `anchor`, `source_url`. Kinds are heading,
prose, code, image and separator. Headings use levels 1..6; other blocks use zero.
Only image blocks may carry a source URL. Every group requires `kind`, `first`,
`after`, `label`, `ordered`, `header`. Group kinds preserve the current retained
container vocabulary. Ordered applies only to lists, header only to cells, and
nonempty labels only to notices. Half-open block ranges must be in opening order,
nested or disjoint, within bounds and within the configured nonempty nesting depth.
This is representation validation, not full HTML grammar or proof of extraction.

No field is optional, even for empty strings, zero levels or false flags. Null,
unknown, duplicated (including escaped duplicate names) or incorrectly cased fields
fail. Input field order and legal JSON escapes may vary. Integers must be unsigned
in spelling, decimal without fractions/exponents, representable as Go int and within
configured limits. A negative zero spelling is rejected. Unknown format/content/
processor versions or enum values fail explicitly; there is no fallback.

Strings must be valid UTF-8 without NUL. Malformed escaped UTF-16 surrogate pairs
are rejected before the standard decoder can replace them. Real U+FFFD text and
valid surrogate pairs are preserved. No whitespace folding, Unicode normalization,
HTML interpretation or text preparation occurs. Code spacing/newlines remain exact.

The writer emits compact records in fixed field order using standard Go JSON
escaping, without a final newline. Empty/nil groups encode as `[]`; decoded empty
groups normalize to nil. This preserves semantic contents, not Go slice nilness.
Other whitespace/escape spellings can decode to the same article but have different
stored byte hashes. Normal writers always use this canonical encoder. The exact
encoding is guarded by golden tests; changed escaping requires compatibility review.

## Bounds, ownership and failures

All limits are positive and caller configured: encoded bytes, blocks, groups,
individual string values, aggregate decoded string-value bytes, group depth and
captured HTML byte count. Keys are fixed by the schema and are not charged as
string values. Aggregate values also cannot exceed encoded bytes, a lower bound
on the size of any valid encoding. These are logical bounds, not exact heap quotas.
There is no runtime default derived from the repository's 1 MiB source-file policy.

Decode checks bytes and raw UTF-8 before parsing; a linear escape scan prevents
surrogate repair. A schema-driven standard JSON token reader uses a small required/
duplicate-field bitset and checks array counts before appending records. Schema
nesting is fixed at three levels. One decoded string token can be allocated before
its per-value ceiling is checked, bounded by the admitted encoded input. Unknown
nested values are rejected immediately, not recursively materialized.

Encode validates structure, counts and values before encoding. It marshals one
bounded metadata/block/group record at a time and appends only within its encoded
output budget. A whole-article Encoder would buffer the whole value before a
bounded writer saw it. One record's temporary bytes and Go slice capacity are not
an exact heap bound. Any failure returns nil encoded bytes or a zero decoded article;
errors contain no captured payload. SHA-256 integrity checks on downloaded objects
remain separate from schema validation.

Both operations require a deadline and check cancellation cooperatively. Standard
UTF-8/JSON operations are not interrupted mid-call. Encode borrows all article
aliases unchanged until return. Decode borrows its input until return and owns its
returned strings and slices; wiping input later cannot change search text. Neither
operation retains input, starts goroutines or performs file/network I/O.

The v1 semantic validator is deliberately independent of current passage grouping:
a grouping change must not redefine whether a retained v1 file is valid. Recovery
can decode structure before choosing compatible passage/term/index versions from
its generation metadata. The codec alone does not make a complete searchable
published generation or a persisted citation.
