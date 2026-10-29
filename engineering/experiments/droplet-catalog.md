# Live catalog publication and isolated restore

## Question and workload

Can the bounded publisher use the existing droplet catalog and real R2, and can an
encrypted off-host catalog backup recover a published quote without trusting old
receipts? This is a controlled synthetic experiment, not the full Git importer.

Application baseline: PR25, `c3780cf3c93688688c200fd54c5d92824b99f810`.
Server: Ubuntu 24.04, PostgreSQL/pg_dump 16.15, one vCPU and roughly 2 GiB RAM.
The local ordinary-test baseline is PostgreSQL 14.17. Live tests use a dedicated
Concordance database, NOLOGIN owner and runtime role, pool two, role limit four;
Receipt shares the server. Current free resources justify a tiny fixture only.
No PostgreSQL public listener, global tuning, service restart or Receipt ACL change.

The fixture is the publisher test's two logical Markdown paths sharing one retained
HTML article, a figure, a nonempty edition and a known-empty edition. Seven distinct
artifacts plus a generation manifest form the first publication. A second candidate
shares those artifacts but has its own root. Trusted synthetic input identity does
not implement the full captured-input fingerprint or verify a real website build.
The Go experiment and operator inventory are private, disposable evidence, not a
supported command or required workflow.

## Observations

One live run observed these request counts (SDK transport requests, not latency):

| Operation | GET | HEAD | PUT |
| --- | ---: | ---: | ---: |
| First publication | 16 | 0 | 8 |
| Saved-key completed retry | 0 | 0 | 0 |
| Second generation sharing artifacts | 2 | 7 | 1 |
| Fresh-process selected quote | 3 | 0 | 0 |
| Selected quote from restored reference | 3 | 0 | 0 |

The reader used the authoritative generation reference, verified the root's physical
key/hash/length, opened the selected index, found two lexical matches and verified
its article before resolving `Pods\nRetry uploads safely.`. It fetched no figure or
other edition body. These counts do not measure throughput, peak heap or capacity.

Real authentication succeeded only for the intended database. Receipt and postgres
connections and wrong-password authentication failed. Runtime table/schema creation,
DELETE, receipt UPDATE and TEMP creation failed. The first four direct runtime
sessions connected; a fifth failed. The runtime is a trusted SQL writer: narrower
grants reduce privileges, but cannot establish immutable metadata against compromise.
The HBA include parsed with zero errors, and PostgreSQL/Receipt/Caddy remained active.
The first operator installation used SQL-style single quotes around an HBA include
filename; HBA parsing rejected it. The installer restored the original file while
LOGIN remained disabled. The corrected unquoted filename parsed and reloaded without
restart. This was an installation failure, not a publication/R2 fault.

## Recovery sequence

1. Dump only Concordance with server-version `pg_dump -Fc`. Encrypt on the host with
   OpenSSL 3.0.13 CMS, explicit AES-256-GCM and RSA-OAEP/SHA-256. The RSA-3072 recovery
   key stays off the host; only its public certificate is sent there.
2. Retain the 13,693-byte encrypted archive off-host on the operator's Mac and in a
   distinct operator-backup collection in the existing private R2 bucket. Strict
   immutable creation and a fresh-client verified read protect its expected bytes.
   Local OpenSSL 3 decrypted a roundtrip matching the original dump hash; altered
   ciphertext and an unrelated private key failed. Check success before restoring:
   failed decryption can leave partial output. Encryption is not proof of producer
   authenticity; this archive has trusted provenance and an independent byte reference.
3. Fetch the R2 copy, verify, decrypt, and restore it into a uniquely named postgres-
   owned clone with PUBLIC CONNECT revoked and no runtime HBA allowance. Use
   `pg_restore --no-owner --no-acl --exit-on-error`, then quarantine all restored
   namespaces transactionally. The one restored namespace advanced from epoch 1
   to 2 and became inactive before application use.
4. Reapply only clone object grants. An admin-only peer session with `SET ROLE
   concordance_app` runs the actual catalog API: `BindNamespace` refuses inactive
   state. No application CONNECT grant or HBA broadening is used. The initial probe
   tried scanning bytea directly into a Go fixed array; it failed. Using the existing
   catalog lookup API fixed the disposable probe without a production code change.
5. Export the published reference from the restored catalog, then start a fresh
   laptop reader. Full R2 reads of manifest, edition and article reproduce the same
   two matches and quote, without receipt-based reuse. Close sessions, remove only
   the exact ephemeral clone and temporary plaintext dumps, retaining encrypted
   backup references and the recovery key separately.

## Limits and revisit conditions

This tests a point-in-time catalog restore with surviving immutable objects. It is
not simultaneous loss of both stores, an automatic recovery service, re-establishing
publication trust, network commit ambiguity, unattended backups or a production
migration. Restored namespace reactivation stays disabled. R2 objects are retained;
no deletion, GC or bucket policy changes were made.

Backup scheduling, monitoring, retention, a second protected key copy, acceptable
loss window and recovery time remain open. Runtime login/bootstrap metadata are
separate from a single-database dump. Database access separation does not isolate
resource contention or host failure. Revisit hosting as the workload grows or
production availability/private-data requirements appear. See the
[hosting decision](../decisions/catalog-hosting.md) and
[operator procedure](../../deploy/postgres/README.md).
