# Typed local search and source reads

Status: accepted thin local adapter. `concordance tools --target NAME` exposes
`search(edition, query)` and `read(source_ref, scope)` through MCP over standard
input/output. It invokes the installed CLI with argument slices, preserving its
source checks and fresh per-call readers. No shell program is constructed.

## Why an adapter

An agent can already use the CLI, but compound shell programs can be rejected by
its execution permissions before reaching search. Typed tools remove that shell
construction step. They do not improve ranking, guarantee client authorization,
or reduce remote opening costs. Keeping the CLI alone is the simplest control.
Keeping readers alive across calls could reduce reads, but changes activation,
client ownership and memory lifetime; that remains a separate decision.

The official MCP Go SDK v1.4.1 supplies protocol negotiation and tool dispatch.
Its [typed tools](https://github.com/modelcontextprotocol/go-sdk/blob/580f2a09be5c5b7ed4d868aa74ad1019dfd5160a/mcp/server.go)
and [stdio transport](https://github.com/modelcontextprotocol/go-sdk/blob/580f2a09be5c5b7ed4d868aa74ad1019dfd5160a/mcp/transport.go)
implement the existing [MCP contract](https://modelcontextprotocol.io/specification/2025-06-18/server/tools).
We use low-level tool registration to preserve raw arguments and reject duplicate,
unknown, null and malformed string fields before decoding can normalize them.
The SDK is infrastructure; it does not define collection permissions or evidence
correctness. Its JSON decoder and request queue need application limits.

A preliminary standard-client check exercised initialization, tool discovery,
real search followed by a same-generation read, and missing-edition/malformed-ref
refusal. That retained the thin-adapter recommendation. It did not establish
agent usability, performance superiority or the production lifecycle safeguards.
Revisit this choice if actual clients cannot use the two tools reliably or repeated
cold opens dominate complete tasks after the interface is held fixed.

## Inputs and authority

The operator configures the target, credentials, result limit, deadline and optional
existing reranker. Tool callers provide no executable, route, environment or flags.
Search requires an exact nonempty edition and query; it resolves the configured
target on each call. Read accepts a source reference under the configured collection
and defaults to `exact`; `section`, `parent`, `previous` and `next` retain the existing
[source-read contract](captured-source-reading.md). Activation never redirects an
old reference. The reference is a locator, not permission or a session token.

The subprocess inherits trusted operator configuration. The host must authorize
collection use and keep catalog/object routes stable. Only search and read are
exposed, with no import or activation tool. Client permissions still apply.

## Bounds and outcomes

Search defaults to ten results; the operator may choose 1 through 100. The normal
CLI encoded-output limits remain 16 MiB for search and 256 KiB for read. An explicit
`--max-output-bytes` cap can lower them. Read keeps its separate 64 KiB body cap.
The default deadline is one minute, or five for opt-in reranked search; an explicit
operator deadline may be at most five minutes. Other CLI admission checks remain
in force. Valid source text can exceed an encoded budget and fail without truncation.

Tool success carries the existing CLI JSON in one MCP text item. It is not copied
into `structuredContent`. The MCP envelope escapes that text again; the payload
cap is not an all-wire-byte or heap limit. Subprocess failure, overflow, invalid
success output and cancellation return a sanitized tool error with no partial
success evidence. Child stderr is discarded. A later transport write failure can
still leave a partial protocol frame.

The session admits at most 256 single-object JSON messages, 256 KiB per message,
and 8 MiB of aggregate input payload. Counts include initialization, notifications
and cancellations; they are not a promise of 256 searches. Batches are rejected
before initialization too. Input EOF, malformed frames or exceeded bounds close
the session. The client reconnects explicitly. The cumulative caps also bound the
SDK's otherwise unbounded request queue; they do not bound all process memory.

At most one subprocess runs per server. Another overlapping tool call receives
busy rather than waiting in an application queue. Each operation owns its output
buffer and deadline, cancels on overflow and waits for the child. The server owns
its input/output streams and independently cancels operations on disconnect or
signal. It closes those streams before waiting so a peer that stops reading cannot
hold shutdown on stdout. This is necessary because the pinned SDK detaches request
contexts from server cancellation and drains handlers before closing streams.
A persistent model worker can still drain previously submitted work independently;
joining the CLI process is not proof that remote or worker computation has stopped.

## Verification

Synthetic tests exercise literal arguments, strict fields and byte bounds, busy
admission, no partial evidence on failure, cancellation with a waited child, EOF
and SIGINT during work, unread-output shutdown, protocol initialization/tool calls,
and frame/session limits. Ordinary checks require no hosted services or model.
Keep the native CLI as the behavior oracle. A live smoke check is read-only;
agent answer quality and whole-task cost require a separate frozen comparison.
