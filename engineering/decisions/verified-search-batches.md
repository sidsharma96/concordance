# Verify a complete quote batch before rendering results

Status: accepted option A. The user chose verification of the whole requested
batch before printing, rather than per-hit quote-unavailable output. This document
specifies the internal implementation; a public command is not implemented yet.

## Workload and invariant

An exact published reader returns a complete bounded lexical ranking. Article
bodies are fetched lazily for quotes. If the second quote fails after the first
succeeds, a command must not present the first as a successful result batch.
Both the individual quotes and their aggregate retained bytes need admission.

Add `Reader.SearchQuoted` beside the existing separate Search/Quote methods. It
accepts the same prepared query and search limits, plus positive MaxQuoteBytes and
MaxTotalQuoteBytes. It searches once, then quotes every hit sequentially through
the existing strict Quote path. Only complete success returns a QuotedResult;
every error returns its zero value. Do not truncate matches, skip failed hits,
substitute a generation or change BM25 scores/order. A known-empty edition still
succeeds with no hits; an absent edition is rejected when opening the reader.

QuotedHit contains the existing temporary Reference, score and owned quote string.
QuotedResult owns its hit slice and carries the existing ranking-version label.
References retain the opened reader state and stable object routing, exactly like
ordinary results. They are neither persisted citations nor authorization. Separate
opens still reject each other's references; callers resolve metadata through the
same reader. No extra location/image copies or public wire format are introduced.

Check quote limits before Search, which performs its existing query admission.
For each complete quote, compare its byte length with remaining total capacity
before appending; subtraction avoids overflowing a summed size. This bounds
retained logical quote bytes, not whole-process memory. One candidate quote can
temporarily coexist with a full admitted batch before an aggregate rejection.
Existing reader/article limits bound the fetched/decoded article separately.
The hit count remains bounded by the complete-search MaxMatches contract.

Use the existing live-deadline context requirement and check cancellation before
returning success. Preserve wrapped underlying failures and return no partial hit
slice. No goroutine, cache, writer or background task is introduced. Caller-owned
concurrent admission and client/routing lifetimes remain unchanged.

## Cost and simplest alternative

For H matched passages, this adds H sequential strict article reads after opening,
even when several hits share an article. Transferred bytes and repeated decoding
follow those reads; this choice does not optimize them. Additional retained state
is O(H + Q), where Q is the sum of returned quote bytes; Search and one Quote also
have their existing scratch/decoded-body costs. Copying a result wrapper shares
its mutable hit slice, as ordinary Go slice ownership does.

The simplest alternative renders after each successful quote. That gives earlier
useful output and less buffering but leaves partial output on a later read failure.
Explicit per-hit failure states would preserve useful matches during outages, with
a more involved output/consumer contract. Revisit that policy for a concrete
degraded-availability requirement. Any future renderer must separately bound its
encoded output: quote-byte limits do not count metadata or JSON/terminal escaping.
A writer error or broken pipe can still leave partial bytes after rendering starts;
this is a data-verification boundary, not transactional standard output.

## Established examples and fit

Elasticsearch 8.19's [Search API](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/search-search.html)
explicitly permits partial results with `allow_partial_search_results=true` and
returns an error without partial results when false. Its failure unit is a shard
request, not our article read after a complete local ranking. It demonstrates an
explicit policy, not a reason to copy its distributed machinery or defaults.

ripgrep 14.1.1's [sequential search](https://github.com/BurntSushi/ripgrep/blob/4649aa9700619f94cf9c66876e9549d83420e16c/crates/core/main.rs#L100-L142)
reports a per-file error and continues. Its surrounding run selects error status
except the quiet-mode exception. Its independent local-file workload makes useful
partial output reasonable; emitted bytes cannot be retracted. We choose a complete
verified batch for the first bounded command because successful consumption is
simpler to interpret. Neither example measures our performance.

## Verification and remaining boundary

Exercise the real local PostgreSQL/HTTP published-reader fixture: ranking and exact
quotes match separate Search/Quote calls; failure on the second quote returns zero
after the first read succeeded; byte ceilings are inclusive and fail without
truncation; bad inputs/search overflow do no article I/O; cancellation and corrupt
articles return zero; old readers remain pinned; empty results need no article read.
Preserve evidence and inspect concurrent calls with independent result storage.

Connection configuration, public result/citation encoding and CLI installation
remain the next checkpoint. This slice supplies the accepted verification behavior
without deciding those contracts or modifying hosted schema, data or target state.
