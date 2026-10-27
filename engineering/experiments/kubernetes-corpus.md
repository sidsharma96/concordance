# Kubernetes corpus inventory

Status: measured raw-source inventory using `tools/corpusaudit` and the existing
bounded `gitsource.Capture`. No processor, manifest serialization, indexing or search
measurement is included. The [corpus decision](../decisions/representative-corpus.md)
defines the intended use and growth direction.

## Reproduce capture

Use Go1.27.2 and Git2.54.0 to match the observed environment (darwin/arm64). One
successful inventory execution per selected directory and revision was observed.
These are exact byte/count observations, not timings, memory profiles or capacity
limits. The tool's defaults are audit safety ceilings, not production import limits:
20,000 enumerated entries, 8 MiB listing, 4 MiB per selected body, 128 MiB selected
body bytes and a two-minute capture deadline. CPU analysis checks cancellation
before reporting; it is not interrupted within each marker scan or blocked output.

Acquire the public repository in ignored local storage. A partial clone saves
unselected blobs, but materialize the English docs before running Capture, which
intentionally disables lazy fetching. No source templates or build scripts are run.

```sh
mkdir -p .tools/corpora
git clone --filter=blob:none --no-checkout --depth=1 --single-branch \
  --branch release-1.36 https://github.com/kubernetes/website.git \
  .tools/corpora/kubernetes-website
git -C .tools/corpora/kubernetes-website sparse-checkout set \
  content/en/docs layouts/shortcodes
while read -r edition commit; do
  git -C .tools/corpora/kubernetes-website fetch \
    --filter=blob:none --depth=1 origin "$commit:refs/corpus/$edition"
  git -C .tools/corpora/kubernetes-website -c core.hooksPath=/dev/null \
    checkout --detach "$commit"
done < engineering/corpora/kubernetes.pins
```

Run from this checkout, using a recorded commit, not a moving branch:

```sh
go run ./tools/corpusaudit \
  -repo .tools/corpora/kubernetes-website \
  -commit 1de955ebabe7e17da1ebb4f582635491227f4157 \
  -dir content/en/docs/concepts/workloads/pods -inventory
```

Repeat for each pin. For the growth workload change `-dir` to `content/en/docs`.
Reports contain summary rows, sorted shortcode names, and optional quoted paths,
raw SHA-256 and byte lengths. A failed capture emits no success inventory. Missing
objects, unsupported entries, exceeded limits and cancellation are errors, not an
empty successful collection. Downloads and reports stay under ignored `.tools/`;
normal checks never require them. Keep this local acquisition available while
reproducing captures; eventual cleanup must target only this disposable download.

## Observations

Bytes are raw selected Markdown bytes, excluding images, templates, externally
included examples, search artifacts, manifests and protocol/provider overhead.

| Edition | Pods files | Pods bytes | Full English files | Full English bytes |
| --- | ---: | ---: | ---: | ---: |
| 1.34 | 10 | 148,700 | 1,528 | 10,942,356 |
| 1.35 | 13 | 192,502 | 1,573 | 11,299,911 |
| 1.36 | 15 | 213,817 | 1,672 | 15,508,276 |

The initial total is 38 document-version entries and 555,019 raw bytes. Their 26
distinct body hashes account for 443,065 bytes. Across all three full English
snapshots, 4,773 entries total 37,750,543 bytes; 2,550 distinct hashes account for
27,903,852 bytes. Aggregate hash totals combine the per-file inventories, preserving
every document/version entry. They indicate repeated raw payload, not a measured
storage bill or request saving; metadata and verification still cost work.

The largest selected body is 60,559 bytes in the initial corpus and 441,140 bytes
in the growth workload. Each full English selection ignores 68 non-`.md` entries;
these may matter to rendered content. No selected entry was excluded after capture
to make the report pass. The audit does not establish which files are published
articles: some `.md` files are Hugo resources with rendering disabled.

Every initial file has a frontmatter marker and a shortcode marker. Feature-state
markers occur in 9, 11 and 12 initial files respectively; code-sample markers occur
in 2, 3 and 4. Markers can appear inside examples or comments. The regex/byte scan
is a deliberately simple inventory, not a syntax parser, dependency resolver or
percentage of successful extraction. This inventory does not build or crawl a site;
the subsequent [render reference](kubernetes-rendering.md) tests selected output.

## One concrete dependency

At the pinned 1.34 revision, `ContainerRestartRules.md` in
`content/en/docs/reference/command-line-tools-reference/feature-gates/` records
alpha with `defaultValue: false`. At the pinned 1.35 and 1.36 revisions it also
records beta from 1.35 with `defaultValue: true`. The selected `pod-lifecycle.md`
references this data through `feature-state`, rather than containing the whole
notice. Its template reads feature-gate resources and their stages. A parser that
merely removes the shortcode loses information relevant to the support question.

The pinned 1.35 data for `RestartAllContainersOnContainerExits` records alpha and
disabled by default; 1.36 adds beta and enabled by default. This is a documented
default, not proof a particular cluster enables it. The Pod lifecycle example also
uses `code_sample` to include YAML from outside the selected Pods directory.

Reproducibility references (full identifiers are in the pin file):

- [1.34 feature-gate data](https://github.com/kubernetes/website/blob/47132eb5367cfdcba0a3b33a0b3ffc81185bd7e8/content/en/docs/reference/command-line-tools-reference/feature-gates/ContainerRestartRules.md)
- [1.35 feature-gate data](https://github.com/kubernetes/website/blob/8817d358864b70a9710c5ad34f78df6a239be33d/content/en/docs/reference/command-line-tools-reference/feature-gates/ContainerRestartRules.md)
- [1.36 restart-all data](https://github.com/kubernetes/website/blob/1de955ebabe7e17da1ebb4f582635491227f4157/content/en/docs/reference/command-line-tools-reference/feature-gates/RestartAllContainersOnContainerExits.md)
- [Feature-state template](https://github.com/kubernetes/website/blob/8817d358864b70a9710c5ad34f78df6a239be33d/layouts/shortcodes/feature-state.html)
- [Code-sample template](https://github.com/kubernetes/website/blob/8817d358864b70a9710c5ad34f78df6a239be33d/layouts/shortcodes/code_sample.html)
- [Upstream license](https://github.com/kubernetes/website/blob/1de955ebabe7e17da1ebb4f582635491227f4157/LICENSE)

These observations motivated the subsequent Kubernetes-specific render experiment.
They do not by themselves establish complete processing. They also
do not measure manifest size: the subsequent [layout decision](../decisions/manifest-layout.md)
selects one bounded manifest, while serialized metadata size and decoded memory
still require a candidate schema. No generation
was published and no corpus bytes were uploaded to R2 by this experiment.

## Cost and next evidence

For N selected files and B body bytes, Capture retains O(B) body memory. The audit
scans those bodies and sorts a copy of descriptors: O(B + N log N) time, O(B + N + K)
memory for K distinct marker names, plus bounded listing/report overhead. Counts
are not an allocation profile. Git currently launches a subprocess per selected
blob; acquisition overhead remains a separate scaling concern.

Before using the corpus as a search evaluation, define dependency handling, create
source-linked questions with edition-specific expected evidence, and separate
development questions from held-out relevance judgments. Exercise citation recovery
and missing-edition refusal before making quality claims. Later stress independent
collections and retained publications separately from adding more copies of these
files. Revisit the initial corpus if it does not expose meaningful applicability errors.
