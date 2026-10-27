# First Git capture experiment

Status: implemented internal component under the accepted Git/Markdown-first direction
in `internal/gitsource`.
The internal component reads an existing trusted local repository at an explicitly
supplied full commit object ID. Remote fetching/authentication, reference resolution,
Markdown processing, canonical retry fingerprints and object publication are separate
work. The CLI still does not import documents.

## Operations and representation

Use the installed Git executable for object decoding rather than implementing Git
pack files or adding a Go Git dependency. This extends the existing contributor Git
requirement. Read commit/tree/blob objects, never checked-out files. Disable inherited
Git environment overrides, replacement objects and lazy fetching. The local clone
and its configuration are trusted, not a sandbox for arbitrary repositories. No
checkout, fetch, filters, hooks or repository writes are part of the operation.

Capture takes a repository path, full commit ID, selected directory and explicit
positive limits. It verifies the commit, locates the selected tree, reads a bounded
NUL-delimited listing, and then reads selected regular `.md` blobs one at a time.
Selection is case-sensitive and recursive within the directory. Other file formats
are outside this explicit selection and counted separately; they are not processed
as Markdown. Submodules and selected Markdown symlinks/LFS pointers are unsupported
errors, not silently dereferenced or treated as document contents.

Return a concrete snapshot with commit/root, documents (repository-relative path,
Git blob ID, raw bytes and SHA-256) and count of entries outside the Markdown
selection. This does not supply logical source identity, product version, processing
version, authorization or a catalog fingerprint. Values belong to the caller. No
universal source interface, persisted snapshot format or future-format fields are
introduced. A fixture can construct the same concrete capture values when a processor
becomes their consumer; do not add an empty adapter framework now.

Limits bound listing entries and bytes, each selected blob and total selected bytes.
Check advertised sizes against remaining budgets before reading blobs, then verify
actual lengths and Git object hashes. Abort on subprocess errors, malformed metadata,
missing objects, unsupported selected entries, cancellation or exceeded limits; return
no partial snapshot as success. Cancel and wait for a subprocess when output exceeds
its cap. Do not include captured contents or Git stderr in errors.

## Invariant and baseline cost

Every returned document belongs to the supplied commit and selection, irrespective
of working-tree edits or refs moving during capture. Repository GC or missing objects
can still make capture fail; the local object database is not our recovery storage.

Let E be listed entries, M the listing bytes, D selected documents and B their bytes.
Parsing, path sorting and hashing take O(M + E log E + B) in the worst case with
bounded path lengths, plus Git's object traversal/decompression and process startup.
Returned raw bytes use O(B); metadata/list parsing uses O(M+E).
The baseline starts a Git process for commit/tree inspection, listing and each selected
blob. That is O(D) process launches, no remote requests by design, and no claim of
production throughput. Bounded output is not a bound on all Git subprocess CPU or
memory; the caller must supply a deadline and trusted repositories.

The simplest alternative reads the checked-out directory, but that can mix local
edits with a recorded commit. Streaming into object storage or using `cat-file --batch`
can reduce retained bytes/process overhead later. Keep this exact bounded baseline
until a real corpus shows the need; the internal result does not impose a permanent
corpus size limit or a public memory contract.

## Evidence and next boundary

Use synthetic temporary repositories with real Git. Exercise pinned bytes despite
uncommitted/untracked changes and a newer commit, SHA-1 and SHA-256 repositories,
selected-root scoping, unusual paths, unsupported entries, missing/invalid/noncommit
IDs, selected blob/count/list/total limits, corruption checks and cancellation. Unit
tests/fuzzing of metadata parsing should exercise malformed/truncated records and
allocation bounds. No production documents or network access are needed.

Real local tests pass for the cases above, including SHA-1/SHA-256 and replacement
refs. Running-child cancellation is not yet fault-injected: pre-cancellation, the
output-cap cancellation mechanism and a real Git output overflow are tested separately.
These observations do not establish remote capture, sandboxing or scale performance.

This is capture only, not an importer, durable snapshot or storage milestone. Keep
raw check evidence private. Before wiring R2, choose immutable-object write/retry
behavior with a concrete ambiguous-write sequence; keep publication authority in
PostgreSQL. Complete fingerprint encoding and recovery still need their own review.

Sources: [Git tree listing](https://git-scm.com/docs/git-ls-tree),
[Git object reads](https://git-scm.com/docs/git-cat-file),
[Git environment](https://git-scm.com/docs/git).
