# Keep tab labels attached to their panels

## Decision

`htmlarticle` accepts WAI-ARIA tab structures. It omits each tab list and stores
each tab panel as a `Notice` group whose `Label` is its tab's whitespace-folded
text. The label then reaches the existing label search field, passage context,
search/read output and reranker serialization unchanged, because those
consumers already handle labelled notices. No stored format, profile field,
processor version or downstream package changes.

The user selected this option over a new `tab-panel` group kind and over
keeping tab text as unlinked list text.

## One example

The Kubernetes 1.36 Volumes page shows one `hostPath` example per node type:

```text
[ Linux node ] [ Windows node ]        role="tablist" with role="tab" buttons
  panel 1  aria-labelledby → Linux tab      YAML for Linux
  panel 2  aria-labelledby → Windows tab    YAML for Windows
```

Before this change, the `<button>` elements made the whole page fail with
`ErrUnsupported`. A passage quoting panel 2 now carries the label
`Windows node`, so an agent cannot mistake the Windows example for the general
rule.

## Rules

The [WAI-ARIA tabs pattern](https://www.w3.org/WAI/ARIA/apg/patterns/tabs/)
defines the structure: tabs live in a `tablist`, and each `tabpanel` has
`aria-labelledby` naming its tab. The extractor relies on these standard
attributes, not on Kubernetes class names, and accepts only a complete
one-to-one structure. Each of these fails with `ErrUnsupported`:

- a tab that is not a `<button>`, or a tab outside a tab list;
- other text or elements inside a tab list;
- a missing, duplicate or document-wide repeated tab ID;
- an unreferenced tab, or two panels sharing one tab;
- an empty label, or block content inside a tab;
- a panel that is not a `div`.

That way, verbatim text is never silently dropped or misattributed. Profile
omissions apply first, as in the main walk. Label bytes count against
`MaxBlockBytes` and `MaxOutputBytes`.

## Compatibility

The processor label stays `htmlarticle/v1`. That is safe because tab handling
starts only when the article contains a `<button role="tab">` that the walk
would reach, and every such input failed before (`<button>` is not an accepted
element). Without one, tab roles keep their earlier meaning: inline `<a
role="tab">` lists and lone `role="tabpanel"` containers extract exactly as
before. A test compares those cases with the same HTML minus its role
attributes.

On the complete Kubernetes 1.36 `/docs` render and all 41 retained Pods pages,
736 previously accepted pages produce byte-identical articles, and 37 more now
extract. These include Service, Deployment, Job, Volumes, PersistentVolumes and
Ingress.

## Alternatives

**A dedicated `tab-panel` group kind.** This gives clearer naming, but it changes
the article JSON codec's closed kinds, the passage builder, the label field, the
search/read JSON and the frozen reranker serialization. That is about eight
packages for the same label. It is a clean follow-up if the `notice_labels`
naming proves confusing.

**Keep tab text as plain list text.** Docsy, the theme behind the Kubernetes
site, indexes each page's flattened `.Plain` text in its offline search index
([pinned template](https://github.com/google/docsy/blob/8fd67088bc6253c56f2fef26bdf2bed357509a3d/theme/assets/json/offline-search-index.json)).
That works for whole-page search, but a passage containing only panel 2 would
lose `Windows node` entirely.

## Limits

About 20 authored Kubernetes pages still fail on other constructs: `<tt>`,
`<details>`, merged table cells, nondefault list numbering, an `<iframe>`, or
more than one article container. Merged cells and list numbering remain
deliberately unsupported. Generated API and command reference pages are a
separate scope question.
