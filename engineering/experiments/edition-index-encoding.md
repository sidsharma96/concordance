# Compare two explicit edition-index encoding samples

The maintainer accepted [counts and saved locations
together](../decisions/edition-artifact-binding.md) without another ordered-word
copy. The maintainer selected binary with JSON inspection. The follow-up cost
comparison below informed that choice; the [binary component
experiment](binary-edition.md) describes the separately implemented codec. The
retained-article JSON decision does not choose an index encoding. This comparison
uses disposable valid-input samples, not a production codec or recovered generation.

## Same information, two representations

Both samples contain the same edition and processing tags, exact retained-article
references, passage locations/lengths, edition term total and a sorted word dictionary.
Each word has sorted rows containing passage ordinal, body count, heading count and
notice-label count. No ordered prepared-word arrays are saved. Original text remains
in separate article objects. Source/document/revision associations and a complete
manifest are outside these samples.

The JSON sample uses named object fields, with each posting represented by four
integers in an array. Empty sections are `[]`. It uses `encoding/json.Marshal` and
`Unmarshal` on explicit sample structs, not private `Index` state. This sample decoder
is not the strict closed/bounded production decoder that would be required.

The binary sample uses a sample magic header, length-prefixed UTF-8 strings, counted
sections and unsigned variable-length integers. A small integer occupies fewer bytes
than a large one; Go's `encoding/binary.AppendUvarint` and `Uvarint` write/read this
base-128 representation. Passage ordinals are absolute, not gaps between neighbors.
Article digests use 32 raw bytes rather than JSON's 64 hexadecimal digits. The sample
has basic size/count guards; it has not undergone hostile-input decoder testing.
Neither sample uses general compression, delta encoding or skip data.

## Observed byte sizes

The workload is the existing 38 retained Kubernetes article-version entries across
three pinned editions, with 1,889 passages, 84,053 term occurrences, 7,566 distinct
words summed per edition and 58,362 word/passage pairs. The experiment ran on Go
1.27.2 darwin/arm64. Encoded size is deterministic for these sample records; this is
one valid-input round-trip comparison, not a repeated timing benchmark.

| Edition | Sample JSON bytes | Sample binary bytes |
| --- | ---: | ---: |
| 1.34 | 276,324 | 100,118 |
| 1.35 | 348,334 | 127,475 |
| 1.36 | 384,856 | 141,232 |
| Total | 1,009,514 | 368,825 |

Binary is about 63.5% smaller for these uncompressed samples. Both include their
headers, article descriptors, ranges, lengths and postings. They exclude article
bodies, logical-source/citation metadata, manifests, object-store/provider overhead
and any retained ordered fields. The known-empty control is separately 267 JSON
bytes and 122 binary bytes; it is not included in the real-edition totals.

For both samples, decode compares every record with the input and canonical
re-encoding reproduces the exact bytes. Counts expand into one possible word order
solely to reuse the original scan as an oracle; they do not recover the original
order or pass the old ordered-field fingerprint check. All 28 query cases preserve
430 exact frozen ranked references/scores. Recovered locations select the expected
article/range, with 138,032 summed quote bytes checked against independent original
block joins after article JSON verification and decoding. Known-empty and absent
editions remain distinct. This is local comparison evidence, not a new published
reference trust path, persistent index loader or remote recovery drill.

## Alternatives and follow-up costs

The initial recommendation favored versioned JSON: explicit records are easy to inspect,
and the existing article/location work already exercises strict JSON parsing and
compatibility handling. Its measured sample cost is more encoded bytes. A production
reader still needs all schema, bounds, arithmetic, binding and ownership checks;
using the standard library alone does not supply them. Versioning permits a later
binary reader without changing source acquisition or ranking, but requires deliberate
retained-format compatibility rather than silently reinterpreting old bytes.

The binary sample demonstrates a real byte reduction on this workload and exercises
compact integer encoding. It also introduces a custom diagnostic/compatibility
surface and needs independent malformed/truncation/overflow tests and inspection
tools. It is a credible alternative if smaller edition files are worth that work
now. The initial size-only comparison measured no CPU, allocations, retained heap,
cold-open latency or compressed sizes. The follow-up below adds scoped prototype
measurements, not a production speed guarantee. The initial JSON baseline recommendation was an engineering trade-off, not a
performance result. It was revised after the follow-up measurements and maintainer
discussion; the selected binary format includes readable JSON inspection.

The maintainer chose binary after comparing readability, implementation/recovery work and
observed bytes. Revisit compact encoding when measured edition sizes, cold-open costs
or an explicit design goal justify it. Neither encoding addresses a huge single
edition, paged metadata, reader lifetime or safe deletion by itself. Do not extrapolate
this small corpus to a million-document storage bill.

## Follow-up prototype measurements

A separate valid-input cost probe compressed these same samples with Go's default
gzip level, then verified exact decompression and equivalent records. Across all
three editions, gzip JSON totals 189,938 bytes and gzip binary 163,693 bytes: binary
is 13.8% smaller after both are compressed. Compression remains unselected; these
are not the slightly different implemented v1 file sizes.

Five 200 ms local benchmark samples on Go 1.27.2 darwin/arm64, Apple M1, default
GOMAXPROCS 8 measured checksum plus prototype decoding of the largest edition:

| Sample | Time, ms/op | Allocated MB/op |
| --- | ---: | ---: |
| JSON | 4.817–5.143 | 2.36 |
| Binary | 0.614–0.619 | 2.36 |
| Gzip JSON | 5.861–5.899 | 3.33 |
| Gzip binary | 1.470–1.481 | 2.76 |

These valid-input parsers do not perform equivalent complete production validation,
construct the query index or read R2. Allocation totals are not retained/peak heap.
The binary prototype writer also allocated more bytes than JSON despite smaller
output, due to its buffer growth. Neither file size nor this prototype establishes
production memory or full search speed. With the same in-memory query representation,
encoding affects cold load work rather than BM25's query algorithm. Request counts,
article bytes, full manifests and retained generations remain separate costs.

Private probe `.tools/probes/edition-codec-cost/` and raw compression/decode/encode
logs `.tools/reviews/edition-codec-compression.txt`, `edition-codec-decode-bench.txt`
and `edition-codec-encode-bench.txt` preserve this follow-up. No remote writes occurred.

## Reproducibility

Private disposable Go probe: `.tools/probes/edition-codec-sample/`. Raw output:
`.tools/reviews/edition-codec-sample.txt`. The capture helper is copied unchanged from
the previous audit. Source inventory SHA-256:
`f82481e1d451e5b1901e87231158f3ddeb338ff06bc5295b5cb81138cd268052`.
Frozen ranking report SHA-256:
`da2f4eddd71ccea8cceec2a02ad40ecc2639f207821cce31bbf5ab088ef4543e`.
The probe checks both identifiers and source hashes before comparison. No frozen
inputs changed; no remote writes, publication or provider fault experiment occurred.
Normal repository checks require neither this private corpus nor disposable probe.
