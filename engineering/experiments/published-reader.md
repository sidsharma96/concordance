# Reusing a catalog-published edition reader

## Workload and invariant

The [accepted reader](../decisions/published-reader.md) owns one published
generation/edition view for repeated queries. The synthetic local producer builds
two logical Markdown documents sharing one retained HTML article, an original
figure and an empty control edition. Actual article, field, count-index and manifest
components construct the fixture. The normal publisher confirms and publishes it
through real PostgreSQL and the S3 SDK against a local HTTP fixture.

The reader must preserve exact ranking and provenance, fetch no article during
search, keep old views unchanged after a new publication, and expose no reader or
partial query/quote on failure. These are internal operations, not CLI commands.

## Observed operation counts

`TestReaderReusesOpenedStateAndPinsGeneration` compares the reader with the writer's
independently held index. One reader serves 100 queries with exactly two opening
GETs and no further search requests. Reopening/searching separately 100 times needs
200 GETs. The complete ranks/scores remain equal. These are HTTP operation counts,
not latency, heap, relevance or production-throughput measurements. The fixture is
tiny; it does not establish the cost of opening the real flat manifest.

After publishing a different generation, the old reader still returns its original
matches and exact quote. The new reader sees the changed text and rejects the old
reader's handles before any object request. Two logical paths retain distinct
provenance despite shared article bytes. Caller changes to returned location/image
metadata and hit slices do not mutate opened state.

`TestReaderStrictReadsSurviveQuarantineAndCatalogClosure` publishes with actual
verification receipts, quarantines them, opens the published view, then cancels the
opening context and closes the catalog pool. A fresh query and verified quote still
succeed: three GETs total, zero HEADs and zero PUTs. This checks application-held
state and strict surviving-object reads; it is not another catalog restore or R2 run.

## Failures exercised

- Missing/unpublished generation before object I/O; missing edition without fallback;
  a known-empty edition with an actual required index, and its missing-index failure.
- Invalid configuration/IDs and missing/canceled deadline before database access;
  root/index/article byte admission before each corresponding GET, and physical
  catalog-key mismatch before the first GET.
- Missing/corrupted manifest, index and article; valid-byte manifest identity and
  index/article association mismatches through deliberately inconsistent catalog
  fixtures that the normal publisher would reject.
- Cancellation while root reading is blocked, with no partial reader; provider
  unavailability after opening, while local searches remain usable and quotes fail.
- Zero/cross-state/out-of-range references, incompatible/empty/wrong-edition queries,
  complete-match overflow and quote output limits, with no partial success.
- Concurrent queries and quotes against unchanged owned state under the race detector.

The narrow `Manifest.EditionIndex` accessor returns only copied reference values
and preserves absent versus known-empty editions. It avoids cloning all provenance
just to fetch one index. Existing independent artifact/quote checks remain in place;
article verification still hashes in three layers and decodes twice.

## Reproduction and limits

Set `CONCORDANCE_TEST_DATABASE_URL` to a dedicated LOCAL PostgreSQL database with
schema-creation permissions, following CONTRIBUTING, then run:

```sh
go test -count=1 -v ./internal/publishedsearch ./internal/generationmanifest
make verify
```

The source checkpoint was checked with Go 1.27.2 on macOS arm64 and isolated
PostgreSQL 14.17. Local verification includes uncached race tests and build. No
hosted schema suite, object deletion, provider write, new format, dependencies,
GitHub Actions or production migration is introduced. Private check/review reports
remain under ignored `.tools/reviews/published-reader-*`.

Per-call limits do not bound simultaneous readers or process heap. Each reader
loads the complete manifest and one edition; independent readers do not share a
manifest cache. Quote availability still depends on surviving objects and the
caller's object-client lifetime. Authorization remains the owner's responsibility;
this does not implement live permission revocation for future private connectors.
Automatic latest/reload, durable citations, retention/deletion and public CLI
configuration remain separate work. Revisit repeated article checks only with a
measured cost and an ownership-preserving checked representation.
