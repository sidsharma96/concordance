# Keep the prepared inputs with the saved import key

Status: accepted option B. Each local import job owns a verified copy of its prepared
HTML, recipe and capture descriptor, alongside a generated saved operation key.
The bounded local component now implements this ownership; importer CLI composition
follows it. Read the [local experiment](../experiments/saved-import-jobs.md). This
decision does not claim that command exists yet.

## One retained job, one intended operation

The CLI will save both the key and original prepared inputs before remote writes.
Rebuilding the producer's original directory must not replace the job's bytes.
Reopening requires a compatible implementation of the job's recorded processing
rules, so retain a compatible build for unresolved attempts across upgrades. A retry
opens that same job and key; a deliberately new import creates another job
with a fresh key, independently of object deduplication. The candidate generation
ID is a proposal; the catalog-returned result remains authoritative.

Keep the [captured-input comparison](captured-input-identity.md), complete Git
membership, publication and uncertain-outcome contracts. Local readiness means
only that retry material was saved; PostgreSQL alone decides publication. Jobs
never execute recipes, activate targets, create catalog schemas or delete objects.

## Ownership and costs

Read the original descriptor/payloads once into bounded owned buffers and verify
their hashes. Write the job copy from those exact buffers, without reopening the
producer's files. Keep the job's final descriptor pin separate from the semantic
input fingerprint. Bound file/metadata counts, payload bytes and all later reads.

Files are private and exclusively created. Check writes, synchronization and closes,
then install a complete readiness marker without overwriting an existing path.
Do not accept partial preparation as a runnable job. An error can leave partial
local material or a complete installed job; retain the path for inspection. The
filesystem's synchronization semantics are an assumption, not a tested power-loss
or hostile-local-filesystem guarantee. Operator-owned directories stay protected.

The current restricted capture has 38 pages, 41 unique HTML/recipe payloads and a
743,407-byte descriptor. Their declared byte total is 21,439,296 (about 20.4 MiB),
checked by file size and the existing hash audit. This excludes pinned Git inputs.
A retained job adds a copy of those prepared bytes if the original is also retained;
multiple jobs can duplicate them locally. No local content-addressed store or
performance advantage is claimed. Revisit storage layout when retained-job disk
use supplies evidence, rather than adding another deduplication protocol now.

Both alternatives still require the registered Git repository's exact commits and
dependencies, including on completed retries while the importer rebuilds/preflights.
The job is not a Git backup, portable execution environment, automatic checkpoint
journal or recovery from loss of its own disk. Published generations remain
independently readable from the remote stores after writer-local files are gone.

## Why own the copy

A small record pointing at an existing capture uses less disk and fewer filesystem
operations. It is suitable when an external immutable capture store already owns
retention. We chose an owned copy so ordinary producer rebuilds and cleanup cannot
remove the rendered input needed by a saved-key retry. Hashes detect changed input;
they cannot reconstruct it.

[AzCopy v10.34.1's plan](https://github.com/Azure/azure-storage-azcopy/blob/39083e963e007a879b5ce096f62083af4f209c0c/ste/JobPartPlan.go#L56-L80)
keeps transfer identity and source/destination metadata; its
[resume command](https://github.com/Azure/azure-storage-azcopy/blob/39083e963e007a879b5ce096f62083af4f209c0c/cmd/jobsResume.go#L101-L135)
uses the saved job with current authorization. That illustrates reference-based
retry state, with external bodies still required. We do not adopt its mutable
per-transfer journal or memory mapping.

[Terraform v1.17.1's saved-plan writer](https://github.com/hashicorp/terraform/blob/ff6329e4c2a159b46ec2b73d36bfa3d0ef6560c9/internal/plans/planfile/writer.go#L18-L52)
packages configuration/state snapshots and dependency locks. It illustrates owning
relevant inputs, while external providers/runtime remain required. Its source is
BUSL, and its overwrite/synchronization behavior is not our creation protocol.
Our record has no credential fields; the operator supplies trusted current routing
and nonsecret source identity separately from any authentication material.

## Next composition boundaries

The local record must bind collection, registered source identities and compatible
processing policy. Source routes and credentials remain trusted runtime configuration.
A raw descriptor hash is a transport pin, not authorization or provenance proof.
Opening must validate the same descriptor bytes it decodes and the same payload
buffers subsequently passed to the importer.

The full command still needs bounded source-registration configuration, reader-safe
namespace reopening, CLI limits and result display. Namespace setup must stay explicit;
completed-key recovery must retain its zero-S3 behavior even when receipt reuse is
quarantined. Import and activation remain separate actions. No common Rewind schema,
shared engine or new distributed ownership is introduced by this local job slice.
