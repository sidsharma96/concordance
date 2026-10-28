# Compare two explicit edition-index encoding samples

The user accepted [counts and saved locations together](../decisions/edition-artifact-binding.md)
without another ordered-word copy. The physical index encoding is still open.
The retained-article JSON decision does not choose an index encoding. This comparison
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

## Proposed choice, not an implemented conclusion

Versioned JSON is the proposed first baseline: explicit records are easy to inspect,
and the existing article/location work already exercises strict JSON parsing and
compatibility lessons. Its measured sample cost is more encoded bytes. A production
reader still needs all schema, bounds, arithmetic, binding and ownership checks;
using the standard library alone does not supply them. Versioning permits a later
binary reader without changing source acquisition or ranking, but requires deliberate
retained-format compatibility rather than silently reinterpreting old bytes.

The binary sample demonstrates a real byte reduction on this workload and offers
integer-encoding learning. It also introduces a custom diagnostic/compatibility
surface and needs independent malformed/truncation/overflow tests and inspection
tools. It is a credible alternative if smaller edition files are worth that work
now. It is not evidence of faster decoding or querying: neither CPU, allocations,
retained heap, cold-open latency nor compressed sizes were measured. The JSON baseline
recommendation is an engineering trade-off, not a performance result.

Before choosing, compare readability and implementation/recovery work against these
observed bytes. Revisit compact encoding when measured edition sizes, cold-open costs
or an explicit learning goal justify it. Neither encoding addresses a huge single
edition, paged metadata, reader lifetime or safe deletion by itself. Do not extrapolate
this small corpus to a million-document storage bill.

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
