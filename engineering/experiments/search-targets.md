# Explicit catalog-backed search targets

## Workload and invariant

The [accepted target](../decisions/search-targets.md) maps a friendly name to one
collection, published generation and default edition. Its
[conditional update contract](../decisions/target-updates.md) keeps an old operator
from silently replacing another selection. Existing reader semantics remain exact:
new resolutions observe a new default, while resolved selections and opened
readers keep the generation they captured.

The catalog fixture uses real local PostgreSQL with isolated schemas. Application
tests build actual retained HTML article JSON, binary count/location indexes and
a manifest, then publish through the real catalog/object publisher and an HTTP
S3 fixture. Two logical documents share article bytes and an explicitly empty
edition has its own index. This is synthetic producer evidence, not a new renderer,
hosted schema change or live R2 experiment.

## Observations

- `TestTargetChangesReplayAndSupersededRequests` reconnects after discarding a
  successful caller result, then recovers the exact next revision without another
  increment. A later revision conflicts with the old request even after selection
  contents return to their earlier value. Returned metadata is independently owned.
- `TestTargetConcurrentIdenticalAndDifferentUpdates` uses two store instances.
  Sixteen identical expected-revision requests converge on one next revision;
  sixteen competing different tuples produce one winner and fifteen conflicts.
  The persisted tuple is exactly the winner, not mixed fields.
- `TestTargetPublicationBoundsAndUnavailableAreDistinct` rejects unpublished
  generations and nonzero-revision creation of an absent target. Invalid/canceled
  requests do not create it; database failure is not absence. A simulated write
  failure retains the conservative unknown-outcome category with no partial target.
- `TestTargetRevisionCannotWrap` checks the final increment and retry at the
  signed 64-bit maximum; the resulting target stays readable but cannot advance.
- `TestExplicitTargetSwitchPinsResolvedAndOpenReaders` confirms that publication
  alone creates no target. Activation performs two GETs for manifest/index and no
  object writes in the fixture. New resolutions use B after a switch; an already
  resolved or opened A still returns A's exact quote. Explicit known-empty and
  absent editions remain distinct, with no version fallback.
- `TestActivationRejectsMissingAndCorruptDefaultsBeforeWrite` rejects an unpublished
  generation, missing default edition and missing/same-length-corrupted manifest or
  index. The original target remains unchanged. During a later object-service
  outage, activation preflight fails while its saved catalog result remains
  separately discoverable.
- `TestInvalidTargetRequestsFailBeforeDatabaseOrObjectIO` uses a closed pool to
  distinguish local rejection from accidental database use. Invalid revisions,
  overrides, names and missing/canceled contexts produce no object request.

These are state/operation-count observations, not latency or scale measurements.
Reconnection after a discarded caller result does not simulate a lost PostgreSQL
write acknowledgment. A real transport interruption, target schema deployment and
operator command remain separate work.

## Reproduction and limits

Use a dedicated local PostgreSQL database with schema-creation permission as in
CONTRIBUTING. The tests create and remove isolated schemas; never run them on the
hosted catalog. Then run:

```sh
go test -count=1 -v ./internal/catalog ./internal/searchtarget
make verify
```

`catalog.CreateTargetSchema` is explicit additive setup after the import schema.
An existing table is an error, not a migration or compatibility proof. The hosted
runtime has not been migrated and no target/default version was installed there.

The low-level catalog writer consumes trusted metadata; its foreign key proves a
published generation, not an edition's existence. The concrete application uses
`searchtarget.Activate` for actual strict reader preflight. No remote read is held
inside a database transaction. Provider availability can change afterward, and a
retry can fail preflight before discovering a completed catalog update. Resolve
the current target separately in that case; do not invent a rollback or choose a
new generation silently.

The caller owns authorization, object routing, deadlines and concurrent-call
admission. A target is not an authorization credential. There is no deletion/reset,
revision history, automatic activation, latest-version sorting, cache, public CLI
configuration/output contract or portable citation schema. A missing requested
edition in the chosen generation does not trigger a search of retained generations.
