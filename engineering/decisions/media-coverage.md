# Retain figures; search authored text first

## Decision

The first searchable generation includes authored text and retained original figures.
Search can use alt text, captions and surrounding prose. It does not interpret words,
arrows or relationships depicted only inside an image. Declare this capability scope
per document and collection; do not describe the figures as understood.

The user selected this option and deferred optional interpretation of highly
informative images. That later capability needs a new processing version and
published generation, with derived descriptions labeled separately from source
quotes and with input, method/model and evaluation provenance. No interpretation
provider or model has been selected or authorized.

## One example

The Pods overview and lifecycle pages reference `/images/docs/pod.svg` in all three
pinned editions. The six occurrences have separate document/version associations.
The exact Git objects currently contain identical 62,158-byte figures. Their bytes
may share one immutable object within an authorized collection; their identities
and citation relationships remain distinct.

```text
retained article ── authored text ──> searchable passages
       │
       └── figure reference ──> exact original bytes
                                  inspectable evidence
                                  contents not interpreted
```

## Capture and publication

Source preparation resolves required assets against the captured revision with
explicit source-specific rules and limits. Unsupported addresses, missing assets,
corruption and excessive input fail explicitly. Never fetch a mutable live URL as
an unnoticed substitute. Keep the original address, exact revision, captured path
and verified content reference distinct.

The initial Kubernetes profile maps root-relative `/images/...` addresses to
`static/images/...` in the pinned repository. Other address forms require a deliberate
mapping extension; this is not a universal scraper. Bound references before dedup,
unique files, metadata, individual bytes, total bytes and time. The importer must
also bound these across its whole generation, not only one article.

Required assets join the other immutable objects confirmed before the PostgreSQL
publication transaction. Coverage and processing versions belong in the future
versioned input fingerprint and manifest. Those schemas and the complete importer
are not implemented. Local capture is not remote retention or publication, and
archiving figures alone does not meet the searchable-generation success contract.

Original SVG can contain active content. Retention does not authorize embedding it
as trusted same-origin HTML. A future viewer needs a safe download or isolated
display boundary; no viewer is implemented by this decision.

## Trade-off and revisit condition

This scope preserves visual evidence while keeping the first retrieval work focused
on version-correct text and storage recovery. It can miss questions whose answers
appear only in a diagram. The query engine cannot automatically know every such
question; a coverage declaration is not a promise to detect or refuse them.

Interpretation could make those questions useful, especially for diagram-heavy
documentation, slides or PDFs. It also adds errors about relationships, evaluation,
latency, provider failures and cost. OCR or extracting SVG labels alone does not
establish diagram understanding. Revisit with concrete useful questions that authored
text cannot answer and judge derived answers against their retained originals.
