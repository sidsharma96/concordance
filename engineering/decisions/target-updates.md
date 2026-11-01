# Change a named search target without losing another operator's update

Status: accepted design for [catalog targets](search-targets.md). The bounded
internal catalog and application operations are implemented. No hosted schema
change or public command exists yet.

## Workload, ownership and invariants

One trusted catalog schema serves the first documentation collection. An operator
explicitly makes a published generation/default edition available under a friendly
target name. Search resolves that name once to an owned value, then uses the
existing exact-generation reader. Old readers do not change when the name moves.
No import automatically changes a target; no target deletion, background cache,
automatic version sorting or retention mechanism is introduced.

Target names are unique within the configured application-owned catalog schema,
not globally across database installations. The internal caller authorizes target
use and the resulting collection before object access, and owns trusted routing,
deadlines and concurrent-call admission. A target or revision is not authorization.
Future public/multi-tenant services must define their naming/permission scope before
exposing these calls to untrusted clients.

Keep the collection, generation and default edition as one atomic tuple. Each
identifier is exact bounded UTF-8 text, at most 256 bytes with no NUL. Revision is
a positive signed 64-bit counter. Generation must be a catalog-published reference;
the application must check the default edition before installing the tuple. Every
error returns a zero target or selection, never a partial result.

## Catalog representation and compare-and-set

A separate `concordance_search_targets` table has name as primary key, the three
selection fields, and revision. Its collection/generation pair references the
existing unique published-generation pair in `concordance_imports`; an unpublished
binding cannot satisfy that foreign key. It stores no object payload or credentials.
`CreateTargetSchema` is explicit additive setup after the import table exists;
existing target tables are errors rather than assumed-compatible migrations.

The catalog receives a `TargetUpdate`: name, expected revision, and desired tuple.
Expected revision zero means create only if absent, producing revision one.
Otherwise an UPDATE replaces the tuple only where the stored revision equals the
expected revision, producing expected+1. A stale expected revision cannot overwrite
another operator's change. The counter never wraps; the maximum stays readable
but cannot be advanced. There is no deletion/reset API that reuses old revisions.

These are single-statement autocommit writes, with bounded RETURNING metadata.
If no row is returned, read the current target. Treat it as the same desired change
only if its revision is exactly expected+1 and its entire tuple matches. Concurrent
identical requests may therefore both observe the same result; this proves the
state, not which caller wrote it. Any other current revision/value is a conflict.
Missing state under a nonzero expected revision is also a conflict, not permission
to create it. Ordinary lookup absence remains distinct from database unavailability.

If a write/acknowledgment error cannot establish the outcome, preserve a dedicated
unknown-outcome error. The operator retains the expected revision and desired tuple
for retry or explicitly resolves the current target. An exact retry never advances
the counter twice. If a later operator has already changed the target again, an old
retry conflicts and cannot restore its old choice. There is no claim to recover
the full history of superseded updates or distinguish identical competing callers.

For example, two operators read revision four. One installs B at five. A stale
request for C using four conflicts. An identical B/four retry can observe B/five.
After a subsequent explicit change reaches six, the old B/four request conflicts
even if the tuple at six happens to equal B. This avoids treating a later return
to the same contents as the original update. Compare against a blind last-writer
update, which could silently replace a later operator's selection.

## Actual application boundary

Use a small concrete `internal/searchtarget` coordinator rather than making the
read-only `publishedsearch` package own activation policy. The catalog's SetTarget
is trusted metadata plumbing, analogous to its existing Publish method: it cannot
prove edition existence from SQL alone. Its documented caller precondition is
fulfilled by `searchtarget.Activate`.

Activation admits update identifiers/revision locally, then calls the existing
`publishedsearch.Open` for the candidate tuple. That validates published status,
the physical manifest key, declared byte caps, strict root/index bodies and exact
default-edition bindings. It performs no receipt mutation or object writes and
starts no background work. Then SetTarget performs the atomic conditional catalog
write. Immutable published metadata and disabled deletion preserve the association;
the foreign key independently prevents a missing generation at write time. The
caller keeps routes/clients stable through return. No remote read is held inside a
database transaction, and a later provider outage can still prevent a future open.

Using Open normally costs two existing bounded object GETs per activation attempt
(before any client retries or redirects) and
transient manifest/index state, but avoids a second implementation of those safety
checks. First activation has one generation lookup plus one target write; a stale
or identical retry may also need one target read. Preflight may fail before
recovering an earlier update during a provider outage; resolving current catalog
state remains available separately. This is not a durable operation journal.

Resolution reads one target row and returns a `publishedsearch.Selection`. Empty
requested edition uses the saved default; a supplied bounded edition overrides
that one field. The caller then opens the exact selection, which rejects missing
editions without fallback. Resolve does not claim that an override exists until
Open validates it. The selected target generation determines the available editions;
retained older generations are not silently searched to find a missing version.

## Research, costs and validation

Kubernetes' [resource-version contract](https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions)
and Elasticsearch 8.19's [conditional updates](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/optimistic-concurrency-control.html)
reject stale writes instead of silently losing another writer's change. Kubernetes
resource versions are opaque and Elasticsearch also uses a primary term. We do
not copy either distributed protocol: this is one PostgreSQL row with an explicit
local monotonic counter and a narrow identical-state retry rule.

For T target names and P published imports, primary/foreign-key B-trees perform
logarithmic indexed lookup/update work and return bounded scalar metadata. No
generation or target enumeration is needed. Remote activation reads are bounded
by the existing reader; catalog connection limits alone do not bound waiting callers.
Revisit a separate checked-manifest activation path only with measured cost and
equivalent safety checks. No measured latency or million-document claim is made.

Validate against real dedicated local PostgreSQL: creation, switching, saved tuples
and old readers, conflicting and identical concurrent operators, exact replay,
superseded retry, revision exhaustion, unpublished/missing generations, invalid
metadata, pre-cancellation and unavailable databases. Application tests use real
retained article/index/manifest bytes with an HTTP object fixture to reject absent
or corrupt default editions before target writes. An actual lost network reply is
not implied by reconnecting after discarding a successful caller result.

No hosted migration, default edition, CLI result format or connection configuration
is selected by these internal primitives. Review their implementation independently
before the authorized source PR and merge. The next checkpoint remains the public
search command's configuration/output behavior.
