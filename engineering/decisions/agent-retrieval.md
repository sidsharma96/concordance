# Agent retrieval, then recorded-work discovery

Status: accepted project direction. Build retrieval over deliberately captured
collections, starting with documentation, then test the boundary with discovery
of agent work recorded by Rewind. This preserves the documentation/RFC and Go
algorithm, storage and recovery learning goals. It does not select a new API,
source schema, backend or deployment.

## Outcome and ownership

An agent should obtain relevant evidence from the intended captured source, under
an explicit scope and work budget. One first task is finding the documentation
for a selected product edition. A later task is finding similar failed tool calls
within an authorized project and inspecting their exact recorded responses.

Concordance owns retrieval and its evidence bindings. Rewind continues to own
record acceptance, current reader permission, expiration and recorded playback.
An exact known-operation lookup can use Rewind directly. A discovery index does
not replace that authority, and its absence of results cannot establish that an
operation never happened. A future asynchronous index needs explicit visibility,
retry and recovery rules before it can make freshness claims.

Documentation editions, captured source revisions, generation membership and
Rewind execution IDs have different meanings. Keep their source-specific
identities and locations explicit. Do not encode operational records as HTML or
turn authorization into query words. Current permission must govern access to
private candidate text and model inputs, not just the final displayed results.

The current implementation remains the bounded Git/HTML snapshot path described
in the [architecture](../ARCHITECTURE.md). Its manifest, importer and published
reader are concretely tied to that format. A second source must prove a clean
boundary rather than accumulate provider conditions in shared retrieval code.
No universal document model or plugin framework is selected by this direction.

## Competing formulations and alternatives

The useful quantity is the time and cost until an agent has enough correct
evidence for its task. A slow or unsuccessful task can come from missing source
content, candidate eligibility, ranking, opening an index, fetching and checking
bodies, model work or excessive returned context. Another model or database does
not fix all of these problems. Exact lookup or a narrower authorized scope can
also remove the need for a relevance search.

Keeping today's trusted-operator CLI is the smallest working control. It already
returns verified passages but its text is not a stable machine interface. The
accepted direction adds an actual agent consumer before choosing a machine
contract. It does not authorize parsing that text as a durable result schema.

Live-web integration is a credible alternative when the agent needs information
outside retained collections. Existing search and extraction services can avoid
building global coverage. Their adoption, cost, freshness, failure and data-handling
contracts remain obligations. A URL or extracted excerpt alone does not establish
membership in our published generation or the retained quote authority.

Rewind-first investigation is also credible: it gives retrieval a concrete
operational consumer. It would make typed project, tool, status and time filters,
index visibility and private-record policy primary work. That is a larger query
and lifecycle contract than another documentation processor. We will use concrete
Rewind questions to challenge boundaries while documentation remains the first
working retrieval task.

Building a general search database is a separate responsibility choice underneath
these products. Incremental writes, selective persistent reads, cache admission,
compaction, retention and operations are substantial work. Using an existing
backend can shorten application work but replaces some deliberate implementation
learning. Keep the current scans and postings as correctness baselines; compare
existing systems on the same workload and guarantees before selecting a backend
or claiming a performance advantage.

## Primary examples and limits

[Exa Search](https://exa.ai/docs/reference/search) and
[Contents](https://exa.ai/docs/reference/get-contents) expose discovery, highlights
and deeper content access. Its
[Snapshot contract](https://exa.ai/docs/search/snapshot) supports stored historical
content and explicitly distinguishes that cutoff from current candidate-retrieval
signals. Historical versions alone are not a unique product claim. The inspected
[ContentsOptions](https://github.com/exa-labs/exa-js/blob/53d3539006c0fad341e42c821b6d36ff144c4eb9/src/index.ts#L55-L98)
separates text/highlights, freshness and historical selection. This is a public
client contract, not evidence of its proprietary index implementation.

[Parallel Search](https://docs.parallel.ai/search/search-quickstart) returns
objective-focused excerpts. Its pinned
[SearchParams and excerpt settings](https://github.com/parallel-web/parallel-sdk-typescript/blob/e802ec7de05cac7eef6f1b3d10eecb9d7431840c/src/resources/top-level.ts)
include character budgets and search modes. Characters, encoded bytes and model
tokens are different quantities. Its
[evaluation guidance](https://docs.parallel.ai/search/evaluating-search) compares
complete tasks with a controlled harness. We have not benchmarked these services
or inferred an appropriate latency target from their published figures.

[Browserbase Search](https://docs.browserbase.com/platform/search/overview) can use
Exa or Parallel as its provider. The pinned
[Search.web and SearchWebParams](https://github.com/browserbase/sdk-node/blob/90f6295c3c90640a5691c0e16ccd083c5a617242/src/resources/search.ts)
make that composition explicit. Its separate
[Fetch contract](https://docs.browserbase.com/platform/fetch/overview) identifies
limits such as no JavaScript execution and directs interactive work to browsers.
This supports separating acquisition responsibilities; it does not prove that
conversion preserves our passage and source guarantees. Browser recordings are
also different from Rewind's exact saved tool-response playback.

[Turbopuffer's architecture](https://turbopuffer.com/docs/architecture) and
[postings design](https://turbopuffer.com/blog/fts-v2-postings) remain useful
mechanism references for object-backed indexes, filtering and caching. Their
workloads do not select a storage engine for Concordance. Keep the existing
[read-path analysis](../experiments/read-path-research.md) and its limits.

## Response screening and next checkpoint

Before a descriptive byte audit, the provisional recommendation was a small set
of complete verified passages in the first response, with deeper reads available
when needed. The frozen check counted illustrative compact UTF-8 JSON from the
already recorded development reranking results. It ran no new queries, inference,
relevance grading or timings and used no reserved cases.

For 60 ordinary-question cases, five passages plus selected metadata used a median
4,104.5 bytes, a nearest-rank 95th percentile of 7,269 bytes and a maximum of
9,101 bytes. None exceeded an illustrative 16 KiB budget; 30 exceeded 4 KiB.
These cases repeat 20 needs across three editions. The representation omits future
wire-envelope details and is neither the current text CLI nor a supported schema.
It establishes no token budget, latency, answer completeness or trace/RFC capacity.

The result retained the provisional response direction. It supplies no reason to
require another read call for every result or introduce a new excerpting model.
Actual limits and oversize behavior still need a deliberate consumer contract.
The CLI default remains ten results; the audit's five is not a product setting.
Current complete-match admission, exact edition selection and all-or-zero strict
quote verification remain unchanged. Do not truncate a retained authored unit to
meet a new presentation budget silently.

The first entry point is now optional CLI JSON, with its
[machine result and failure contract](agent-search-json.md). The next checkpoint
is evaluating complete tasks in an actual selected agent client. Existing reader handles
are temporary, not persisted citation or authorization tokens. A later read must
bind exact retained evidence, not resolve a friendly target again and accidentally
read a newer source.

Measure a complete concrete task before optimizing: source availability, candidate
coverage, ranking, cold opening, body verification, model work and output. Compare
misses, updates and failures as well as successful warm queries. Choose latency,
freshness, concurrency and size requirements with the user; company headcount is
not a workload. Reopen this direction if actual tasks primarily need uncaptured
live information, operational discovery becomes the primary requirement, or an
established backend meets the same guarantees with materially lower total cost.
