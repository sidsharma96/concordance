# What counts preserve, and what ordered fields cost

The accepted [index layout](../decisions/edition-index-layout.md) is one bounded
object per exact edition under one generation manifest. The bounded HTML binary codec now exists; complete published-generation
recovery bindings remain open. This experiment informed the accepted
[artifact-binding direction](../decisions/edition-artifact-binding.md):
keep counts and saved locations together without another ordered-word copy. It does
not choose a format or implement a loader.

## The remaining association

`htmlpassagemap.Capture` produces body ranges and ordered prepared fields together.
The saved table binds to exact article JSON and a fingerprint of those ordered
fields. Its existing `Open` verifies the fields before exposing a reader. The
counted index deliberately omits word positions and order, while preserving separate
body/heading/label frequencies. Counts cannot reconstruct that field fingerprint.

For example, `retry upload retry` and `retry retry upload` have the same counts,
but different ordered fields. Retained articles still preserve the original text;
this concerns an additional prepared representation, not losing quotation bytes.

The private probe sorts words within each passage field, retaining every field's
counts, then compares matching/ranking with the frozen baseline. All 28 cases retain
430 exact ranked references/scores and quotes through the original checked readers.
Every one of the 38 article maps rejects the reordered fields with `ErrMismatch`.
That rejection is expected: the existing reader checks stronger ordered-field
agreement than retrieval from counts requires. No production check was weakened.

## Illustrative ordered-field payload

The probe writes an explicit, uncompressed JSON sample for each real edition. It
contains a sample-format tag, edition ID, passage/field/term version tags and ordered
body/heading/label word arrays. Empty arrays normalize to `[]`. JSON decode compares
every field and canonical re-encoding before accepting the sample. This is a
measurement fixture, not the product schema or the serialization of private Index
state. The known-empty query control is excluded from real-edition byte totals.

| Edition | Passages | Occurrences | Sample JSON bytes |
| --- | ---: | ---: | ---: |
| 1.34 | 504 | 22,664 | 208,375 |
| 1.35 | 656 | 29,054 | 270,636 |
| 1.36 | 729 | 32,335 | 301,601 |
| Total | 1,889 | 84,053 | 780,612 |

These bytes include this sample's JSON keys, framing and small version headers.
They exclude count postings, articles, body-range records, source/citation metadata,
the generation manifest, provider overhead and any compression. They are not a
production storage forecast, retained heap, latency or a binary-format comparison.
The input is the existing 38-article Kubernetes sample, not an enterprise corpus.

## Accepted direction and remaining design

One credible baseline retains the ordered prepared fields alongside persisted
postings and locations. Existing field-fingerprint checks remain available; retained
fields also support exact preparation debugging. The trade-off is extra data to
store and, when using that check, read and hash. Verifying loaded postings against
those fields would require additional traversal; retaining them alone does not
prove the postings are semantically correct.

The user selected count postings and their saved body locations in a single
versioned edition artifact, with exact article references. A trusted canonical writer
produces them together. A reader must verify the actual artifact bytes against its
reference from the published generation, then validate its schema, counts/statistics,
locations, article bindings and versions before exposing search/quote data. It must
not accept an unchecked caller's field hash as a replacement for the old check.
The existing `htmlpassagemap.Open` remains unchanged as the ordered-field baseline.
The selected [binary component](binary-edition.md) now supplies the bounded reader;
its caller must still establish trusted generation associations.

That second design avoids an additional ordered-field copy. It relies on the
canonical writer's preparation association and the integrity of the pinned artifact;
it does not independently regenerate every indexed word from source on each open.
An artifact checksum cannot certify a malicious or semantically incorrect writer.
Independent rebuild/audit comparisons remain necessary evidence. Both approaches
already require correct preparation; neither matching hashes nor retained fields
alone authenticate the publisher or authorize access.

The agreement selects the recovery direction. The separate [binary experiment](binary-edition.md) describes the implemented
bounded reader. This earlier experiment implements no loader itself. Counts
preserve current keyword/BM25 behavior; exact quoted text still comes from the
retained article. Query preparation needs compatible term rules
under either design. A single edition can still exceed budgets: persistent index
blocks, paged metadata, selective readers and safe deletion remain scaling work.

## Reproducibility and limits

The disposable Go probe is `.tools/probes/edition-index-audit/`; raw observations
are `.tools/reviews/edition-index-audit.txt`. It copies the existing capture helper,
checks frozen source/query identifiers, uses bounded local inputs and changes no
frozen corpus files. Source inventory SHA-256:
`f82481e1d451e5b1901e87231158f3ddeb338ff06bc5295b5cb81138cd268052`.
Frozen ranking report SHA-256:
`da2f4eddd71ccea8cceec2a02ad40ecc2639f207821cce31bbf5ab088ef4543e`.
The probe ran on Go 1.27.2 darwin/arm64 and made no remote writes. Ordinary checks
need neither this private corpus nor the disposable probe. No provider-fault,
publication, host-loss, recovery-latency or large-corpus claim follows.
