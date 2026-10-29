# A file handoff for prepared Git/HTML

`internal/capturefiles` transports the accepted trusted
[prepared capture](../decisions/prepared-capture.md) without running a website
builder. `ReadDirectory` reads one `capture.json` and separate HTML/recipe files;
`Encode` writes descriptor bytes for a preparer. `tools/captureaudit` checks those
local files without Git, a renderer, database or object service. No import command,
automatic journal, filesystem-write protocol or new publication guarantee is added.

## Versioned descriptor

The format is `concordance.git-html-capture/v1`. Its closed JSON shape is:

- Root: `format`, `editions`.
- Edition: `id`, `captures`.
- Capture: `source`, `revision`, `directory`, `preparation`, `toolchain`, `recipe`,
  `dependencies`, `profile`, `pages`.
- Profile: `id`, `article_class`, `omit_classes`, `omit_ids`, `notice_classes`.
- Page: Git `path`, `html` file reference.
- File reference: payload `path`, lowercase 64-hex `sha256`, positive integer `bytes`.

Every field is required, including empty arrays. Dependencies are full Git paths;
the recipe and HTML references name regular files beneath the capture directory.
Source Git paths retain their existing meaning; portable payload paths additionally
reject colons, absolute/traversal/backslash/control spellings and the reserved
`capture.json` name. The parser rejects unknown/duplicate/escaped-duplicate keys,
nulls, missing fields, malformed Unicode, numeric coercion and trailing JSON.
Objects have at most nine required keys. List counts are admitted before decoding
and appending their elements; token allocations remain bounded by encoded input.

The descriptor carries every current `preparedimport.Capture`, `Page` and full
extraction-profile field. It carries no source routing/credentials, collection,
operation key or candidate ID. Those remain trusted caller configuration. A later
archive transport can carry the same logical layout; no tar/compression is added.
Payload names and descriptor formatting do not become semantic import inputs.
The existing fingerprint still compares actual HTML/recipe bytes, source metadata
and processing rules through `PublishCaptured`.

As inspected in October 2026, OCI image-spec v1.1.1 separates its
[directory layout](https://github.com/opencontainers/image-spec/blob/147f9c13cedb47a0c4d9a11a222961073d585877/image-layout.md)
from transport as tar/zip/shared filesystems. That is prior art for separating a
metadata/file layout from its wrapper, not an adoption of OCI or a registry. OCI
allows some missing blobs with external fulfillment; this reader fails on every
missing required payload. Unreferenced local files are not opened. Their presence
does not affect complete source membership, which the importer checks in Git.

## Read, verify and use the same buffer

The operator supplies and protects a stable trusted directory. On supported native
platforms, `os.OpenRoot` confines relative opens to that tree. It is not a filesystem
snapshot, hostile-local-filesystem sandbox or guarantee against mounts/devices.
The reader checks `Lstat` before opening and `Stat` afterward for regular-file/size
admission; those checks assume no adversarial concurrent replacement. Final symlinks
are rejected. An ancestor symlink cannot escape the root on supported native systems.

All metadata, path relationships and advertised payload occurrence budgets are
checked before payload opens. Repeated payload paths must have identical refs;
each unique path is read once into owned storage through a byte-limited reader.
An extra probe byte detects growth; exact length and SHA-256 are checked against
that same returned buffer. There is no verify-then-reopen interval. Each file and
the root are closed on success/failure, with close errors preserved. Context checks
are cooperative between reads and cannot preempt blocked local filesystem I/O.

The returned editions own their metadata and byte buffers. References to one
payload path may share a backing buffer. Keep every alias unchanged through
`ImportPrepared`; neither reader nor importer retains them afterward. Encoding
borrows unchanged metadata, writes required empty lists rather than nulls, and
returns independent bytes. It does no filesystem write or sealing/durability work.
Checksums establish byte agreement, not source authority or build execution.
Pinned local Git objects are still needed for the subsequent import/retry.

## Admission and costs

Descriptor bytes, total value bytes, individual strings, editions, captures, pages,
dependencies and file-reference occurrences have positive ceilings. Per-file and
aggregate payload occurrence bytes are separate. Sharing a path avoids a second
read/buffer but does not bypass the occurrence budget. Metadata is admitted before
maps or wire copies. `Encode` can allocate escaped output before a smaller final
encoded ceiling rejects it; admitted strings/counts bound that allocation.

Let D be encoded descriptor bytes, P its admitted paths, and B unique payload bytes.
Parsing and validation cost O(D + P) expected map work; read/hash work is O(B).
Each unique payload has Lstat/open/Stat/read/close work; the descriptor is also read.
Memory includes descriptor/parser tokens, copied metadata, maps and all B owned
bytes, followed by the importer's own staging/index scratch. No exact heap, latency
or million-document capacity is established. Rereading files later would reduce
retained buffers only by changing the current borrow/verification contract.

## Evidence

Tests use real temporary files and an independent handwritten wire fixture. They
check required empty lists, full field/profile fidelity, owned bytes after directory
removal, documented same-path aliases, duplicate/path conflicts, malformed Unicode
and schema/numbers, missing/truncated/oversized/same-sized corrupt bodies, traversal,
final/ancestor outside symlinks, regular-file restrictions, exact/one-below budgets,
native probe-length overflow and context requirements. A bounded decoder fuzz target
round-trips accepted metadata; a short run is limited exploration, not proof.

The dedicated local PostgreSQL 14.17/SDK-HTTP test reads the directory, removes it,
then publishes its owned buffers through the actual prepared importer. A completed
retry adds no object requests. After clearing caller buffers and removing local Git,
fresh object clients search the stored index and verify `Pods\nRetry uploads safely.`
at `docs/a.md`. The tool reports only after all local file checks succeed; its tests
cover invalid arguments, resource failures and failed output writes. `make verify`
uses a dedicated local database; no hosted CI or new live provider write is involved.

## Kubernetes reference audit and the next limit

A private disposable preparer checked all 38 retained HTML files against their
original render-result lengths/hashes. It reconstructed a conservative Git dependency
list from each exact pin and the recorded build mount roots (with English-only
content as specified in the original experiment). The old staging directories no
longer exist. Recipes state this reconstruction and the existing toolchain/reference
limits; the audit does not establish a precise renderer-read inventory or production
build equivalence. Selected print outputs are explicitly excluded.

The resulting 892,077-byte descriptor passes `captureaudit`: three editions/captures,
38 pages, 12,830 declared dependencies and 20,696,354 payload reference bytes
(20,693,182 HTML bytes plus recipes). Payload file checks do not acquire Git dependencies.

| Edition | Declared dependency occurrences | Git tree dependency bytes |
| --- | ---: | ---: |
| 1.34 | 4,129 | 353,007,660 |
| 1.35 | 4,247 | 372,687,261 |
| 1.36 | 4,454 | 391,575,215 |

The total is 1,117,270,136 reference bytes. Grouping by Git blob ID estimates
405,685,982 distinct bytes across 5,712 blobs; this is a Git tree metadata estimate,
not verified SHA-256 object bytes, actual R2 storage or heap measurement. Static
files account for 875,136,654 reference bytes and an estimated 299,068,899 distinct
bytes. The largest candidate dependency is a 16,483,782-byte unrelated blog photo.
Mounted files are a conservative superset, not proof every file was read.

This evidence does not justify silently dropping dependency paths or merely raising
import limits. The original renderer disabled static copying, but templates can have
other dependencies, and output segmentation is not input isolation. Dependency scope
and a restricted-input preparation experiment need review before a real-corpus
publication. No Kubernetes generation was published in this slice.

The command for an already prepared trusted capture is:

```sh
go run ./tools/captureaudit -directory /path/to/capture
```

Its experimental defaults are 4 MiB descriptor, 8 MiB per HTML/recipe file, 128 MiB
payload occurrence bytes, 32 editions, 128 captures, 10,000 pages, 100,000 dependency
paths, 10,128 file refs, 8 KiB strings, 4 MiB string values and a 15-second cooperative
deadline. They are audit ceilings, not importer or production capacity promises.
Selected private checks, fixtures and reference results remain under ignored
`.tools/reviews/capture-files-*` and `.tools/reviews/capture-dependency-cost.txt`.
