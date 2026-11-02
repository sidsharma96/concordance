# Search the published Kubernetes corpus by name

The hosted catalog now has target `kubernetes` at revision 1, selecting the existing
published Kubernetes corpus with exact default edition `1.36`. This is an explicit
operator choice, not automatic selection of the latest version. Overrides `1.34`
and `1.35` remain exact selections inside that same generation.

## Setup and ownership

The existing droplet runs PostgreSQL 16.15; the catalog schema belongs to
`concordance_owner`. Read-only preflight found no target table. The reviewed
`deploy/postgres/install-targets.sql` reused `internal/catalog/targets.sql` in one
transaction and applied `target-grants.sql`. The administrator session used a
five-second lock timeout and 30-second statement timeout, without changing server
settings. Owner, foreign-key binding and narrow runtime grants were verified;
existing catalog-table ACLs stayed identical.

The runtime has SELECT, INSERT and UPDATE only on collection, generation, default
edition and revision. It cannot update the name, delete/truncate the table or create
tables. These are accidental-operation safeguards for a trusted writer, not
untrusted-user authorization. No roles, HBA rules, Receipt permissions/data, global
configuration or service restarts changed.

Local PostgreSQL 14.17 checks exercised wrong-database refusal, rollback after a
simulated grant-role failure following CREATE, unchanged base ACLs, permitted
runtime reads/inserts/choice updates and denied DELETE/TRUNCATE/name UPDATE/CREATE.
An unpublished generation failed the foreign key; repeating installation failed
without changing the existing target. An isolated no-owner/no-ACL dump restore
required base and target grants before runtime target reads worked. Those destructive
and failure checks used only the local fixture, never the hosted catalog.

## Public command results

The CLI implementation is unchanged from PR 40; the binary was built with Go 1.27.2
on darwin/arm64. Native PostgreSQL service/password files are operator-owned mode
0600, with the existing runtime identity and catalog search path. An owned SSH
forward used the existing pinned host key and strict checking. Existing private R2
settings were passed to the child process without executing the configuration as
shell code or placing credentials in arguments. The forward closed after checks.

`target activate` used the original published generation, default `1.36` and
expected revision zero. Its strict default preflight succeeded, then creation
returned revision 1. A deliberate identical retry returned that same value without
advancing the revision. `target show` worked with all R2 settings removed.

The query was `ContainerRestartRules`. Default and explicit `1.36` produced identical
complete output. Every successful query returned two passages from
`content/en/docs/concepts/workloads/pods/pod-lifecycle.md`:

| Edition | Captured Git revision | Quote bytes in ranked order |
| --- | --- | --- |
| Default / 1.36 | `1de955ebabe7e17da1ebb4f582635491227f4157` | 104, 285 |
| 1.35 | `8817d358864b70a9710c5ad34f78df6a239be33d` | 104, 285 |
| 1.34 | `47132eb5367cfdcba0a3b33a0b3ffc81185bd7e8` | 106, 285 |

Decoded display quotes matched the original strict-reader SHA-256 records for each
edition and ranked position, independently of the new command output. Requesting
absent `1.33` failed with zero stdout bytes; a nonexistent term succeeded with zero
matches. A final catalog-only show remained revision 1/default `1.36` after all
queries. No import, object write/delete, receipt write or publication change ran.
These checks establish exact source/quote preservation, not reader-judged relevance,
provider-fault coverage, request counts or latency. Each command opened fresh clients;
no new cache or retrieval algorithm was introduced.

## Backup and limits

A fresh server-version `pg_dump -Fc` included the new target-table data and existing
catalog. It was encrypted on the server using the existing recipient certificate,
CMS AES-256-GCM and RSA-OAEP/SHA-256 before off-host retention. The private recovery
key stayed off the server. Local decryption reproduced the exact original dump hash
and a PostgreSQL archive header; temporary server files were removed. Prior encrypted
archives, key custody and backup references were preserved. This new archive is a
private local off-host copy, not a new R2 backup upload or complete restore drill.
The earlier synthetic isolated restore result remains separate. Scheduling, a second
protected key copy, loss/recovery targets and simultaneous loss of both stores remain
open. Automatic deletion and automatic default activation remain disabled.

## Selected evidence

Private `.tools/reviews/hosted-target-results.json` records the functional checks;
`hosted-target-*.stdout`/`.stderr` retain actual public-command output.
`hosted-target-sql-checks.log` covers isolated SQL failures and restore grants;
`hosted-target-verify-1.log` records full local `make verify` including uncached race
tests, vet and build. Pre-write code/architecture reviews are bound to
`hosted-target-prelive-evidence.txt`. `hosted-target-backup-check.json` records backup
hash checks. Exact activation identity/profile paths remain in the protected operator
configuration; no credentials, host or live collection/generation routing is committed.
The original saved import operation and capture remain unchanged recovery material.
