# Read surrounding captured source

A useful search hit can omit a prerequisite elsewhere in its section. Keep search
focused and provide an explicit bounded read with section, previous/next passage
and parent navigation. Matching and source inspection solve different problems;
this does not change all-term BM25 or make reranking the default.

## Alternatives and evidence

Automatic neighbors avoid another call but add text to every result and can miss a
more distant prerequisite. Whole-document output is closest to a native file read,
but can consume much more model input. Keeping search-only or exporting captured
files remains a credible control. Source inspection does not repair candidate
eligibility or prove that an agent will notice a qualification.

[LlamaIndex's pinned sentence-window parser](https://github.com/run-llama/llama_index/blob/26c7dc3e9846045864cf93ab92cb13bc80a3394e/llama-index-core/llama_index/core/node_parser/text/sentence_window.py)
attaches neighboring sentences to small retrieval units.
[LangChain's pinned parent-document retriever](https://github.com/langchain-ai/langchain/blob/e5e1d6c70521347e88ad0064b7cd58021fba09aa/libs/langchain/langchain_classic/retrievers/parent_document_retriever.py)
indexes children associated with larger stored parents. These separate matching
size from reading size; neither supplies our authored ranges or publication checks.
[Exa Contents](https://exa.ai/docs/reference/get-contents) separates content access
from discovery. Concordance must always read the exact captured generation rather
than substitute live content. These comparisons introduce no dependency.

The pre-recorded structural check compared immediate neighbors, containing sections,
parents and full documents around known missing evidence. Neither fixed neighbors
nor the nearest section covered every witness. This supports explicit navigation,
not automatic expansion or a claim of better final answers. Revisit the policy if
a controlled agent comparison shows that navigation is unused, excessively costly
or still fails to expose necessary context. Raw corpus records remain private.

## Reference and command boundary

JSON search adds `source_ref`; existing response-local `r1` labels stay temporary.
`read --scope section SOURCE_REF` opens that captured section. The default `exact`
scope reads the reference's complete block range. Returned `navigation.previous`,
`next` and `parent` values are references to follow with default scope; null means
unavailable. Explicit scope flags can also navigate from any returned reference.

The `cs1.` locator encodes canonical JSON as unpadded base64url, bounded to 144 KiB (including worst-case JSON escaping of admitted strings).
It contains collection, generation, edition, edition-local slot, source/native ID,
expected article SHA-256 and a half-open block range. Re-encoding must reproduce
exact bytes; unknown/duplicate/missing/null fields and noncanonical values fail.
It is neither encrypted nor signed, and never establishes authorization. Operators
may read the configured authorized collection, not only previously issued locators.
No object URL, credential or target name is embedded. Future formats need a new
explicit typed locator rather than being coerced into HTML ranges.

Parsing and collection checks precede connecting. `publishedsearch.Open` checks
catalog publication and strictly verifies the exact manifest/index. `ReadSource`
checks source/slot/hash/index bindings and declared article size before its GET,
then verifies the article's bytes and provenance. Recomputed canonical passages
must contain both anchor boundaries. A reference from generation A still reads A
after activation of B, or fails if A is unavailable. It never resolves to B.

`htmlpassage.Navigate` owns HTML section interpretation. Outer headings end at the
next peer/ancestor heading; headings inside notices, tables or other containers
cannot split sections. Boundaries expand to whole canonical passages. Previous
and next are outside the current returned range; parent is strictly larger, with
the document root as the final explicit parent. Owned output preserves LF-joined
body text, ancestor headings and notice labels with their exact block ranges.
Labels remain separate from body quotes. Whole authored units never silently split.

Read defaults to 64 KiB body, 64 KiB heading/label context and 256 KiB encoded JSON,
with existing positive artifact/count/deadline bounds. Oversized reads fail rather
than truncate or silently page. Read an exact/adjacent passage or deliberately
increase the appropriate limit. All verification/format failures return no success
body; a later stdout device failure can still leave partial bytes. The output
format is `concordance.docs-read/v1`.

## Cost and validation

Each fresh CLI read opens the manifest/index and strictly downloads one entire
article. Smaller returned sections reduce model input, not article-transfer bytes.
Beyond canonical Build, navigation uses linear block/group/passage scans, an outer
heading stack of depth at most six, and linear scratch. Text ownership is independent
of read buffers. Metadata and index remain bounded in-memory state; this is no
million-document claim, persistent service, cache or new concurrency mechanism.

Synthetic tests exercise activation between search/read, native identity/hash/slot
mismatches, missing editions/generations, corrupt bodies, complete authored units,
section/parent/neighbor edges, canonical locator decoding, cancellation and output
limits. The agent comparison freezes the same questions, corpus, model, instructions
and budgets; only capability instructions differ. Historical search-only results
are historical controls, not an isolated causal estimate of this feature's effect.
