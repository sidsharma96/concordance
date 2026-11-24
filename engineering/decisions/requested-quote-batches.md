# Return a requested batch of verified quotes

Status: accepted option A. The maintainer chose an interactive default of ten results,
a positive --limit N, and explicit --all for complete output. This supersedes
the command's former default of quoting every match. Every-word matching, BM25,
exact edition selection and complete candidate admission remain unchanged.
Ten is an initial reading/download budget, not a measured optimum.

## One ranking, two counts

Search once with the existing complete-match ceiling, then select the first
min(limit, matched) references from that same immutable ranking. Read and verify
only those articles. Return the complete indexed-match count beside the selected
verified hits. The display says "Showing 10 of 619 indexed matches"; it does not
claim the other article bodies were read. Any selected read, binding, quote-byte,
aggregate-byte or cancellation failure returns a zero result before printing.
Do not skip a failed selected hit or fill its place from a lower rank.

Keep the existing Reader.SearchQuoted complete-all contract. Add the explicit
Reader.SearchQuotedFirst(ctx, query, searchLimits, quoteLimits, limit) operation
with a strictly positive limit and the same QuotedResult. Add TotalMatches int
to that result; it comes from the same Search result as Hits and equals their count
for SearchQuoted. Both paths use one private implementation so verification,
remaining-byte arithmetic and zero-on-error semantics have one owner. Only the
private helper may use zero to mean all; the new public operation rejects zero.

The helper validates context/quote bounds, calls Search once, bounds the selected
count, allocates an owned QuotedHit slice for that count, and quotes the temporary
ranking prefix synchronously. Its temporary slice view is borrowed only through
this call; the returned mutable hit storage is independent. Quote strings own
assembled text and references retain the same reader state/client lifetime.
Copies of a result still share its hit slice. No new retained aliases or cache.

## Command contract and admission

The CLI defaults to --limit 10. Enabled --all conflicts with an explicitly
supplied --limit, even if that explicit value is ten. --all=false leaves the
limited mode enabled. A nonpositive limit fails before connection setup. Both
routes keep all existing query/reader/quote/formatting/deadline admissions and
connection cleanup. --all uses the unchanged complete-all reader operation.

--max-matches remains a complete candidate ceiling, default 1000. Exceeding it
still fails even if the requested quote count is smaller. Lowering it already
bounds work but cannot return a useful first page when more matches exist. A quote
limit bounds selected article reads and display volume; it does not bound index
opening, candidate enumeration/full sorting, whole-process heap or total transfer.
No continuation cursor, optimized top-k algorithm, automatic reload or adoption of
the diagnostic any-word rule is included.

## Work, failure and alternatives

For H admitted ranked matches and positive limit L, K=min(H,L) additional article
reads are required after opening; all mode requires H. Each hit still reads a whole
bounded article, including repeated bodies. Search retains its existing work and
memory. The quote batch adds O(K + B) owned state for B selected quote bytes, plus
one article/read/decode scratch and the complete Search result. Opening a fresh
reader also reads the manifest and selected index. No latency/provider measurements
or cumulative-transfer bound follow from this count.

A quote failure after earlier successes leaves no successful batch or output.
An unavailable unselected article is not read and cannot be described as verified.
A later formatter/writer failure still has the existing semantics: formatting
fails before stdout, but a final device error can leave partial bytes. Metadata
and shown/matched counts must stay bound to one reader, generation and edition.

Keeping complete output is useful for exhaustive inspection/export and remains
explicitly available. Paging ten results with a continuation cursor would support
navigation, but adds generation/query/version/tie binding and reader/authorization
lifetime rules across requests. It remains a separate checkpoint. A request cache
could reduce repeated downloads but adds memory/lifetime policy and does not reduce
the number of shown passages; no cache is selected. Revisit ten with actual reader
needs, pagination with navigation demand, and matching independently with the
frozen development set. Reserved queries remain unused for tuning.

## Primary mechanism and checks

Elasticsearch 8.19's [pagination guide](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/paginate-search-results.html)
separates requested hit count from matching and describes a default of ten.
At pinned 8.19.0 commit 93788a8c2882eb5b606510680fac214cff1c7a22,
[SearchService.DEFAULT_SIZE](https://github.com/elastic/elasticsearch/blob/93788a8c2882eb5b606510680fac214cff1c7a22/server/src/main/java/org/elasticsearch/search/SearchService.java)
is ten. That distributed engine's fetch, shard and paging machinery differs from
our strict article verification; its default does not prove ten is optimal here.
The motivating 619 matches come from our offline any-word trial, not typical
current product queries. Every-word keyword queries had a 4.5-candidate median;
the current worst-case command admission remains 1000 matches.

Use real local PostgreSQL/HTTP producer-reader fixtures. Check default ten versus
12 complete matches, positive explicit limits, all mode and conflicting/invalid
flags before connection. Preserve exact ranking/scores/provenance and total counts
from one Search; verify inclusive selected-byte limits, match overflow before
article I/O, no read of an unselected hit, selected later-read/cancellation failure
with empty result/stdout, owned independent results, empty selections, cleanup and
short writes. Ordinary builds/tests remain independent of private teaching notes and
the corpus. No hosted schema/data changes or live provider experiment are required.
