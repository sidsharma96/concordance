# Verify only the requested quote batch

The accepted [requested-batch contract](../decisions/requested-quote-batches.md)
now has a local reader/CLI implementation. Matching, statistics, complete ranking
and candidate overflow remain unchanged. SearchQuotedFirst selects a positive
prefix; SearchQuoted retains complete output. Both use one verification loop and
return TotalMatches from the same complete Search that supplied the selected hits.

## Synthetic end-to-end observations

The actual local producer/catalog/SDK/reader fixture publishes twelve logical
documents sharing retained article bytes. Every quote is still a separate strict
read. Native PostgreSQL 14.17 and a controlled HTTP object peer are used, with no
R2 access, hosted schema change, corpus retuning or timing measurement.

Selected lines from the real targeted test log, excluding source prefixes/timings:

    default: shown=10, indexed matches=12, GETs=12, cleanup=1
    explicit: shown=1, indexed matches=12, GETs=3, cleanup=1
    larger than count: shown=12, indexed matches=12, GETs=14, cleanup=1
    all: shown=12, indexed matches=12, GETs=14, cleanup=1
    disabled all: shown=1, indexed matches=12, GETs=3, cleanup=1

Each fresh command reads one manifest and one index before the selected articles.
This measures operations in this fixture, not provider latency or transfer bytes.
A library-only test after opening observes one GET for a one-hit batch with two
indexed matches, preserving the original generation/reference/score/quote even
after another generation publishes.

## Failure and ownership boundaries

A forced second-GET failure is not reached with limit one; requesting two hits
returns a zero result after the first succeeded. Cancellation at the second
selected read also discards the prefix. Per-quote and aggregate byte caps apply
only to selected quotes; exact inclusive caps succeed and a one-byte shortage
returns zero rather than truncating. The result still reports all indexed matches
only on success. A match ceiling of one rejects two matches before any article
read even when the requested quote count is one.

The command rejects zero/negative limits and enabled --all with an explicit
--limit before connecting. An initial test caught the Visit callback failing to
record explicit --limit; fixing that callback restores the intended early refusal.
--all=false permits an explicit limit. Empty selections, large limits above the
match count, independent mutable result slices, pinned handles, safe text,
connection cleanup and short writes retain their existing contracts.

Primary ignored records are .tools/reviews/result-batch/targeted-1.log,
first-admission-failure.txt and verify-1.log. The toolchain is Go 1.27.2 on
darwin/arm64 with the dedicated local PostgreSQL server. Ordinary builds/checks
remain independent of private teaching notes and corpus material. Final independent
code and architecture approvals must match the staged source and evidence before commit.
Private teaching prose is checked separately; no book deployment follows.

For H admitted matches and requested limit L, article reads are min(H,L), plus
fresh manifest/index opening. Full ranking, decoded-index memory, per-article
bytes and formatted output remain separate costs. No page cursor, caching,
optimized top-k, result-wire schema or broader production matcher is selected.
