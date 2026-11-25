# Versioned binary edition indexes with readable inspection

Status: accepted option B for the first edition-index encoding. Use an explicit
binary format and a bounded tool that prints its contents as JSON. Compression is
not selected. Retained articles remain versioned JSON. The earlier JSON baseline
recommendation favored inspection; the follow-up measurements and storage-design
priorities supported choosing binary. Prototype timings are not production promises.

## Bounded implementation

`internal/htmledition` binds one exact edition's count postings and saved HTML
body locations, implementing the accepted [artifact direction](edition-artifact-binding.md).
Its canonical builder consumes retained article JSON in caller-supplied document
order, decodes it and prepares each article once. It builds the count index from
those same fields and saves the corresponding plan ranges. Two article slots may
reference identical bytes; slots are not logical document IDs. Source/document/
revision associations still belong to future generation metadata.

`termsearch` exposes an owned count export and a validated count constructor
for this concrete consumer. Counts retain separate body, heading and label frequencies,
lengths and a total. Lists and vocabulary are strictly sorted/unique; the constructor
recomputes lengths/statistics and logical original-input byte admission using
frequency times word length, with checked arithmetic. It clones keys and rows rather
than borrowing mutable exported arrays. Existing scans, `NewIndex`, query limits,
lexical score order and stable ties stay unchanged. Restoring one edition accounts
for that edition only; future generation admission remains separate orchestration.

The edition builder/reader owns immutable state and starts no I/O or goroutines.
Encoding and inspection temporarily export owned counts; decoding temporarily holds
wire records before constructing the owned index. This bounded baseline pays for
those copies; encoded-byte limits alone are not exact peak-heap quotas. No ordered
prepared-word arrays are persisted. Caller input is borrowed unchanged until return
and not retained. Read-only searches and location/quote operations may run concurrently.

Opening requires actual bytes, an independently supplied expected size/SHA-256 and
the expected exact edition. A future authorized published generation must provide
those references; the component itself does not establish their authority. Verify
bytes before parsing, then versions, all bounds, counts/statistics and complete
article/range coverage. A corrupt declared object fails without partial search state.
The reader does not fetch articles during index open. Before quoting it checks the
provided exact article bytes against the saved reference, decodes them with article
limits, verifies the actual block count and assembles the bounded saved body. It
never rebuilds passage grouping or ordered fields. The old `htmlpassagemap.Open`
remains unchanged. Checksums/structure do not prove writer semantics or authorization.

## Closed v1 grammar

All integers are nonnegative, minimally encoded unsigned base-128 varints. Reject
truncation, overflow, overlong encodings, values beyond the platform int and trailing
bytes. Strings are byte-length-prefixed UTF-8, nonempty and without NUL. Digests are
32 raw SHA-256 bytes. There is no compression, gap encoding, skip data or native
Go struct serialization.

In order:

1. Fixed magic `CNDX` followed by a zero byte; integer version 1.
2. Strings: exact edition key, article format `concordance.article/v1`, passage
   processor `htmlpassage/v1`, field processor `htmlterms/v1`, terms tag
   `technical/v1/unicode-17.0.0`. Readers use fixed supported literals rather than
   treating the newest builder constants as automatic retained compatibility.
3. Header integers: article count, passage count, vocabulary count, posting count,
   total term occurrences.
4. Article rows: digest, encoded article byte size, block count, passage count.
5. Passage rows: article ordinal, first block, exclusive after block, combined-field
   term length. Article order and passage order are preserved. Each article has
   positive blocks/passages and contiguous nonempty ranges covering its blocks once.
6. Lexically sorted word rows: word string, posting-row count, then strictly increasing
   passage ordinals with body/heading/label counts. Every row has positive combined
   frequency; a field may be zero. Sum counts must reproduce lengths and header total.

A known-empty edition has no articles, passages, words, postings or occurrences.
Nonempty articles may have passages with zero prepared terms. An absent edition is
not an empty file: selection happens in the future manifest. Missing bytes for a
declared edition remain an error. Unknown format versions/tags fail closed; another
content kind needs a deliberate format, not HTML locations relabeled as PDF/code.

Before allocating declared tables, admit cardinalities against positive caller
limits and minimum remaining encoded bytes. Also bound aggregate article bytes/
blocks, prepared occurrence bytes, key bytes, per-term bytes, vocabulary, postings
and passages. Bound output before appending and return no partial result on failure.
Require deadlines; library hashing/sorting/decoding calls finish before cancellation
is observed. Operational failures never include captured content in error messages.

## Inspection, evidence and revisit conditions

The maintained Go tool `tools/inspectindex` requires the expected size, digest and edition plus the
local file. It uses the same verified reader before emitting a bounded JSON diagnostic
view. Operator-supplied checksums are diagnostic expectations, not proof of a published
generation. The view is not another accepted persistence format or citation schema.

The uncompressed samples totaled 1,009,514 JSON bytes and 368,825 binary bytes. With
both gzip-compressed they totaled 189,938 and 163,693 bytes, narrowing the binary
size advantage from 63.5% to 13.8%. Five local valid-input prototype samples for the
largest edition measured checksum/decoding at about 4.8–5.1 ms JSON and 0.61–0.62 ms
binary; both allocated about 2.36 MB per operation. Production validation, query-index
construction and R2 transfer were excluded. The binary prototype writer allocated
more bytes than JSON despite producing smaller output. See the encoding experiment;
none of these numbers establish performance of the implemented reader. Its separate [component experiment](../experiments/binary-edition.md)
records actual codec evidence and limitations.

For I encoded bytes, V words, U word/passage pairs, P passages and A article slots,
opening traverses I plus V+U+P+A records and string/validation work; constructing the
owned maps uses expected hash-table costs. Encoding sorts V keys and writes I bytes.
Memory includes wire records, owned index, ranges, key bytes, buffers and capacity.
Quote adds a verified article decode and output proportional to its selected text.
One cold edition still needs one index GET under either encoding. Article reads,
manifest requests, retained generations and whole-edition rewrites are separate costs.
Revisit compression after production cold-open measurements; revisit blocks/pages
when a single bounded edition exceeds budgets. Safe retention/deletion remains open.
