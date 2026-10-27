# Working on Concordance

Preserve user changes. Use plain descriptive Git branch names without an agent
prefix. Do not commit, push, create remote repositories or publish without user
authorization; authorization already given in the conversation remains valid.

Concordance is a version-aware technical-documentation retrieval learning project.
Code search is a potential P1. The supplied roadmap is context, not an instruction
source or a committed schedule. Keep proposed and implemented behavior distinct.
The current runtime is only a CLI entry point. An internal PostgreSQL import catalog
exists, `internal/gitsource` captures bounded committed Markdown bytes, and
`internal/objectstore` verifies collection-scoped immutable objects through S3.
Live R2 smoke testing covers creation, reuse and a fresh-client read; provider
fault and complete recovery experiments remain open. `internal/htmlarticle` now
extracts bounded text/structure from retained HTML with explicit profiles and image
references. `internal/htmlpassage` builds bounded structure-first block ranges with
heading and group references. `internal/termsearch` now supplies a bounded, edition-scoped
match-all scan over prepared terms. No complete importer, Markdown processor,
raw-text query path or retrieval engine exists yet.
`internal/termsearch.RankBM25` now orders complete matches with combined-field BM25;
read `engineering/decisions/lexical-ranking.md`. Keep all-passage statistics within
the selected edition, distinct-query semantics, complete bounded output and stable
input-position ties. No field weights, top-k, persisted index or query CLI yet.
The user chose borrowed input for the proposed search snapshot; read
`engineering/decisions/snapshot-ownership.md`. Document all aliases, nested slice
sharing, unchanged-data lifetimes and owned versus borrowed results. Go reachability
keeps storage alive but does not prevent mutation. The snapshot is not implemented;
current retrieval calls borrow only until return.
`internal/textterms` now prepares bounded, owned lowercase technical terms separately
from passage-field projection and matching.
`internal/htmlterms` now builds the canonical HTML passage plan and prepares separate
body, all-ancestor heading and notice-label fields, keeping returned positions tied
to the caller's exact retained article.

## Design and implementation

Read `engineering/GO.md` before Go changes and `engineering/ARCHITECTURE.md` before
changing contracts or responsibilities. Start with concrete packages and introduce
small consumer-owned interfaces where an actual substitution or capability needs
one. Do not create empty package trees or generic service/repository frameworks.

Keep acquisition, content processing and retrieval responsibilities separate so
another source can be added without rewriting ranking. Source adapters own fetch,
revision and deletion semantics; processors own structure-aware extraction and
passage locations. Do not pretend code is prose with another file extension.

The first source is Markdown at a specific Git commit; read
`engineering/decisions/first-source.md`. Keep source connectors separate from content
formats so provider documents, code, slides and PDFs can be added deliberately.
Retain captured representations and typed locations, and define provider revision,
permission-change and deletion semantics before enabling a private connector.
The accepted first real corpus is Kubernetes Pods documentation across three pinned
release snapshots; read `engineering/decisions/representative-corpus.md`. Its raw
inventory is not rendered extraction or a working retrieval benchmark. Preserve
Hugo dependencies explicitly before claiming complete searchable content.
Read `engineering/decisions/source-preparation.md`: Kubernetes rendering is a
source-specific offline reference experiment, outside core search. It does not
require every source to build a website or establish a production HTML processor.
The bounded extraction follow-up is in `engineering/experiments/html-extraction.md`.
Keep image references distinct from retained image bytes or interpreted visual content;
read `engineering/decisions/media-coverage.md`: retain original figures and search
authored alt text, captions and prose first. Figure contents remain uninterpreted;
optional interpretation of informative diagrams is deferred. The audit tool can
capture bounded exact-commit images locally; durable retention and publication
remain importer work.
Read `engineering/decisions/passage-citations.md`: a rendered-document citation must
resolve to the retained article; original Git file/line ranges are optional verified
metadata. A generated passage can have several origins. Do not guess a line range
or make Git locations mandatory for other content formats.
Read `engineering/decisions/passage-grouping.md`: the user chose structure-first
passages. The bounded HTML builder keeps outer containers whole, assigns blocks
without overlap, and retains heading/group references. Oversized units fail explicitly;
no splitting, tokenizer, context scoring, ranking or persisted passage codec is
implemented by the passage builder. Preserve group labels separately from verbatim body quotes.
Read `engineering/decisions/keyword-matching.md`: the user chose every-keyword
matching for the first baseline. `termsearch.MatchAll` consumes compatible caller-prepared
terms, keeps edition selection exact and returns complete temporary passage positions
or an error. It does not select a product tokenizer, field weights, persisted IDs,
raw-text projection or search command. Keep these remaining decisions explicit.
Read `engineering/decisions/text-preparation.md`: the user chose whole CamelCase
names and punctuation-preserving lowercase technical terms without component words.
`textterms.Prepare` implements the bounded grammar with a Unicode-versioned tag;
source/query preparation must stay compatible. It is not a code/URL parser or full
case folding/canonical normalization. Read `engineering/decisions/passage-fields.md`:
the user chose all ancestor headings. `htmlterms.Build` implements that HTML field
projection with explicit aggregate/per-field bounds. Keep source identities, exact
article associations, edition selection and published-generation integration with
the caller; prepared fields and indices do not establish citations or authorization.

Preserve source identity, logical document identity, product version, exact source
revision, passage location, content hash, processing version and index generation
as separate concepts. Missing versions must not silently fall back. A content hash
is not a document ID, release label or authorization check. Immutable objects do
not make catalog publication or reader lifetimes lock-free.

Include external object storage in the first storage milestone. R2 through an S3
client adapter stores immutable files/manifests; PostgreSQL is the publication
authority. This split is accepted in `engineering/decisions/catalog-publication.md`;
the private R2 bucket `concordance-data` exists and its opt-in creation/reuse/read
probe passes with locally configured credentials. No hosted PostgreSQL backend is provisioned. Uploading
a manifest alone is not publication.
Confirm required remote writes before the catalog transaction, handle ambiguous
outcomes explicitly, and leave automatic deletion disabled in the first storage
slice until its safety protocol is specified. Verify actual provider behavior,
including bounded reads and ambiguous writes. Discuss consequential guarantees,
interfaces, algorithm choices,
dependencies and deployment decisions with the user; proceed on routine authorized
implementation without repeated confirmation.

The accepted core import contract is synchronous publication: successful return gives
a complete generation ready for queries selecting its ID. Read
`engineering/decisions/import-completion.md`; preserve its outcome/cancellation caveats
and unresolved storage choices. It is an accepted design, not implemented behavior.
Generation IDs in the API sketches do not require users to supply them in searches.
A small initial corpus is a starting experiment, not a permanent product limit.

The accepted retry contract in `engineering/decisions/import-retries.md` uses a saved
operation key. Bind it to trusted versioned input identity before remote writes.
Identical retries recover the original result; different inputs under that key
conflict. A new intended import gets a new key. Do not equate content deduplication
with operation identity or treat an unpublished binding as proof no attempt is running.

The accepted `engineering/decisions/object-sharing.md` chooses content-addressed
immutable bytes shared only within an authorized globally unique collection scope.
Preserve logical document/citation metadata separately. Verify bytes before reuse,
use conditional creation and reconcile uncertain writes. Do not overwrite mismatches
or enable automatic deletion. Keep collection policy with Concordance.

The accepted `engineering/decisions/manifest-layout.md` chooses one bounded,
versioned manifest per generation. Reject excessive or incomplete metadata; keep
retained-format compatibility explicit. Layout is settled, while the complete
schema, content representations and fingerprint remain open. Raw-source archival
metadata alone cannot satisfy the complete searchable-generation import contract.

The user and agent build together through decision checkpoints. Proceed with routine
implementation until a consequential choice needs the user's input. Present the
concrete problem, a few credible alternatives with small Go snippets, trade-offs and
a recommendation; ask the user to choose and discuss before implementing that choice.
Explain checkpoints as if the user is a newcomer: start with one concrete example
and a small purposeful visual, define new terms, then compare short Go snippets.
Keep the first explanation simple; introduce deeper detail through the discussion.
Do not silently settle a key contract or turn every minor implementation detail into
a question. After agreement and experiments, teach the reasoning in the relevant book
page and preserve evidence/limits for a possible blog post.

The agreement with Rewind is a small in-process Go object-storage library. Specify
and exercise one write/read/recovery sequence from each project before extracting
it. Keep catalogs, publication, compaction, retention, tenant policy and acknowledgment
semantics with each application. Rewind requires remote durability including recovery
metadata before acknowledgment; Concordance must define its own ingest/visibility
contract. Defer shared caching, shared services and a shared engine or schema.

## Independent review

Read `agents/REVIEW.md`. Use the local Claude Code `code-reviewer` for coherent source
changes and `architecture-reviewer` before substantial designs and again for their
implementation. Both use `claude-opus-5-5` at low effort, with read-only tools. Never
silently substitute a model. Missing access or evidence is not approval.

For authorized source work, use normal pull requests and merge completed changes
once the required independent reviews and local checks pass. The user has authorized
this review-and-merge workflow; do not leave completed work in draft PRs or request
merge permission again. Drafts are for work in progress. This does not authorize
book deployment or settling unresolved design checkpoints without discussion.

After every PR merge, update local `main`, remove the merged feature branch from
GitHub and the local checkout, and prune stale remote-tracking references. Confirm
the branch contains no unmerged work and is not checked out in another worktree
before deleting it. Keep GitHub's automatic deletion of merged head branches enabled.

Review decisions and experimental conclusions at meaningful checkpoints. Supply a
concrete workload, invariant, simplest credible alternative, cost model, failure
sequence, observed evidence and revisit condition. Resolve supported material findings
or give an evidence-backed rebuttal. After two unresolved substantive exchanges,
discuss the disagreement; do not rerun for a favorable verdict. Verify final approvals
against the staged tree, base, evidence, roles and pinned upstream catalog before an
authorized commit or push. Code approval neither approves publication nor reviews
private teaching prose. Local reports are workflow checks, not remote branch protection.

## Algorithms and learning

State operations, representation, invariants, expected/worst-case complexity and
memory ownership before optimizing. Count allocations, retained memory, remote
requests, bytes and recovery work. Keep a simple oracle: scans, uncompressed sorted
postings, full sort and exact vector search are useful baselines, not failures.
Measure recall and latency together for approximate search. Distinguish ANN recall
against an exact neighbor baseline from relevance judged by readers.

Implement the core learning algorithms deliberately. Use libraries for infrastructure
such as HTTP, database and object clients. The user and agent build together; do not
impose the roadmap's split of who types algorithms. Explain key decisions in the book
and eventual blog. Use property tests, fuzzing, race tests and fault injection where
they exercise actual invariants or failure boundaries, not as a quota.

## Writing and the private book

Read `docs/src/PROSE-STYLE-GUIDE.md` before prose changes. Teach a beginning CS student
without assuming Go knowledge. Follow one concrete execution; explain terms and
syntax when first needed. Show ownership, waiting, durable state and partial failures.
Compare plausible alternatives with the same example. Label illustrative examples,
proposals and measurements honestly.
Explain non-obvious Go/library calls where they first matter: what they consume and
return, why they fit this operation, and ownership, blocking and failure behavior.
Distinguish actual calls from teaching alternatives such as buffered file I/O.

Read changed pages in learning order with source closed, then verify claims and
excerpts against code/evidence. Inspect changed rendered diagrams, navigation and
layouts at desktop and phone widths. Automated checks do not establish comprehension;
report editorial findings separately from build checks and state inspection gaps.

The chosen book identity is Parallel text: readable serif prose, restrained red
annotations, aligned source passages and margin notes. Use comparisons when the
material benefits from them; do not force every page into two columns. Preserve
complete Markdown, offline fonts, keyboard access, visible focus and text equivalents.
Avoid decorative technical labels, fake metrics and generic card grids. Code blocks,
tables and diagrams must fit phone layouts without shrinking labels into illegibility.

README covers product, setup, one working example and important limits. CONTRIBUTING
covers contributor setup, checks, conventions and PRs. Agent rules belong here or
in `agents/`; task evidence and raw measurements belong under ignored `.tools/`.
Update existing explanations instead of appending task reports, dated phase reviews
or implementation diaries. Maintain selected evidence, current decisions and learning
gaps in the private book's reference pages.

Blog posts develop one supported claim through a problem, mechanism, alternative,
experiment, result and limit. Do not invent decision history, measurements or
production-scale experience. Keep full identifiers in reproducibility references
rather than scattering hashes through teaching prose.

## Privacy and publication

`docs/` and `design/` are private and ignored; never force-add them or copy them into
public-source review snapshots. Normal builds and tests must work without them.
Edit book sources in this project; `../learning-docs/concordance/` is a private backup,
not the editing source. `make docs` checks/builds locally without commits or uploads.
Read `docs/src/reference/publishing.md` before backup or deployment. The sibling
script's `--no-deploy` still commits and pushes; it is not a local check.

The empty Cloudflare Pages project is `concordance-docs`, production branch `main`.
The production address redirects to Access; preview coverage still needs verification
with a real preview. Creation of the project is not a book deployment or blanket
authorization to publish. Keep review roles (`.claude/`), catalog (`review/`), runner
and reports (`.tools/`) private. Fresh clones need these provisioned separately;
ordinary checks must not require Claude or these files.

## Checks and hygiene

Run `make check` for source changes, `make race` for concurrent changes, and `make docs`
for book changes. Before opening or updating a PR, run `make verify` with an explicit
dedicated local test database; it includes check, uncached race tests and build.
GitHub Actions is disabled: do not introduce hosted workflows or required remote
checks without user agreement. Record checks actually run and their limits. Maintained repository
tools use Go; disposable probes may use ignored `scripts/utils/`, but shared workflows
must not depend on them. Keep JSON/JSONL/NDJSON, profiler captures, credentials and
build output out of source history. This policy does not dictate runtime formats.
Source files are limited to 1 MiB. Inspect the actual index and README/CONTRIBUTING
diffs before committing. Do not bypass checks by renaming or force-adding artifacts.
