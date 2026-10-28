# Save passage locations with search data

Status: accepted option A. Save the canonical passage-to-article/body-range lookup
produced during indexing. Reopening a generation must read those saved locations
rather than rerun the historical passage builder to discover them. The complete
index, manifest, source associations and citation API remain separate work.

## One result after a restart

Suppose search returns passage 7. Its saved location says which exact retained
article contains it and which half-open block range to quote. The reader must use
that generation's association, not run today's grouping algorithm and assume its
seventh passage means the same thing. A changed grouping rule can move a boundary
without changing the article's text.

Retained article text remains the quote authority. Locations contain addresses,
not another copy of passage text. Bind them to the exact article and the prepared
search data that used those ranges. Preserve processing versions, document/source
identity, exact edition and revision through the generation's referring metadata.
An article digest checks bytes; it is not a logical document identity or permission.
Temporary `htmlsearch.Reference` values must not be serialized.

The first bounded component may store an article-local location record. That does
not require one remote object per passage or finalize where records live within
the future search artifacts. Other formats need their own typed locations; HTML
block ranges are not a universal PDF, provider-document or code-symbol address.

## Alternative and costs

Option B saves the article and processing versions, then reconstructs the lookup
using the exact historical passage builder and original input order. It avoids a
saved location table, but recovery repeats preparation work and requires keeping
those old algorithms runnable. Running the latest builder is not a safe substitute.

Option A saves choices the writer already computed. With P passages, the body-range
table adds O(P) metadata and capture work, excluding representation bindings and
encoding overhead. A loaded array permits O(1) location lookup. Actual bytes,
decoded memory and remote reads depend on the eventual format and placement; no
performance advantage or enterprise capacity follows from this choice alone.
Old format readers still need maintenance even when old grouping algorithms are
unnecessary for location recovery.

## Failure boundary

An index from one preparation and locations from another can return a plausible
but wrong quote. Validate versions, counts, exact article associations and the
search-data binding before exposing locations. Bounds checks alone cannot prove
that an in-range location matches the indexed text. The writer must derive both
from one canonical preparation, with independent quote/search comparison evidence.
Hashes detect accidental artifact mixing; they do not authenticate a malicious
writer who supplies mutually consistent forged artifacts.

Missing, corrupt, incompatible or unauthorized content fails explicitly. Do not
invent empty results, silently regroup passages or switch to another generation.
Confirm all required artifacts before PostgreSQL publication; partial uploads remain
unpublished. This decision adds no deletion, retention or reader-lifetime guarantee.

The representative workload has 38 article-version entries and 1,889 passages.
Measure the saved table on that corpus before considering compression or paging.
Persistent indexing, paged metadata, bounded readers and safe retention/deletion
remain necessary scaling work. Revisit layout when measured opening or metadata
cost approaches explicit budgets. Stable passage identity across generations is
additional history/annotation work, not a requirement of this choice.
