# PostgreSQL catalog setup

These files provision a dedicated Concordance catalog on a PostgreSQL server. They
were exercised on PostgreSQL 16 on a small remote host reached through SSH.
They are not migrations and must not run against an existing Concordance database.
Run them as the server's administrator; leave other databases on a shared server alone.
Keep host names, credentials, backup references and execution evidence outside Git.

## Initial setup

1. For a remote server, verify the SSH host against its trusted key. Check the live
   server's version, database/role names, free disk/memory, service health and HBA
   parse errors.
   Preserve a private copy of the current HBA file. Do not disable host checking.
2. From a checkout with the same layout, run `psql -X -v ON_ERROR_STOP=1 -d postgres
   -f deploy/postgres/bootstrap.sql` as the PostgreSQL administrator. It uses the
   existing catalog schemas, creates the restricted roles and leaves LOGIN disabled.
   Setup spans multiple transactions; inspect partial state after any error.
3. Install `concordance-access.conf` beside the actual `SHOW hba_file` path. Prepend
   an `include concordance-access.conf` directive before existing rules, preserving
   them. Check `pg_hba_file_rules` has no errors, then reload configuration without
   restarting PostgreSQL. On failure restore the saved HBA and reload it. Restrict
   write access to administrator-owned configuration files.
4. In a private administrator session, set `password_encryption = 'scram-sha-256'`
   and use psql's `\password concordance_app`; do not put passwords in shell arguments,
   SQL history, logs or source. Grant `CONNECT ON DATABASE concordance TO
   concordance_app`, then `ALTER ROLE concordance_app LOGIN`. Store the credential
   in a private secret file or password manager. Do not give the runtime owner-role
   membership. Verify real authentication before relying on the setup.
5. Use a local loopback SSH forward to the server's loopback PostgreSQL port, with
   strict host checking and `ExitOnForwardFailure`. Close the owned tunnel after
   use. Configure pgx's database `concordance`, user `concordance_app`, search path
   `catalog` and `MaxConns=2`. Do not expose PostgreSQL publicly. SSH protects the
   forwarded transport; a local `sslmode=disable` connection relies on that tunnel.
6. Test a real publication/retry and a fresh reader. Also test wrong-password and
   other-database rejection, denied table/schema creation and deletion, and the
   four-connection limit. Confirm other databases on the server remain healthy
   and PostgreSQL still listens only on loopback. Never run schema-creating test
   suites against this DB.

The writer is trusted to use the catalog APIs. Its SQL privileges permit the DML
those APIs need; they do not protect the catalog against a compromised writer.
The role limit counts sessions, not waiting callers or per-query memory.

## Add named search targets

For an existing catalog, connect as its administrator and run from this checkout:

```sh
psql -X -v ON_ERROR_STOP=1 -d concordance -f deploy/postgres/install-targets.sql
```

The script checks the database name and catalog owner, then creates the existing
`internal/catalog/targets.sql` table and applies its grants in one transaction.
It changes no existing catalog tables, login settings, HBA rules or object data.
An existing target table is an error; inspect it rather than rerunning bootstrap
or assuming its schema matches. Failure before commit rolls back this addition.
A lost commit reply still needs inspection of the table, owner and grants.

The runtime receives SELECT, INSERT and UPDATE on the choice/revision columns;
it cannot rename or delete a target, create tables or truncate this table. This
trusted writer can still issue arbitrary permitted SQL. These grants are not an
untrusted-user authorization boundary. Keep target names scoped by the application
and use the CLI's configured collection and expected-revision update guard.

Verify the table owner is `concordance_owner`, the foreign key points at
`catalog.concordance_imports`, and runtime reads/inserts/choice updates work while
DELETE, TRUNCATE and name UPDATE fail. Explicitly activate an already-published
generation and a chosen exact default edition with `concordance target activate`;
creating the table does not select a default. Keep the exact activation flags for
inspection/retry if the reply is lost. See the [operator setup](../../README.md#configure-operator-access).

## Backup and isolated restoration

Use the server's compatible `pg_dump -Fc` for this database alone. Encrypt before
sending the archive off the host, keeping the decryption key off the host. One tested
option is OpenSSL CMS with explicit AES-256-GCM and an RSA-OAEP recipient certificate;
validate decrypt roundtrip, corruption and wrong-key rejection with the installed
versions. An encrypted archive does not authenticate a trustworthy producer by
itself. Retain an independently verified immutable object reference for retrieval.
A backup of another database on the same server does not include Concordance.

Restore only a trusted archive into a uniquely named, administrator-owned clone.
Revoke PUBLIC CONNECT before restoration; do not grant the application CONNECT or
broaden its HBA rules. Use `pg_restore --no-owner --no-acl --exit-on-error` so restored
ownership cannot make the runtime an owner. Before application use, transactionally
set every row in `catalog.concordance_object_namespaces` to `active=false` and
advance its epoch, saturating at bigint maximum. Check every restored row is inactive.
Never reactivate it automatically. This is an administrative restore procedure,
not a runtime migration or automatic recovery implementation.

Reapply `grants.sql` within the clone to test runtime object permissions. If the
backup contains the search-target table, also apply `target-grants.sql`; do not run
`install-targets.sql` against a restored existing table. An admin-only
peer session may `SET ROLE concordance_app` for the test, without enabling runtime
connections to the clone. Confirm namespace binding refuses quarantined state. Read
an existing published reference from the restored catalog and fully verify its R2
manifest, edition and article before resolving the expected quote. Keep production
and restored references separate; do not select a live catalog reference accidentally.
Close probe sessions, drop only the exact ephemeral clone and remove temporary
plaintext dumps. Retain encrypted off-host evidence and key separately.

This tests catalog rollback with surviving objects. It does not establish unattended
backups, key redundancy, object-store disaster recovery, production capacity or an
uptime guarantee. See the [hosting decision](../../engineering/decisions/catalog-hosting.md).
