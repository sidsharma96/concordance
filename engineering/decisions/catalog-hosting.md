# Catalog hosting for live experiments

Use the existing DigitalOcean droplet's PostgreSQL server for the first synthetic
publication and catalog-recovery experiments. The user selected this instead of a
new managed database account. This is a learning deployment, not a production
availability or capacity promise.

## Boundary

R2 still holds immutable artifacts. PostgreSQL remains the authority for published
generation references. Hosting the catalog on a droplet does not change that split.

Give Concordance its own database, a NOLOGIN owner and a restricted runtime login.
Keep PostgreSQL listening on loopback, reach it through an authenticated SSH tunnel,
and keep the runtime pool at two connections with a role limit of four. An HBA rule
admits that login only to Concordance: privileges granted through PUBLIC would
otherwise allow connections to other databases. Preserve other applications' rules.
The runtime gets existing catalog DML, not schema ownership or deletion privileges.
It is a **trusted writer**: narrow SQL grants do not enforce immutable publications
or defend against a compromised writer inserting false receipts. The catalog APIs
preserve those invariants; this deployment adds no adversarial SQL boundary.

The database shares CPU, memory, disk and outages with Receipt. Dedicated database
names and grants separate access, not resources. Start with small synthetic fixtures;
revisit hosting before unattended ingestion, material resource contention, sensitive
private connectors or stronger uptime requirements.

## Recovery

Keep an encrypted, single-database logical backup off the droplet, with its recovery
key stored separately. A successful dump is not a successful recovery. Restore into
an inaccessible clone; quarantine every restored object-verification namespace before
any application use. Old receipts must not authorize new publications after rollback.
Already-published references can still be read using full object-body verification.
The drill requires surviving R2 objects; it does not recover a lost object store.

A one-off backup/restore establishes one tested recovery point. Backup scheduling,
alerts, retention, a second protected recovery-key copy and acceptable data-loss and
recovery-time targets remain separate operational work. Do not imply these are solved
by a droplet snapshot or one successful drill. Automatic R2 deletion stays disabled.

## Evidence and alternative

PostgreSQL 16's [HBA rules](https://www.postgresql.org/docs/16/auth-pg-hba-conf.html)
use the first matching record without fallthrough and support included rule files.
Its [grants](https://www.postgresql.org/docs/16/sql-grant.html) are additive, including
PUBLIC privileges. These mechanisms support a rule scoped to the new login without
changing Receipt's privileges. Its [pg_dump](https://www.postgresql.org/docs/16/app-pgdump.html)
can make a consistent single-database archive while the database is in use; roles
are separate. [pg_restore](https://www.postgresql.org/docs/16/app-pgrestore.html)
allows an isolated target. These documented mechanisms do not establish resource
isolation, backup freshness or production recovery objectives for our workload.

A separate managed PostgreSQL service would reduce server administration and shared
host risk, while adding an account, provider limits and potentially cost. Local-only
PostgreSQL remains the test baseline but misses the live network and host boundary.
The existing droplet fits this small experiment; it is not evidence for a large
production corpus. See the [operator procedure](../../deploy/postgres/README.md).
