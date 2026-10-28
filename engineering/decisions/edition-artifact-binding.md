# Keep counts and passage locations in one verified edition artifact

Status: accepted option B. Persist count postings and saved passage locations together
in each edition's versioned index object, with exact retained-article references.
Do not retain an additional ordered prepared-word copy for the first reader. The bounded [binary codec/reader](edition-binary.md) now implements this direction;
the complete manifest and publication path remain open.
The [edition layout](edition-index-layout.md) and single generation manifest remain.

## Follow one recovered result

An example passage contains `retry upload retry`. Its counts are `retry: 2` and
`upload: 1`; its saved location says which blocks of which retained article to quote.
The writer must derive both from the same canonical preparation, then save them in
one immutable edition artifact. The published generation must pin that artifact's
exact bytes and its logical associations.

A reader first obtains the edition reference from an authorized published generation.
It checks the actual index bytes against the expected size and SHA-256, then checks
the supported format and processing versions, schema, bounds and relationships.
A field hash supplied by an unchecked caller is not a substitute. A valid checksum
for another edition is insufficient: the selected edition and generation's expected
reference must agree. No silent version or generation fallback is allowed.

Before search, validate sorted unique vocabulary and passage lists, positive posting
frequencies with nonnegative separate field counts, valid passage ordinals and
agreement between counts, passage lengths and edition statistics. Check arithmetic
without integer overflow. Saved passage rows must refer to declared article slots
and valid nonempty body ranges, with complete nonoverlapping article coverage under
the supported HTML version. Keep known-empty editions explicit and distinguish them
from absent editions and missing declared objects.

Index opening can check article descriptors and ranges against declared block counts
without fetching every article body. Before quoting, fetch the exact referenced
article, verify its size/hash and supported article format, and check its actual
block count and the requested range. A missing or mismatched declared body is an
error, never an empty quote or a fallback to the current website. The bounded binary reader now implements this sequence; its caller still supplies
bytes and trusted references. Loading one edition
does not establish integrity of every other artifact's body.

## What is trusted, and what is checked

The checksum detects disagreement with the pinned bytes. Structural validation
rejects inconsistent representations. Neither proves that a writer indexed the
right words or bound a plausible range to the right logical document. The canonical
writer remains responsible for producing counts and locations from one preparation;
independent scan/rebuild and quote comparisons remain the correctness oracle.
Checksums also do not authorize a publisher, collection, search or citation.

Future generation metadata must pin the exact edition artifact and its article,
source, logical-document and revision associations. Article hashes describe bytes,
not document identity. Two logical documents may share identical article bytes.
The complete manifest and input fingerprint are still separate design work.
Confirm all required objects before the PostgreSQL publication transaction, using
the accepted receipt/namespace protocol where applicable. This choice does not
change cross-store failure handling or permit automatic deletion.

The existing `htmlpassagemap.Open` remains unchanged. It verifies original ordered
fields, which counts cannot reconstruct. Keep it as a baseline; do not add an entry
point that accepts an asserted field hash to bypass that check. The [binary component](../experiments/binary-edition.md) documents the new edition
reader and its compatibility/ownership rules.

## Alternative and cost

Option A retains ordered prepared fields alongside counts and locations. It preserves
the current field-fingerprint check and aids exact preparation debugging. Verifying
postings against those fields would still require additional traversal; simply
retaining both does not establish that they agree.

Option B avoids that additional representation and its storage/read/hash work.
The [sample experiment](../experiments/edition-index-payload.md) measured 780,612
uncompressed JSON bytes for ordered fields across 38 retained article-version entries.
That is an illustrative extra payload, not measured savings in a selected index
format or a production bill. Original article text remains available for quotes
and later semantic processing. Word positions and phrase/proximity matching remain
deferred.

For an edition with I encoded bytes, V distinct words, U word/passage pairs, P passages
and A article descriptors, a full reader must at least read/hash I bytes and traverse
V + U + P + A records, plus string and arithmetic checks. A validated in-memory index
still owns postings, keys and passage metadata; byte limits alone are not exact heap
quotas. A quote adds the selected article's verified read, decode and output costs.
No latency, retained-memory or large-corpus result follows from choosing B.

Revisit ordered-field retention if recovery must independently reproduce preparation
on every open or exact intermediate debugging becomes a requirement. An offline
rebuild audit is a separate alternative to paying that cost on every reader. Revisit
index blocks/pages when measured edition size or cold opens exceed budgets. Persistent
indexing, paged metadata and safe retention/deletion remain necessary scaling work.
The user separately chose binary for the edition index, with JSON inspection;
article JSON remains unchanged. The [encoding samples](../experiments/edition-index-encoding.md)
preserve the alternatives and limits of the evidence. Complete published-generation
bindings and recovery remain the next integration work.
