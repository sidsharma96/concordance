# Form passages from authored structure

## Decision

The user chose structure-first passages: keep coherent authored paragraphs, notices,
lists, tables, figures and code intact, with heading/context references. A passage
is a candidate excerpt for retrieval, not a prescribed number of model tokens.
Its location belongs to the exact retained article and processing version.

Fixed overlapping word windows and whole heading sections remain comparison
baselines. Fixed windows are simpler but can cut code or notices and duplicate
ranked text. Sections preserve broader context but can mix unrelated facts and grow
large. Structure-first is the initial hypothesis, not a measured ranking winner.

## Keep evidence and context connected

The pinned Pod lifecycle notice gives a feature stage and default. Its body does
not contain `ContainerRestartRules`; that name occurs in the notice's label and
following prose. Preserve group labels and heading ancestry alongside body locations.
Keeping references for display does not by itself solve matching across passages;
which context contributes to retrieval scores still needs an explicit experiment.

```text
retained article
  ├─ passage body: exact blocks
  ├─ enclosing headings: references
  └─ group labels: references
```

Do not independently rank both a parent container and every child as default
copies of the same body. Do not confuse a heading's version text with the document's
edition. Required article citations and optional verified Git origins remain as
specified in [passage citations](passage-citations.md).

## First bounded implementation

The initial in-memory HTML passage builder partitions blocks without overlap,
keeps outer authored containers whole and associates leading headings/separators with
the following unit. It preserves group metadata and heading references without
copying the article's text into every passage. This is a format-specific grouping
operation, not a universal representation for code, PDF pages or provider blocks.

Bounds cover admitted blocks/groups/metadata, body bytes, passage count and repeated
context references/text. An oversized unit fails explicitly with no partial result;
no automatic splitting or omission is implied. These are caller-configured admission
limits, not a permanent product size ceiling. A later splitting protocol must keep
valid locations and authored structure or explicitly declare its supported limits.

The returned ranges require the same unchanged article. There is no persisted
article codec, generation manifest, ranking engine or publication in this slice.
Changing grouping rules requires a new processing version and later generation;
retained source remains available for comparison without fetching mutable content.

Revisit grouping and context scoring with version-specific questions, comparing
candidate coverage, relevance, duplicate results, bytes and latency against the
simple baselines. Current small structural sizes are observations, not production
upper bounds. Do not silently drop content to make a benchmark pass.
