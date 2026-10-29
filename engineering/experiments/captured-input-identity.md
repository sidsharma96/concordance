# Captured-input comparison and publication

The [accepted input-identity decision](../decisions/captured-input-identity.md)
now has a bounded implementation. `generationmanifest.Manifest.InputFingerprint`
projects an already validated Git/HTML manifest and the producer's explicit rules.
`generationpublish.PublishCaptured` first performs complete artifact preflight, then
computes this identity and binds it before any remote request. Neither operation
captures source or proves truthful/complete preparation.

## Versioned comparison specification

The catalog's fingerprint version is `concordance.git-html-input/v1`. Its SHA-256
preimage uses fixed-order fields, not the manifest JSON or exported struct layout:

- Every string: an eight-byte unsigned big-endian UTF-8 byte length, then exact bytes.
- Every list: an eight-byte unsigned big-endian count, then its records.
- Every object reference: raw 32-byte SHA-256 followed by an eight-byte unsigned
  big-endian nonnegative byte length. The validated manifest supplies those values.

The ordered root fields are fingerprint-version string, collection, retained article
format, retained index format, article processor, passage processor, field processor,
term-preparation version, profiles, sources, editions. Supported processing tags are
closed to `htmlarticle/v1`, `htmlpassage/v1`, `htmlterms/v1` and
`technical/v1/unicode-17.0.0`; retained formats are the manifest v1 codec's literal pins.
A changed framing/field meaning needs a new fingerprint version and support for old
outstanding comparisons. An operational deadline, pool size or admission ceiling
is not a processing transformation and is not hashed.

| List | Record fields in order | Ordering |
| --- | --- | --- |
| Profiles | ID, article class, omitted classes, omitted IDs, notice classes | ID; membership lists sorted by token |
| Sources | ID, exact repository identity | ID |
| Editions | ID, captures, documents | ID |
| Captures | source ID, revision, directory, preparation, toolchain, recipe reference, files | source ID |
| Files | path, Git blob ID, raw reference | exact path |
| Documents | resolved source ID, resolved source-file path, native ID, location, profile ID, captured HTML reference | preserved slot order |

Lists use exact bytewise string order; no path, case, URL or Unicode normalization.
Capture/file ordinals resolve to names before hashing, so correctly remapping an
unordered metadata list preserves identity. Document slots remain meaningful for
stable ranked ties. A producer must establish deterministic slots before constructing
its index; the fingerprint never rearranges an already built index.

Profiles must exactly cover used document profile IDs. Missing, extra or duplicate
IDs, invalid tokens, duplicate membership tokens and omission of the root class fail.
The entire profile policy is included even when its ID is unchanged. A profile name
alone cannot identify changed settings. Identity strings must not contain credentials.

Candidate generation IDs, derived article/index refs and derived image block ordinals
are excluded. Original media/dependency bytes remain covered by captured files;
image addresses already occur in retained HTML under the fixed v1 mapping contract.
The fingerprint is not authorization, verification proof or a build attestation.

## Admission, ownership and failures

`maxBytes` bounds the exact framed metadata stream, including prefixes and version
strings. A first traversal counts without hashing or sorting. Only admitted metadata
allocates ordering scratch and profile-ID maps; a second traversal hashes in canonical
order. Manifest metadata was independently bounded when opened. This extra ceiling
does not bound the original captured payloads, Go heap, codec allocations or RSS.

Work is O(B + sum(N log N)) for framed bytes B and sortable list sizes N. Scratch is
O(metadata ordering and profiles); body bytes are not retained or downloaded by the
fingerprint. Sorting and individual bounded hash writes are nonpreemptive; traversal
checks a deadline context cooperatively. Errors expose a zero digest. The immutable
manifest is shareable; callers keep rules and all aliases unchanged through return.
The digest is an independently owned array value.

`PublishCaptured` borrows the same bodies as `Publish`, performs the same byte/count
admission and every index/article check, and derives `catalog.Request` inside the
publisher from the validated manifest and supplied rules. It accepts no caller digest.
The trusted owner still establishes acquisition completeness, provenance and matching
actual processing. The lower-level trusted-request `Publish` remains available;
neither method is the public source importer.

Invalid artifacts/rules fail before a key is bound. A conflicting key performs no
extra S3 requests. An identical completed binding returns its original result without
S3, including after quarantine; full local preflight still runs. New confirmations
still precede the manifest and guarded `PublishInNamespace`. Ambiguous outcomes,
nonwaiting per-instance admission, no automatic resume and no deletion are unchanged.

## Scoped evidence

Unit tests freeze independently framed vectors: the empty-edition preimage is 267
bytes; the richer source/profile/recipe/files/two-document fixture is 1,273 bytes.
Both exact budgets pass and one byte less fails. Expected hashes were computed from
the field specification independently of the production writer. These sizes describe
comparison metadata, not object bodies, heap or a production-size estimate.

Sensitivity cases cover collection/source/repository, commit/scope, preparation,
toolchain/recipe, actual inventory/blob/body refs, document associations/HTML, exact
edition presence, slot order and full profile policies. Candidate/output changes
preserve identity. Remapped source/edition/capture/file order and profile-set order
preserve it. Tests also cover version rejection, malformed/missing/duplicate policies,
context admission and deterministic cancellation during traversal, caller ownership
and concurrent immutable reads.

Publication tests use the actual SDK with a synthetic HTTP service and dedicated
local PostgreSQL 14.17 schemas. A first publication saves the derived fingerprint;
a new candidate with identical inputs recovers the original result with zero extra
S3 calls. Changed captured HTML, scope or same-ID profile policy conflicts with zero
extra S3 calls. A deliberately modelled producer bug changes derived article/index
bytes under unchanged inputs/rules; the completed retry returns the original result
and never uploads that candidate. This is not observed renderer nondeterminism or
permission to omit processor version updates. Invalid input never binds or uploads;
new keys permit new publications, and completed retries survive quarantine.

These are local invariant/failure checks. No new live R2/droplet run, source producer,
cheap retry shortcut, durable capture journal, throughput or million-document evidence
is claimed. Research and the alternative comparison are in the accepted decision;
selected raw logs remain private under `.tools/reviews/captured-inputs-*`.
