# Agent search through optional JSON

Status: accepted and implemented CLI contract. The
maintainer selected optional JSON from the existing trusted-operator command after
reviewing the concrete evidence proposal. This does not add an HTTP/MCP service,
ordinary-user authentication, durable citation resolver or second source format.

## Workload and alternatives

A local agent with shell access asks for guidance in one product edition and
needs exact captured evidence, rather than terminal display text to parse. Keep
the default human output as the control. Another encoding cannot repair missing
source text, candidate eligibility or poor ranking. Repeated opening, body checks
and model work remain separate performance questions.

The alternatives were keeping human text, adding JSON to the existing command,
and running a persistent local MCP server. JSON reuses operator configuration,
selection and failure handling without a protocol dependency or reader lifecycle.
Each invocation still opens the manifest and whole selected edition index. MCP
can expose discoverable tools and retain readers, but needs deliberate admission,
refresh, retention and client presentation rules. Revisit it if the actual client
requires MCP or measured complete tasks miss an agreed budget due to reopening.
Live-web services remain credible for uncaptured information, with different
source/freshness and operating obligations.

Primary examples support the responsibilities rather than select our schema:

- [GitHub CLI formatting](https://cli.github.com/manual/gh_help_formatting) and
  [search code implementation](https://github.com/cli/cli/blob/eeb44bc432c0107de002dadd34a61b4e2bd15882/pkg/cmd/search/code/code.go),
  CodeOptions.Exporter/AddJSONFlags/codeRun, support optional machine output next
  to human output. They do not define retained citations.
- [MCP tools, explicit protocol version](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)
  defines structured results and separate tool errors. It does not ensure a host
  hides metadata from a model or includes structured/text content only once.
- [Exa ContentsOptions](https://github.com/exa-labs/exa-js/blob/53d3539006c0fad341e42c821b6d36ff144c4eb9/src/index.ts)
  separates content/freshness settings. This public client is not proof of our
  published-generation membership or its proprietary index implementation.
- [Notation Matters](https://arxiv.org/html/2605.29676v2) studies input-only and
  output-format agent loops. Inspected converter serialize at
  [b31385bb](https://github.com/lkutschka/notation-matters/blob/b31385bb66ffdfd0b9da7eeaef03870a74aeafb1/shared_format/src/shared_format/converter.py)
  uses json.dumps defaults, not compact JSON. Open-weight prompt-mode results
  and repair costs do not establish an optimal format for our eventual client.

## Contract

`concordance search --json [existing flags] TARGET "QUERY"` emits exactly one
compact UTF-8 JSON object plus LF on success. Flags still precede positional
arguments. Text remains the default. `--json` works with ordinary `--limit` and
`--all`, and with explicit `--rerank`; existing flag conflicts remain errors.

The format is `concordance.docs-search/v1`. Required fields:

| Object | Fields and meaning |
| --- | --- |
| Root | format, status="ok", target, selection, retrieval, results |
| selection | collection, generation, edition, all resolved once before opening |
| retrieval | policy, terms_version, ranking_version, indexed_matches, ranked_candidates; model only for reranking |
| model | id, revision, input_version, runtime: the pinned worker contract, not proof of inference |
| results item | id, source, evidence; array order is ranking order |
| source | kind="git", id, repository, revision, native_id, location, blob_id |
| evidence | kind="retained_html_blocks/v1", article_sha256, article_bytes, first_block, after_block, quote, headings, notice_labels |

Source values are captured manifest observations, never freshly generated URLs
or inferred Git line ranges. The digest is 64 lowercase hexadecimal characters.
The block interval includes first_block and excludes after_block. The article
reference identifies the exact retained article JSON; quote is LF-joined extracted
block text, not original Markdown/HTML bytes. headings and notice_labels are
verified canonical context, separately represented as arrays, including [] when
none. A heading included in the body need not appear again as separate context.

Labels r1, r2, etc. refer only to this response. Store the entire response with
them. They are not read tokens, signatures, independent proofs or permissions.
No private Go Reference, storage route, credentials, raw query or uninterpreted
model score is serialized. Source text, including instruction-like text, remains
untrusted data. Verification establishes integrity and exact published bindings,
not source truth, renderer completeness or relevance.

Ordinary policy is all_terms_bm25. indexed_matches and ranked_candidates both
count the complete admitted ranking. Rerank policy is any_terms_bm25_then_bge:
indexed_matches counts the complete union; ranked_candidates counts the first
at most 100 lexical candidates actually scored. Results may be a smaller prefix.
Unselected ordinary bodies remain uncertified. Rerank candidate evidence is
verified before scoring, then selected without another article read. Neither
count asserts answer coverage or the number of distinct source documents.

Success with no matches has status="ok", zero counts and results=[]. An absent
edition fails; an explicitly known-empty edition succeeds with its actual index.
Errors use the existing failing exit and sanitized stderr, with no success object
on stdout. There is no JSON error-envelope promise: setup failure or a crash may
prevent delivery. A partial device write can still expose incomplete bytes.
Consumers must check exit status and parse the complete response before use.

Within v1, consumers may ignore added fields. Existing required field meaning,
native kinds, body representation and error/count guarantees cannot change
silently. Unsupported source/representation kinds need a deliberate new schema;
do not coerce operational records or RFC formats into Git/HTML.

## Cost, bounds and ownership

Ordinary JSON uses complete all-term Search and a selected prefix, sharing one
strict whole article GET per distinct physical reference within that request.
Every logical passage still gets its own manifest/index checks, article decoding
and canonical context rebuild. The existing edition-wide article-byte ceiling
also bounds the selected distinct-byte read plan before GET. Human text keeps its
existing quote-only path. JSON can therefore fail an additional canonical-context
limit even when text would succeed. Per-passage context uses the existing rerank
ceiling of 64 KiB and existing canonical-plan bounds.

Reuse the trusted command's positive bounds. --max-quote-bytes bounds each body;
--max-total-quote-bytes still counts only returned body bytes. JSON additionally
admits aggregate body/context bytes against --max-output-bytes, then checks the
complete encoded envelope against that same ceiling, including metadata, escapes
and LF. Never silently shorten/skip a passage or lower the requested count. Match
overflow still fails before quoting; output count does not bound ranking work.

For K selected passages, U distinct articles and B body/context bytes, presentation
retains O(K+U+B) values alongside complete ranking and one article/plan's scratch.
No article bytes survive the request. Groups fill the original ranked positions;
failure order can differ from the old per-hit loop. A group observes one verified
body, not a later storage change within the request; the next request reads afresh.
Output text admission precedes the CLI's separate aggregate quote check, so an
aggregate quote failure can occur after more reads than in the per-hit loop.
Immutable
owned evidence strings/slices stay unchanged through encoding. JSON output is
assembled before stdout; encoding/json can allocate complete-value scratch before
the bounded writer rejects a write. Output admission is not a process heap, model
token or all-wire-traffic cap. Rerank input/admission and grouped article budgets
remain unchanged. No cache, workers, concurrency or dependency is added.

Any read, association, context, limit, cancellation or encoding failure discards
the success batch. A retry starts a new read operation and can resolve a newer
target. A future later-read feature must pin the original generation, reauthorize
and verify the exact representation, with explicit retention/unavailability rules.
Current reader-bound handles cannot implement it by serialization.

## Distinguishing checks and limits

Before implementation, the recommendation was inline concrete evidence, separate
from encoding/transport. The smallest check used the real published-reader
fixture with local PostgreSQL and synthetic S3 HTTP, then projected and roundtripped
the fields. It preserved exact body/context, count distinctions, shared-byte
logical identities and old-generation output; foreign handles, absent editions,
limits and a later read failure failed without a partial batch. This retained the
recommendation while making the lack of durable read tokens explicit. The probe
tested generation pinning, not target-table activation, model comprehension or
real-provider performance. Private evidence records the prior recommendation and
executed check; production command tests exercise the exposed contract.

No new relevance tuning, model runs or private engineering-judgment evaluations
are part of this interface change. Compare model-facing JSON, labelled passages
and compact records on fixed complete tasks after selecting an actual consumer:
correct source/edition/citations, abstention and errors, tool actions, repairs and
whole-trajectory cost. Existing serialization/token screens are not proof of a
best agent format or faster complete tasks.


Captured source follow-up adds `source_ref` to each result. This bounded locator
pins the published selection, document and article range; it does not change
response-local labels or grant permission. See [captured-source reading](captured-source-reading.md)
for the exact-read command, encoding, navigation and limits.
