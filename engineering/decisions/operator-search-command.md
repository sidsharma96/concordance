# First search entry point: a trusted operator command

Status: accepted direct-client option A and human-readable output first. The first supported search command runs
on a trusted operator's machine and connects to PostgreSQL and R2. An authenticated
HTTP search service remains a later boundary for ordinary users. The commands are implemented and exercised against local PostgreSQL/HTTP
fixtures. The [hosted setup and public CLI check](../experiments/hosted-search-target.md)
also pass against the existing corpus, with explicit default edition `1.36`.

## Accepted responsibility

The operator already has authorized catalog and object-store access. A friendly
name resolves once to the catalog-selected collection, generation and exact
edition. An explicit edition overrides the saved default without changing it.
Reuse the existing textterms preparation, published reader and SearchQuoted batch;
keep complete matching/ranking and all-or-zero quote verification unchanged.

A successful command must show the edition actually searched and sufficient
source/provenance information to identify its retained passages. A known-empty
selection remains success, while a missing selection or any read/verification
failure is an error. Generation IDs remain operator metadata, not required normal
search arguments. Target activation is explicit and separate from search/import.

Do not distribute backend-store credentials as the eventual end-user permission
model. A future service can own those clients, authenticate callers and authorize
collection access before storage. The current direct command is for trusted
operators, not a private-document multi-tenant service.

## Command composition

The search command is:

```text
concordance search --edition 1.35 kubernetes "restart policy"
```

Use Go's standard flag handling, as the existing tools do. Keep argument validation,
trusted local connection setup, query preparation and output rendering at the
command boundary. Do not create a generic service/repository or plugin framework.
The command owns a signal/deadline context, a bounded database pool and HTTP
transport; it closes its pool and idle connections on every path. It does not
spawn SSH, load shell scripts, run imports or alter schemas during search.

Reuse the locked pgx driver's PostgreSQL service/password-file configuration.
Existing R2 probes use CONCORDANCE_R2_ENDPOINT, CONCORDANCE_R2_BUCKET,
R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY. A configured collection scope must be
checked against the resolved target before object access. A target name is not
permission, and neither endpoints nor credentials come from query text or
catalog-controlled routes. No secret command-line flags or automatic shell-file
execution is proposed. Concrete connection settings and initial limits are specified below and require
implementation review with the consumed client APIs.

Prepare the raw query under the existing technical-term version before opening
objects; reject empty/punctuation-only queries deliberately. Resolve the name
once, check the authorized collection, open that exact reader, and assemble all
quotes with SearchQuoted. Resolve hit metadata using that same reader. Format
into a separate bounded buffer only after validation/collection succeeds, then
write it with short-write/error checking. No result body escapes on a data or
formatting failure. A later output-device failure may still leave partial bytes.

The public command needs positive query/count, manifest/index/article, quote,
encoded-output and timeout bounds. Their meanings are distinct. Initial settings
must admit the published 38-document corpus and be documented as operator
admission settings, not heap or million-document guarantees. Do not silently
truncate matches or quotes. No performance conclusion follows from the design.

## Accepted output and concrete implementation

The user selected human-readable text first. JSON is deferred until its versioned
result/citation fields have an actual consumer and a deliberate compatibility
contract. The text display is not a stable script format or durable citation codec.
Keep the existing separate library APIs available to programmatic callers.

Implement in cmd/concordance with concrete private command/configuration/rendering
functions, not another service package. The existing run(args, out) dispatches
search through a signal context. An injectable private connection function is the
one test substitution: production creates actual pgx/S3 clients, integration tests
supply actual isolated PostgreSQL and local HTTP object clients. No simulated
reader success or new general test framework is needed.

Search takes exactly a friendly target and one quoted query argument, with flags
before the target. --edition overrides the saved default; an explicitly empty
edition is invalid. --timeout defaults to60s. Require explicit PGSERVICE and use
pgxpool.ParseConfig with a URL-encoded explicit service selector so native
PGSERVICEFILE/PGPASSFILE and standard connection
settings retain their driver's precedence. Set pool MaxConns=2, MinConns=0 and
MinIdleConns=0. Keep the service/password files operator-owned trusted local
configuration, not a hostile-filesystem sandbox or whole-process memory quota.

Require CONCORDANCE_COLLECTION as the authorized single-collection scope, plus
the four existing R2 settings named above. Validate bounded nonempty UTF-8 values,
reject NUL, and require an HTTPS R2 S3 endpoint without userinfo/path/query/fragment
or a nondefault port. Configure a private HTTP transport with20s client timeout,
bounded response headers and idle connections; use the existing SDK infrastructure
and no new dependency. Close the database pool and idle transport on success and
failure. Connectors return owned clients and one cleanup function; all query work
remains synchronous. The command does not retain clients after return.

Expose byte ceilings for manifest/index/article (8MiB each), edition-wide summed
article bytes (64MiB, distinct from one quoted article), total prepared input
(16MiB), query input (4KiB), quote (64KiB), aggregate quotes (8MiB) and formatted
output (16MiB). Expose final matches (1000), passage count (20000), document count
(10000), term occurrences (1000000) and query terms (64). Limits reject excess,
not truncate. Remaining initial structural ceilings are64sources/editions,
4096captures,20000files/images,8192-byte metadata strings,200000blocks/groups,
512group depth and20000vocabulary terms; detailed fixed bounds remain named in
code and documented. They are bounded operator settings, not a universal product
capacity. Use the existing real-corpus reader's compatible field/string versions.
Normally H matching passages cause H+2 logical GET operations: manifest, selected
index and each quoted article. With default1000matches and8MiB article cap,
successful article body reads alone can approach8GiB (derived upper bound, not a
measurement); the60s deadline and real provider conditions may stop earlier.
This is neither cumulative transfer admission nor an HTTP-redirect/header bound.
Use a lower complete-match limit to bound requests; do not add an unmeasured cache.

The prior live reader admitted this first corpus with lower compatible component
counts/bytes. Local command execution is now exercised through the real
PostgreSQL/HTTP pipeline; the hosted experiment separately checks exact corpus quotes.

The header shows target, exact edition, collection and generation once. Each hit
shows source ID/native document key, captured revision and retained block range,
then all quote lines in ranked order. Do not guess original Git line numbers or
reconstruct a current website URL. Escape terminal-control characters in all
source-controlled strings with standard-library string quoting; explain the
visible escape convention. A separate bounded io.Writer-compatible byte buffer
admits every formatted write before appending. Render locations serially instead
of retaining another collection of copied image/provenance metadata. Check context
between hits/lines and immediately before the final write. All data, metadata,
formatting and deadline failures return before any result body is written. Check
short final writes and preserve device errors, without claiming atomic stdout.

Wrap infrastructure causes in an error whose displayed message is safe and whose
Unwrap retains errors.Is classification. Do not print raw driver configuration,
credential, provider or query-bearing parse errors. Help/version require no
credentials and write errors remain observable. A successful no-match/known-empty
query exits successfully; errors retain the runtime's unsuccessful-exit behavior.

For an operable setup/recovery path also expose `target show NAME` and
`target activate --generation G --edition E --expected-revision N NAME`.
Activation is only command wiring for the accepted searchtarget.Activate/CAS
protocol: read/verify the candidate, then conditionally update its target. Scope
comes from CONCORDANCE_COLLECTION, not a second untrusted route. Add optional
TargetUpdate.ExpectedCollection to the trusted catalog API. Nonempty values must
be valid identifiers and the SQL UPDATE additionally requires the current row
collection to equal that scope. The CLI always sets it to its configured scope;
empty preserves existing trusted in-process callers. Create-if-absent remains
unchanged, and the existing exact-next/full-value retry check grants no ability
to change an out-of-scope row. Failed scope predicates return a conflict, without
exporting the other collection's metadata. This is a capability needed by the
new command, not a new authorization or naming system. It needs catalog
write privileges but still performs no object writes. Show needs only the native
catalog service and collection scope, never R2 credentials or access; it can inspect
an earlier uncertain update during object-store outage. Both own a deadline and
bounded safe scalar text output (32KiB). Activation errors, cancellation or output
write failure do not promise that the target was unchanged; retain exact generation,
edition and expected revision and inspect show/retry deliberately. Do not auto-retry
with a new revision. No target deletion or schema-creating command is introduced.

The admin setup uses deploy/postgres/install-targets.sql to apply the existing
internal/catalog/targets.sql with owner and column grants transactionally, outside search. Document this prerequisite rather than
requiring unpublished private helper programs. Exposing the existing explicit
activation policy is routine public operator wiring, not a newly chosen default.

Read-only search does not install the target table or select a hosted default.
Those require explicit operator setup. The hosted experiment now installs that
table and activates the existing corpus; synthetic fixture checks remain separate. Verify failure on the second quote leaves stdout empty, rendering overflow
leaves stdout empty, cross-collection targets cause no object GET, explicit missing
edition never falls back, defaults and captured provenance are visible, control
characters cannot issue terminal commands, deadline/cancellation propagates, and
short-writer errors survive. Test real config/pool cleanup separately from the
local fixture connector. No timing or live-provider claim follows from local tests.

## Primary examples and limits

PostgreSQL16 [psql](https://www.postgresql.org/docs/16/app-psql.html) is a terminal
frontend connecting directly to the database. Its [service files](https://www.postgresql.org/docs/16/libpq-pgservice.html)
name connection settings independently of command code. The locked pgx v5.11.0
pgconn.ParseConfig consumes PGSERVICE, PGSERVICEFILE and PGPASSFILE; verify that
local dependency contract when wiring the command. This is an operator-client
analogy, not a claim that all search users should receive database access.

Elasticsearch8.19's [Search API](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/search-search.html)
accepts HTTP queries and requires index-read privilege. It illustrates the future
service-mediated access model, not a reason to add its distributed machinery or
claim a particular latency advantage. Both connection models can reuse our reader.

GitHub CLI's [formatting guide](https://cli.github.com/manual/gh_help_formatting)
documents plain text by default and optional JSON fields for supported commands.
Its mature command suite has real scripting consumers. That supports option B as
a credible product direction; it does not select Concordance's retained-passage
schema or prove both formats are needed before the first operator command.
