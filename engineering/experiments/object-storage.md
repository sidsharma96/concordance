# First immutable object adapter experiment

Status: implemented in `internal/objectstore` under the accepted
[object-sharing decision](../decisions/object-sharing.md). Real SDK HTTP fixtures
pass locally. The explicit live R2 smoke probe also passes for creation, reuse and
a fresh-client read. This is not complete import recovery or provider fault evidence.

## Concrete component

Use the official AWS SDK for Go v2 S3 client, pinned S3 module v1.114.5 (Go >=1.24,
compatible with our Go1.27 baseline). This is infrastructure, not a custom S3 signing
or HTTP client. Borrow a concrete configured `*s3.Client`; its owner supplies endpoint,
region, credentials, HTTP timeouts and trusted SDK customization. The collection store
owns no credential discovery and must never log captured data or provider messages.

The internal `objectstore` owns one bucket/authorized globally unique collection ID
and a positive maximum object size. Collection scope uses a reversible unambiguous
hex encoding of UTF-8 bytes, not path joining or a document/content hash. The proposed
key is `v1/collections/<encoded ID>/sha256/<digest>`. This is an experimental internal
storage naming version, not a public connector or document schema.

Concrete operations: Ensure a caller-owned byte slice (stable until return), and
Read an immutable reference of digest/size. Return caller-owned bounded buffers.
Require a deadline and validate scope/reference/size before remote access. No delete,
listing, range, multipart, cache, catalog transaction or shared Rewind module is added.

## Verification and bounded attempts

Ensure computes the raw hash, does one bounded GetObject, and reuses a correct
existing object. Only a modeled NoSuchKey response permits conditional PutObject.
After any create attempt, do one bounded verification GetObject. Return success
only for verified expected bytes. If the object is wrong, return integrity failure;
if verification fails after the create attempt, return uncertain outcome with
sanitized underlying causes. Retry Ensure with the same bytes and authorized scope.
The stored object is opaque `application/octet-stream`; interpretation belongs in
generation metadata. No ETag or caller-written metadata is treated as raw SHA-256.

Use single-attempt per-operation SDK options to keep hidden retries from multiplying
the baseline. Required-only SDK request/response checksum configuration avoids relying
on unsupported optional R2 checksums; our explicit SHA-256 readback remains authoritative
for this adapter. Disable SDK payload/debug logging for these operations. SDK/transport
and credentials are trusted, not an arbitrary third-party middleware sandbox.

Read checks an advertised response length if supplied, then reads at most expected
size plus one byte, rejects extra/truncated/wrong-hash content and closes the body on
every path. A close failure cannot be returned as successful integrity verification.
Preserve errors.Is/As causes but expose only safe operation text from
adapter errors; server error messages can contain content and must not be formatted.
NoSuchBucket and AccessDenied must not become NoSuchKey.

Cap error/control bodies at 64 KiB before SDK decoding, with one overflow-probe byte.
The pinned SDK buffers error XML before returning a service error; the successful-body
limit alone would not protect that path. Oversized errors must not be parsed as
successful absence. The transport still owns header limits and connection buffering.

## Cost and experiments

For B object bytes, Ensure's hashing/read verification is O(B) time and O(B) response
memory, in addition to the caller's input buffer. A verified existing object uses
one GET; a missing object normally uses GET(absent), conditional PUT and GET(verify).
At most three SDK logical requests per call, with retries disabled; custom transports
are outside that attempt count. New objects transfer their body both to and from
storage. Duplicate objects still incur a verification read. This saves stored bytes,
not a promise to avoid every repeated transfer or to improve latency.

Compare with a blind conditional PUT plus readback, or a HEAD-first metadata check.
The first may upload duplicate bytes; the second does not verify raw content by
trusting ETag/metadata alone. Measure before optimizing. Storage namespaces grow with
unique serialized contents; no deletion means unreferenced objects accumulate.

Use the real SDK against a synthetic httptest S3 endpoint for deterministic tests:
new/repeated/empty bytes, same bytes in different collections, identical concurrent
creators, wrong stored content, lost PUT response after commit, absent after an
uncertain write, permission/bucket errors, truncated/excess/wrong-content reads,
pre-cancellation, post-write cancellation and read/close faults. Count requests and
assert conditional headers; faults occur at explicit handler/channel boundaries,
never arbitrary sleeps. These tests do not establish actual R2 behavior.

The user authorized creation of the dedicated `concordance-data` bucket. Wrangler
reports Standard storage in WNAM, public r2.dev access disabled and no custom domains.
The only lifecycle rule aborts incomplete multipart uploads after seven days; no
completed-object expiration rule was listed. Keep that provider cleanup distinct
from application object deletion, and leave it unchanged.

With S3 credentials configured locally, the opt-in `TestR2LiveImmutableObject` passed
once on Go1.27.2 darwin/arm64 against the private bucket. It ensured the same 45-byte
synthetic body twice, checked equal references and read matching bytes through a
new S3 client. The test body reported 2.08 seconds; this single smoke execution is
not a latency benchmark. One object remains in its unique collection namespace.
Full identifiers and the observed output are kept in private local evidence.

The test loads credentials from its process environment; `.env` is ignored and
loaded explicitly by the shell. No credentials appear in evidence. Bucket metadata
alone is not adapter evidence, and this smoke test does not exercise a competing
conditional create, lost provider replies, cancellation, corrupt objects or full
PostgreSQL publication/recovery. Those remain required before the storage milestone
is complete. Do not expose the bucket or extract shared storage implicitly.
