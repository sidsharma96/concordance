# A saved import can be published and retried from the command line

Status: the accepted start/retry workflow composes local owned jobs with the
existing bounded Git/HTML importer and explicit target activation. This experiment
uses synthetic Git, isolated local PostgreSQL schemas and a controlled HTTP object
peer. It is not a new hosted/R2 publication or a production-size admission result.

## What the command owns

Start requires a new protected job directory under an existing parent. It saves
HTML/recipe inputs and the generated key before connecting. Retry opens that same
directory, without the producer or a new key. Both validate the actual saved
descriptor/payload bytes and current processing labels, then compare collection,
source IDs and observed repository identities with a bounded trusted operator
profile. Checkout routes and native current credentials stay outside the job.

The concrete connection adapter is shared with existing commands. Every successful
connection transfers cleanup to the command. OpenNamespace performs a SELECT for
the exact physical binding and never creates/reactivates a namespace. It returns
an observation of inactive namespaces so an existing completed key can reach its
catalog result before receipt confirmation. Fresh confirmation/publication checks
active state and epoch as before. An unfinished key may bind input metadata before
the quarantine gate fails; inactive observation is not a write authorization.

Errors preserve local material and safely describe the stage without printing
driver/provider diagnostics or input bodies. A catalog result can precede an error
or short stdout write. Retry uses that original key to recover the actual result.
The candidate ID is never used as a substitute for the catalog-returned generation.
No import activates a target or deletes its previous generations.

## Local functional checks

The command fixture starts with two committed Markdown files associated with one
authored HTML body. It imports, explicitly activates and searches that generation,
then publishes changed input into a second saved job. The target still selects the
first generation until a separate expected-revision activation. Search sees the
second result while an already-open first reader still quotes its original text.
A third explicit target revision switches back to the first generation.

After removing the original prepared directory, the fixture quarantines receipt
reuse and retries the first job through newly constructed pools/catalogs/object
clients. It returns the original generation with zero object requests and leaves
the target revision unchanged. A new import job under quarantine fails before
object requests and has no published candidate.

A separate fixture forces stdout to short-write after publication. The result
exists in PostgreSQL; another start refuses the existing job before connection.
Explicit retry recovers the same generation without the producer or object requests.
Another fixture denies object creation: the first call fails without a published
generation, retains its ready job, and an explicit retry completes that original
key after the producer directory is removed.

Connection failure and a missing namespace retain the saved key. Missing namespace
lookup neither inserts a namespace nor contacts the object peer. Source identity
mismatch fails before connecting. Malformed, duplicate, unknown/case-altered,
null, Unicode, excessive count/byte and relative-path source profiles are refused.
Source decoding has a fuzz target. Binary help exercises the real import dispatcher;
full composition tests substitute only the connection adapter, not the importer.

The catalog's read-only pool test opens existing/missing bindings under PostgreSQL
default_transaction_read_only. It returns inactive observations after quarantine;
receipt lookup and a writer's guarded publication still reject them. Other physical
bindings remain absent and the namespace count remains unchanged.

## Checks and practical limits

Local verification uses Go 1.27.2 on darwin/arm64 and dedicated PostgreSQL 14.17,
with hosted access and live R2 tests disabled. Test logs and independent review
records stay private under .tools/reviews/import-start-retry. No new timing,
throughput, peak memory, model inference or relevance measurement is claimed.

These are newly constructed clients, not a power-loss or real hosted-process
recovery experiment. Exact Git objects and compatible processing remain required.
The source profile is trusted operator data and does not prove source provenance.
Protected local paths and filesystem synchronization semantics remain assumptions.
Component ceilings do not bound whole-process heap or simultaneous commands.

A separate local corpus probe used the existing restricted Kubernetes capture and
unchanged pinned Git repository with the command defaults. It published all 38 pages
across three editions into isolated PostgreSQL/HTTP storage: 5,017 conditional PUTs
and 10,034 GETs for cold confirmation. Explicit activation of a disposable local
target followed by a 1.36 ContainerRestartRules search returned two strict quotes.
After quarantine, newly constructed clients retried the same job with zero GET,
HEAD or PUT requests. The probe substituted the connection adapter only; no hosted
provider was used. Original capture and Git material remained unchanged. These
counts are functional evidence, not a latency/throughput benchmark. Its disposable
job, database schema and HTTP bodies were local and removed after the probe.

The next native-hosted exercise should preserve an original job/key and verify
publication, explicit activation and strict quotes through native configuration.
No automatic schema/namespace setup, restore reactivation, durable upload journal,
managed renderer, retention/deletion or background resume was added.
