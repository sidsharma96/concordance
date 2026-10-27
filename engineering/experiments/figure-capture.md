# Exact-revision figure acquisition

The accepted [media scope](../decisions/media-coverage.md) keeps original figures
and searches authored text first. This experiment adds local acquisition, not
interpretation, durable retention, indexing or publication.

## Operation and boundary

`internal/gitsource.CaptureFiles` selects explicit unique repository-relative paths
at a full lowercase commit ID. Unlike `Capture`, it has no Markdown suffix filter.
It validates commit/tree identity, bounded literal tree metadata, exact requested
path membership, regular-file modes and all advertised sizes before blob reads.
Each body must match its Git blob ID and separately receives a raw-byte SHA-256.
Symlinks, trees, gitlinks and LFS pointers fail. Any error returns a zero snapshot,
even if earlier blobs were already read. Returned buffers belong to the caller.
A context deadline is required; every started Git child is canceled and waited on.

The existing Git wrapper disables inherited Git environment, system/global config,
replacement refs and lazy fetch. The caller still owns a trusted, stable local
repository and must authorize access. Application output bounds do not sandbox
Git's internal memory, decompression or filesystem work.

`tools/articleaudit` optionally accepts paired `-repository` and `-commit` flags.
Its Kubernetes-specific mapping supports clean root-relative `/images/...` addresses
as `static/images/...`. It explicitly rejects other roots, page-relative addresses,
external/data URLs, query/fragment suffixes, percent/backslash spellings, control
characters and dot segments. Supporting another source or Hugo bundle requires a
deliberate resolver extension, not a change to extraction or ranking.

The supplied HTML must be associated with that commit by the caller's preparation
process. The audit does not independently prove how a local HTML file was rendered.
It captures the directly referenced original bytes; it does not recursively inspect
assets for further dependencies or establish that arbitrary SVG is self-contained.
Retained SVG is not safe trusted HTML. No viewer or execution of assets is added.

The experimental ceilings are 256 image occurrences per article before deduplication,
128 unique paths, 256 KiB listing output, 4 MiB per file and 16 MiB total file bytes.
The tool's deadline covers extraction and Git capture. Duplicate paths are captured
once in an operation while the article keeps every original block association.
These ceilings are not product limits or a generation-wide budget. The future
importer must bound aggregate references, captures and retained bytes as well.

For F selected paths, P path bytes, L listing bytes and B captured bytes, application
validation/sorting costs O(P + F log F) bounded-length comparisons and retained data
grows with P + L + B. Git's tree lookup and object decompression work are separate.
Each nonempty capture starts three metadata commands and F blob commands. Distinct
paths sharing one blob are initially read separately; no cross-operation cache exists.
A recursive whole-repository scan would do unrelated work for these six references.

## Reference observations

Run the tool from the checkout against retained local HTML and its exact recorded
source commit, for example:

```sh
go run ./tools/articleaudit \
  -file .tools/corpora/render-1.36/selected/docs/concepts/workloads/pods/pod-lifecycle/index.html \
  -repository .tools/corpora/kubernetes-website \
  -commit 1de955ebabe7e17da1ebb4f582635491227f4157
```

The inputs require the private offline reference preparation described in
[source preparation](../decisions/source-preparation.md); they are not checked-in
fixtures or prerequisites for ordinary tests. The report prints metadata only,
`image_capture local_only`, `image_content_interpreted false` and `published false`.
A missing required asset produces an error and no successful partial report.

The 38 retained pages across the three pins passed the optional capture:

| Edition | Articles | Image occurrences | Files captured across per-article calls |
| --- | ---: | ---: | ---: |
| 1.34 | 10 | 2 | 2 |
| 1.35 | 13 | 2 | 2 |
| 1.36 | 15 | 2 | 2 |

All six occurrences resolve to `static/images/docs/pod.svg` at their respective
exact commits. Each captured body is 62,158 bytes with SHA-256
`67d68432546521f097ddfd10ce762b079c6cef64c18dee78d11d72a0e0c9a0f0`.
The same source path is read again for a separate article call; these counts are
not unique immutable object counts or measured storage savings. A read-only XML
inspection found embedded data references rather than external dependencies in this
particular SVG; that observation does not generalize to arbitrary assets.
Raw reports remain private under `.tools/corpora/figure-capture-results.txt`.
No figure was uploaded to R2 by this experiment.

Synthetic repositories test SHA-1/SHA-256 revisions, binary/literal-path capture,
working-copy and replacement-ref independence, caller ownership, missing files,
unsupported modes/LFS, inclusive and excessive byte/count/listing limits, required
deadlines and cancellation. Tool tests check unsupported address forms, occurrence
limits before deduplication, separate image blocks and failure without a report.
These executions do not establish adversarial repository isolation, complete source
coverage, safe rendering or retrieval quality.
