# Supply the full ancestor heading context

## Decision

The user chose all ancestor headings for the first retained-HTML field adapter,
rather than only the nearest heading. A short paragraph can name a default while
its ancestors name the product and feature. Preserving that context lets an
explicit multi-term query qualify without changing the paragraph's quotation.

This continues the accepted separate body/heading/label fields and match-all rule;
it does not choose ranking weights. Broad headings may admit more generic matches.
Nearest-heading-only context is narrower but can discard useful answers before
ranking. No comparison establishes a relevance winner. Revisit with new questions,
independent reader judgments, added/missed candidates and processing/memory cost.

## Retained HTML projection

The first concrete adapter builds the existing structure-first plan from the exact
unchanged article, then prepares three separate fields for every planned passage:

- Body: block `Text` within the body range, including authored code, image alt text
  and captions present as blocks. Join successive blocks with one LF character.
- Headings: every referenced ancestor heading that remains in scope throughout
  the body, in the plan's order, joined with LF. Leading headings already in the
  body are not added again as referenced context. Do not use unrelated headings.
- Labels: every nonempty notice label in the passage's group references, in plan
  order, joined with LF. Labels remain metadata, not verbatim body quotations.

The adapter does not add image URLs, inferred figure descriptions, whole-document
text or guessed Git lines. Distinct authored repeated terms remain occurrences;
no component-word expansion or deduplication is introduced. Each field uses the
same `textterms.Prepare` grammar/version as queries. A projection processor tag
versions field coverage separately from term preparation and passage grouping.

The output pairs each prepared field set with the same-position plan entry.
The caller retains the exact article and its source/document, edition, revision,
profile and generation associations. Output positions and hashes do not establish
identity, authorization, durable citations or publication. A content hash cannot
prove that caller-supplied article metadata is authentic.

## Bounded first adapter

Build the plan internally rather than accepting arbitrary caller-supplied ranges.
The existing passage builder validates article structure and bounds plan metadata.
Then count all projected field bytes, including LF separators and repeated context,
before allocating joined text views. Per-field preparation limits bound its raw
input, raw/lowercase term bytes, occurrences and total output. Aggregate term count
and bytes are checked after each bounded field preparation: temporary output can
include the prior admitted output plus one bounded field. These are logical bounds,
not an exact heap quota or streaming parser.

Any invalid input, unsupported structure, excessive view/output or cancellation
returns no partial prepared article. There is no silent dropping of headings or
oversized bodies. The result owns its fields, strings and plan metadata; it does not
retain the input article buffers. Its plan indices still require the exact article
for interpretation. No goroutines, cache, storage access, score or query command
is introduced. Format-specific projection stays outside the shared matcher so a
future PDF/provider/code processor need not adopt HTML's block representation.
