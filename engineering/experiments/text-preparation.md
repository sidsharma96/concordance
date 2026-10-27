# Bounded technical-term preparation

## Operation and representation

The accepted [policy](../decisions/text-preparation.md) preserves whole technical
names. `internal/textterms.Prepare(ctx, text, limits)` returns `Prepared` containing
the grammar/Unicode version tag and owned term strings in occurrence order.
`termsearch.MatchAll` is unchanged and remains independent of text preparation.
An application adapter still needs to choose passage fields and retain their source
associations. No CLI query path, source registry, ranker or persisted term format
is implemented by this component.

The scanner keeps only the start byte offset of the active term and constant-size
UTF-8 lookahead. Letters, decimal digits and underscore start terms; combining
marks, plus and hash continue them; dot, slash and hyphen require a following start
character. The exact grammar and its exclusions are in the decision record.
Lowercase mapping is per-rune `unicode.ToLower`, not canonical normalization or full
case folding. The compatibility tag includes `unicode.Version` so a Unicode table
upgrade requires deliberate re-preparation instead of mixing terms silently.

## Admission, ownership and failure

Positive limits cover complete input bytes, every term occurrence, both raw and
lowercase bytes per term, and total lowercase term bytes. A first scan validates
UTF-8/NUL and checks counts before allocating output. It counts lowercase UTF-8
widths because a rune's byte width can grow or shrink when mapped. Remaining-byte
subtraction avoids overflow. A second scan emits each term through its own builder,
so even an unchanged lowercase term does not retain a large input allocation.

Input is borrowed unchanged until return; the returned slice and strings belong to
the caller. The operation starts no workers, makes no remote requests and retains
no cache. It requires a deadline and checks cancellation during both scans and
lowercasing, including immediately before success. Every failure returns a zero
`Prepared`, never a successful prefix. Empty and punctuation-only text can prepare
zero terms; `MatchAll` still rejects an empty query rather than matching everything.
Logical ceilings do not bound exact heap consumption or instant CPU preemption.

With fixed Unicode tables, work is worst-case linear in input and emitted bytes.
Scanning state is constant beyond the output and its current builder. Returned
memory grows with occurrence count and output bytes, including ordinary allocation
overhead. No latency or allocation measurements establish a production capacity.

## Evidence and limits

Synthetic examples cover whole names, surrounding punctuation, Unicode spellings,
repeated occurrences and intentionally unsupported syntax. Boundary tests cover
all byte/count limits, lowercase width expansion and contraction, invalid UTF-8/NUL,
missing deadlines and cancellation at each cooperative check. A backing-address
test checks that terms do not retain the original input allocation; no unsafe string
construction or mutation is used. An external-package fixture connects `Prepare`
to exact edition-scoped matching without choosing a real-article projection.

Five hundred fixed-seed generated inputs and a bounded fuzz target compare output
with a separate declarative regular-expression grammar and lowercase mapping.
They also check re-preparation of space-joined output is idempotent. The oracle
does not reuse production scanner predicates. Test passing is evidence for tested
executions, not a proof of correctness or general relevance. Local evidence and
review records remain under ignored `.tools/reviews/technical-terms-*`.

Raw source bytes and retained article locations are unchanged. Preserving a name
does not preserve every technical spelling, infer a programming-language symbol,
or make a partial-name query succeed. Evaluate new questions and sources before
adding expansions; those require a new preparation tag and newly prepared data.
