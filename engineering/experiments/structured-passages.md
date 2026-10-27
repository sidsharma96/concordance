# Structure-first passage ranges

The user chose [authored structure](../decisions/passage-grouping.md) over fixed
word windows or whole sections as the initial passage direction. The concrete
`internal/htmlpassage` builder consumes a retained `htmlarticle.Article`. It does
not rank, tokenize, persist, publish or interpret figures.

## Representation and invariant

Each body is a nonempty half-open block range. Bodies are ordered, contiguous,
disjoint and exhaustive. Outer nonempty groups stay whole; nested groups become
metadata references in the same passage. Standalone headings/separators join the
next unit; an unmatched suffix becomes the final body. Group references preserve
labels as metadata separately from verbatim source text. Empty groups at a body
boundary belong to the next body, except end-of-article groups belong to the final
body. Every group has one owner.

Heading references are outside the body and remain in scope throughout it. A
heading inside the body can supersede an old peer; the old peer is then removed
from context rather than accidentally describing the new section. A six-level
heading stack and an independent slower test oracle exercise this boundary.

The returned plan owns ranges/reference slices, not article strings. The caller
must keep the exact article unchanged and available for every use of its indices.
These indices alone are not an article identity, citation API or authorization
check. `htmlpassage/v1` versions the grouping rule; persistent formats remain open.

Validation rejects unsupported processing versions/kinds, invalid UTF-8/NUL strings,
invalid block metadata, unordered/crossing/out-of-bounds group intervals and invalid
kind-specific flags. It does not prove full HTML grammar or rendering provenance.
Count and logical string-byte admission precede per-block/group allocation. Logical
input bytes include enum strings, processor/profile, text, anchors, URLs and labels;
they do not measure distinct heap allocations. Body bytes count `Text` with one LF
between blocks. Context bytes count referenced heading text and group labels,
repeated per reference. References and passages also have total count ceilings.
A required context deadline is checked cooperatively; no goroutines are started.
Any failure returns a zero plan, including failure after earlier bodies were built.
An oversized unit fails explicitly rather than splitting or dropping a suffix.

## Cost and simpler alternatives

For K blocks, G groups, T logical input string bytes, P passages and R returned
references, the builder's own work is O(K + G + T + R), including validation.
Metadata grows with K + G + P + R; existing article strings remain caller-owned.
The group-end stack validates nesting and selects outer units. Each block/group is
visited a constant number of times, and heading depth is at most six. No remote
requests, object writes or text copies are made by the builder.

Per-block ranges would be simpler but could detach table cells or notice paragraphs
from their container. Whole sections retain more context while combining unrelated
facts. Fixed windows provide regular sizes but need exact source mapping and can
duplicate overlapped text. None is a measured relevance winner here. The retained
article supports comparisons in a later processing version without a live refetch.

## Reference run

`tools/articleaudit -passages` optionally builds this plan after extraction, before
printing a metadata-only report. The experimental defaults are 20,000 blocks,
40,000 groups, 8 MiB logical input strings, 20,000 passages, 8 KiB joined body bytes,
200,000 references and 8 MiB repeated context text. `-max-passage-bytes` changes the
body ceiling and requires `-passages`. They are audit admission settings, not
permanent product limits or a generation-wide resource policy. Missing or excessive
input produces an error with no successful partial report. Existing asset capture
is independent; all requested local operations must succeed before the report.

The retained 38-page Kubernetes reference passed these settings:

| Edition | Pages | Passage bodies | Joined body bytes | Largest body | References | Context bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1.34 | 10 | 504 | 125,942 | 3,393 | 2,122 | 21,939 |
| 1.35 | 13 | 656 | 162,677 | 3,393 | 2,805 | 30,907 |
| 1.36 | 15 | 729 | 181,178 | 3,393 | 3,060 | 35,262 |

Totals: 1,889 bodies, 469,797 joined body bytes, 7,987 references and 88,108 logical
context bytes. Each block/group was checked to occur in exactly one body/owner;
all 149 code blocks remain inside whole bodies. Of the references, 3,508 describe
groups and 4,479 describe headings. These are in-memory counts, not encoded index
sizes or stored duplicate strings. No searchable generation was published.

The feature-gate notice's body lacks `ContainerRestartRules` in all three lifecycle
articles. Its retained notice label contains the name; the following prose does too.
The builder preserves the label's group reference, but has not decided whether it
contributes to matching or scoring. Displaying context cannot establish retrieval.
The corpus pins and extraction profile remain those in the existing
[HTML experiment](html-extraction.md).

A small warm microbenchmark used the extracted 1.35 lifecycle article, prepared
outside timing, on Apple M1/macOS arm64 with Go 1.27.2. Three 100-iteration samples
measured 68,184 / 57,097 / 66,427 ns per build, with 49,976 allocated bytes and
350 allocations per build. These allocations are not retained heap or peak memory;
the observation excludes parsing, file reads, storage, indexing and queries. It
neither establishes a production capacity nor justifies optimizing the baseline.
Raw reports/probe sources remain ignored under `.tools/corpora/structured-passages-*`
and `.tools/probes/structured-passages/`.

Tests compare scope/coverage against a slower oracle, preserve nested containers and
empty-group boundaries, check independent metadata ownership and exact/excessive
budgets, reject malformed metadata and exercise cancellation. Fuzzing generates
bounded nested structures and checks the same invariants. Revisit with useful
version-specific queries and a context-scoring experiment; large-unit splitting
requires its own supported-location and complete-coverage protocol.
