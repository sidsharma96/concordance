# Cite retained articles, with optional source locations

Status: accepted direction. The user chose to keep original Git file/line locations
optional. A bounded in-memory HTML structure processor now exists; the persisted
article format and citation API are not implemented. The sketches below illustrate
the choice rather than fix a schema.

## Required quote, optional origins

A citation for rendered content must resolve the quoted passage in the exact retained
article for its published generation. Retain the captured HTML and a versioned derived
article containing its extracted structure. Bind a location to that immutable article;
block positions cannot be carried over to a different article or processing version.
An upstream URL or heading anchor can aid navigation but cannot replace the retained
quote authority. Resolve references within the authorized collection and generation;
a content hash alone is neither document identity nor authorization.

Original Git locations are optional additional metadata. Include an exact captured
revision, file and line range only when a verified mapping exists. A template-expanded
notice may combine Markdown, feature data and included examples, so its origins can
be a list rather than one parent-file line number. Contributing inputs and the exact
source of quoted words have different meanings; define that distinction before
populating mappings. Do not guess the parent Markdown lines for generated text.
When mappings are supplied, validate them against the captured inputs and reject
invalid metadata. Absence is valid and must be shown as unavailable, not fabricated.

Illustrative Go shape; named reference/span types and storage encoding remain open:

```go
type RenderedCitation struct {
    Article    ArticleRef
    Blocks     BlockRange
    GitOrigins []GitSpan
}
```

The slice can be empty when no verified source mapping is available. A saved-article
location remains required. Source repository/path/commit provenance is retained
separately even when precise line mappings are unavailable. Other processors can
retain Markdown spans, provider blocks, PDF pages or code symbols; this does not
make Git fields mandatory in a universal citation interface.

## Why this starting point

The bounded Kubernetes reference rendered 38 selected articles across three exact
snapshots and checked notices and included YAML. A displayed notice does not have
one trustworthy parent Markdown line range. Requiring complete original-source maps
would add renderer-specific instrumentation or source-aware expansion before the
first retrieval experiment. These remain credible options for edit/blame navigation.

Retained-article citations provide a verifiable quote while allowing those maps to
be added later. Costs include retaining HTML and derived structure and building a
safe quote reader. The selected article HTML totals about 20.7 MB; derived article
size and mapping costs are not measured. With H retained HTML bytes, T extracted text,
K blocks and S origin spans, retained content/metadata grows with H + T + K + S,
in addition to raw sources and search artifacts. This is a representation cost,
not an implementation memory bound or performance result.

Missing, corrupt or unauthorized retained content fails explicitly; do not substitute
current live text or guessed locations. All required artifacts must be confirmed
before catalog publication. No automatic deletion or complete citation recovery is
implemented. Displaying a quote does not authorize executing retained HTML.

Revisit mandatory original-source mapping if exact edit/blame navigation becomes a
primary workflow. Bounded structure extraction now has a concrete
[experiment](../experiments/html-extraction.md). Next define visual coverage, the
article/location format and passage grouping through their own workload and review.
The complete input fingerprint and generation manifest schema remain open.
