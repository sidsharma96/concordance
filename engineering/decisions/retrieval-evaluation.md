# Evaluate known useful passages before changing retrieval

Status: the maintainer approved 30 authored question intents, two query phrasings and
three pinned documentation editions, plus six negative controls. Labels received independent source-evidence review before freezing. The
bounded offline runner receives its own implementation review before the baseline.
A canonical passage can be long: the preStop answer is inside one outer list
covering 17 blocks. Its actual range is retained; shortening a fixture reference
would invent a passage the engine cannot return. Known-useful is not noise-free.
No retrieval-policy change or semantic model is selected by this checkpoint.

## Workload and labels

The set contains 30 information needs, each with a keyword query and an ordinary
question, across editions 1.34, 1.35 and 1.36: 180 answer-seeking runs. Six separate
single-phrasing controls across those editions add 18 runs. These are correlated
variants of 30 authored intents, not 198 independent user observations. Thirty is
an initial labeling budget for 38 captured document versions, not a statistical
sample-size recommendation or evidence of enterprise representativeness.

Twenty whole intents are for development. Ten are reserved across both phrasings
and every edition. The default initial run uses development plus controls: 138
runs. The maintainer subsequently released the remaining 60 for
[frozen reranker validation](../experiments/reserved-reranking-validation.md).
They stay outside tuning and are now observed validation evidence, not untouched
questions. The fixture keeps its original reserved split label and frozen hash.
This separation is procedural, not a secret or proof that pretraining never saw
the public material or authored questions.

A known useful reference contributes decisive factual evidence to the declared
information need. A question can need several passages; finding one reference
is not a claim to have supplied the complete answer. The same concept may move
between files or change feature maturity. Judge every edition separately from
its retained text. A missing file is not proof that a feature is unavailable.

The machine-readable golden set remains an ignored local artifact under the
repository's no-JSON-in-history policy. Commit the authored question inventory,
runner, review method and frozen set hash; retain a separate local backup of the
set. Ordinary checks use synthetic Go fixtures and require no corpus/private data.
This is not an exception or a renamed diagnostic artifact. The first set SHA-256
is `12debe31676faddc2ed38d33228ce5df0d3c2179b268eb1a19596772ba7d342f`.

References bind a source path, exact canonical article SHA-256, half-open block
range and quote SHA-256 within a pinned edition. Article/field/passage/term rules
are pinned independently. These are evaluation-fixture addresses, not a newly
supported public citation or result-wire schema. Full captured HTML/text is not
copied into source history. Candidate references and their rationales are checked
against exact retained passages before freezing the set and inspecting rankings.

The positive pool is deliberately incomplete. Every unjudged result is unknown,
not irrelevant. Report known-reference success at ranks one and five and the
first known-reference rank, with integer numerators/denominators grouped by edition
and phrasing. Do not call these exhaustive recall, precision, answer completeness
or a confidence estimate. Unknown results in the first five remain visible for
later judgment. Ranking evaluation at five does not change complete product output
to top-k. A new useful passage can extend a later version of the reference set;
never relabel a miss merely to improve a score.

Controls ask for a nonexistent identifier or topics outside the selected Pod
material. Review that scope before freezing. Incidental lexical matches are still
valid behavior for the existing keyword engine; report their count separately,
without silently adding answer generation or an abstention contract. Corrupt
inputs, incompatible processing, query/limit errors and deadlines are errors,
never ordinary relevance misses.

## Offline runner

`tools/evalsearch` will read a caller-supplied trusted prepared capture through
`capturefiles.ReadDirectory`, verify the expected descriptor and document inventory,
and reconstruct canonical article JSON plus one count/location edition at a time.
Use the existing `htmlarticle`, `htmlarticlejson`, `htmledition`, `textterms` and
`termsearch` operations; no second tokenizer, matcher, scoring formula or citation
resolver. Sort logical slots by source ID and full path as the importer does. Preflight
builds/validates each edition and discards its index; the query phase rebuilds
selected editions. That extra local build keeps one index reachable at a time and
prevents querying before every fixture binding has passed.
Validate every referenced article/range/quote, including reserved cases, before
running any query. This validates fixture bindings, not source-preparer provenance.

The command takes `-capture`, `-set`, `-split development|reserved|all` and a positive
`-timeout`. The subsequent [any-word comparison](../experiments/match-any.md)
adds explicit `-compare-any`, restricted to development, with full diagnostic
body/context evidence; ordinary mode and product search stay unchanged. The
[fixed noise-control comparison](../experiments/query-noise.md) adds development-only
`-compare-noise`, including both prior baselines, query-only question-word removal
and one explicit half-overlap threshold. It selects no product rule. Development
is the default. The [local reranking trial](../experiments/local-reranking.md) adds
mutually exclusive `-rerank-export` and `-rerank-scores FILE` modes, defaulting to
development. Explicit `-split reserved -validate-reserved` selects only the released
validation intents; comparison flags and all remain forbidden for this route.
Export carries no relevance labels; checked local scores bind the exact reconstructed
first-100 any-word bundle. This selects no production model or search behavior.
The Go audit requires no PostgreSQL, R2, model runtime or credentials
and makes no remote writes, activation or imports. Existing local payloads are
borrowed unchanged while processing; returned canonical bytes and the immutable
index remain owned until their edition is finished. Downloaded/prepared corpus
material stays private and external to ordinary source tests.

Read the trusted set under an 8 MiB cap with a closed version/field schema. Capture
and component count/byte limits remain explicit, compatible with the first corpus.
All capture payloads coexist in RAM; a complete edition build also has ordinary
scratch/decoded/index costs. This is bounded small-corpus admission, not a precise
heap limit or million-document claim. One process works sequentially, with no
queue, background workers or cache. Reference checks currently decode a complete article per reference occurrence,
including repeats; no checked-article cache is introduced. Work is linear in those
occurrence-weighted bytes plus both local construction passes;
queries retain existing posting-intersection and full-sort costs. No timing reruns
or provider-request performance conclusion belongs in this baseline.

After all selected cases succeed, emit one bounded diagnostic JSON report with the
set/capture identities, processing tags, split, counts, per-case complete match
count, first known-reference rank and first-five addresses/scores/judgment markers.
No partial report on corpus/query/format failure. Final writer errors can still
leave partial bytes. The format is internal diagnostic output, not the product's
deferred stable machine interface. Meaningful tests cover misbound articles/ranges,
incomplete inventories, erroneous query/limit cases, unknown versus known hits,
rank-five boundaries, split grouping and checked final writes.

## Primary comparisons and limits

Elasticsearch 8.19's [ranking evaluation documentation](https://www.elastic.co/guide/en/elasticsearch/reference/8.19/search-rank-eval.html)
uses typical queries and manually rated documents. It calculates standard metrics
from those judgments. We use a small known-positive set and explicitly avoid
claiming exhaustive judgments or inheriting its distributed query machinery.
BEIR's [pinned data loader](https://github.com/beir-cellar/beir/blob/ef83d29307061c65d04b035b4f4e7c18bd8374af/beir/datasets/data_loader.py)
separates corpus, queries and relevance labels. We borrow that separation without
its model/framework dependencies or claiming its benchmark diversity.

The simplest alternative is the current exact keyword smoke test. It verifies
bytes and version selection, but cannot establish useful results for varied
phrasing. A larger public benchmark would provide more questions while losing
this corpus's exact edition/processing bindings. Revisit size and metrics when
labels stabilize, additional source topics or actual user questions arrive, or
judging returned unknown passages exposes a systematically incomplete pool.

The next, separate [RFC corpus and question preparation](rfc-text-evaluation.md)
retains raw text and source-span evidence. It does not reuse this HTML passage
fixture schema or make the reserved Kubernetes cases pristine again.
