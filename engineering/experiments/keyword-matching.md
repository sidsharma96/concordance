# Exact scan over prepared keywords

The accepted [matching decision](../decisions/keyword-matching.md) requires every
query term. `internal/termsearch.MatchAll` now implements that candidate-selection
operation. It accepts prepared terms; it does not tokenize text, score results,
build postings, read storage or expose a search command.

## Representation and invariant

A corpus has a term-version compatibility tag and uniquely keyed editions. Each
edition contains ordered passages with separate body, heading and label term slices.
An empty edition is valid. The selected edition's passage order supplies temporary
result positions; the caller retains their exact source/citation association.
These positions and the compatibility tag are not authorization or persistent IDs.

The matcher compares exact term strings. It neither splits identifiers nor changes
case, Unicode form, punctuation or spacing. A token policy must be chosen and applied
by a caller before this operation. Matching a tag cannot prove correct preparation.

First admit edition/passage/term counts and logical string bytes across all input,
including unselected editions and repeated strings. Validate strings, unique edition
keys and compatible term tags before matching. Query duplicates are admitted as
provided but represent one matching requirement. Empty queries are invalid.

A query map assigns each distinct term a small position. While scanning one eligible
passage, reusable flags record which query terms have occurred across its fields.
The count increases at most once per query term. Emit the passage position exactly
when every flag has been reached; reset flags before the next passage. Input order
makes output order deterministic without sorting. No term from another passage or
edition can complete the current passage's match.

All positive limits belong to the caller. They cover editions, passages, corpus
term occurrences, query term occurrences, logical input string bytes and returned
matches. Excessive output is an error, not an implicit top-k selection. Any failure,
including late cancellation, returns a zero result. A deadline is required and
checked cooperatively; the operation starts no goroutines. Limits are not a precise
heap or CPU quota, and admitted text is not automatically authorized text.

The operation borrows unchanged input until return and retains none of it afterward.
It owns only its returned integer slice. The caller still needs the correct passage
registry when interpreting those integers. There is no index generation, persisted
format or publication in this component.

## Cost and oracle

For B logical string bytes, E editions, N passages, O corpus term occurrences,
Q query terms and H matches, admission and validation inspect the complete input.
Matching scans terms only in the selected edition. Expected work is linear in
B + E + N + O plus the query input, using hash lookups. Hash maps do not promise
constant worst-case lookup time. The extra space is O(E + Q + H); caller input
already exists outside that working space. Input text is not copied or retained.
There are no remote requests.

Tests keep an independent nested-loop oracle: for each passage and each query term,
scan all supplied field terms using equality, without lookup maps. Its worst-case
comparison count grows with selected term occurrences times query occurrences.
Generated and bounded fuzz fixtures compare results with this simpler operation.

## Checks and reference corpus

Tests cover cross-field and label-only matches, query duplicates, no combination
across passages, exact strings, missing/empty editions, compatibility errors,
invalid metadata/terms, exact/excessive budgets and complete-output failure behavior.
A deterministic context seam cancels the final check after matches were built;
no timing sleeps establish the boundary. Input immutability and independent result
storage are checked separately.

A private adapter compared the new matcher with the previously frozen 28
query/edition cases over 38 retained Pods articles and 1,889 original passages.
It used the same explicitly experimental Unicode-runs preparation as those cases,
not a new product tokenizer. All candidate IDs agreed, including feature-label
matches and missing-edition/absent-identifier controls. This is a mechanical
matching check, not reader relevance, ranking quality or production capacity.
The private corpus is not required for normal source tests.

The first public query path still needs a chosen tokenizer, source-to-term adapter,
identity/citation binding, ranking and the existing publication/recovery work.
Preserve this scan as an oracle when implementing sorted postings; no current
measurement justifies replacing it with an optimized index yet.
