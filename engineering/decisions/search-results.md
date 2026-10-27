# Return references and quote on demand

## Decision

The user chose references with quotes produced on demand (option B). Search returns
complete bounded ranked matches as small handles; an explicit quote operation
materializes only the passages a caller requests. Input remains borrowed under the
[snapshot ownership decision](snapshot-ownership.md). An independent quote string
can outlive that borrowed state; a reference keeps the state reachable.

The concrete first consumer is retained HTML, not a universal source/citation API.
`internal/htmlsearch` composes the canonical `htmlterms.Build` projection and
`termsearch.RankBM25`. Source acquisition, raw-text query preparation, source identity,
retained HTML objects and published-generation integration remain separate. This
component does not implement a Markdown processor or make HTML blocks a universal
location type for future PDF/provider/code processors.

## Result and quote contract

An opaque reference is bound to one snapshot state and a local edition/passage
position. Reject zero and cross-snapshot references explicitly. Copies of a snapshot
wrapper share the same state; separately built snapshots have different state even
when given identical inputs. Handles are temporary, not persisted IDs, authorization
or proof of catalog publication. Do not serialize them as citations.

Location lookup returns only values: edition and article positions in the original
input, article-local passage position, and its canonical half-open body block range.
The caller preserves the exact input ordering and associations with logical document,
source, captured revision, profile and generation metadata. Original Git line ranges
are still optional verified metadata. A location is a partial in-memory association,
not a complete durable citation. No mutable article/term slices escape through results.

Quote reads retained article `Block.Text` within that body range, joining successive
blocks with one LF. It preserves the extracted text, including code whitespace and
image alt text already present as blocks; it adds no notice labels, ancestor headings,
URLs, inferred image text or guessed Git lines. This is extracted article text, not
byte-for-byte raw HTML or reconstructed original Markdown. Displaying a quotation
never authorizes execution of retained HTML.

Require a deadline and positive quote-byte ceiling, count text and separators before
allocation, and reject excess rather than truncate. Every failure returns an empty
string and an error. A successful nonempty string owns newly assembled storage, not
a view of the source article. Strings are immutable in safe Go; the snapshot never
reuses that quote's buffer. An empty retained body text may produce an empty quote
successfully; callers distinguish failure by its error.

Search keeps the accepted every-keyword rule, exact edition, fixed combined BM25,
complete-match limit and no partial output on failure. Empty and missing editions
remain distinguishable. There is no implicit top-k or fallback. A display limit
belongs to the caller; it does not alter scoring statistics or matching completeness.

## Storage and lifetime

Construction borrows the caller's edition/article input, including nested block and
group arrays. It owns freshly computed term arrays/strings and compact body-location
tables. Full passage-plan context references are temporary preparation work; fields
already carry their searchable terms. It does not retain raw HTML bodies or images
that were never part of the input. Context metadata can be added deliberately later.

Keep every input alias unchanged from construction through the last dependent use.
A single reference can keep the whole state, original backing allocations and prepared
terms reachable. The library exposes no mutable borrowed block views, but the caller
still has its own aliases. Search/quote scratch is local per call; concurrent reads
require unchanged input, not locks or pooling added here. No goroutines, I/O, cache,
reference counting or explicit release protocol is introduced.

Build admits all-edition aggregate counts for editions, articles, blocks, groups and
logical input strings before preparation. Per-article field/passage limits still
apply. Aggregate passage, prepared occurrence and output-byte ceilings apply across
all articles after each bounded article preparation; a temporary can include one
prepared article and its full plan plus admitted prior output. These are logical
ceilings, not exact heap quotas; borrowed short slices may retain larger allocations.
Per-article projected-view bounds and the article-count ceiling also bound total
projection work; there is no separate premeasured global projection-byte pass.

## Alternatives, workload and revisit

Borrowed block results avoid string materialization but expose additional mutable
aliases and extend the unchanged-data obligation through every escaped view. Eager
owned quotes make display a single operation but assemble text for undisplayed hits.
On the retained corpus, `pod conditions` has 77 matches in edition 1.35. Displaying five
is an illustrative caller choice, not a selected top-k policy. On-demand quotation
would assemble those five rather than all 77. No latency or memory savings have been
measured. Quoting every result remains valid when a caller needs them all.

Construction includes existing passage/projection work plus aggregate counting and
flattening of field/location entries. Search still pays the existing all-input
validation, selected-edition scans and full-sort costs; a snapshot does not silently
cache BM25 statistics or bypass admission. Mapping M ranked hits adds O(M) work and
storage plus edition selection. Location lookup is O(1). A quote visits its selected
blocks twice, counts/copies its bytes, and allocates O(quote bytes). All operations
make zero remote requests. Full construction cost also includes repeated heading/
label projection and the passage builder's group/reference work, not just raw text.

Test the actual extraction/preparation/search/quote sequence, source associations,
zero/cross-state references, exact and excessive aggregate/quote limits, cancellation,
unchanged input, independent quote storage and concurrent readers. Compare ranked
locations/scores against direct public component composition and quotes against a
simple slice-and-join oracle. Revisit with actual caller mutation needs, retained
memory measurements, corpus growth or a persisted citation/reader-retention design.
