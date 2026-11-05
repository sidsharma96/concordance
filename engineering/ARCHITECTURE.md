# Concordance architecture

Concordance retrieves technical documentation from exact captured revisions and
product editions. The public CLI provides trusted-operator prepared import, search, target inspection and explicit
activation alongside help/version. The
internal Git/HTML import path builds and publishes searchable generations; an
operator experiment has published the first Kubernetes corpus and read its indexes
and verified quotes in a fresh process. The reusable internal published-generation reader now owns one exact selection
for repeated searches. The import CLI saves verified retry material before
synchronous publication, with explicit retry and separate activation. See the
[publication experiment](experiments/kubernetes-publication.md) for that run's scope.
The [native CLI exercise](experiments/hosted-import-command.md) now publishes another
result over the same retained corpus, recovers it with the object route blocked,
and verifies a separate test target without redirecting the existing default.
The [historical update replay](experiments/documentation-update.md) additionally
checks changed source, explicit selection, a held old reader and rollback through
the native CLI and hosted stores.

## The implemented path

```text
operator import start -> save owned local job -> open/check saved job
operator import retry -> open/check existing job, same key
  -> trusted source identity/collection comparison -> existing namespace observation
  -> preparedimport below -> published generation, activation remains separate

operator search CLI
  -> admit/prepare query -> resolve target once -> check configured collection
  -> publishedsearch.Open -> SearchQuotedFirst (--all: SearchQuoted)
  -> bounded escaped text -> stdout
operator target show/activate -> catalog inspection / checked conditional update

trusted offline preparer
  -> capturefiles: descriptor + owned HTML/recipe buffers
  -> preparedimport: exact Git membership + declared dependencies + processing
       -> retained article JSON + per-edition binary indexes + JSON manifest
  -> generationpublish: validate -> bind saved key -> confirm objects/root
       -> catalog.PublishInNamespace: make the generation visible

publishedsearch.Open: catalog-published generation + exact edition
  -> strict manifest/index reads -> owned immutable selected reader
  -> repeated local lexical search -> reader-bound references
  -> on-demand strict article read -> provenance checks + exact quote
  -> optional SearchQuoted: complete ranking + every strict quote, or zero result
  -> SearchQuotedFirst: complete ranking + requested strict prefix, or zero result

searchtarget.Resolve: friendly catalog name + optional exact edition
  -> copy one generation/default tuple -> publishedsearch.Open as above

searchtarget.Activate: explicit operator choice + expected target revision
  -> strict default reader preflight -> catalog conditional target update
```

The preparer owns rendering provenance and the completeness of its declared build
inputs. The importer independently captures the selected committed Markdown and
declared dependencies, verifies exact page membership, and runs actual extraction
and indexing. It retains recipe bytes as configuration; it never executes them.

| Responsibility | Current packages | Boundary |
| --- | --- | --- |
| Source acquisition | `gitsource` | Bounded committed Markdown and explicit Git files; no renderer |
| Prepared file transport | `capturefiles` | Strict descriptor and root-relative regular payload files; no Git or remote stores |
| Extraction and passage preparation | `htmlarticle`, `htmlpassage`, `textterms`, `htmlterms` | HTML structure, ranges and compatible technical terms; no source routing |
| Lexical retrieval | `termsearch`, `htmlsearch` | Exact-edition matching/ranking; the snapshot borrows articles |
| Durable artifacts | `htmlarticlejson`, `htmlpassagemap`, `htmledition`, `generationmanifest` | Versioned bounded representations and exact bindings |
| Published retrieval | `publishedsearch` | Own one catalog-published generation/edition; local queries and on-demand verified quotes |
| Search selection | `searchtarget`, catalog targets | Resolve a friendly name once; explicitly activate a checked default with a revision condition |
| Import composition | `preparedimport` | Complete selected membership, processing, staging and publication |
| Storage/publication | `objectstore`, `objectreuse`, `catalog`, `generationpublish` | Immutable object integrity, scoped receipts and catalog visibility |

These are concrete in-process components, not independently deployed services.
Introduce small consumer-owned interfaces when a real substitution needs one; avoid
a dynamic plugin system or generic service/repository framework.

## Acquisition and content are separate

The [first source](decisions/first-source.md) is Markdown at an exact Git commit.
The [representative corpus](decisions/representative-corpus.md) uses Kubernetes Pods
documentation across three pinned releases. Kubernetes needs a source-specific
[offline preparation step](decisions/source-preparation.md) to render shortcodes,
notices and dependencies faithfully. That requirement belongs to this source; other
sources may supply Markdown, provider blocks, HTML or exports.

The [trusted handoff](decisions/prepared-capture.md) pairs selected Markdown paths
with prepared HTML and explicit extraction profiles. `capturefiles` reads each
unique payload once, checks its declared bytes and returns the same owned buffer.
Returned aliases can share that buffer. Root-relative `os.Root` access constrains
paths but is neither a filesystem snapshot nor a hostile-filesystem sandbox.
Source registration, credentials, routes and the saved operation key stay outside
the descriptor. See the [transport experiment](experiments/capture-files.md).

`preparedimport` rejects missing, extra or duplicate page mappings. It captures the
complete selected Markdown inventory and declared dependencies at the requested
commit, including referenced original images. Logical slots sort by source ID and
full Git path. Git acquisition uses a bounded sequential batch process after full
count/byte admission; cancellation and errors still wait for that process to exit.
See [Git capture](experiments/git-capture.md).

The [restricted Kubernetes build-input profile](decisions/kubernetes-build-inputs.md)
retains checked required inputs and original figures while excluding unrelated
static files. Changed source pins or preparation require revalidation. Neither
selected rendered output nor a dependency list proves preparation completeness;
that assertion remains with the trusted preparer.

A source adapter owns revision, permission-change and deletion semantics. A content
processor owns headings, blocks, pages, slides or code symbols. Future connectors
must define reliable revisions, authorization and deletion before handling private
documents. Code search is a possible P1 and needs code-aware processing rather than
prose with another extension. Ranking should not know which provider supplied text.

## Identity and evidence

Keep source, logical document, product edition, captured revision, passage location,
content hash, processing version and generation distinct. Shared bytes can occur in
several logical documents or editions; a hash proves neither identity nor permission.
The [document-identity contract](decisions/document-identity.md) uses configured
source ID plus exact full repository-relative path for Git. Renames are not inferred.
The manifest binds edition-local slots to their exact capture, article and provenance;
the same logical key may appear across editions but not twice in one source/edition.

A query selects an exact edition within one published generation. Missing editions
must not silently fall back; explicitly empty and absent editions remain distinct.
A future mutable label such as `latest` must resolve once per request. The public
query API need not ask users for generation IDs, but its implementation must pin one
so text, counts, statistics and citations agree.

The [named search target](decisions/search-targets.md) hides that generation choice
from normal users. `searchtarget.Resolve` copies one catalog tuple and applies an
explicit edition override or its displayed default. It does not open an index or
claim the override exists; `publishedsearch.Open` performs those checks. Explicit
activation first opens the candidate default strictly, then conditionally changes
the target's collection/generation/default tuple using an expected revision. A
stale operator conflicts, while exact-next-revision same-tuple retries converge.
No import automatically activates its result. See the [update contract](decisions/target-updates.md)
and [local evidence](experiments/search-targets.md). The additive target schema is
opt-in setup; its [hosted installation and CLI check](experiments/hosted-search-target.md)
now selects the published Kubernetes corpus with explicit default `1.36`. The operator CLI
uses a configured collection guard on SQL updates, so names in another collection
cannot be redirected. Existing trusted internal callers can keep unscoped updates.

The [retained article is the quote authority](decisions/passage-citations.md).
Verified original Git file/line ranges are optional additional metadata; generated
content may have several origins. The HTML processor owns bounded text blocks and
nested group intervals. [Original figures are retained](decisions/media-coverage.md)
and authored alt text, captions and surrounding prose are searchable. Figure
contents remain uninterpreted; derived diagram descriptions need a separate explicit
processing version and generation.

## Artifacts and validation

Each generation has [one bounded JSON manifest](decisions/manifest-layout.md),
[separate article objects](decisions/article-storage.md), and
[one binary index per exact edition](decisions/edition-index-layout.md). Shared exact
article bytes remain separate from logical article slots. All required artifacts
must be confirmed before the generation becomes visible.

`htmlarticlejson` preserves retained HTML text and structure through a strict
[versioned JSON codec](decisions/article-json.md). `htmledition` derives count postings
and saved body ranges together, then owns the immutable state recovered from its
[binary format](decisions/edition-binary.md). It checks expected bytes, edition,
closed tags, minimal integers, counts, statistics and range coverage. The index has
[no extra ordered-word copy](decisions/edition-artifact-binding.md). Article bytes
are verified before quotes are assembled with LF separators. Diagnostic JSON
inspection is separate from authentication and publication.

`generationmanifest` owns immutable opened metadata. `Objects` lists distinct-byte
references to confirm; the list alone proves no remote existence. `OpenEdition`
checks the selected index's article references and order. `Resolve` copies one
slot's provenance, and `VerifyArticle` checks exact bytes, extraction profile and
image bindings. Loading one edition does not verify the bodies of every other
object. These checks bind artifacts, not the truth of the preparer's assertions.
See the [manifest experiment](experiments/generation-manifest.md).

The three JSON readers share `jsonunicode.CheckSurrogates` to reject malformed
escaped UTF-16 before `encoding/json` can replace it. Each caller retains its own
schema, limits and domain errors, checks raw UTF-8, and uses the standard decoder
for JSON grammar. The shared scanner borrows bytes only through return and uses
linear time and constant extra space; it is not a general JSON parser.

`htmlpassagemap` remains the bounded reader for saved ranges checked against ordered
prepared fields. `htmlsearch` remains a borrowed in-memory search snapshot. These
components and the scan ranker provide independent bindings/oracles; the durable
edition reader does not change their ownership contracts.

The [saved-job component](decisions/saved-import-jobs.md) owns a bounded verified
HTML/recipe copy and generated retry identity in a private local directory.
`capturefiles.Copy` writes the same buffers it verified; only transport paths
change. `importjob` synchronizes local inputs/metadata before installing a complete
readiness marker, then Open rechecks every pin and compatible processing label.
Local readiness is separate from catalog publication, and errors can leave either
partial or complete local material. Jobs keep no local Git routes or credential
fields. Pinned Git objects remain required. The [import command](decisions/import-command.md)
composes start/retry with trusted runtime source registration and native connections.
It compares saved collection/source IDs/repository identities before connecting and
opens an existing namespace by exact physical binding without setup. Inactive handles
permit completed-key recovery, while receipt confirmation and guarded publication
retain their active/epoch checks. No automatic resume is included. See the [local experiment](experiments/saved-import-jobs.md).

## Publication and recovery

[PostgreSQL is the publication authority; R2 holds immutable objects](decisions/catalog-publication.md).
Objects are named by SHA-256 within an authorized globally unique collection scope.
Conditional creation and uncertain-write reconciliation prevent overwriting
unexpected bytes. A PostgreSQL transaction cannot roll back an R2 write.

The accepted import contract is a [complete snapshot](decisions/import-snapshots.md)
and [synchronous publication](decisions/import-completion.md). Success returns a
complete generation ready for exact-generation queries. Omission changes only new
generation membership; acquisition failure fails the import rather than deleting a
document. Patch imports remain deferred.

A [saved operation key](decisions/import-retries.md) identifies the intended import.
`Manifest.InputFingerprint` compares captured inputs and full processing rules,
excluding candidate IDs and derived article/index bytes. `PublishCaptured` derives
this fingerprint after artifact preflight; `preparedimport` constructs those
artifacts from actual capture/processing. Identical retries recover the original
publication, while changed inputs conflict. Metadata hashing still cannot prove
rendering provenance. Keep the exact handoff for a deliberate retry; see
[captured-input equality](decisions/captured-input-identity.md).

The publication sequence is local validation, catalog binding, artifact confirmation,
root confirmation, then `PublishInNamespace`. A completed binding returns its
original result without S3 access. Preparation is rebuilt before that lookup in the
current importer. Errors yield no partial result, but publication may already have
committed if its reply was lost. Cancellation is cooperative, not rollback. A failed
attempt can leave unreferenced objects, and an unpublished binding does not prove
that no attempt is running.

`objectreuse` uses [PostgreSQL verification receipts](decisions/verification-receipts.md)
and conditional HEAD for previously checked immutable objects; misses and uncertain
writes use full verification. Proofs bind the physical namespace and captured epoch.
Guarded publication rejects stale or quarantined epochs; never replace that gate
with strict `Publish`. Strict object body reads always check length and SHA-256.
Restored published references can be read strictly while receipt reuse remains
quarantined. ETags are not content hashes.

[Bounded confirmation overlap](decisions/confirmation-concurrency.md) permits up to
eight object chains per call, with sequential behavior as the zero-value default.
Admission and receipt lookup precede workers; cancellation joins all workers before
return or quarantine. One nonwaiting caller per importer/publisher instance is
admission, not distributed ownership. Request/staging limits count logical admitted
bytes, not whole-process heap or aggregate concurrency across instances.

The [catalog on the existing droplet](decisions/catalog-hosting.md) uses a restricted
runtime login over SSH. Live synthetic publication and encrypted off-host backup
with isolated restore/read have been exercised; restored receipts stayed quarantined.
The [real Kubernetes publication](experiments/kubernetes-publication.md) additionally
exercises a deliberate same-key retry, completed-key recovery without S3, and a fresh
strict read. This is not unattended backup, production HA, restored publication
trust or exhaustive provider-fault coverage. Run schema-creating tests only on a
dedicated local test database.

Automatic deletion stays disabled until references, in-flight writes and reader
lifetimes have a safety protocol. Persistent indexing, paged metadata and safe
retention/deletion are required scaling work before claiming million-document
support. The current bounded in-memory composition also needs measured memory
pressure and streaming consumers before file staging can make stronger heap claims.

## Retrieval and algorithm progression

[Structure-first passages](decisions/passage-grouping.md) keep outer containers
whole and partition body blocks without overlap. They carry heading/group references;
oversized units fail explicitly rather than being silently split. `htmlterms` projects
body, all ancestor headings and notice labels separately. `textterms` prepares owned,
lowercase technical terms with whole CamelCase names and punctuation preserved under
a Unicode-versioned grammar. Source/query preparation must be compatible. It is not
full case folding, stemming, normalization or a code/URL parser.

[Every distinct query term must match](decisions/keyword-matching.md). `termsearch`
provides both a scan oracle and an immutable counted-posting index. Sorted posting
lists are intersected and complete results ranked using
[combined-field BM25](decisions/lexical-ranking.md) with exact-edition statistics,
fixed k1/b, positive IDF and full sorting. Query duplicates/permutations do not alter
scores; exact ties preserve input order. Intermediate intersections may exceed the
final-match limit. Scores are temporary ranking values, not confidence or citations.
Field weights, optimized top-k selection and phrase/proximity positions remain deferred.
The CLI defaults to ten verified quotes with positive --limit, or explicit --all.
Its [requested batch](decisions/requested-quote-batches.md) reports the complete
indexed-match count separately, preserves full candidate admission/sorting and
verifies every shown quote before output. Unshown article bodies are not read.

`htmlsearch` [borrows caller input](decisions/snapshot-ownership.md) unchanged through
dependent use and owns prepared terms/locations. Opaque hit references keep that
state alive; Go reachability does not prevent mutation through caller aliases.
[Quotes own assembled strings](decisions/search-results.md), while references retain
the state. Zero/cross-state references fail. Standalone scan calls retain no inputs
after return. The [published reader](decisions/published-reader.md) owns immutable manifest/index
state for one exact edition. It retains the object client for lazy strict quotes,
not the catalog/opening context. Copies share state; independently opened readers
reject each other's hit references. Caller-owned routing and client lifetimes remain
explicit. Per-open declared byte caps apply before GET; no global cache or automatic
reload/activation is introduced.

The accepted [verified-batch policy](decisions/verified-search-batches.md) adds
`Reader.SearchQuoted` without changing the separate Search/Quote methods. It
assembles every ranked hit's strict quote before returning any hits; a later read,
validation, cancellation or byte-limit failure returns a zero result. Positive
per-quote and total-quote limits bound logical returned text; the existing search
limit bounds hit count. References retain the same reader, while each result owns
its hit slice and quote strings. It still performs one full bounded article read
per hit, including shared articles. Rendering needs its own encoded-output budget
and can suffer partial writes after verification. This is neither a public output
schema nor a CLI, cache, top-k or transactional-output implementation.

The accepted [local reranking direction](decisions/local-reranking.md) keeps a
separate persistent model worker and an opt-in command, now composed through the
[bounded framed protocol](decisions/worker-protocol.md). Go retains exact reader
handles and citations; the Python process receives texts and bindings without
catalog/R2 configuration. The model stays the pinned offline MPS runtime, with
whole-query admission, complete token preflight and joined draining. Every error
fails the rerank request rather than falling back. The explicit candidate prerequisites now exist: RankBM25Any and
SearchAny retain exact-edition statistics with any-word eligibility and lexical
sum/tie order. Existing Search stays match-all. SearchAnyEvidenceFirst admits the
complete union, selects a positive prefix and checks its entire per-hit declared
article-byte plan before GET. It returns strictly verified body quotes with separate
canonical ancestor headings/notice labels, reader-bound handles and aggregate text
admission, or zero result on any error. SearchAnyEvidenceGroupedFirst now adds
[request-scoped encoded-body reuse](decisions/grouped-article-reads.md) with a distinct
article-byte read plan before GET. It preserves each passage's independent checks,
owned text and ranked position; no body state is retained between calls. The
uncached API remains the oracle. The explicit --rerank command uses the grouped operation before complete model
scoring and display-prefix selection; ordinary search remains separate. See
[the candidate experiment](experiments/search-candidates.md) for ownership/work
limits and the offline composition oracle.

Semantic embeddings and vector search remain future work. Exact vector search is
the oracle before ANN experiments; keep embedding model/dimensions/metric and
processing versions compatible. Measure neighbor recall separately from reader-judged
relevance. Hybrid retrieval, compression, skip data, Bloom filters and sharding need
concrete workloads and comparisons against simple scans, postings and full sorts.
Count allocations, retained memory, remote requests, transferred bytes and recovery
work alongside CPU complexity.

## Agreement with Rewind and roadmap differences

Share only a small in-process Go object-storage library after each project exercises
one write/read/recovery sequence that establishes its needs. Keep catalogs, logs,
publication, compaction, retention, tenant policy and acknowledgment semantics with
each application. Rewind requires remote durability including recovery metadata
before acknowledgment. Concordance's visibility promise comes from its own catalog.
No shared engine/schema/service, cross-checkout imports or shared cache is implied.

The supplied roadmap is context rather than a committed schedule. Broad crawling,
hybrid search, content-hash document identity and reusing Rewind unchanged are not
current contracts. A managed preparer, durable capture journal/WAL, automatic resume
and an HTTP API are unimplemented.
A small first corpus is an experiment, not a permanent product limit. Explicit
target activation and trusted-operator search are exposed by the CLI. Native
PostgreSQL profiles and explicit R2 environment settings configure connections;
text is for reading, with no stable JSON/citation codec. Hosted target setup now passes;
reader-safe retention remains open. Roadmap performance claims are not Concordance
measurements.
