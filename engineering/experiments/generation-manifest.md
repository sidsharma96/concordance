# Bind a saved edition to its generation

`internal/generationmanifest` implements the accepted single, versioned JSON manifest
for an initial Git/HTML representation. It binds existing immutable-artifact codecs
to logical source/document/revision metadata. This is an internal metadata component,
not a complete importer, source registration, trusted preparer, input fingerprint,
authorization check, migration or catalog publication/recovery implementation.

## Supported representation

The closed root tag is `concordance.git-html-generation/v1`, with literal compatibility
tags `concordance.article/v1` and `concordance.html-edition/v1`. Do not substitute the
latest writer constants when reading retained versions. The explicit wire fields are:

| Record | Fields |
| --- | --- |
| Root | format, collection, generation, article_format, index_format, sources, editions |
| Source | id, repository |
| Edition | id, index, captures, documents |
| Capture | source, revision, directory, preparation, toolchain, recipe, files |
| File | path, blob_id, object |
| Document | capture, file, native_id, location, profile, html, article, images |
| Image | block, address, file |
| Byte reference | sha256, bytes |

Every field is required. Arrays can be empty; nulls are not substitutes. References
use lowercase SHA-256 hex and nonnegative integer lengths. HTML, article and index
objects must be nonempty; raw files and recipe bytes may be empty. Ordinals and sizes
reject negative signs, fractional/exponent forms, strings and overflow. Keys match
exactly after JSON escape decoding; duplicates, case variants, unknown/missing fields,
malformed Unicode or trailing values fail. Actual U+FFFD and valid surrogate pairs
remain valid strings. No blob keys, credentials, buckets or download URLs are inferred.

Source IDs are configured logical identities, separate from observed repository
strings. This profile admits one capture per source per exact edition. Revision and
blob IDs are full lowercase Git IDs with matching widths. Files retain full clean
repository-relative UTF-8 paths, case-sensitive and without normalization. Non-UTF8,
control characters, backslashes, absolute/traversing/unclean paths fail explicitly.
Native ID and location both equal the primary captured Markdown path in this initial
profile; it must be under the capture directory. A path key can recur across editions
or different sources, but not twice within the same source/edition.

Document order is exactly the edition index's logical article-slot order. Shared
bytes do not collapse logical documents. Image rows are increasing unique article
block numbers; `/images/...` maps exactly to `static/images/...` in the same capture.
Other address mappings need a deliberate supported profile. No figure interpretation
or safe same-origin embedding policy follows from retaining its original bytes.
Original Git line mappings are not supplied by this component; the retained article
remains quote authority.

Preparation/toolchain fields describe the configured producer. Recipe is a retained
byte input containing its configuration/description; this component never executes
it or interprets it as a universal build schema. Trusted source preparation must
still define and check complete selected inventory, raw/dependency/config identities
and the connection to exact rendered HTML. Hashes and labels cannot prove derivation
or repair unknown provenance in the offline Kubernetes reference experiment. Other
source/content formats need explicit typed compatibility work, not HTML coercion.

## Operations, checks and ownership

`Encode` validates a borrowed `Description`, then emits compact, fixed-key-order JSON
one bounded record at a time. Nil lists encode as `[]`. It preserves list order rather
than sorting index slots into different associations. The result owns its bytes.
This is a versioned writer, not a canonicalizer for logically equivalent unordered
capture requests or the complete captured-input fingerprint.

`Open` admits encoded bytes, checks independent expected length/SHA-256, validates
Unicode and strict tokens/relationships, and checks expected collection/generation.
It owns immutable private metadata and retains no caller byte buffer. Supplying an
expected hash oneself does not establish publisher authority or collection access.
Zero/nil manifests fail. Read methods may run concurrently; contexts need deadlines,
checks are cooperative and failures expose no partial result.

- `Objects` returns owned distinct `(SHA256, bytes)` references in first-reference
  order, excluding the manifest itself. Contradictory lengths for one digest fail.
  It includes every declared recipe, file, HTML, article and index reference, even
  those not needed by a particular query. It is a list to confirm, not positive proof.
- `OpenEdition` verifies the exact selected index bytes/id through `htmledition.Open`,
  then its article count/order/refs and declared image block bounds. It checks int64
  narrowing before bridging to the index's Go `int`. A missing edition is explicit;
  a declared known-empty edition still requires its real index object. Other index
  bodies and article bodies are not fetched or certified by this operation.
- `Resolve` copies one article slot's source/revision/location and original input,
  recipe, HTML/article and image references. It owns its image slice; strings are
  immutable values. Work is O(editions + sources + this document's images), without
  copying all document metadata for each hit. `Describe` is an O(metadata) diagnostic
  deep slice copy; mutable views of private state do not escape.
- `VerifyArticle` checks actual article bytes, decoded HTML digest/length/profile,
  and exact complete image block/address associations. It returns owned decoded
  structure. It does not fetch original HTML, source, recipe or figures, rerun
  extraction, or prove that the writer's text/source association was semantically
  correct. Quotes still use the same manifest-selected edition's saved range.

Source preparation and the writer remain trusted for semantic correctness, just as
with the counted index. Artifact checks detect swapped bytes and inconsistent
relationships. They do not establish acquisition completeness or authorization.
All required objects and manifest still need confirmation before catalog publication;
receipt-based publication must use `PublishInNamespace`. No automatic deletion exists.

## Resource model

All limits are positive caller-selected logical ceilings, not production size policy
or exact heap/CPU quotas. Global source/edition/capture/file/document/image counts
apply before growing decoder lists. Per-source file/document quotas and cross-reference
checks apply after that globally bounded decode, before exposing any state. String
limits count decoded value strings including repeated reference hex; fixed schema keys
are excluded. One decoder token can allocate under the encoded-byte cap before its
smaller value limit is checked. One encoded record can allocate under admitted string
budgets before its smaller output cap is checked.

Reference-byte admission sums every declared occurrence, including shared objects;
image ordinals do not charge their file bytes a second time. Physical confirmation
de-duplicates exact references separately. The manifest's own byte cap is distinct
from referenced-artifact byte caps. Maps, string storage, slice capacities, parser
buffers and diagnostic copies add overhead beyond logical counts.

For M encoded bytes and N records, opening reads/hashes M bytes and traverses admitted
records/strings; hash-map checks have expected linear aggregate work. It materializes
the complete flat metadata. This is not paged metadata or a million-document result.
Composing strict objectstore reads, manifest article verification and index quotation
currently hashes an article three times and decodes it twice, without extra network
reads. This conservative composition has not been benchmarked or optimized; there
is no cache or general compression.

## Observed tests and limits

The controlled fixture has two logical Markdown paths sharing one retained article,
one figure per document, one nonempty edition and a known-empty control. Its actual
manifest is 2,082 bytes and names seven distinct artifacts. These tiny synthetic
inputs do not measure Kubernetes metadata or production capacity.

Round trips preserve exact canonical bytes and logical associations. Tests cover
every truncation of the valid fixture, hostile field/number/Unicode forms, exact budget
boundaries counted independently from the JSON tree, all configured limits, duplicate
keys within and across sources/editions, swaps and count/range/profile/image mismatches,
alias mutation, zero state, concurrent reads and every observed cooperative cancellation
poll. Reopened quotes agree with independent joins of retained article blocks.

The SDK/HTTP fixture confirms all named objects plus manifest, clears writer buffers,
and uses fresh SDK/object clients to reopen. It reads manifest and one selected index
before search, then one selected article for a verified quote: three GETs total on
that path. Missing/corrupted declared article bytes and missing index objects fail.
It keeps a manifest reference in memory; it does not exercise PostgreSQL publication,
real R2 faults, writer-host loss or a production preparation pipeline.

Run the maintained checks with `go test ./internal/generationmanifest`; the decoder
fuzz target is `FuzzOpen`. A short fuzz pass is scoped evidence, not proof of all
inputs or application latency. No full generation fingerprint, live import, query
CLI, hosted PostgreSQL, retention/deletion or reproducible build is implemented here.
