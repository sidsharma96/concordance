# Bounded extraction from the retained HTML

`internal/htmlarticle` now extracts owned text and structure from one captured HTML
article. `tools/articleaudit` is its concrete consumer for the Kubernetes offline
reference. This is not an importer, production renderer, persisted article codec,
citation API or retrieval engine. No corpus objects were uploaded or published.

## Operation and representation

`Extract` borrows UTF-8 HTML until return. The caller supplies an explicit selection
profile, positive budgets and a context deadline. Input byte/token admission precedes
HTML5 parsing. A full-tree traversal checks node/depth budgets and requires exactly
one main container. Failure returns no partial article. Callers own authorization,
admission, stable input and returned data; there are no added goroutines or fetches.

The result keeps processor/profile identity and raw HTML SHA-256/length separate
from ordered blocks and container groups. The raw hash is not a derived article ID,
logical document ID or authorization check. Groups preserve half-open block intervals
for paragraphs, notices, nested lists/items, definition lists, tables/rows/cells,
figures/captions and quotations. Header cells and ordered lists remain distinguishable.
Blocks retain headings, prose, code, image references and separators. These Go types
are an in-memory representation, not a settled persisted schema.

Prose whitespace is folded without inserting spaces at every inline tag boundary.
Code retains decoded HTML5 text, indentation and line breaks; HTML parsing itself
normalizes newlines and may remove the first newline in a preformatted element.
Links retain their visible text; the captured HTML retains destinations. Anchors
are navigation hints, not exact source spans. This processor supplies no Git maps.

The reference profile selects `td-content`, omits `article-meta`, `copy-code-icon`,
`icon-copycode`, `section-index`, `td-heading-self-link` and `pre-footer`, and recognizes
the checked feature/alert/pageinfo/callout classes as notice containers. The profile
belongs to this consumer; ranking and the extractor do not contain Kubernetes names.
Another class/profile is exercised by synthetic tests, not a dynamic plugin registry.

Unknown semantic elements, foreign SVG/MathML, nondefault list numbering and merged
table cells fail explicitly. Images preserve alt text and source references; image
bytes are not fetched and depicted content is not interpreted. Known copy icons are
omitted by the profile. CSS visibility and JavaScript behavior are not evaluated.
These limits must remain visible before any complete import can be claimed.

## Parser and resource bounds

The official `golang.org/x/net/html` package is pinned at v0.61.0, source revision
`c1d31df183e51b1075a2fd670d6f6f4efb871773`. Its
[package documentation](https://github.com/golang/net/blob/c1d31df183e51b1075a2fd670d6f6f4efb871773/html/doc.go)
distinguishes tokenization from HTML5 tree construction. A tokenizer alone does not
give the browser's repaired tree; the tests exercise implicit paragraph closure and
table foster parenting. This is infrastructure reuse, not a copied parser.

The [tokenizer buffer behavior](https://github.com/golang/net/blob/c1d31df183e51b1075a2fd670d6f6f4efb871773/html/token.go)
requires a one-byte probe for inclusive token ceilings. Its admission token stream
is not a precise bound on the parser's allocations or every tree-construction step.
The [parser](https://github.com/golang/net/blob/c1d31df183e51b1075a2fd670d6f6f4efb871773/html/parse.go)
also rejects nesting beyond 512 elements. Our node/depth traversal checks happen
after parsing; they are not preallocation heap quotas. Context checks do not preempt
parser CPU between reader calls. This is a bounded admitted-input experiment, not
a hostile-content sandbox or exact memory/deadline guarantee.

Input bytes, token bytes/count, traversed nodes/depth, block/group counts, individual
block text and total retained string bytes have separate positive ceilings. The
output byte budget includes text, anchors, URLs, labels and processor/profile names;
it excludes shared enum strings, slice capacity and struct overhead. Output strings
do not retain the input buffer or DOM. A failed parser's diagnostic is not exposed
as captured document text.

The audit defaults are ceilings for this experiment: 4 MiB input, 128 KiB per token,
200,000 tokens/nodes, depth 512, 20,000 blocks, 40,000 groups, 1 MiB block text and
8 MiB output strings, with a 15-second context. They are not measured product limits.
It reads one trusted stable regular file, rejects a final symlink and a known `_print`
aggregate, checks read/close errors and emits stats only after successful extraction.
Regular-file I/O is not context-preempted or sandboxed against hostile replacement.

Example with an already acquired retained article:

```sh
go run ./tools/articleaudit -file /path/to/retained/pod-lifecycle/index.html
```

## Observed reference workload

Inputs are the 38 mapped article HTML files from the
[render experiment](kubernetes-rendering.md), not live pages or a fresh build.
Each input retains its own hash. Print aggregates remain excluded. One successful
full extraction pass produced:

| Edition | Articles | HTML bytes | Block text bytes | Blocks | Groups | Code blocks | Image references |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1.34 | 10 | 5,252,154 | 125,482 | 964 | 956 | 33 | 2 |
| 1.35 | 13 | 7,159,284 | 162,068 | 1,265 | 1,224 | 55 | 2 |
| 1.36 | 15 | 8,281,744 | 180,517 | 1,390 | 1,328 | 61 | 2 |
| Total | 38 | 20,693,182 | 468,067 | 3,619 | 3,508 | 149 | 6 |

Block text excludes structural/identity metadata and retained source/HTML. These
counts are not a compression ratio, persisted artifact size, completeness score or
retrieval benchmark. Image references are occurrences, not distinct assets. Both
the Pods overview and lifecycle article reference a diagram in each edition.

Assertions inside extracted notice groups verified `ContainerRestartRules` defaults
across all three editions and `RestartAllContainersOnContainerExits` in 1.35/1.36.
The extracted included restart-all YAML matched the earlier normalized source hash
after trimming edge newlines only. Feedback prompts did not appear in any output.
This checks selected semantic content and filtering, not every factual assertion or
complete visual meaning. Raw per-article results remain ignored local evidence.

## Allocation observation and alternatives

An unprofiled Go benchmark on Apple M1, macOS arm64, Go 1.27.2 processed the largest
selected article: the retained 1.35 lifecycle HTML, 638,699 bytes, hash
`c0afdf4d7a9dfe0aa0b9930a559230d6893751027f9576b3317889c107825335`.
Input was read before timing; each call performed admission, parse, selection and
extraction, and retained its final article. Three samples of five iterations observed
10.70–10.75 ms/op, 4,131,684–4,131,716 B/op and 107,494 allocations/op. These are
allocated bytes over a call, not retained heap, peak RSS, cold-start or system capacity.
No network, object-store, indexing or query work is measured.

Let H be input bytes, N DOM nodes and O returned string/structure size. Our admission
scan and traversals cost O(H + N + O) with bounded profile rules; the HTML5 parser's
worst-case work is separate and is not claimed to be linear. Space includes the
borrowed input, parser/DOM allocations, output and a depth-bounded traversal stack.
Process one page at a time rather than retaining all DOMs. The existing Git capture's
aggregate source memory is a separate component and has not been optimized here.

Stripping every tag is simpler but includes page chrome and loses code/table/notice
boundaries. A tokenizer-only stream can retain less state but does not construct the
HTML5 tree. The measured DOM baseline is the initial oracle, not proof streaming is
unnecessary. Revisit it if larger pages exceed admission budgets or measured parser
allocation/latency dominates; do not infer scale from company headcount.

Synthetic tests cover ownership, inline spacing, code, nested intervals, main-container
selection, unsupported structures, inclusive/admission/output bounds, invalid input,
cancellation and CLI file/report boundaries. A short two-worker fuzz run exercised
184,565 inputs without a failure; this is not proof about all malformed HTML.
The full local verification includes PostgreSQL and uncached race tests. GitHub
Actions stays disabled. Independent implementation review is separate from the
earlier approved design plan and from private book review.

The accepted [media scope](../decisions/media-coverage.md) retains original figures
and searches authored text first. Visual contents remain uninterpreted. The
Kubernetes audit tool can now optionally capture these references from an exact
local Git commit, as described in the [asset experiment](figure-capture.md).
Next decide passage grouping and the concrete article/citation encoding. A complete
searchable generation must declare and satisfy its coverage.
