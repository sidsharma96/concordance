# Read-only conditional HEAD experiment

This is a narrow provider observation supporting the proposed
[receipt protocol](../decisions/verification-receipts.md), not an implemented optimized
verifier or approval of a receipt persistence schema.

## Workload and operation

On Go 1.27.2/macOS arm64 with the already pinned AWS S3 v1.114.5 client, read the
existing 45-byte synthetic object retained by the earlier R2 smoke test in the
private application bucket. No new object, receipt or generation was written.
Credentials were loaded locally without printing them. The disposable probe and
full identifiers remain in ignored local evidence, not fresh-clone dependencies.

The probe used a deadline, one-attempt SDK operations and a trusted HTTPS endpoint.
It read at most expected length plus one byte, checked raw SHA-256/length and a
successful close, and captured the ETag from that same GET response. It then:

1. Sent HEAD with `If-Match` using that verified response's opaque ETag and checked
   returned length and tag.
2. Sent HEAD with a deliberately wrong ETag and required HTTP 412.
3. Sent HEAD to a fresh synthetic missing key and required HTTP 404.

Transport instrumentation counted SDK HTTP requests and response-body bytes consumed
by the client. Header/TLS bytes and underlying connection activity were not measured.

## Observed result and limits

One execution passed. The valid HEAD returned the expected length and tag. The
wrong condition returned 412 `PreconditionFailed`; the missing key returned 404
`NotFound`, not the strict GET path's modeled `NoSuchKey`. The GET count was one
with 45 response-body bytes; HEAD count was three with zero response-body bytes.
No PUT, DELETE, LIST, policy change or receipt operation occurred.

This confirms those operations interoperated with R2 on that one object. It is not
a latency benchmark, byte-savings measurement for a real corpus, durability proof,
same-tag corruption detector or missing-bucket/permission fault test. A HEAD match
does not independently verify the body. The proposed receipt/namespace/catalog
protocol still needs implementation, race/fault tests and fresh-reader recovery.
The current `Ensure` and `Read` contracts remain unchanged.

Private evidence: `.tools/reviews/verification-receipts-provider.txt` and
`.tools/probes/receipt-head/`. Revisit the provider conclusion when SDK/provider
behavior changes or the exact production operation differs from this probe.
