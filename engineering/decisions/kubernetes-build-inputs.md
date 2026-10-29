# Retain the checked Kubernetes build inputs

Status: accepted, option 2. Use the checked restricted preparation profile for the
first Kubernetes Pods corpus. This selects retained build inputs, not searchable
document membership or a generic dependency-discovery algorithm.

## Scope and invariant

Keep the original pinned English-content, template, asset, data, translation,
configuration and lock inputs, replacing the broad static tree with the original
figures referenced by all selected articles. The trusted preparer records the exact
path list, source pin, toolchain, recipe and exclusions before building. Image-provided
tools/modules remain identified through the toolchain image.

The checked profile is `kubernetes-pods-restricted-reference/v1`; its recipe format
is `kubernetes-restricted-reference/v1`. The extraction profile remains unchanged
because all 38 selected HTML pages matched the retained reference by exact length
and SHA-256. The three exact pins and toolchain limitations remain in the
[render reference](../experiments/kubernetes-rendering.md). Original figures are
retained; their visual contents remain uninterpreted.

Missing input, incomplete selected-page membership, changed HTML in the reference
comparison, failed rendering or exceeded bounds fail the preparation. Do not silently
publish a subset or revert to a broader dependency list. New source pins or changed
preparation need renewed validation and an explicit recorded dependency declaration.
The same test does not establish a universal minimum or prove arbitrary future builds.

Concordance still consumes the [trusted prepared handoff](prepared-capture.md).
It captures complete selected Markdown, declared dependencies and original figures
at the pin, then builds articles, indexes and the manifest. It does not run recipes,
infer a dependency closure or certify render execution. Source registration and
producer provenance remain trusted. A changed recipe/dependency set is new intended
input: use a new operation key, keeping original bytes for exact retries.

## Evidence, alternative and cost

The [restricted-input experiment](../experiments/capture-files.md#restricted-inputs-reproduced-all-38-reference-pages)
ran one successful build per pin. Both profiles produced identical selected HTML.
Including selected Markdown and dependencies, the broad set has 1,117,825,155 raw
occurrence bytes and an estimated 406,129,047 distinct Git bytes. The checked set
has 242,874,975 occurrence bytes and an estimated 107,573,369 distinct bytes, a
derived 73.51% reduction. Dependency path occurrences fall from 12,830 to 10,407;
the largest raw input falls from 16,483,782 to 5,605,160 bytes.

Keeping every original mounted file was the simplest credible alternative. It
avoids narrowing that staged archive, but preserves unrelated static assets and
adds acquisition, hashing, retention and metadata work. We choose the checked set
because the reduced inputs reproduced the selected pages while preserving figures.
Git-byte sharing estimates are not measured R2 size, request counts, heap or speed.
Queries need their selected index/article, rather than every raw build dependency.

As inspected in October 2026, Bazel 8.4.0's
[`SandboxHelpers.processInputFiles`/`SandboxInputs.limitedCopy`](https://github.com/bazelbuild/bazel/blob/526225e836561307065f2389c9e2163064fda084/src/main/java/com/google/devtools/build/lib/sandbox/SandboxHelpers.java)
construct/filter declared build-action inputs. We borrow the explicit-input principle
for a source-specific offline build, without adopting its build machinery or claiming
universal hermeticity. Hugo 0.144.2's
[`ShouldExcludeCoarse`/`ShouldExcludeFine`](https://github.com/gohugoio/hugo/blob/098c68fd18f48031a7145bedab30cbaede48858f/hugolib/segments/segments.go)
select outputs rather than file access; the current
[segment documentation](https://gohugo.io/configuration/segments/) retains the full
page graph. Therefore selecting 38 outputs cannot justify keeping only 38 Markdown
inputs.

## Admission, failure and revisit

Measure the actual local importer construction and reader costs before selecting
first-corpus input/object budgets. This decision sets no production defaults or
whole-process memory guarantee. All local checks must precede key binding and object
writes; all required remote objects still precede guarded PostgreSQL publication.
R2/PostgreSQL responsibilities, uncertainty handling, receipts and disabled automatic
deletion remain unchanged. No real generation was published by the preparation trial.

Revisit this profile when validation changes selected output, a needed dependency
falls outside its scope, the corpus expands or measured preparation/retention work
justifies a more precise dependency mechanism. Preserve the original reference and
the accepted profile's exact handoff; do not treat cached local images as a complete
host-loss rebuild plan.
