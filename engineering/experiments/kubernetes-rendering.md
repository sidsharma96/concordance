# Bounded Kubernetes render reference

Status: observed offline experiment under the accepted
[source-preparation boundary](../decisions/source-preparation.md). It produces
inspection artifacts, not a production renderer, common HTML processor or importer.
No corpus objects were uploaded to R2 and no generation was published.

## Inputs and environment

Use the exact website commits in [kubernetes.pins](../corpora/kubernetes.pins), not
moving branch names. Source staging came from `git archive` of each pin: English
content (including examples), layouts, assets, data, translations, static files,
archetypes, configuration, lockfiles and license. Root files absent in older
snapshots were not invented. No source scripts, API reference generator or deployment
targets ran. Non-English content was not acquired; this is an English-text reference.

The upstream Makefile names a container using its Hugo version and a 12-character
SHA-256 prefix over the concatenated Dockerfile, Makefile, netlify.toml, .dockerignore,
cloudbuild.yaml, package.json and package-lock.json. The expected old tag
`v0.133.0-734755ed3181` returned `MANIFEST_UNKNOWN`. The 1.36 tag
`v0.144.2-9d3727724f2d` was available. Anonymous registry access used a separate Docker
client configuration; no application credentials were passed into containers.

| Editions | Observed Hugo Extended | Node | npm | Build environment |
| --- | --- | --- | --- | --- |
| 1.34, 1.35 | 0.133.0 | 20.17.0 | 10.8.2 | Locally reconstructed image |
| 1.36 | 0.144.2 | 24.18.1 | 11.12.1 | Published upstream arm64 image |

The 1.36 image matches its snapshot's package-lock hash, but its Node version differs
from the snapshot's Netlify Node 20.17.0 setting. It is therefore not evidence of
bit-for-bit production equivalence. The old environment uses the specified Hugo and
Node versions with that snapshot's exact npm lockfile; npm lifecycle scripts were
disabled. Git acquisition used HTTPS instead of unauthenticated SSH. Both old
snapshots have the same lockfile. Hugo's old official release archive was checked
against its published checksum before extracting the binary.

The reconstruction added Git and CA certificates from Debian repositories. Their
acquisition is not a fully locked cold-rebuild recipe. The resulting local image ID
identifies the environment actually run, but is not a remotely provisioned image or
an enduring host-loss recovery artifact. Do not call these builds reproducible merely
because their top-level inputs have version labels.

Reproducibility identifiers for this observation:

- 1.36 arm64 image: `gcr.io/k8s-staging-sig-docs/k8s-website-hugo@sha256:44fe19ff6a9a90bc092f22b8ab2b9f19330d52d8a6b0cad93f325006851efb3c`.
- Reconstructed local image: `sha256:35e10ec37a5099b5960dba08f77850a00f8e190452a50c0a9cda7b41fdebe916`.
- Reconstruction base: official Node image index `sha256:2394e403d45a644e41ac2a15b6f843a7d4a99ad24be48c27982c5fdc61a1ef17`.
- Hugo 0.133.0 extended linux-arm64 archive SHA-256: `159404a685eab33cec2c1efa93e3469066ed26e207f447a43426be3f5b851840`.
- 1.34/1.35 package-lock SHA-256: `5b88aa47a39b97c11d84347f2ba1edb3ec375c436cd047973eba7979c6c1d7b4`.
- 1.36 package-lock SHA-256: `bd31b190913e464d6ecfbe76632e8b7bb5c5cbd78fa6b04c870a84c1452067c3`.

No application dependency changed. The disposable acquisition/render/inspection
probes and raw commands/results are ignored local artifacts; normal checks and shared
repository workflows do not require them or the downloaded website. No generic
renderer service or provider abstraction was introduced.

## Render bounds and deviations

Successful renders ran sequentially in Linux arm64 Docker containers, with read-only
root/source mounts, no network, no application environment, dropped capabilities,
no-new-privileges, UID/GID 1000, two CPUs, 2 GiB memory with equal memory+swap setting,
128 PIDs, 256 MiB scratch and 256 MiB output tmpfs. An external 180-second deadline
and 512 MiB file-output ceiling bounded the client. Owned containers were removed
on success/failure. These settings bound this experiment, not production capacity.

The original English-segment attempt copied unrelated website assets and hit the
256 MiB output ceiling. It was stopped and treated as failure, not partial success.
The final profile selects only English paths
`{/docs/concepts/workloads/pods,/docs/concepts/workloads/pods/**}` and sets
`staticDir = []`. It retains the parsed English page graph for template lookups but
does not copy the complete site's static assets. This is a text-content reference,
not a complete offline website or proof that images are represented faithfully.

Git-derived metadata is disabled for exported staging. The clock is fixed at the
shared commit timestamp, `2026-08-26T17:35:38Z`; production environment is selected.
Asset resources and caches use bounded scratch. A first invalid `--resourceDir`
CLI option was corrected to a config override; output tmpfs ownership was corrected
before a successful build. None of those failed attempts counted as content evidence.

The effective renderer invocation, inside the bounded container, uses a small
recorded override in addition to the pinned source's hugo.toml:

```toml
resourceDir = "/tmp/resources"
enableGitInfo = false
staticDir = []
[segments.probe]
[[segments.probe.includes]]
lang = "en"
path = "{/docs/concepts/workloads/pods,/docs/concepts/workloads/pods/**}"
```

```sh
hugo --source /src --config /src/hugo.toml,/tmp/probe.toml \
  --destination /output --cacheDir /tmp/cache \
  --clock 2026-08-26T17:35:38Z --environment production \
  --renderSegments probe --enableGitInfo=false --noBuildLock
```

See Hugo's [segment behavior](https://gohugo.io/configuration/segments/) and
[resource directory setting](https://gohugo.io/configuration/all/#resourcedir).
The recorded invocation was exercised with the specific older binaries above;
current documentation alone does not establish old-version compatibility.

## Observed content

Each edition had one successful bounded render. No repeated-build determinism test,
unprofiled latency benchmark or process-memory profile was run.

| Edition | Mapped article pages | Extra print page | Article HTML bytes | All emitted bytes |
| --- | ---: | ---: | ---: | ---: |
| 1.34 | 10 | 1 | 5,252,154 | 7,515,989 |
| 1.35 | 13 | 1 | 7,159,284 | 9,547,427 |
| 1.36 | 15 | 1 | 8,281,744 | 10,804,761 |

The known path mapping from source inventory (`_index.md` to directory index;
other `.md` to a named directory index) matched all 38 selected articles after
excluding each generated `_print/index.html`. The latter aggregates content and
would duplicate passages if treated as another ordinary article. Each mapped page
had one nonempty `td-content` container. This is a useful selector observation,
not a verified general article extractor or complete semantic-coverage check.

Inspection used known HTML div selectors and text collection, not a production
Markdown/HTML parser or ranking pipeline. In the Pod lifecycle page, the captured
notices showed:

| Feature | 1.34 | 1.35 | 1.36 |
| --- | --- | --- | --- |
| ContainerRestartRules | v1.34 alpha, disabled by default | v1.35 beta, enabled by default | v1.35 beta, enabled by default |
| RestartAllContainersOnContainerExits | Not asserted by this check | v1.35 alpha, disabled by default | v1.36 beta, enabled by default |

The version shown by a feature notice is its stage introduction, not automatically
the documentation edition. These defaults match the pinned feature-gate data; they
do not establish actual cluster configuration. The 1.35/1.36 included restart-all
YAML text matched the same revision's example file after trimming edge newlines
only. Its normalized SHA-256 was
`bb20e960b6a7b12a687a7a9f5d14f68e06adf930b1e688ad9c67c53f6919c389`.

Raw HTML includes substantial navigation and page chrome. Do not index the entire
HTML text. The [bounded extraction follow-up](html-extraction.md) now preserves
headings/code/tables under an explicit profile. Duplicate handling, persisted
citation coordinates and declared media coverage still need a complete importer.
Source Git line locations cannot simply be assigned to template-expanded HTML passages.

## Retained evidence and remaining work

Local output archives retain the rendered bytes; selected-page reports record each
path, length and hash. The output tar SHA-256 values are:

- 1.34: `c6154c16653ee9be47e16c59a65c7f9683f03d2ed0bbb15dfa8b3be5e0c22d8c`.
- 1.35: `f6bfaf81ccf5ddfc3f7ae833766ef70a074c10d13bcd68f0ebc3ccb0d249fa0f`.
- 1.36: `7f1bc00801c5d4b942fe572deb243201c952e0e3f557a5f970f425f0a8ae5e16`.

These identify observed artifacts, not deterministic future-build hashes. Scratch
source exports can be reacquired from pinned Git objects; they are not application
retention or remote backup. Local artifact/image loss remains possible.

This experiment supports using upstream rendering as a Kubernetes reference for
the checked content. It does not establish a generic scraper, complete HTML/PDF/code
support, safe arbitrary builds, a durable input fingerprint or successful publication.
The extraction follow-up uses the retained representation and synthetic invariants;
passage grouping and the persisted citation format remain open. Keep site-specific
preparation outside ranking and choose
an actual consumer-owned interface only when a concrete substitution needs one.
