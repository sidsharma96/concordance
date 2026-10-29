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
NUL-delimited listing, and then reads selected regular `.md` blobs sequentially
through one bounded `cat-file --batch` process for that selection.
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
Commit/tree inspection and listing still use individual bounded commands; selected
blob reads share one process per capture. Blob transport uses O(D) requested IDs
and O(1) process launches, plus O(B) read/hash work. The request stores only admitted
fixed-length object IDs and separators; no body stream is accumulated as a second
batch-wide buffer. There are no remote requests by design. Bounded application data
is not a bound on all Git subprocess CPU or
memory; the caller must supply a deadline and trusted repositories.

The simplest alternative reads the checked-out directory, but that can mix local
edits with a recorded commit. The previous one-process-per-blob transport remains
an independent test oracle. A [real-corpus local construction](prepared-import.md#local-construction-of-the-selected-kubernetes-corpus)
made its 10,445 blob-process cost concrete; a bounded transport experiment supported
batching before this implementation. Batching does not stream away returned bytes:
the internal result still owns every selected body. A cross-call process cache or
parallel acquisition would add lifetime/resource policies and remains separate work.

## Bounded batch framing and process lifetime

Both Markdown and explicit dependency acquisition admit their complete tree metadata,
regular-file modes, counts, individual lengths and aggregate bytes before reading
any selected blob. The private reader sends only those full object IDs to one
`git cat-file --batch` process. Empty selections start no blob process. It retains
the existing environment/configuration controls and disables replacement/lazy reads.

For each admitted entry, a 128-byte buffered reader requires the exact canonical
`OID blob size` header plus LF. The largest supported header is 90 bytes; an overlong
or wrong header fails before body allocation. The body length comes from admitted
tree metadata, never the response. `io.ReadFull` reads that exact number of bytes,
then a separate LF is required. Git blob hash, raw SHA-256 and LFS rejection retain
their previous meaning. Each file owns an independent body even when paths share a
blob. Empty bodies preserve the previous nil-byte-slice representation. After the
last record, unexpected bytes, missing EOF or failed process exit prevent success.

`exec.CommandContext` constructs the child; `StdoutPipe` gives the call a reader,
and `Start` launches it. Go's exec machinery owns copying the bounded stdin request
and watching cancellation. The call reads stdout completely before `Wait`, which
joins that work and reaps the child. A malformed response kills the child before
waiting. Cancellation also closes the owned stdout reader: killing Git alone cannot
unblock a read if a descendant still holds a pipe open. A `sync.Once` closure checks
that explicit close once across cancellation and ordinary cleanup. Context, close,
process and protocol causes remain wrapped/joined; any failure returns no documents.
This joins the owned process, not an arbitrary hostile process tree.

As inspected in October 2026, Git 2.54.0's
[`batch_objects`](https://github.com/git/git/blob/94f057755b7941b321fd11fec1b2e3ca5313a4e0/builtin/cat-file.c)
provides this successive-object protocol, with framing exercised by its
[own tests](https://github.com/git/git/blob/94f057755b7941b321fd11fec1b2e3ca5313a4e0/t/t1006-cat-file.sh).
GitLab documents a [Gitaly process cache](https://docs.gitlab.com/administration/gitaly/configure_gitaly/#cat-file-cache)
for reuse across repository RPCs. Concordance keeps a single selection's lifetime
instead; no cache, process pool, parallelism, public API or storage format is added.

## Evidence and next boundary

Use synthetic temporary repositories with real Git. Exercise pinned bytes despite
uncommitted/untracked changes and a newer commit, SHA-1 and SHA-256 repositories,
selected-root scoping, unusual paths, unsupported entries, missing/invalid/noncommit
IDs, selected blob/count/list/total limits, corruption checks and cancellation. Unit
tests/fuzzing of metadata parsing should exercise malformed/truncated records and
allocation bounds. No production documents or network access are needed.

Real local tests pass for the cases above, including SHA-1/SHA-256 and replacement
refs. Batch bodies match an independent single-object process oracle in both object
hash formats, including binary/empty bytes and independent storage for shared blobs.
Parser cases cover missing/wrong/overlong headers, size coercion, truncation,
same-sized corruption, separators, trailing records and LFS. Real Go child fixtures
check nonzero exit after valid output, malformed output, failed starts and cancellation
with blocked stdin/stdout. Pipe handshakes establish waiting boundaries without sleeps;
every started child is waited. Windows skips extra-file handshake tests, whose
synchronization is unsupported there. A bounded parser fuzz target admits only the
canonical exact body and checks zero partial results. Short fuzz runs are exploration.
These observations do not establish remote capture, sandboxing or scale performance.

This component remains local capture. The [prepared importer](prepared-import.md)
now supplies processing and publication separately. Raw batch checks and experimental
cost evidence stay private under `.tools/reviews/git-batch-*` and
`.tools/reviews/restricted-import-batch-cost-*`.

Sources: [Git tree listing](https://git-scm.com/docs/git-ls-tree),
[Git object reads](https://git-scm.com/docs/git-cat-file),
[Git environment](https://git-scm.com/docs/git).
