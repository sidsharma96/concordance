# Exact text navigation without a model

The [decision](../decisions/offline-text-navigation.md) records the recommendation
before the check: retain one bounded, verified text corpus and let independently
budgeted sessions look up documents, find literals and read exact byte intervals.
This checks whether follow-up reading can expose already-captured evidence. It
does not check whether an agent chooses that evidence or answers correctly.

## Inputs and admission

The existing `engineering/corpora/rfc-text.pins` inventory has SHA-256
`c6f4638c8df5b8b9451b423a226628e80806b1d052efb4e8df6e16f6d40079fe`.
Explicitly exclude `rfc101`, `rfc178` and `rfc237` for unsupported UTF-8, and
`rfc542` for NUL. The loader performs no encoding conversion or implicit omission.
The remaining 996 documents contain 56,255,149 raw bytes. The largest has 502,941
bytes; IDs and relative paths total 17,748 bytes. They fit the diagnostic limits
of 64 MiB total text, 8 MiB per document and 1 MiB logical metadata. These are
admission counts, not heap measurements or production scaling evidence.

## The missing definition

An earlier fixed snippet from RFC 9110's If-Match section referred to strong
comparison without supplying its definition. The diagnostic consumer can find
that definition elsewhere in the same exact captured document:

```sh
go run ./tools/rfcnav -root "$CAPTURE_BODIES" \
  -pins engineering/corpora/rfc-text.pins \
  -exclude rfc101,rfc178,rfc237,rfc542 -id rfc9110 \
  -op find -literal 'Strong comparison' -limit 4
```

The observed literal match is `[171112,171129)`, with `more=false`. A subsequent
invocation with `-op read -start 171111 -end 171242` returns exactly 131 source
bytes:

```text
"Strong comparison":  two entity tags are equivalent if both are not
      weak and their opaque-tags match character-by-character.
```

Both invocations identify the 502,941-byte document with SHA-256
`21c1cdce6ab0e5509b04d84a28000836c7a087cf786efe6f04877ebfff47232a`.
The excerpt's SHA-256 is
`818b4c1849a03788916e86e33d02f2b8d1e1744135dcce744aeae9cdfadd3dcc`.
The CLI counts the lookup and navigation as two calls in each invocation; only
the read charges returned source bytes. Its escaped diagnostics are human output,
not a machine protocol. Each invocation reloads the corpus; an in-process harness
can share one opened corpus across independently owned sessions.

## What the check establishes

The result retains the recommendation: the missing context is available through
bounded navigation without changing extraction, ranking or models. The synthetic
lifetime test also closes the root, changes/removes the input files and confirms
that the opened corpus still returns its verified original text. A new open
rejects changed bytes. This is owned snapshot behavior, not a filesystem snapshot.

Tests cover metadata admission before I/O, hash/content failures, exact and
over-limit reads, UTF-8 boundaries, cumulative read budgets, failed-attempt call
accounting, cancellation without partial results and literal pagination. A simple
scan is the find oracle, including a bounded fuzz run. Independently owned sessions
share the corpus under the race detector. No new dependency or model call is needed.

No answer labels, earlier scores, reserved questions or model configuration change.
No latency advantage, answer-quality improvement or retrieval winner follows from
this hand-selected navigation check. The next experiment needs frozen questions,
common tools and budgets, and separate judgments of discovery, reading and answers.
