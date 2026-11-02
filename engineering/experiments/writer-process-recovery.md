# Recover after terminating the generation writer

The local `TestWriterProcessRecovery` drill exercises the real generation publisher,
catalog, SDK and strict published reader with deterministic synthetic producer
inputs. It adds no runtime API, dependency, shared engine or automatic resume.
The baseline is `1256d52583d1733603403b4e10df0ad837a21f2a`; test files and this
record are the separately reviewed change above it. The smaller fresh-client and
returned-error tests remain independent oracles.

## Failure sequence and visibility

| Case | Interruption | State before deliberate retry |
| --- | --- | --- |
| Acknowledged | Publication returned successfully, then writer killed | Original generation visible |
| PUT reply lost | Conditional PUT stored, HTTP socket closed before reply; strict GET reconciles, publication succeeds, then writer killed | Original generation visible |
| COMMIT reply lost | Final publication committed, actual COMMIT and idle replies withheld, pgx connection closed; publisher returns empty uncertain result, then writer killed | Original generation visible despite caller uncertainty |
| Before publication | All artifacts/root and receipts confirmed; writer killed at final generation UPDATE barrier | Binding exists; original generation absent; eight uploaded objects retained |

The child tracer matches only `UPDATE concordance_imports SET generation_id=$3,`
in `catalog.(*Store).publish`. Namespace/receipt commits are already complete.
The parent observes the unpublished binding before it arms the local pgproto3
proxy and resumes the UPDATE. The proxy requires successful COMMIT completion and
idle ReadyForQuery before dropping both replies. It does not discard an already
successful Go return or fault an arbitrary earlier transaction. The worker sets
`synchronous_commit=on`; local commit completion still cannot protect the server
host or establish remote PostgreSQL availability.

Each writer is forcibly killed and reaped, and its private working directory is
removed before replacement starts. The fresh child receives only deployment
configuration, role/fault selection and original operation/input identity encoded
in the deterministic test producer. It receives no saved publication, manifest
reference, catalog snapshot, object map or writer files. A fresh catalog resolves
the original key. The strict reader verifies root, exact edition index, source
revision, stable ordering of two logical documents sharing one article, and the
exact quote `Pods\nRetry uploads safely.`. The newline denotes the real line break.

The replacement changes its candidate ID while retaining the operation key and
input fingerprint. A completed import returns the original reference with no S3
access. In the pre-publication case, the reader rejects the original generation
without S3 requests, then deliberate retry reuses seven receipts with conditional
HEAD, uploads a new root, and publishes the replacement candidate. The old root
remains unreferenced and retained. A changed fingerprint conflicts before object
access; a final fresh reader still recovers the unchanged publication and quote.

## Recorded body I/O

These are observations from `.tools/reviews/recovery-first-pass.log`, using Go
1.27.2 on darwin/arm64, native PostgreSQL 14.17 and a surviving controlled HTTP peer.
Snapshots occur at I/O-free child barriers. Request bytes are actually read from
HTTP request bodies at the peer; response bytes are successful peer body writes.
They exclude headers, TCP/TLS overhead, SDK buffers and guaranteed client delivery.
The disconnected successful PUT has an empty response body. Missing-object XML
bodies are included. No timing or peak-memory result is claimed.

| Phase | Cases | GET | HEAD | PUT | Request body bytes | Response body bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Initial writer | All four | 16 | 0 | 8 | 3,215 | 3,511 |
| Replacement initial strict read | Three published cases | 3 | 0 | 0 | 0 | 2,883 |
| Replacement initial visibility check | Before publication | 0 | 0 | 0 | 0 | 0 |
| Completed same-key retry | Three published cases | 0 | 0 | 0 | 0 | 0 |
| Deliberate unfinished retry | Before publication | 2 | 7 | 1 | 2,058 | 2,095 |
| Changed-input conflict | All four | 0 | 0 | 0 | 0 | 0 |
| Final strict read | Three published cases | 3 | 0 | 0 | 0 | 2,883 |
| Final strict read | Before publication | 3 | 0 | 0 | 0 | 2,892 |

The first publication retains eight distinct objects. The unfinished retry adds
one root for nine retained objects; parent assertions compare all original and
new object bodies exactly. Counts differ from Rewind because Concordance publishes
a complete generation with a root, per-edition indexes and retained representations.
They are not evidence that one adapter is inherently cheaper. Real-corpus/provider
counts and missing live body instrumentation remain in the
[counterpart assessment](shared-object-assessment.md).

## Bounds, checks and limits

Children have 40-second process deadlines, 35-second operation contexts, one pool
connection, an 8-second SQL statement timeout, bounded explicit HTTP transport and
64 KiB control streams. Stderr retention is 16 KiB and checked after Wait, including
race diagnostics. The local plaintext-only proxy refuses TLS/nonlocal configurations,
admits at most four connection pairs, bounds frames to 1 MiB and each forwarded
direction to 8 MiB, and sets 30-second connection deadlines. Tracked connections
close and forwarding goroutines join. The controlled peer uses existing 1 MiB body
limits with at most 32 retained keys. Test callers are serial; this is not a public
hostile-input server or a production proxy.

Use a dedicated disposable database only:

```sh
CONCORDANCE_TEST_DATABASE_URL='postgres://local-test-server/disposable?sslmode=disable' \
  go test -count=1 -run '^TestWriterProcessRecovery$' -v ./internal/generationpublish
```

Without the explicit test database the parent skips. The child entry test does
nothing unless launched with the private worker environment. No hosted catalog,
R2 access, secrets, external corpus, backup or deployment is involved. Services
survive on the same host: this proves writer-process replacement, not machine
loss, provider storage loss, failover, scheduling backups or disaster recovery.
A timeout does not establish writer death; the parent explicitly kills and waits.
There is no deletion and no inferred permission to delete uploaded orphan objects.

Independent architecture design approval preceded implementation. Full
`make verify` passed with the environment above, including vet, ordinary tests,
uncached race tests, repository checks and build. Live provider tests were disabled.
The initial harness failures were test setup only: the child needed the explicitly
configured schema separate from the original DSN, and the exact retained passage
includes its heading. No production bug or runtime change was required. Final code
and architecture approvals must match the exact staged source before commit/push. The [shared checkpoint](../decisions/shared-object-recovery-checkpoint.md)
still defers scope/addressing, maximum versus exact read length, write/reference
shape and admission/deadline ownership to a joint design discussion. No library API
or extraction is approved by these local counts.
