# Recover a bounded binary HTML edition

`internal/htmledition` now implements the accepted [binary encoding](../decisions/edition-binary.md)
for one edition's count postings and saved body locations. Its Build, Encode, Open,
Search, Article, Location, Quote and Inspect operations are internal components.
`tools/inspectindex` prints a checked local file as bounded JSON. There is no complete
manifest, importer, published generation, raw-query CLI or production migration.

## Representation and ownership

Build decodes exact retained article JSON and prepares each article once through
`htmlterms.Build`. Counts and saved locations originate in that same preparation.
The immutable edition keeps a `termsearch.Index`, article descriptors and body ranges.
The wire grammar is closed v1 with absolute varint passage IDs and separate field
counts; no ordered-word arrays, compression or native Go struct dumps are stored.

`termsearch.ExportEdition` first admits the selected edition under explicit build
limits, then returns independently owned sorted keys/count rows. `NewCountIndex`
validates sorted unique keys/rows and positive combined frequencies, recomputes
lengths/total and occurrence-weighted input bytes with checked budgets, and creates
an immutable owned index. Restored admission covers this edition only; it does not
change `NewIndex`'s all-input validation or validate other generation artifacts.
Count exports and wire records add temporary copies; byte limits are not peak-heap
quotas. No mutable aliases escape the edition. Read-only operations support concurrent
callers; quotation output owns its assembled text. Each Quote verifies and decodes
the complete supplied article anew; no article cache or I/O is hidden in the component. Composing strict objectstore.Read
with Open/Quote currently hashes those bytes in both layers; that adds CPU without
another network request. The local open benchmark below excludes the storage layer.

Open requires an independently expected size, digest and exact edition. It hashes
the actual input before parsing, rejects unsupported tags/versions, malformed UTF-8,
NULs, truncation, overflowing/overlong varints and trailing bytes, and admits table
counts against both logical limits and minimum remaining bytes before allocation.
It validates counts/statistics, article slots and complete nonoverlapping range
coverage before exposing search state. It verifies article descriptors at open;
actual article bytes/hash/format/block count are checked before quoting. No passage
builder or term preparation runs on recovery. The original ordered-field
`htmlpassagemap.Open` stays unchanged.

This checks byte/structural consistency, not publisher authorization or semantic
preparation. A bad writer can bind plausible counts and ranges incorrectly. Future
authorized generation metadata must pin exact index/article/source/document/revision
associations. Two document slots may share article bytes; a digest is not identity.

## Tests and fault boundaries

Two hundred generated count transports compare exact restored BM25 scores/order with
the unchanged scans and independently check selected-edition admission. Tests mutate
exports/imports to check ownership; malformed counts, overflow, ordering, lengths,
statistics and budgets fail without partial results. Edition tests compare reopened
results, article locations and quotes against the unchanged `htmlsearch` snapshot.
They cover known-empty editions, zero-term passages, identical article bytes in
separate slots, exact references, concurrent readers and every observed cooperative
cancellation boundary of build/open/encode/inspect/quote/count transport.

An independent spec-shaped test writer constructs invalid wire relationships the
canonical writer cannot emit. Tests exercise every truncation of a valid fixture,
nonminimal/overflowing integers, bad tags/strings, giant counts in tiny files,
posting/count/length disagreements, article/range failures and each file budget.
A synthetic valid saved range can differ from today's grouping and still quote its
saved blocks; this demonstrates recovery without claiming a historical writer.
A descriptor with plausible but incorrect block count opens structurally and fails
on actual article verification, making the lazy-verification boundary explicit.

The fresh-client SDK/HTTP fixture writes immutable articles and the real binary
edition, discards writer buffers, opens/searches with one index GET, then reads the
selected article for a verified quote. Missing/corrupted article bytes fail explicitly.
This is a local S3-protocol fixture, not live R2 or catalog publication recovery.
The inspection tool tests expected-reference validation before output, bounded files/
JSON, invalid arguments, cancellation, write failure and short writes.

A 10-second decoder fuzz request completed 353,762 executions over 11.216 seconds
with four workers, checking rejection or exact canonical re-encoding. Passing fuzz
runs are observations, not a proof about all inputs. Raw output is private under
`.tools/reviews/binary-edition-fuzz.txt`.

## Frozen corpus and local open cost

The existing 38 Kubernetes article-version entries reopen through the actual codec:

| Edition | Encoded bytes | Article slots |
| --- | ---: | ---: |
| 1.34 | 100,093 | 10 |
| 1.35 | 127,451 | 13 |
| 1.36 | 141,208 | 15 |

The real-edition total is 368,752 bytes. The separate known-empty control is 96 bytes.
Canonical re-encoding is byte-exact. All 28 frozen query cases preserve 430 exact
ranked identities/order/scores and 138,032 summed quote bytes against independent
retained-block joins. Caller encoded buffers can be cleared after open. These are
index bytes, excluding article bodies, full source/citation metadata, manifests and
provider overhead. They differ slightly from the earlier illustrative sample format.

On Go 1.27.2 darwin/arm64, Apple M1, default GOMAXPROCS 8, five controlled 200 ms
benchmark samples opening the largest edition measured 1.263–1.381 ms/op,
1,867,871–1,867,939 allocated bytes/op and 13,646 allocations/op. This includes hashing,
wire/relationship validation and constructing the owned query index from already
resident input. It excludes Build, R2/network, manifest/source validation, article
reads and queries. Allocation totals are not peak or retained heap. A first trial
that overlapped the inspection command was discarded before this controlled run.
There is no equivalent complete strict JSON reader benchmark, so this does not
establish a production JSON/binary speed ratio or end-to-end cold-open latency.

Private disposable probe: `.tools/probes/binary-edition-audit/`. Selected raw output:
`.tools/reviews/binary-edition-audit.txt`, `binary-edition-open-bench.txt` and
`binary-edition-inspected.txt`. Source inventory SHA-256:
`f82481e1d451e5b1901e87231158f3ddeb338ff06bc5295b5cb81138cd268052`.
Frozen report SHA-256:
`da2f4eddd71ccea8cceec2a02ad40ecc2639f207821cce31bbf5ab088ef4543e`.
The probe verifies these identifiers and all source hashes. Normal checks need no
private corpus/probe. No remote writes, complete generation or scale claim follows.

## Remaining work

Generation metadata still needs exact source/document/revision and artifact bindings,
full input identity, admission, confirmation and publication/recovery integration.
Loading one edition never validates every other object's body. Compression, block
paging, bounded caches and safe retention/deletion remain separate choices/work.
All required objects must be confirmed before catalog publication; automatic deletion
remains disabled. Measure real cold reads and retained memory before scale claims.
