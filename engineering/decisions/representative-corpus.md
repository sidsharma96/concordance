# First representative corpus

Status: accepted direction. Begin with release-aware support and platform questions
over a selected Kubernetes documentation area across three releases. Expand to full
English documentation, more releases and then other projects. This selects an
evaluation workload, not a completed connector, processor or retrieval capability.

## Why this workload

A platform engineer needs the instructions applicable to the software they run.
Similar words in another edition are insufficient. Public Git Markdown gives us
inspectable revisions, reproducible questions and no dependency on private company
documents. Start with `content/en/docs/concepts/workloads/pods` in
`https://github.com/kubernetes/website`. It covers lifecycle, restarts, sidecars and
other operational questions with observable differences between editions.

Release branches 1.34, 1.35 and 1.36 were resolved once to the exact commits recorded
in [kubernetes.pins](../corpora/kubernetes.pins). These are documentation snapshots,
not release-launch snapshots, patch-version binaries or observations of a cluster's
actual configuration. Keep the product-version label and exact Git revision separate.
Do not silently replace a pinned revision with a newer branch tip.

## Growth and evidence

The [corpus inventory](../experiments/kubernetes-corpus.md) records bounded capture
of 38 Markdown files across the initial three snapshots and 4,773 across their full
English documentation directories. Those larger directories include hidden data
pages; file counts are not rendered article counts. Neither is a benchmark for a
100-person or 10,000-person company, a growth forecast or a measured search workload.

Grow the experiment along separate axes: selected documents, retained editions,
published history, independent collections, update volume and concurrent queries.
Replicating a public corpus can stress counts but cannot establish real relevance,
access-control behavior or content diversity. Company headcount does not determine
manifest size, traffic or collection boundaries.

Synthetic fixtures remain the oracle for malformed inputs, faults and exact invariants.
Real documentation supplies realistic source structure and candidate relevance questions.
Do not copy the downloaded corpus into tests or source history; preserve upstream
attribution and the pinned license reference with any later redistributed material.

## Limits and alternatives

Kubernetes source Markdown uses Hugo shortcodes, frontmatter, external code examples
and feature-gate data. All initial selected files contain shortcode markers. The
current capture retains raw Markdown only; the inventory does not render these
constructs or establish complete passages. Do not strip unsupported syntax and claim
complete indexing. Dependency capture and processing semantics require a separate
choice before searchable publication. The full English tree also contains Markdown
files configured not to render as articles.

Synthetic-only data would be simpler but would miss these dependencies. A private
company knowledge corpus would introduce source permission, connector and reproducibility
work before we can evaluate release correctness. Kubernetes is a first public workload,
not evidence that private PDFs, provider documents or enterprise permissions work.

Revisit the corpus if its questions fail to distinguish release applicability, or if
the targeted product workload cannot be represented by these documentation sources.
The subsequent [manifest decision](manifest-layout.md) selects one bounded manifest
per generation. Complete content representation, schema and input fingerprint remain
open; corpus selection and raw inventory do not establish those contracts.
