# Retained article JSON experiment

The accepted [JSON decision](../decisions/article-json.md) now has a bounded internal
encoder/decoder. It retains HTML input metadata, blocks and groups in explicit v1
wire records. It stores no document IDs, source revisions, prepared terms, passage
plans or postings. The complete manifest/fingerprint/import protocol remains open.

## Operations and cost

Encode borrows an unchanged article, admits counts/values/structure, marshals one
wire record at a time and returns owned compact bytes. Decode admits its byte slice,
checks Unicode escapes, consumes standard JSON tokens under the closed schema and
returns an owned article. Required-field masks distinguish missing from false/zero;
normalized keys catch escaped duplicates. Integers keep their original spelling via
UseNumber and are parsed without floating-point rounding. No partial results escape.

For E encoded bytes, V string-value bytes, B blocks and G groups, traversal and
encoding/decoding work is O(E + V + B + G), with constant-sized field lookup. Group
range validation uses a stack; each range is pushed/popped at most once, with
O(min(G, configured depth)) stack space. Outputs own O(E) encoded bytes or O(V+B+G)
decoded state. Transient parser tokens are bounded by admitted input, and a temporary
encoded record by its admitted strings/fields. Library buffering, slice capacity
and object overhead are not exact heap quotas. Context checks do not preempt a
standard-library call. No allocations or latency benchmark has been run.

## Evidence

Public tests check exact canonical bytes and an independent standard JSON view,
real extraction round trips including code/Unicode/images/containers, exact and
excessive bounds, field presence/case/duplicates/null/types, numeric spellings and
overflow, unknown versions/enums, invalid ranges/flags, malformed UTF-8 and escaped
surrogates, legitimate U+FFFD, trailing data and every truncated prefix of a sample.
They check ownership, cancellation at every observed cooperative Err boundary,
empty-group normalization and noncrossing group intervals. A bounded fuzz target
checks accepted input and canonical round trips, allowing canonical expansion to
exceed an input's original size budget without calling that corruption.

A real SDK HTTP fixture exercises Encode -> immutable object Ensure -> fresh-client
Read -> Decode -> search/quote. Recovered quotes agree with independent joins of
original block text; notice labels are not inserted into body quotes. Wiping the
retrieved byte buffer does not mutate decoded articles. This is not live R2,
publication or host-loss recovery evidence.

The private Kubernetes audit reconstructed all 38 retained article-version entries
from encoded bytes, compared every article field and canonical re-encoding, then
reran the frozen search audit. All 28 cases retained 430 ranked references with exact
identities/order/scores, and every quote matched the original block join. The files
totaled 1,024,848 encoded bytes; the largest was 109,827 bytes. These counts cover
only these article files, not raw source/HTML, media, manifests, indexes, receipts,
heap use or a representative enterprise workload. They do not establish compression
ratios, relevance, speed or million-document capacity.

Raw measurements, pinned source/baseline identifiers and disposable capture helpers
remain private in `.tools/reviews/article-json-*` and `.tools/probes/article-json-audit/`.
No R2 writes, catalog publication, dependencies, hosted workflows or migrations were
needed. Normal tests/builds do not depend on that private corpus.

## Remaining work

Choose and validate complete generation source/citation associations, fingerprints
and manifest schema before publication. Keep captured source/media retention and
processor compatibility explicit. Persisted indexing, paged metadata, bounded readers
and safe retention/deletion remain necessary scaling work. Measure actual encoding/
opening cost on a larger corpus before deciding on compression or another codec;
retain readers for older format identifiers. This component's byte preservation
does not certify that the original extraction captured every source feature.
