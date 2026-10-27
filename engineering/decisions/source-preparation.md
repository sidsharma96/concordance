# Source-specific preparation, format-specific processing

Status: accepted boundary and a Kubernetes-only offline rendering experiment.
The user agreed that capturing and understanding different sources can vary, while
retrieval remains shared. Rendering is not a requirement for every future source
and is not implemented as a production ingestion path.

## Responsibilities

Acquisition retains authorized source material and provenance. A source-specific
preparation step may need a website's renderer and dependencies. A format processor
extracts structure and passages from the retained representation. Retrieval consumes
those passages and metadata; it must not invoke Hugo, scrape a provider or understand
Kubernetes template names to rank a query.

For example, plain Git Markdown may be processed directly. A templated Git website
may need its own build profile; a published website may instead provide captured
HTML through HTTP. A provider API can supply blocks or an export with its own
revision and permission semantics. These are future integration examples, not a
universal scraper, implemented connectors or a finalized plugin interface.
Even HTML main-content selection may need a site profile. Keep that configuration
outside ranking and avoid a generic framework before concrete consumers need one.

## Kubernetes experiment

The initial source contains template instructions whose output depends on feature
data and included YAML. Instead of first implementing part of Hugo ourselves, use
the pinned website's rendering behavior as an offline reference. Retain original
source provenance and exact rendered bytes with renderer/config/dependency identities.
Do not fetch a mutable live page and claim its bytes came from our pinned Git commit.

The [render experiment](../experiments/kubernetes-rendering.md) produced all selected
10/13/15 article pages and verified two changing feature notices and an included
YAML example. It also found one generated print page per edition. These observations
do not establish full extraction coverage, determinism, production build equivalence,
search relevance or citation recovery. No importer or common HTML processor exists yet.

Do not let a build see application credentials or execute templates in a query
process. Pin and bound each experimental build; missing inputs or failed rendering
are explicit failures. A container is a limited resource/access boundary, not proof
that arbitrary hostile templates are safe. Production admission and build provenance
still need a concrete implementation and tests.

## Alternatives, costs and revisit

A limited source-aware expander could retain precise Git spans and avoid a whole
site build. It would need truthful semantics for the supported templates, dependencies,
translations and parameters, including multiple origins for an assembled passage.
Removing unknown instructions is not complete extraction. Source expansion remains
a credible choice where Git-line citations or build restrictions dominate.

Rendering reuses the site's semantics but adds toolchains, dependency acquisition,
build resources and retained HTML. Raw rendered pages include navigation and generated
duplicates; they cannot simply be indexed whole. HTML citation locations must refer
to the retained representation, not invented mappings to Git source lines.

The three selected article sets contain about 20.7 MB of rendered HTML versus
555,019 bytes of source Markdown; these are different representations with different
included material, not an efficiency benchmark. Static assets were excluded from
the bounded render. Extra storage and extraction work must be measured on actual
retained objects before making cost claims.

Revisit the preparation approach if pinned builds cannot be acquired/bounded or if
the processor loses meaningful notices, tables or code. The unavailable older image
and reconstructed environment make this a reference experiment, not an accepted
repeatable deployment pipeline. Define passage/citation semantics and a complete
input fingerprint before fixing the manifest schema or publishing searchable content.
