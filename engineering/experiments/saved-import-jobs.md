# Reopen an import after its producer directory is gone

Status: the bounded local saved-job component implements the accepted
[owned-capture decision](../decisions/saved-import-jobs.md). The import command and
its native source/namespace configuration are still a later composition. Local
jobs preserve retry inputs; PostgreSQL remains publication authority.

## One verified read becomes one owned copy

`capturefiles.Copy` loads and hash-checks the descriptor and unique payloads before
its first sink callback. It writes those same owned buffers, not reopened producer
files. Only transport filenames change to a flat bounded set. Original Git paths,
profiles, source/edition metadata and payload bytes are preserved. The caller owns
destination creation, synchronization and any partial files. Callback bytes are
borrowed unchanged only through return. Shared payload paths stay shared; existing
file/payload admission continues to count every reference occurrence.

`ReadDirectoryExpected` compares exact descriptor length/hash on the same bytes it
decodes, before payload reads. The existing `ReadDirectory` behavior remains the
unconstrained transport entry point. Both return the same verified buffers later
used by the importer. Expected lengths bound the read to at most expected+1 bytes,
while the absolute descriptor ceiling and exact pin have distinct errors.

`importjob.Prepare` accepts trusted collection and nonsecret source identities,
copies the capture into a new private directory, generates one random saved key
and candidate, then writes a closed versioned record. It stores current processing
labels and final descriptor reference; the original descriptor reference is
provenance diagnostics only. Repository checkout routes and credential fields are
absent. Metadata/sources and supplied path strings have explicit limits.

Every file is exclusively created, written, synchronized and closed with errors
checked. Capture, job and parent directory entries are synchronized before the
ready marker is installed. That marker is a version plus record checksum: a fully
synced/closed private temporary file is linked under READY without overwrite.
Only this own marker inode is hard-linked; producer payloads are copied. The temp
name is removed and directories synced afterward. A final sync/close error can
leave a complete installed job even though Prepare returns zero/error; retain the
path and Open it rather than create another intended key silently.

Open validates the marker, exact record bytes, required case-sensitive fields,
unique keys, Unicode, null/type/number rules, source counts before array allocation,
current processing labels, the owned descriptor and all payloads. It checks exact
source-ID membership; trusted runtime registration must still supply the matching
observed repository identities and authorized local Git routes. It is read-only,
uses no catalog/object clients, and performs no Git or renderer operation. Returned
metadata/input buffers are owned and mutable; wrapper copies share slices, separate
Opens do not. Keep all aliases unchanged during the synchronous import.

## Actual local checks

The unit/race tests remove the producer after the first copier callback and still
reconstruct identical output. Another test removes it after Prepare, then opens
the job twice with the same key and independent buffers. Existing directories are
never overwritten; two concurrent Preparations have one winner and one existence
failure. A job inside its producer, including a symlink alias, is rejected so removing
the producer cannot also remove the saved job.

Fault injection covers twelve write/directory-sync/marker-install/remove boundaries
in the small fixture. No valid marker is accepted before complete local material;
errors after installation can reopen the original saved key. Separate file tests
exercise short writes and write/sync/close errors with unconditional cleanup.
Fresh child processes exit before or immediately after marker installation; parents
wait/reap them, remove the producer and invoke another process to open. Before the
marker, opening fails; after installation, the original key and exact capture open.
These are process-interruption checks, not simulated power loss or disk failure.

The PostgreSQL/controlled HTTP integration imports the saved fixture after producer
removal, publishes two documents, quarantines receipt reuse, then retries the same
key with the original captured namespace handle. It returns the original generation
with zero object requests after full local preflight. A strict published reader
still quotes `Pods\nRetry uploads safely.`. This does not implement the future
read-only namespace reopen API or prove fresh-client receipt trust after restore.
No hosted database/schema, R2 or other provider was accessed.

An opt-in local audit copied the real restricted Kubernetes capture and reopened it:
three editions, 38 pages, 44 owned regular files and 21,438,337 encoded file bytes.
The key was unchanged, and opened editions/payloads equaled the existing reader's
output exactly. The original capture/Git recovery material was untouched. The
new disposable audit job was removed because it never had remote effects. That
count excludes filesystem metadata overhead and pinned Git dependencies. It is
neither an import of a new real generation nor a latency/memory measurement.

The bounded record fuzzer and seeded malformed-input tests pass. No timings were
benchmarked. Full local checks use the dedicated PostgreSQL test database, not the
hosted catalog; private raw reports remain under `.tools/reviews/saved-import-jobs/`.

## A standard decoder was less strict than our record

The first case-altered-field test unexpectedly accepted `COLLECTION` despite
`DisallowUnknownFields`. The private `record-case-first.log` records the failure.
Go's standard JSON decoder supports case-folded field matches, so that option alone
does not enforce our closed case-sensitive schema. The record validator now checks
exact object shapes, required keys and source-array count before typed decoding.
The regression passes. Source-string/control/duplicate/null/Unicode/size/policy
failures return zero results, and errors identify incompatible policy field names
without printing arbitrary stored values.

## Limits and next composition

The job is an owned Git/HTML handoff, not source authority, a portable Git archive,
a distributed lease, a resumable per-object upload journal or a backup of its disk.
Pinned Git objects remain required even for completed retries while the importer
rebuilds/preflights. A compatible processing implementation remains required across
upgrades; Open rejects changed labels explicitly rather than silently substituting
new rules. A separate catalog status query is not added here.

The operator protects stable local paths. Native `os.Root` confines supported path
operations but is not an adversarial-local-filesystem sandbox or content snapshot.
Filesystem sync semantics are assumed; blocked local I/O is cooperatively bounded,
not forcibly interrupted. Concurrent invocations and retained jobs remain
operator-admitted. Full capture buffers, encoding scratch, repeated reads and
runtime bookkeeping add memory beyond logical budgets. Jobs duplicate payloads
locally; remote object-sharing/receipt/publication protocols are unchanged.

Next, compose source registration and namespace selection without accidental setup
or credentials in the job, and preserve completed-key recovery under quarantine.
Target activation remains an explicit separate command. Private documentation
updates are local and excluded from source history and publication.
