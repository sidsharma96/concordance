# Publish and recover a saved job through the hosted CLI

Status: the public import command published the retained Kubernetes capture through
native PostgreSQL profiles and actual R2 access. A fresh-process completed retry
returned the original generation with its R2 route blocked. A separate test target
then returned six independently known strict quotations across three editions.
The original `kubernetes` target stayed at its existing generation, default edition
1.36 and revision 1.

This exercises the [accepted start/retry workflow](../decisions/import-command.md).
It adds no product code, default change, schema or new source connector.

## Inputs and permitted effects

The source binary was built from f7f45ce, whose runtime code matches merged PR 57
at 3b25cce14eddd859f31a44b4463e419200ac3f10. The only intervening tracked change was
an unrelated ignore file. The original restricted capture, source Git commits,
collection and existing physical namespace were retained. A new local job owns its
verified HTML/recipe copy and CLI-generated saved key. Original operation and capture
descriptor hashes remained unchanged.

Connections use the existing restricted hosted-catalog runtime login, native service and
password files, explicit R2 settings and an owned strictly checked SSH forward.
Read-only preflight verified the exact database, user, catalog schema and active
namespace at epoch 1. The existing target matched its retained selection; the new
test name was absent. No administrator SQL, DDL, grants, HBA changes, namespace
creation/reactivation/quarantine or deletion ran.

The explicit start used eight confirmation chains and a 20-minute whole-command
deadline. Other import admission limits remained defaults. GOMEMLIMIT=768MiB was a
soft runtime hint, not a measured heap ceiling. Local before/after records keep the
exact binary, source, profile and job identities privately.

## Publication and shared objects

The command saved its new job, completed local Git/processing preflight, confirmed
required objects and published through PostgreSQL. Its printed generation agreed
with the catalog result, and the local job record remained unchanged.

A separate read-only inspector opened the old and new roots through catalog-pinned
references, checked their physical keys and exact body hashes, then compared every
non-root hash/length reference:

| Observation | Result |
| --- | ---: |
| Published documents | 38 |
| Exact editions | 3 |
| Distinct declared objects including root | 5,017 |
| Non-root references identical to the previous generation | 5,016 |
| New manifest bytes | 2,444,101 |
| Previous manifest bytes | 2,444,095 |
| New declared bytes including root | 132,106,459 |
| Retained figure references read and verified | 6 |

The roots differ; every required non-root reference is shared. This establishes
the difference in required references for these two generations, not a measured
bucket occupancy or billing delta. The inspector's catalog pool was read-only.
It verified figure bytes without interpreting or rendering their contents. The
public command has no per-request transport observer, so this experiment does not
report its total HEAD/GET/PUT attempts.

This used a collection with prior receipts and surviving objects. It is not a cold
upload or a throughput, latency or memory benchmark.

## Completed retry with the object route blocked

After publication, a fresh public-command process received HTTPS_PROXY pointing
at a loopback HTTP proxy that refuses CONNECT. NO_PROXY excluded only loopback,
leaving the PostgreSQL TCP/SSH route usable. The proxy forwarded no traffic,
decrypted no TLS, and recorded only a connection-attempt count.

The native connection factory clones Go's default transport, which preserves its
ProxyFromEnvironment behavior. As a positive control, a fresh search against the
existing target failed with zero stdout and one proxy connection attempt. The
object route was therefore demonstrably blocked for the same native client factory.

A separate fresh `import retry` process then reopened the same job, repeated local
preflight and returned the original catalog generation with Duplicate=true.
It made zero additional proxy connection attempts. Its immutable job-record hash
was unchanged, and the original target stayed unchanged.

This is completed-result recovery while R2 is inaccessible to that process. It is
not successful document retrieval during an outage: the control query failed.
Constructing the object client performs no object request, and the completed-key
branch returns before receipt confirmation or guarded publication. The experiment
did not simulate an R2 provider outage, corrupt an object or quarantine live trust.
Incomplete-import recovery and ambiguous database commits retain their separate
local fault evidence and open provider experiments.

## Separate activation and strict reading

The operator explicitly activated the new result under a new retained test name,
with default 1.36 and expected revision 0. The original target was never redirected.
Three fresh public searches selected 1.34, 1.35 and 1.36 and returned two hits each.
All six source paths, exact Git revisions and quotation hashes matched the original
independent reader record. Quote byte lengths were 106/285 for 1.34 and 104/285 for
1.35 and 1.36.

Postflight found the exact published result, the new test target at revision 1,
and the namespace still active at epoch 1. The original target selection, original
operation/capture and new saved job were unchanged. The new generation, job and test
target remain retained recovery material; no automatic cleanup or deletion ran.
Every owned SSH forward closed after its action.

## Evidence and limits

Actual environment: Go 1.27.2, darwin/arm64 and hosted PostgreSQL 16.15. This is one
normal hosted publication and completed retry, using the existing prior-receipt
collection. It does not establish production capacity, complete provider faults,
loss of either storage host, restore reactivation or scheduled backup coverage.
No relevance evaluation, reserved-set tuning, model inference or timing benchmark
ran. The earlier local command tests remain the controlled failure/ownership oracle.

Required independent code/architecture baseline reviews inspected the actual
operator and native call path before live writes. Their tree, evidence, roles,
catalog, operator and binary bindings were checked. Those approvals do not approve
later source changes. Public record changes receive their own normal review.

Private evidence under `.tools/reviews/hosted-import/` includes the before/after
target observations, start/retry/control/activation/search outputs, publication,
blocked-route and read summaries, root/reference inventory, postflight checks and
pre-live review bindings. The original job and test-target identity are located
through its protected operator-context record. No credentials, host routing or
live generation/collection identifiers are committed.
