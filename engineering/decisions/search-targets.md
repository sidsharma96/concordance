# Resolve user-facing search targets

Status: accepted option B, a shared PostgreSQL target with explicit operator
selection. Generation IDs stay internal to normal search. Bounded internal target
resolution/activation is implemented; no search command or automatic activation
exists yet.

## Accepted boundary

Normal search users name documentation such as Kubernetes. They do not need to
know or supply an internal generation ID. They may request an exact documentation
version, select an available version, or use a clearly displayed configured
default. Results identify the version actually searched. An explicitly requested
missing version fails rather than silently using another version.

The application resolves that request to one collection, published generation and
exact edition before calling `publishedsearch.Open`. It keeps this selection fixed
through matching, ranking, provenance and quotation. A generation is one complete
published import; an edition is one product-version selection inside it. Several
generations can contain the same edition label with different captured revisions.
The lower-level reader continues to take these exact internal IDs.

Illustrative command and target, neither implemented yet:

```text
concordance search kubernetes "restart policy"

kubernetes -> collection C, published generation G7, default edition 1.35
              -> open G7 / 1.35 -> show "Searching Kubernetes 1.35"
```

An explicit request for 1.34 uses G7 / 1.34 if that edition exists. No version
request means the configured default, not sorting version strings, choosing the
last import by timestamp or assuming the newest available docs match the user's
installed software. If no default is configured, report the need to choose one;
the selection/listing UX is still open.

## Workload and choices

The first real collection has 38 document/edition slots across three pinned
Kubernetes releases. We want a supported read-only search command for that saved
corpus before adding semantic retrieval or another ingestion path. The two options
below both hide generation IDs from ordinary search users and preserve exact
reader selection. Neither changes ranking, object formats or publication success.

### A: named local configuration

An operator configures a name with a collection, exact published generation and
default edition. The command reads that target, applies any explicit version, and
opens the existing reader. The shape below is illustrative Go, not an API:

```go
target := config.Targets["kubernetes"]
```

This needs no catalog schema change and gives a reproducible single-operator
prototype. Each machine must receive target updates; users can unknowingly keep
different defaults. Connection/routing configuration is still needed with either
option. The local mapping must be bounded/validated and is not permission to read
its collection. An absent target or unpublished generation is an error.

### B: named catalog target

The operator stores that same selection in PostgreSQL. A search resolves the name
once, reads its generation and default edition together, then opens the existing
reader. This illustrative call would require new catalog functionality:

```go
target, err := catalog.ResolveTarget(ctx, "kubernetes")
```

Users sharing the catalog share the configured default. Changing the target only
affects future opens; an already-open reader keeps its old generation. This adds
a bounded indexed metadata lookup and an activation/update operation, schema and
failure tests. It supplies no new object-body cache or offline-serving promise.

The accepted starting policy is explicit operator selection, separate from import:
publishing a generation would not automatically make it the default. An activation
must establish that the generation is catalog-published and that the exact default
edition exists in its checked manifest. The generation/default pair must change
together, and competing operators must not silently overwrite each other's choice.
The bounded [update protocol](target-updates.md) received independent design
review. The internal implementation is now `catalog` targets plus
`searchtarget.Activate` and `searchtarget.Resolve`. Schema installation is an
explicit operation; no hosted catalog change has been made.

The target lookup grants no authorization. The application must resolve it within
the caller's authorized configuration, authorize the resulting collection before
object access, and use trusted object routes rather than arbitrary endpoints from
request data. Missing target/default, catalog unavailability and subsequent strict
read failures remain explicit errors. Restored targets are not publication proof;
the existing catalog and object checks still apply.

## Research and choice

Observed examples:

- Docusaurus 3.10.2's [versioning guide](https://github.com/facebook/docusaurus/blob/f37f9035584917a97a260b91fc2842cba4f8b94f/website/docs/guides/docs/versioning.mdx#L243-L256)
  exposes `lastVersion`, a configured default documentation route distinct from
  the working documentation version. Its workload is static website navigation,
  not querying an R2/PostgreSQL import catalog. It supports separating an explicit
  user-facing default from the underlying captured revision; its navigation and
  browser preference precedence do not define Concordance's request policy.
- Elasticsearch 8.19's [aliases](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/aliases.html)
  let applications address indexes with a stable name and switch its backing
  index through an alias update. Action failure/partial-success controls still
  matter. Its workload includes online reindexing. Concordance's proposed target
  would select one collection/generation plus a default edition, not fan out over
  several indexes or copy Elasticsearch's distributed cluster machinery.
- The pinned [Tantivy CLI tutorial](https://github.com/quickwit-oss/tantivy-cli/blob/6e6a15ea3f38523dad4cfbad3afb9e0e4ec005c2/README.md#L231-L237)
  names a local index directory with `-i` when searching. It is a useful explicit
  target example for a small local operator tool, but has no product-edition or
  shared catalog-selection contract.

Assessment: A is the smallest prototype. B better fits a shared
documentation search product because the default can be changed in one authority
we already operate, without updating every search user's local target file. We
chose B with explicit operator selection and a visible default version. Its
activation implementation still requires independent review. This is a fit judgment,
not a measured latency or scale advantage.

For B, one search would add one target metadata lookup before the existing
generation lookup and strict manifest/index reads. Retrieval remains unchanged.
A target switch between resolution and Open does not change the captured request:
Open uses the resolved old generation and reports a failure if its required data
cannot be read. Automatic deletion remains disabled; a target switch is not a
retention or permission-revocation protocol.

Choose A if this remains a single-operator experiment and manual target-file
distribution is acceptable. Prefer B when several users should share one explicit
default. Revisit caching only after measuring target-resolution overhead and
specifying staleness; do not hide an unavailable catalog by switching generations.

## Remaining checkpoint

The user selected B and explicit default selection. A concrete default version,
connection/configuration and output formats, and public citation schema remain
open. Automatic activation on import is not selected. The first implementation
provides target resolution/activation for trusted internal callers; the user-facing
command will be a separate checkpoint.
