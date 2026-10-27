# First source and extension boundaries

Status: accepted direction. Start with Markdown documentation from a specific Git
commit. The user accepted Git first on the condition that other sources and content
types remain possible. Bounded local Git capture now exists; content processors
and other connectors are not implemented. This decision does not fix a public
connector API or a universal passage schema.

## Separate acquisition from understanding

The first workload captures a selected documentation directory from one resolved
commit, processes Markdown and preserves exact source locations for citations.
Keep repository identity, logical document path, product version, commit, captured
bytes, processing rules and index generation separate. A release/branch name must
be resolved and retained as a concrete revision before it identifies captured input.
Preserve the captured bytes in our object storage; a Git hash alone does not ensure
that an upstream repository will keep them available.

A source adapter answers where material comes from and how it can be acquired:
authentication, document identity, revision observations, bounded acquisition and
deletion/permission observations. A processor answers how the captured content is
understood: structure, searchable passages and precise source locations. These are
separate choices. A Git repository can contain Markdown, code or PDFs; Google Drive
can supply different document formats. An unsupported format must not silently be
treated as Markdown or discarded while claiming a complete import.

Future examples, not supported integrations:

| Material | Acquisition responsibility | Processing and citation responsibility |
| --- | --- | --- |
| Git Markdown | Capture repository/path at a commit | Headings and source spans |
| Source code | Capture repository files and revision | Language-aware symbols, paths and line ranges |
| Google Docs | Capture authorized API content or chosen export | Document structure and locations in that captured representation |
| Notion | Capture authorized pages/blocks and acquisition scope | Block structure and block locations |
| Slide decks | Capture a file or provider representation | Slide identity, text, notes and supported visual content |
| PDFs | Capture a file from any supported source | Page identity, text/layout and extraction quality |

Keep the original captured representation and source-specific metadata alongside
derived passages. A shared searchable text view must not replace the structure
needed for citations or later processing. Scanned pages, figures, tables and slide
diagrams need explicit extraction capabilities and quality checks; plain-text
extraction does not establish support for their meaning.

The catalog and storage path use source-neutral operation/generation identity. Their
input fingerprint must cover the complete trusted captured-input description and
relevant processing settings; its canonical encoding remains to be specified. Source
connectors must not leak transport-specific assumptions into the common lexical
retrieval loop. New features such as symbol navigation, table queries or visual search
may require additional retrieval algorithms. Extensibility does not mean every new
format is solved by one parser or that ranking can never evolve.

## Revisions, permissions and scope

Do not require every provider to supply an enduring Git-like commit. Define what
each adapter actually observed and which bytes we captured. Multiple API reads do
not automatically form one atomic provider snapshot. Product versions apply where
the source defines them; do not invent a release label for an unversioned document
or let absence satisfy an explicitly version-scoped query.

Private-source integrations must define authorization at acquisition and at query,
snippet and citation access, including permission changes. A stale index or retained
snapshot does not grant access. Distinguish confirmed deletion from lost permission,
transient failure and unknown state; do not silently translate them into one signal.
These policies must be settled before enabling a private integration, not added
after its indexed content becomes searchable.

Provider observations informing these constraints: Google offers several document
and presentation export formats, while the Docs API's revision ID has a limited
validity and user scope. Notion page content is retrieved through blocks; a 404 may
mean missing content or missing access. These are examples of different contracts,
not provider contracts implemented or pinned by this decision.

Sources: [Drive export formats](https://developers.google.com/workspace/drive/api/guides/ref-export-formats),
[Docs document revision](https://developers.google.com/workspace/docs/api/reference/rest/v1/documents),
[Notion block retrieval](https://developers.notion.com/reference/retrieve-a-block).

## Why Git first

A commit supplies an explicit project snapshot for a repeatable first capture and
citation experiment. Capturing web pages first would fit published documentation
without repository access, but adds discovery, HTML extraction and changing-page
capture rules to the same experiment. Git first is an implementation order, not a
permanent product restriction or evidence of better retrieval quality.

Build concrete Git and Markdown operations before extracting small consumer-owned
interfaces. Demonstrate the acquisition boundary with a fixture source. Do not
create empty future adapters, a dynamic plugin registry, arbitrary metadata maps or
a universal document framework merely to list possible extensions. When a second
real source/format arrives, exercise its different identity, structure and failure
semantics before fixing a reusable contract.

The first cost baseline is one bounded selected corpus: count selected documents,
captured bytes, acquisition requests, extraction allocations and retained memory.
Selection must not read every blob in a large repository before applying limits.
The capture implementation must specify its enumeration/read bounds and recovery
ownership before making scalability claims. The
[representative-corpus inventory](../experiments/kubernetes-corpus.md) now records
real source counts/bytes, not capture latency, allocations or search quality.

Revisit this starting source if the first real corpus is available only as published
pages or provider documents. Revisit the common representation when a concrete second
processor cannot preserve its structure and citations through it. Next, specify and
exercise the bounded Git capture and complete retry fingerprint, followed by the
actual R2 write/read/recovery path. The catalog component alone does not establish
end-to-end extensibility or storage recovery.
