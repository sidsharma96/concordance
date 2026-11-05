# Document identity and observed location

Status: accepted design. The user accepted refined option A after external research:
separate document identity from location, use reliable source-provided identity where
available, and start Git with explicit source-plus-path identity. The bounded
Git/HTML manifest and prepared importer now bind these identities to complete
captured inputs and publication. Source registration and rendering provenance
remain trusted configuration/assertions; a cross-move registry is not implemented.

## Contract

Within an authorized collection, a configured source ID names the logical source.
Its continuity is an operator assertion, not something inferred from a local checkout,
mutable repository URL or matching bytes. Do not silently repoint it at unrelated
source history. Preserve captured repository provenance and exact revision separately;
historical reads must not reinterpret them through today's mutable configuration.

A document key combines that source ID with the adapter's native document identity.
Keep its observed path or URL, exact captured revision, product edition, processing
versions and byte references separate. A future connector may use a provider document
ID only after its lifecycle and permission contract is defined. Source-provided IDs
do not authorize access or guarantee one atomic snapshot across provider reads.

For the first Git profile, native identity is the exact full repository-relative
path, not a path relative to the selected import directory. Compare path bytes
case-sensitively without Unicode normalization. The initial Git/HTML wire rejects
non-UTF8, control characters, backslashes and unclean paths explicitly; it never
normalizes them into another key. A rename changes this key;
reusing a path retains path identity, not proof of conceptual continuity. Git revision
and edition are separate from the logical key.

For example, `docs/pods.md` becoming `docs/workloads/pods.md` produces a different
Git document key in the next generation. The old generation still binds its original
path, revision and retained bytes. No old citation is redirected to current content.
This neither deletes old objects nor prevents exact-byte deduplication.

Generation metadata must bind each edition-local article slot to its exact logical
document, revision and retained representations. Duplicate document keys within the
same source and exact edition fail admission; the same key across different editions
is expected. Distinct logical documents may share bytes but remain distinct slots.
Do not infer identity from index positions, content hashes, titles, redirects or
Git's rename classifications. Keep those relationships separate from authorization.

The coordinator's trusted captured-input description must include the resolved
source/document/location/revision associations and processing settings before binding
the saved import key and making remote writes. The supported manifest and
[captured-input comparison](captured-input-identity.md) now implement the initial Git/HTML bindings, and the prepared importer checks complete
selected Git membership. Neither metadata encoding nor hashing establishes source
authorization, truthful HTML provenance or dependency completeness. Aggregate and
per-source entry counts, per-key/path bytes and total metadata have explicit admission
limits; production ceilings still need a concrete workload.

## Evidence behind the choice

The [complete-snapshot regression](../experiments/prepared-import.md#complete-replacements-preserve-earlier-evidence)
now checks removal, addition, exact-byte rename and reuse of a removed path. Old
reader references preserve their original path/revision/quote; new generations
record the new path set without inferring lineage. This is local synthetic evidence,
separate from the real-corpus update replay and provider behavior.

- Git stores snapshots rather than persistent per-file identities. Rename and copy
  detection compare content; the configured similarity policy affects the result.
  Its exhaustive fallback may be quadratic, so detection has work limits. This is
  evidence for treating inferred lineage separately, not rejecting useful history
  traversal. See the [Git diff manual](https://git-scm.com/docs/git-diff/2.56.0)
  and the developer's [snapshot explanation](https://github.blog/open-source/git/commits-are-snapshots-not-diffs/).
- Docusaurus defaults to path-derived document IDs and recommends explicit slugs
  for stable URLs across file renames. Its front-matter ID only overrides the final
  segment; a stable URL is not a universal document identity. See
  [document IDs and URLs](https://docusaurus.io/docs/create-doc#document-id).
- Hugo uses explicit aliases to preserve navigation. The pinned Kubernetes
  [control-plane communication page](https://github.com/kubernetes/website/blob/8817d358864b70a9710c5ad34f78df6a239be33d/content/en/docs/concepts/architecture/control-plane-node-communication.md)
  includes `master-node-communication` as an alias. This is an alternate URL, not
  proof of logical lineage or its originating rename. See [Hugo aliases](https://gohugo.io/content-management/urls/#aliases).
- Nextcloud's inspected `Cache::moveFromCache` branch updates paths under the
  existing file ID. Its [source implementation](https://github.com/nextcloud/server/blob/b63fed182ee4c1b712373b92a9e37c51a78d258a/lib/private/Files/Cache/Cache.php#L755-L890)
  supports managed moves; it does not prove every backend preserves IDs. The
  [external-storage limitation](https://docs.nextcloud.com/server/stable/admin_manual/configuration_files/external_storage_configuration_gui.html#limitation-renames-are-not-detected-reliably)
  explicitly describes rescanned renames as deletion/creation with metadata loss.
- Paperless-ngx separates document records from filenames, supports managed
  renaming, and warns that manual storage moves break remembered locations. See
  its pinned [filename-handling documentation](https://github.com/paperless-ngx/paperless-ngx/blob/c21bdab5c88ab0624fcde787049436598ece9e85/docs/advanced_usage.md#file-name-handling).
  This is managed storage, not automatic matching of arbitrary external moves.

A synthetic local Git 2.54.0 experiment replaced one file with two identical files.
Exact rename detection chose the first as the rename; enabling exact copy detection
chose the other. The committed states were unchanged. This illustrates ambiguity
when candidates are indistinguishable; it does not establish a failure rate or cost
for normal renames. Upstream applications were read, not deployed or tested here.

## Alternative, costs and revisit condition

Concordance-assigned IDs plus an explicit recoverable mapping can support bookmarks,
annotations and history across moves. They need trustworthy move or identity input:
generating a UUID does not discover correspondence between two external snapshots.
Copies, path reuse, splits and merges require deliberate policy. Resolve and pin
any mapping per generation rather than consulting mutable latest mappings on reads.

With N document-version entries, the accepted flat manifest needs O(N) bindings
plus key/location bytes. Separating these fields does not inherently add a network
request per document or change posting search. A registry adds durable mapping state,
input-identity and recovery work; batching is possible. No measured speed or scale
advantage follows. Existing publication, verification, no-deletion and retention
limits remain unchanged.

Revisit cross-move lineage when stable bookmarks/annotations/history become a product
requirement, or a concrete source supplies reliable move events or maintained IDs.
Authored IDs need a deliberate adapter/profile contract and scoped duplicate checks;
do not interpret arbitrary front matter or a slug as a stable document ID. Introducing
lineage later requires an explicit compatibility/migration or alias design.
