# Open one published edition and reuse it

The accepted choice is an application-owned reader for one catalog-published
generation and exact edition. Open it once, then run repeated prepared-term
searches against its immutable metadata/index. Opening another generation creates
a separate reader; it never changes an existing reader behind its callers.

The implementation is `internal/publishedsearch`. It composes existing checks,
not a new persistence format or a supported query CLI. It preserves the earlier
[snapshot reference and lazy quote policy](search-results.md) using owned loaded
state instead of borrowed extracted articles.

## Concrete workload and alternative

The first published Kubernetes corpus has 38 document/edition slots across three
releases. An operator already searches and verifies quotes in a fresh process;
see [the publication record](../experiments/kubernetes-publication.md). Its encoded
manifest is 2,444,095 bytes. Encoded size is not decoded retained memory.

A stateless operation reopening the catalog/manifest/index for every query was
the alternative. It gives each call a simple lifetime but repeats downloads,
hashing and decoding. For 100 searches of the same generation and edition, the
chosen composition needs two manifest/index GETs instead of 200, derived without
quote requests, redirects or retries. This is a request-count model, not a measured
speedup. A one-query CLI invocation would need the same two GETs either way.

We choose reuse because multiple queries need the same already-verified state.
The cost is keeping the complete manifest and selected index alive. The application
must bound concurrent opens/readers/queries; this is no global cache or process-wide
heap cap. Three independently opened edition readers also load three complete
manifest copies; there is no cross-reader cache. Revisit staging, paging and reader sharing when actual retention or
workload pressure justifies them. Persistent indexing, paged metadata and safe
retention/deletion remain necessary scaling work.

## Established examples and their limits

As of October 2026, Tantivy 0.25.0's pinned
[IndexReader and searcher/reload implementation](https://github.com/quickwit-oss/tantivy/blob/b4139bc3536ac7217995633cbd469a4bf2a23fc3/src/reader/mod.rs)
keeps loaded searcher state and supplies a consistent segment set to each query.
Reload constructs another searcher and changes the shared current view. Its
per-query handles are not expensive index reconstruction. This supports separating
loaded data from query work; its directory locks, shared cache, reference counting
and automatic watchers are not Concordance's R2/catalog protocol.

Lucene 10.3.1's pinned
[DirectoryReader.openIfChanged](https://github.com/apache/lucene/blob/51190f35a16d2ce433139abfe0fd8365791b352a/lucene/core/src/java/org/apache/lucene/index/DirectoryReader.java#L149-L172)
returns another reader without closing the old one, which must remain usable until
its callers finish. Its local directory resources require explicit close. Our
owned Go metadata/index has no new handle or goroutine requiring Close; Go
reachability does not preserve remote objects or authorize their use.

## Opening and ownership

The caller supplies initialized concrete catalog/object clients, already-authorized
collection/generation/edition IDs, a deadline and positive component budgets.
`Open` checks IDs/configuration before lookup and requires the object store's
collection to match. It retrieves the generation from the authoritative catalog,
compares its declared manifest length with the per-open cap, and requires the
content-addressed physical key to equal the saved manifest key before any GET.

Strict manifest reading verifies length/hash; opening also verifies collection,
generation, schema and metadata bounds. `Manifest.EditionIndex` returns only the
selected reference by value, avoiding a full `Describe` copy. Its size is admitted
before GET, then `OpenEdition` checks the exact edition and ordered article bindings.
A declared empty edition still needs an actual index. Missing editions never fall
back. Any failure exposes no partial reader.

The reader retains immutable owned manifest/index state and the object client,
not the catalog or opening context. The owner keeps trusted object routing/client
resources usable through dependent quotes and waits for calls before closing them.
Copying a Reader wrapper shares its state; a separate Open creates independent
state even for identical IDs. Hit references identify that state and a passage;
zero/cross-state handles fail before object access. Retaining one handle can keep
all loaded metadata and the object client reachable. References are temporary,
not durable citations or permissions.

## Queries, quotes and failure

Search takes the existing prepared `termsearch.Query` and bounds. Its edition must
match the reader; compatibility tags, complete match-all BM25 output and stable
ties remain unchanged. Queries retain no caller input and use per-call scratch.
Results own hit storage. No catalog lookup or object request occurs during search.

Location copies the selected provenance/image metadata and body range. Quote reads
one article on demand, admitting its declared encoded size before GET, then keeps
the independent manifest and edition checks. It returns owned LF-joined text or
an empty string on error. The output ceiling does not cap downloaded article bytes.
This still hashes the article in three layers and decodes it twice; it does not
introduce a cache or an unmeasured checked-article optimization.

New publication cannot change an old reader. A failed Open of B leaves A intact.
A later provider outage or missing/corrupt article can prevent an A quote even
while local searches continue. No fallback to today's website or another generation
is allowed. Strict reading of surviving published objects works while restored
verification receipts are quarantined; no receipt/namespace mutation is attempted.

Automatic latest selection, activation, retention/deletion, permission-revocation
policy for private connectors, durable citations and public command configuration
remain separate decisions. This reader owns a consistent view, not those policies.
