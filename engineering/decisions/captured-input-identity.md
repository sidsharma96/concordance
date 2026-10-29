# Compare captured inputs, not generated results

Status: accepted. The user chose option A: a saved import key compares the complete
captured inputs and processing rules. Derived article and edition-index bytes do
not define whether two requests mean the same import.

## What must match

For the initial Git/HTML format, compare the authorized collection, stable source
and repository identity, exact revisions, complete selected scope and captured file
inventory, logical document/edition associations, actual retained HTML, recipe and
toolchain identity, and the processing rules. Original images and other captured
dependencies belong to the file inventory. A source adapter must establish complete
acquisition; a list of hashes supplied by a public caller cannot prove it.

Captured HTML means exact bytes, including navigation and comments. A different
capture under the same key conflicts even if its searchable words are unchanged.
Keep the original capture for an exact retry. This choice adds no durable draft
journal or automatic reconstruction/resume protocol.

Exclude the saved operation key itself, candidate generation ID, local checkout
path, credentials, attempt timestamps, derived article/index bytes and derived image
block ordinals. A new candidate ID must still recover the original completed result.
A new intended import gets a new saved key, independently of byte deduplication.

The source-specific preparation step remains trusted to supply truthful provenance
and complete captured dependencies. Core processors must implement their declared
versions. Changing a rule requires its version/settings to change. Comparing inputs
does not certify deterministic or semantically correct outputs.

## Why this boundary

Bazel's pinned [remote action schema](https://github.com/bazelbuild/remote-apis/blob/c2c07e3d718d3d490a4958344d9c3e47de0bfaf3/build/bazel/remote/execution/v2/remote_execution.proto#L674)
identifies work through command and input-root digests and describes outputs in a
separate result. Its environment entries have explicit ordering rules. DVC's pinned
[stage cache](https://github.com/treeverse/dvc/blob/56e59829512ff134aa269099a2099587b810b4dd/dvc/stage/cache.py#L28)
omits output hashes from the stage key and stores result metadata separately.
These observations, checked in October 2026, support separating input meaning from
output representation. Their build/data caches do not supply Concordance's saved-key
publication, source authority or recovery guarantees.

Bazel's [documented external-tool limitation](https://bazel.build/remote/caching#known-issues)
also explains the danger of undeclared inputs. Our Kubernetes reference render used
a reconstructed older toolchain and has no repeated-build determinism evidence.
Consequently, comparing only a Git pin and render-recipe label would omit the actual
HTML which our manifest already classifies as retained input.

The stricter alternative includes exact derived article/index bytes as well. It
catches output drift under unchanged version labels but requires those outputs to
compare retries and turns such drift into a conflict. We chose input comparison so
the original published result remains authoritative for the same requested work.
The existing publisher still preflights the entire candidate bundle, so this decision
alone establishes no cheap retry shortcut or measured speed improvement.

## Implementation rules and revisit

Use a separate versioned fingerprint specification, not the complete manifest hash.
The manifest contains the candidate generation ID and generated output references.
Normalize only explicitly unordered metadata; preserve meaningful document-slot
order because stable search ties currently observe it. Bind complete profile policies,
not merely their names. Keep admission and cancellation explicit and bound metadata
before hashing. A changed framing/field meaning requires a new fingerprint version;
retain comparison support for outstanding saved operations.

Confirm actual supplied artifact bodies before trusting their references. Bind the
computed input identity before remote writes, then retain existing guarded publication
and uncertain-outcome recovery. Fingerprinting cannot substitute for source capture,
producer checks, authorization, object confirmation or publication.

Revisit durable prepared captures when rebuilding or retaining original representations
makes real retries unreliable or expensive. Revisit stricter output comparison if
valid output drift under correctly declared rules requires fail-closed handling.
Neither concern permits silently weakening exact captured-input equality.
