# Import a trusted prepared capture

Status: accepted boundary, option A. Concordance consumes prepared HTML; it does
not yet own a website build runner. This extends the offline-only
[source-preparation experiment](source-preparation.md). The implementation must
be distinguished from this accepted contract.

## One handoff, two possible producers

For the first Kubernetes corpus, a trusted operator or controlled build job supplies
HTML mapped to exact source paths, the full Git commit and directory selection,
preparer/toolchain identities, retained recipe bytes and explicit Git dependency
paths. The recipe must record the producer and preparation policy, including
generated-output exclusions such as print duplicates. Recipe bytes are retained
input, never executable commands. This is an internal trusted handoff, not an
untrusted upload endpoint or a proof that a particular build ran.

Concordance independently captures every Markdown document in the selected Git
directory at that commit. Each document must have exactly one supplied HTML page;
missing, extra or duplicate associations fail the whole operation. Generated
output is selected by the preparer under its recorded policy, not silently skipped
by the importer. Supplied page order is immaterial; logical slots are ordered by
source ID and full Git path within each edition, preserving stable ranking ties.

The importer captures declared dependencies and supported original image files at
the same Git pin, extracts retained articles, builds edition indexes and constructs
the generation manifest itself. It retains all required input and derived objects
before guarded catalog publication. A missing dependency, unsupported image address,
processing failure or exceeded admission budget fails the whole import. There is
no successful-subset publication or physical deletion.

The trusted preparer asserts HTML-to-source provenance and rendering-dependency
completeness. Exact membership checks and content hashes cannot prove those
assertions. Toolchain identifiers and retained recipes describe a build; they do
not constitute independently verified execution attestations. Local repository
paths are routing configuration, not logical source identity or retry input.

Any future Concordance-owned runner (option B) must produce this same handoff.
It may improve automation and control of build execution without changing the
importer's processing/publication path. A runner must add its own pinned-input,
isolation, resource, cancellation and child-cleanup contract before implementation.

## Why start with the handoff

As inspected in October 2026, Pagefind runs after a static-site generator and reads
the resulting HTML directory. Its pinned
[running instructions](https://github.com/CloudCannon/pagefind/blob/0b57df879ea1d5986906238cf919cdc508b7b96b/docs/content/docs/running-pagefind.md)
and [`read_file_sync`](https://github.com/CloudCannon/pagefind/blob/0b57df879ea1d5986906238cf919cdc508b7b96b/pagefind/src/fossick/mod.rs)
demonstrate that boundary for generated websites. They do not establish Git
provenance, complete-snapshot admission or transactional publication. Some of that
reader's parse failures log and skip a file; Concordance must fail its complete
selected snapshot instead.

Our [Kubernetes render experiment](../experiments/kubernetes-rendering.md) produced
all 38 selected article-version entries, but encountered an unavailable older
image, an incompletely locked reconstruction and an output-cap failure. Those are
concrete build-management costs. We have no measured builder image footprint,
rendering-latency benchmark or peak-memory profile.

Both A and B need website build tools somewhere. With an external builder, neither
requires linking Hugo or Node into the Go binary. A requires a separate preparation
step; B offers a single workflow but makes Concordance responsible for execution
and toolchain recovery. We choose A to complete import processing and publication
before taking on that responsibility, rather than because A removes dependencies.

## Cost, failure and revisit

The initial composition is synchronous, sequential and bounded in RAM. It borrows
supplied HTML/recipe buffers unchanged until return, owns acquired Git and generated
buffers, and holds no caller buffers afterward. One nonwaiting caller per instance
limits local overlap, not distributed writers or whole-process memory. Component
limits and aggregate input/artifact/count limits must apply together.

Membership uses maps after count admission; expected work is linear in paths,
plus deterministic ordering. Acquisition, extraction and indexing additionally
depend on bytes, structure, terms and postings. Deduplicating object bytes does
not eliminate logical document slots, processing or reference metadata. No new
performance or million-document capacity claim follows from this boundary.

All local preparation must finish before saved-key binding or remote writes. Once
publication starts, its [existing uncertainty rules](import-completion.md) apply:
errors can leave retained objects or a committed publication. A completed retry
uses the same captured inputs and rules to recover its original result, with no
S3 access after local preflight. A new intended input needs a new saved key.
Regenerating a capture after a failure is not guaranteed to reproduce its bytes;
the operator must retain the original handoff to make that retry possible.

Revisit a managed runner when automatic repository-to-generation operation becomes
the immediate requirement or manual preparation is a measured operational burden.
Revisit RAM staging when measured pressure and streaming consumers justify it.
Source registration, a durable capture journal, public import API, CLI packaging,
runner execution and automatic resume remain separate work.
