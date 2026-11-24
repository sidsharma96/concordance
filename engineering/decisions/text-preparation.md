# Preserve technical names in prepared terms

## Decision

The maintainer chose punctuation-preserving technical terms for the first lexical
preparer, after choosing whole CamelCase names. Apply the same lowercase rules to
source text and queries. Keep `C++` distinct from `C`, `readiness-probe` whole and
`v1.35` whole. Do not emit component words, stem, remove common words or add synonyms.
This is lexical preparation of documentation, not parsing source-code symbols.

The simpler alternative separates punctuation: `C++` becomes `c`, and
`readiness-probe` becomes two terms. That enables partial-word discovery but loses
some technical distinctions. Keeping names whole can miss a useful passage when a
reader supplies only part of a name. No relevance or performance measurement proves
this policy better. Revisit with new questions and reader judgments, including
unknown-name searches and names the grammar does not preserve.

## Initial grammar

The bounded first grammar makes the policy concrete:

- A term starts with a Unicode letter, decimal digit or ASCII underscore.
- Letters, decimal digits, underscores and combining marks continue a term. Capital
  boundaries do not split it. A combining mark cannot start a term.
- ASCII `+` and `#` continue an already started term, including at its end. Thus
  `C++`, `C#`, `C++17` and `x+y` remain whole; this is not operator interpretation.
- ASCII `.`, `-` and `/` continue a term only when the next character can start
  one. This preserves `v1.35`, `readiness-probe`, `net/http` and `kubernetes.io/name`.
  A final period and repeated separators are boundaries: `word.` gives `word`,
  while `a..b` gives `a` and `b`. Leading punctuation does not start a term.
- Everything else separates terms. Quoting, URL syntax, command flags, numeric
  signs, Unicode dashes and language-specific spelling have no special parser.

Map each retained rune through Go's `unicode.ToLower`. This is simple lowercase,
not full Unicode case folding, accent removal or canonical normalization. For
example, composed and decomposed accented spellings can remain different keys.
The term-version identifier includes both the grammar revision and `unicode.Version`;
another Unicode table version is incompatible. Original captured bytes stay intact.
Do not reinterpret stored terms under changed rules with an unchanged tag.

## Component boundary and limits

`internal/textterms.Prepare` implements one concrete operation, separate from
`termsearch.MatchAll`. It returns owned term strings, repeated occurrences in input
order, and the version tag used to prepare them. No token is a citation or source
identity. A format adapter still chooses body, headings and labels and preserves
their association with exact retained locations; this decision does not select that
projection or implement a CLI search command.

The [HTML field adapter](passage-fields.md) now supplies that format's projection;
the text preparer itself stays independent of HTML and source identity.

The caller supplies positive input-byte, occurrence-count, per-term-byte and total
output-term-byte ceilings and a context with a deadline. Per-term bytes bound both
raw spans and lowercase output; total output counts term bytes, not slice overhead
or the constant version label. Empty or punctuation-only text can return zero terms;
the matching operation separately rejects an empty query. Invalid UTF-8, NUL,
excessive input/output or cancellation fails with no partial prepared result.

Validate and count before allocating output, then scan again to produce independent
strings. Work is linear in admitted bytes and emitted bytes; temporary scanning
state is constant apart from output. Returned strings must not retain the original
input's backing storage. There are no remote requests, goroutines or retained cache.
These are cooperative logical bounds, not exact heap or CPU quotas. The operation
does not select editions, authorize content, rank results or publish a generation.
