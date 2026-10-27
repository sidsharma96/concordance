# Require every explicit keyword

## Decision

The user chose requiring every keyword for the first keyword-search baseline.
A passage qualifies only when every distinct prepared query term occurs in its
supplied body, heading or label terms. Terms in separate passages cannot combine.
The same term repeated in the query is one requirement. Matching selects candidates;
it does not establish whether they answer the question or which should rank first.

Requiring any term would admit more partial matches, including passages missing an
important word. Requiring every term gives a conservative baseline but can miss
useful passages with different wording. Neither rule alone solves relevance.

The operation is explicitly named `MatchAll`. There is no implicit switch to
partial matches when none qualify. A later alternative needs an explicit operation
or policy; adding it need not change the meaning of existing callers.

## Prepared terms, separate responsibilities

The first `internal/termsearch` component consumes caller-prepared terms. It does
not choose how raw text is split, how identifiers are treated, which case/Unicode
rules apply, or whether to stem words or remove common words. The private comparison
rules are not automatically a product tokenizer. Format processors and their adapters
still own passage structure, source locations and which context is supplied.

The user subsequently chose to keep technical names such as `ContainerRestartRules`
whole in the first text-preparation policy. Do not add `Container`, `Restart` and
`Rules` as searchable terms merely because capital letters mark their boundaries.
This is an accepted policy for future preparation, not a tokenizer implementation
inside `MatchAll`. It settles capital-boundary splitting only; case handling,
Unicode equivalence and punctuation boundaries remain open.

Keeping the name whole gives a simple baseline for explicit feature-name queries.
A query for the separate words `restart rules` cannot match that name alone; those
words must occur elsewhere in the supplied passage fields. The alternative keeps
the whole name and adds its parts, making partial-name discovery possible while
also admitting more matches. No experiment establishes a precision, storage or
latency advantage for either policy. Revisit with new questions where readers do
not know the feature name, and assess useful answers as well as added matches.

Apply compatible preparation to source and query terms outside the matcher. A later
policy adding name parts needs its own term-version identity and newly prepared
data; do not reinterpret existing prepared terms under an unchanged tag. This does
not define source-code parsing or commit to a universal tokenizer for future formats.

Body, heading and label terms remain separate in the input. Matching uses their
union; no scoring weights or body-evidence gate are selected. A query's term-version
tag must match the corpus tag. This guards against accidental incompatible inputs,
but a label cannot prove that the caller actually used the stated rules.

The exact-scan baseline operates within one requested edition key. Edition keys are
opaque and unique in the supplied corpus. Missing editions never fall back, and an
explicitly empty edition remains distinguishable from a missing one. The component
does not define release ordering, mutable aliases, authorization or publication.

## Bounded complete matching

The caller supplies positive limits for editions, passages, corpus term occurrences,
query term occurrences, logical input string bytes and returned matches. All input
is admitted and validated before matching, including unselected editions. Duplicate
terms and repeated string references still count against admission limits.

The operation requires a deadline, checks cancellation cooperatively and starts no
goroutines or I/O. A limit, malformed input, incompatible term version or cancellation
returns an error and no partial result. Excessive matches are rejected rather than
silently truncated. These are internal candidate-enumeration bounds, not a selected
user-facing result limit or a permanent product-size ceiling.

Returned integers are temporary positions within the selected edition's passage
order. They are not durable passage IDs or citations. The caller keeps their exact
association with source/document identity, revision, processing versions, retained
article locations and generation. The matching component neither authorizes those
bytes nor establishes that a generation was published.

No tokenizer, ranker, inverted index, persisted matching format or CLI search command
is included in this slice. Revisit the baseline with explicit alternative matching,
new queries/sources and reader judgments; preserve the scan as a correctness oracle
before optimizing with sorted postings.
