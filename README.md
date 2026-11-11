# Concordance

Concordance searches technical documentation by product version and identifies the
exact captured source behind each result. Core search is written in Go; optional
reranking uses a local Python worker. Code search is a possible later extension.

The CLI searches previously published Git/HTML generations through named catalog
targets. It is a trusted operator tool that connects directly to PostgreSQL and
Cloudflare R2. A trusted preparer supplies rendered Git/HTML captures; the import
command saves their retry inputs and publishes them.

## Getting started

Install Go 1.27 or later, Git and Make, then run from this checkout:

```sh
make build
./bin/concordance --help
./bin/concordance version
```

Help and version need no database or credentials. The version prints `concordance dev`.

## Configure operator access

The catalog schema must already be installed. Search needs a published generation
and a target; importing additionally needs an explicitly provisioned storage
namespace matching its R2 endpoint, bucket and collection. The CLI creates none of
these schemas or namespaces. An administrator installs
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
  Search and activation need object reads, not object writes. Import also needs
  object creation and the catalog/receipt write privileges from explicit setup.

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

## Import a prepared capture

Import accepts an existing trusted Git/HTML capture, not a website URL or an
instruction to run a renderer. Keep the pinned commits and declared build inputs
in local Git checkouts. In a private source profile, register exactly the sources
named by this capture. For example, save this locally under ignored `.tools/`:

```json
{
  "format": "concordance.git-sources/v1",
  "sources": [{
    "id": "docs",
    "repository": "https://example.invalid/docs.git",
    "path": "/absolute/path/to/checkout"
  }]
}
```

The ID must match the capture source ID; the nonsecret repository identity must
remain identical across retries. The absolute path is trusted operator routing,
never taken from the capture.
The profile is bounded to 1 MiB and 32 sources, with closed required fields.
Keep connection credentials in the native configuration above.

With an existing prepared directory and provisioned catalog/storage namespace:

```sh
mkdir -p .tools/jobs
PGSERVICE=concordance-write ./bin/concordance import start \
  --capture /absolute/path/to/capture --job .tools/jobs/docs-update \
  --sources .tools/sources.json
```

Start refuses an existing job path. It saves a verified HTML/recipe copy and
generated key before publishing. Success prints the catalog's actual generation;
activate that generation separately using the command above. Source preparation
remains responsible for truthful rendered HTML and complete build-input declarations.

Keep the job after errors or interruption. Remove/rebuild the producer directory
only after local preparation is complete; retain the job and exact Git objects.
Retry uses the saved copy and original key:

```sh
PGSERVICE=concordance-write ./bin/concordance import retry \
  --job .tools/jobs/docs-update --sources .tools/sources.json
```

A failed reply or stdout write can conceal successful publication. Retry that same
job instead of starting another one. An incomplete or changed job is refused.
Existing completed results remain recoverable while receipt reuse is quarantined;
new object confirmation/publication remains blocked.

`import --help` lists aggregate byte/count limits, object size, confirmation
concurrency and timeout. Defaults admit bounded work in memory, not a whole-process
memory quota. Additional fixed structural ceilings are in the
[import contract](engineering/decisions/import-command.md). Jobs copy HTML/recipe
inputs, not Git repositories or credentials. No automatic activation, deletion,
managed rendering or background resume is included.

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

Search shows up to ten verified quotes by default. Use `--limit 5` to request
another positive count, or `--all` to quote every admitted match. Enabled `--all`
conflicts with an explicit `--limit`. The header reports shown quotes and total
indexed matches separately; unshown article bodies have not been verified.

Ordinary search requires every prepared technical term and ranks matches with BM25. It is
lexical retrieval. A missing requested edition fails rather than falling back;
a known-empty edition or no matches succeeds with zero matching passages.

Every selected quote is verified and the complete display buffered before stdout
is written.
A read, validation or formatting failure produces no result body. A later device
failure can still leave partial bytes. Text is for reading. Add `--json` before
the target for versioned machine output with exact selection/source details,
body quotes, separate headings/notices and match counts. It also works with
`--rerank`. Check the exit status before parsing: errors use stderr and return
no success object. Empty successful results contain `results: []`. Result labels
such as `r1` belong only to their response. Use `source_ref` to read surrounding
source from that exact capture:

```sh
./bin/concordance read --scope section "$SOURCE_REF"
```

Copy `source_ref` from a JSON result into `SOURCE_REF`. Read returns JSON with the
exact source, body and `navigation.previous`, `next` and `parent` references. Follow
one with `concordance read "$SOURCE_REF"`; null means unavailable. References do
not grant access, and activating a newer target does not redirect old references.
Oversized sections fail explicitly; `read --help` lists the limits. Each fresh read
loads the manifest, index and whole article even when returning a small section.
See the [source-read contract](engineering/decisions/captured-source-reading.md).

See the [JSON contract](engineering/decisions/agent-search-json.md) for fields and
compatibility. Ordinary JSON additionally verifies canonical heading/notice
context, capped at 64 KiB per passage; text keeps its quote-only path.

`search --help` lists independent query, artifact, count, quote, display and timeout
ceilings. Excess candidate matches or bytes fail rather than truncate. The quote
limit selects a ranked prefix after complete candidate admission. The selected index
and whole generation manifest are loaded into memory. JSON search downloads each distinct selected article once per request. Text search
still reads each selected hit separately. Neither retains an article cache across calls.
These limits are not whole-process memory quotas or million-document support.
Other initial structural ceilings are listed in the
[command design](engineering/decisions/operator-search-command.md). The default
ten quotes require at most twelve logical GETs in a fresh command: the manifest,
index and ten articles. `--all` can entail up to 1,002 under the default candidate
ceiling. `--max-matches` still bounds the complete ranking; exceeding it is an
error even when `--limit` is smaller. The quote limit does not reduce index loading
or complete matching and sorting.

## Search with the local reranker

The optional `--rerank` mode accepts ordinary questions, retrieves up to 100 any-word
keyword candidates and uses the pinned local model to reorder them. It returns
verified passages and source references, not generated answers. Candidate retrieval
is still lexical: the model cannot recover a passage outside that pool.

The real worker is currently tested on an Apple M1 Mac with 16 GiB and MPS. It
requires Python 3.14.2 and the pinned dependencies; startup refuses another device
or incompatible assets/runtime. From this checkout, prepare it once:

```sh
python3.14 -m venv .tools/reranking/venv
.tools/reranking/venv/bin/python -m pip install -r worker/requirements.txt
go run ./tools/fetchreranker
```

The fetcher obtains about 2.3 GB of public model assets from an exact revision. It
verifies existing files locally and refuses to overwrite mismatches. Model files,
Python environment and caches stay under ignored `.tools/`.

In a separate terminal, start the persistent worker. The launcher passes a minimal
environment rather than forwarding database/cloud credentials:

```sh
./bin/concordance worker --socket "$PWD/.tools/reranking/run/worker.sock"
```

With normal catalog/R2 operator access configured, query from another terminal:

```sh
PGSERVICE=concordance-read ./bin/concordance search --rerank --edition 1.35 \
  --worker-socket "$PWD/.tools/reranking/run/worker.sock" kubernetes \
  "Why is my Pod stuck in Pending?"
```

`CONCORDANCE_WORKER_SOCKET` can supply the socket instead of the flag. Reranking
shows ten passages by default; `--limit` selects at most 100 after the whole candidate
pool is scored. `--all` conflicts with this fixed pool. The output identifies model
reranking and distinguishes scored candidates, displayed passages and total matches.

Each distinct candidate article is strictly downloaded once within the request;
every passage still receives independent verification/decoding. The default distinct
article budget is 64 MiB (`--max-candidate-article-bytes`), candidate evidence is
bounded separately, and the worker admits complete untruncated inputs within its
byte/token limits. The timeout defaults to five minutes for this mode. These bounds
are not process/GPU memory quotas; the loaded model occupies GiB of memory.

A busy, missing, incompatible or failed worker is an error without lexical fallback.
An empty candidate set succeeds without contacting a worker. Ctrl-C/SIGTERM requests
graceful worker draining: an active tensor operation can finish before exit. Startup
refuses an existing socket path; inspect a stale worker/path before removing it.
See the [worker contract](engineering/decisions/worker-protocol.md) for precise bounds.

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and
[the architecture](engineering/ARCHITECTURE.md) for boundaries and scaling work.
