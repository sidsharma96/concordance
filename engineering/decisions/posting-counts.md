# Count occurrences in the first posting lists

Status: accepted option A. Each term's posting list records matching passage
positions and separate body, heading and notice-label counts. Keep uncompressed,
sorted lists as the first comparison with the exact scan and combined BM25.
This choice concerns an in-memory index, not its eventual persistent format.

## Why counts first

For the illustrative passage `retry upload retry`, the word `retry` has a body
count of two. The current every-keyword rule needs presence; BM25 also needs
frequency, passage lengths and edition-local statistics. Neither requires knowing
that the occurrences are at word positions zero and two. Preserve separate field
counts even though today's scoring combines them without weights.

Option B additionally stores occurrence positions within each prepared field.
That is useful groundwork for phrase and proximity matching: an exact error phrase
or nearby action/feature words can distinguish passages which happen to contain
the same words. It adds position lists and compatibility/validation work. Actual
phrase behavior needs rules for field, heading and block boundaries; positions
alone do not implement it. No evaluated query set has established this need yet.

Positions do not provide semantic similarity between different words or paraphrases.
Future embeddings can consume retained passage text independently of this posting
representation. Counts do not prevent semantic retrieval. Adding phrase/proximity
positions later requires extending and rebuilding the index with compatible
processing rules; it is not a free switch on existing count records.

## Representation and guarantees

Within one exact edition, a list contains one entry per matching passage, ordered
by the original passage position. Duplicate occurrences increment a field count;
they do not create duplicate entries. At least one field count is positive.
Empty passages still contribute to the selected edition's passage count and
average-length statistics. Editions never share matching or ranking statistics.

Query duplicates do not add requirements. Keep every-keyword intersection,
lexically ordered distinct-term scoring, the current BM25 formula and full sorted
output with input-position ties. Excessive final matches fail rather than truncate.
The existing scan and full-sort ranker remain the oracle. No implicit top-k,
field weighting, raw-query parser or citation identity follows from this decision.

The index consumes prepared terms, so another source or content processor does not
need to imitate HTML. Source, document, edition, revision and generation metadata
remain distinct. Passage positions are local to their exact input association.
Authorization and durable source/citation associations stay with the application.

## Cost and revisit

For T term occurrences and U distinct term/passage pairs, count postings require
O(U) records with fixed field counts; positional postings additionally store O(T)
positions. Both builders visit all terms. Actual allocated/retained bytes depend on
keys, arrays, maps and encoding; there is no measured savings claim against option B.

The initial corpus has 38 article-version entries, 1,889 passages and 84,053 term
occurrences. Compare exact results and costs on that workload and on skewed lists.
Revisit positions when concrete phrase/proximity queries show useful precision
improvements. Persistent indexing, paged metadata, bounded selective readers and
safe retention/deletion remain necessary scaling work.

Count postings cannot reconstruct the ordered prepared fields used by
`htmlpassagemap.Open`'s fingerprint. Keep those fields as input/reference for this
comparison. Before persisting a count-only index, define verified index/table/
article bindings or retained field data separately. Do not weaken the location
reader into accepting an unchecked caller-asserted digest.
