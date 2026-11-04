# Start publishes; retry keeps the saved job

Status: accepted option A. The first trusted-operator import workflow saves a new
local job and then publishes synchronously. An explicit retry opens that same job,
keeping its operation key and prepared inputs. The command now composes the existing bounded importer;
[local evidence](../experiments/import-command.md) records the tested scope.

## One ordinary operation, one recovery operation

The intended commands are `concordance import start` and `concordance import retry`.
Start claims a new job directory, owns a verified capture copy and persists the
generated key before remote writes. It then runs the existing complete prepared
importer. Successful return means a catalog-published generation. The displayed
generation must come from the catalog result, never the proposed candidate.

An existing destination fails start; it is not an implicit retry or permission to
overwrite. Retry opens the existing job and validates its bytes and compatible
processing labels. It uses trusted current source registration and connection
configuration, not routes or credentials supplied by the saved capture. Source IDs,
observed repository identities and collection must agree before import. Exact Git
objects remain available even on a completed retry while preflight rebuilds.

An error can leave partial or complete local material and can follow committed
publication. Keep the path and key. A valid ready job can be retried explicitly;
missing local readiness cannot authorize remote work. Output failure after commit
also requires the same job, not another start. No automatic resume or deletion.

Import never activates a target. Explicit activation checks the selected published
generation and expected target revision. Existing readers remain pinned, and old
generations stay retained.

## Why combine local preparation and publication

The alternative exposes separate local prepare and remote publish commands. That
pause suits automation stages or inspection before remote effects. It adds a normal
step and an intermediate success meaning: local preparation does not validate full
Git membership or publish search data.

We chose one start command because the existing importer already has a clear
synchronous completion contract. We retain the same local save boundary internally
and make same-intent retry distinct from a new intended operation. Neither workflow
improves the other's retry correctness, remote scaling or processing speed.

[AzCopy v10.34.1](https://github.com/Azure/azure-storage-azcopy/blob/39083e963e007a879b5ce096f62083af4f209c0c/cmd/jobsResume.go#L37-L125)
accepts an existing job ID and current authorization when resuming a transfer.
That illustrates explicit recovery of earlier work; its transfer journal and
partial-transfer resumption are not selected.

[Terraform v1.17.1](https://github.com/hashicorp/terraform/blob/ff6329e4c2a159b46ec2b73d36bfa3d0ef6560c9/internal/plans/planfile/writer.go#L18-L52)
saves configuration, state, plan and dependency locks for a later apply. Its
[source-available CLI](https://developer.hashicorp.com/terraform/cli/commands/plan)
also supports planning within apply. Our local job is a capture copy, not a computed
execution plan, and we inherit neither Terraform's state nor approval semantics.

Revisit a prepare-only command when an actual operator workflow needs a pause,
artifact transfer or separate local preparation stage. Keep the existing bounded
importer and catalogs rather than adding a job service.


## Initial admission and namespace observation

The source profile is trusted runtime data with closed versioned fields and
absolute checkout routes. Each saved collection/source ID/repository identity must
match it before connection. It contains exactly the job's sources, not an ambient
registry. Native PostgreSQL service files and explicit R2 settings retain current
credentials outside jobs.

The command opens an existing namespace by its unique exact physical binding.
OpenNamespace is a read-only observation, including inactive state, not a trust
grant. Receipt operations and PublishInNamespace still check active state and
epoch. A completed key returns its original result before those object/write
operations; unfinished work can bind inputs and then fail the quarantine gate.
Missing bindings require explicit operator setup, never automatic creation.

Defaults admit 512 MiB input occurrences, 256 MiB distinct staged bytes, 64 MiB
prepared payload occurrences, 8 MiB per object/file, 1,000 logical document
occurrences and 20,000 raw file occurrences. Flags adjust those ceilings; capture
metadata is capped at 8 MiB, 32 sources, 64 editions and 256 captures. Source-profile
bytes are capped at 1 MiB with 8,192-byte strings. A ten-minute whole-command deadline
covers local saving and publication; native local filesystem calls stop cooperatively.

Fixed codec/processing budgets include 20,000 article blocks, 40,000 groups,
512 nesting levels, 8,192 body bytes per passage, 20,000 passages per edition,
1,000,000 term/posting occurrences, 20,000 vocabulary entries and 16 MiB prepared
edition bytes. One edition admits 64 MiB declared article bytes. Selected Git
documents/dependencies each admit at most the configured object cap, with at least
128 MiB total per selection; aggregate input admission still follows acquisition.
The hash-work ceiling is 16 MiB. Existing explicit failure replaces truncation.
These are initial operator ceilings, not measured production capacity, heap limits
or a permanent corpus size. Larger workloads require deliberate limit/scale evidence.

Each confirmation batch admits at most 32 objects and the configured object byte
cap in total. Sequential confirmation is the default; the optional concurrency flag
accepts one through eight chains. Simultaneous commands remain operator-admitted.
