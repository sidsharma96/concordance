# Concordance

Concordance searches technical documentation by product version and identifies the
exact captured source behind each result. It is written in Go. Code search is a
possible later extension.

The CLI searches previously published Git/HTML generations through named catalog
targets. It is a trusted operator tool that connects directly to PostgreSQL and
Cloudflare R2. Importing is currently an internal Go API, not a public command.

## Getting started

Install Go 1.27 or later, Git and Make, then run from this checkout:

```sh
make build
./bin/concordance --help
./bin/concordance version
```

Help and version need no database or credentials. The version prints `concordance dev`.

## Configure operator access

The catalog must already contain a published generation. An administrator installs
`internal/catalog/targets.sql` in that catalog's schema and grants suitable access.
For the existing droplet layout, use the transactional
[named-target setup](deploy/postgres/README.md#add-named-search-targets).
This explicit setup is separate from search; schema-creating tests must never run
against the hosted catalog. Target names are unique within the configured schema.

Configure these settings locally:

- `PGSERVICE`: a native PostgreSQL connection profile, including the correct database
  and schema/search path. `PGSERVICEFILE` selects its file; use `PGPASSFILE` for a
  password when needed. The existing droplet is reached through an operator-owned
  SSH forward; the CLI does not create one.
- `CONCORDANCE_COLLECTION`: the authorized collection ID from the published import.
- `CONCORDANCE_R2_ENDPOINT` and `CONCORDANCE_R2_BUCKET`: its HTTPS R2 S3 endpoint and
  private bucket.
- `R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY`: bucket-scoped credentials supplied
  through local credential tooling, not source, command arguments or shell history.
  Search and activation need object reads, not object writes.

An operator with catalog target-write permission explicitly activates a generation.
`GENERATION_ID` below is the saved result of that published import; `kubernetes` is
an example friendly name. The PostgreSQL profile names are examples too:

```sh
PGSERVICE=concordance-write ./bin/concordance target activate \
  --generation "$GENERATION_ID" --edition 1.35 --expected-revision 0 kubernetes
```

Zero creates only if the name is absent. Later changes require its current revision
from `target show`. The configured collection guards both reads and target updates.
An activation error, interruption or output failure can conceal an applied change.
Keep the original generation, edition and expected revision for a deliberate retry;
inspect `target show` rather than automatically advancing the revision. Show needs
only `PGSERVICE` and `CONCORDANCE_COLLECTION`, so it works without R2 credentials.

## Search a published target

With an activated target and a catalog-read profile:

```sh
PGSERVICE=concordance-read ./bin/concordance target show kubernetes
PGSERVICE=concordance-read ./bin/concordance search --edition 1.35 kubernetes "ContainerRestartRules"
```

Put flags before the target and pass the query as one quoted argument. Omit
`--edition` to use the target's configured default. Output shows the actual edition,
collection and generation, then each source/document key, captured revision,
retained block range and quote. A block range `[first,after)` includes the first
block and stops before `after`; it identifies saved article text, not Git lines.
Terminal controls, quotes and backslashes display with Go-style escapes.

Search requires every prepared technical term and ranks matches with BM25. It is
lexical retrieval. A missing requested edition fails rather than falling back;
a known-empty edition or no matches succeeds with zero matching passages.

Every quote is verified and the complete display buffered before stdout is written.
A read, validation or formatting failure produces no result body. A later device
failure can still leave partial bytes. Text is for reading, not a stable script or
saved-citation format; JSON is deferred.

`search --help` lists independent query, artifact, count, quote, display and timeout
ceilings. Excess matches or bytes fail rather than truncate. The selected index
and whole generation manifest are loaded into memory. Each hit downloads its whole
bounded article, including repeated hits sharing bytes; there is no article cache.
These limits are not whole-process memory quotas or million-document support.
Other initial structural ceilings are listed in the
[command design](engineering/decisions/operator-search-command.md). The default
match limit can entail up to 1,002 logical GETs; use a lower `--max-matches` to bound
that work. It remains a complete-result ceiling, not a top-k display limit.

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and
[the architecture](engineering/ARCHITECTURE.md) for boundaries and scaling work.
