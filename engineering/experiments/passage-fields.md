# Retained HTML fields for matching

`internal/htmlterms.Build` implements the accepted [field policy](../decisions/passage-fields.md).
It builds the canonical `htmlpassage.Plan` from the supplied article, then prepares
separate body, full ancestor heading and notice-label fields with `textterms.Prepare`.
The format-specific operation is outside the shared matcher. It supplies no score,
edition registry, query command, durable citation, encoded index or publication.

## Representation and ownership

`Prepared.Fields[i]` and `Prepared.Plan.Passages[i]` describe the same passage in
the exact unchanged caller-owned article. The plan owns ranges/reference slices;
the fields own their slices and normalized strings. No input article buffers are
retained by the result. The caller still keeps source/document, profile, edition,
revision, processing and generation associations and retains cited bytes separately.
Neither a position nor the source HTML hash proves those associations or access.

Projection has its own `htmlterms/v1` processor tag, distinct from the plan's
processor and term grammar version. Body text includes blocks' extracted text,
including authored code and image alt/caption text. It excludes image addresses and
interpretation. All referenced common ancestor headings are kept in their plan order;
leading headings already inside a body are not repeated as ancestor context. Only
nonempty labels enter the label field, without changing the body quotation. LF
separates successive pieces, preventing accidental concatenation into a new term.

## Admission and failure sequence

Caller-configured passage limits bound article admission and canonical plan metadata.
The adapter then measures every raw field and total view bytes, including separators
and repeated context, before joining text views. Per-field text limits bound bytes
and occurrences before the preparer allocates terms. Aggregate occurrences/bytes are
checked after each bounded field, so temporary output can hold the prior admitted
aggregate plus one field. A joined view, article and plan also occupy memory. These
are logical ceilings, not a hard peak-memory quota; strings/builders have overhead.

Oversized context fails instead of dropping ancestors or labels. Invalid structure,
unsupported input, lower-stage errors, excessive output or cancellation returns a
zero `Prepared`. Wrapped lower-stage errors retain `errors.Is` behavior without
including source contents in diagnostics. A deadline is required; checks cannot
preempt arbitrary work inside a string copy. No goroutines, I/O or cache start.

Beyond the existing builder's costs, projection work is linear in visited field
pieces, repeated raw view bytes, terms and normalized bytes. Retained extra output
grows with plan metadata, passage/field structs and terms. Shared/cached heading
terms might reduce repetition later, but would need explicit ownership and budget
rules; this slice deliberately prepares complete independent fields.

## Evidence and remaining work

Public synthetic tests cover ancestor-only subject terms, exclusion of sibling/later
headings, leading-heading duplication, separate labels, code/alt/caption coverage,
omitted URLs, LF boundaries and repeated occurrences. They connect the extracted
article and prepared fields to exact edition matching. An independent projection
oracle materializes raw pieces with `strings.Join` rather than reusing the adapter's
walker; text preparation has its own separate grammar oracle. Generated and bounded
fuzz cases compare projected terms and exact view-byte ceilings. Other tests exercise
aggregate/per-field limits, invalid input, output independence and cancellation,
including a final check after all fields have been built.

A private retained-corpus audit compares every public field and canonical plan with
the earlier explicit projection on the same 38 articles and 1,889 passages. Its
raw evidence and independent review records remain under ignored
`.tools/reviews/passage-fields-*`. This is a composition check, not reader relevance,
latency, allocation or encoded-size evidence. Ranking and a generation/citation
registry still need their own contracts; no searchable generation was published.
