# Judge answers separately from passage coverage

The fixed-context pilot gives no reason to choose a splitter. The strict automated
grader marks 5/12 vector paragraph-first answers fully supported versus 4/12 fixed
windows; the word-based counts are 5/12 and 7/12. All four configurations refuse
all six private-deployment questions correctly. These are provisional model-judged
scores: post-run inspection found questionable completeness judgments, so they are
not a validated answer-accuracy benchmark.

Status: completed private development diagnostic. No application behavior, native
RFC processor, model, index or source contract is adopted. The 40 reserved questions
remain unqueried. Raw questions, contexts, answers, judgments and probes stay ignored.

## Question, alternatives and recommendation before the run

The [passage comparison](rfc-chunking.md) counted coverage of marked excerpts.
Its DNSSEC example showed useful alternate evidence despite zero marked coverage.
A miss might therefore be an incomplete label, insufficient returned text, poor
answer synthesis or a citation that fails to support a plausible claim. Another
encoder or a more complex splitter is not the only formulation of the problem.

Keeping the excerpt metric costs no new inference but cannot answer these questions.
[ALCE](https://aclanthology.org/2023.emnlp-main.398/) separates answer correctness
from citation quality. [RAGChecker](https://arxiv.org/html/2408.08067v1) separates
retrieval and generation with claim-level checking. Their published evaluation
methods motivate stage separation; their datasets, frameworks and judging models
do not establish exact-source correctness on our RFCs. We use a disposable small
pilot rather than adopt either dependency stack. An agent choosing files and using
tools is a credible different workflow, left for a separate comparison.

Before inference, the recommendation was to retain both simple splitters. The
recorded distinguishing rule would reconsider a splitter within each retriever
only with at least three additional fully supported successes among 12 main
questions, no fewer correct control refusals and no extra incorrect or unsupported
asserted claims. Otherwise report mixed/inconclusive. This is a descriptive pilot
threshold, not statistical significance. Higher embedding-preparation costs remain
a trade-off even if the rule passes; it never automatically adopts a product default.

## Frozen workload and reading contract

Select the first and last authored evidence question from each of six development
families: 12 main intents. Include all six development deployment controls. The
already observed DNSSEC alternate-source case is a separate diagnostic, excluded
from the 12-question denominator. Selection preceded answers, but followed the
previous retrieval results; this is neither random sampling nor a blind holdout.
The original 100-question fixture remains unchanged.

Reuse the four saved retrieval conditions over the same 996 admitted RFCs: exact
E5 vector retrieval or existing Go any-word BM25, each with paragraph-first A or
fixed windows B. No query changes, new searches, reranking, context expansion or
embedding inference occur. Return a stable prefix of whole passages under 8,192
raw body bytes, at most 64 passages. Stop before the next passage exceeds the
budget; do not skip or truncate. Actual body bytes and prompt/token overhead vary.
The 48 main bundles contain 365,915 body bytes in total.

There are 76 case conditions: 48 main, 24 controls and four diagnostic conditions,
not 76 independent user questions. A source carries its RFC number, exact captured
body SHA-256, half-open byte range, excerpt hash and case-local S label. All are
reconstructed and checked against the original capture before judging.

## Separate the stages and check the judges

A fresh context-only session first sees the question, returned text and the frozen
reference facts/source sections. It judges sufficient, partial, none or uncertain,
with exact witness quotes. It never sees generated answers. Alternate RFCs are
allowed for generic questions; explicitly named sources and historical comparisons
must retain their required scope.

The answerer sees only the question and returned evidence, without golden facts,
method names or retrieval scores. Each of at most six claims must cite a case-local
label and an exact source substring of at most 400 characters. Missing evidence
requires a partial answer or refusal; public standards cannot establish actual
private deployment state. Cases never share citation labels or evidence.

A separate fresh judging session sees the answers, evidence and source-grounded
reference, but neither method identity nor the context-review verdict. It rates
completeness, factual correctness, citation support and source scope. Full success
requires a complete answered result, every substantive claim judged correct and
fully supported, valid exact citations, and the requested RFC scope. One optional
unsupported assertion can fail an otherwise useful central answer. Report that
strictness rather than interpreting a failed answer as entirely useless.

All roles use the same named model, claude-opus-5-5 at low effort, in separate
no-tools sessions through client 2.1.318. This is session separation, not independent
model families or human validation. Immutable service weight revision and sampling
temperature are unavailable. Up to five independent cases from one condition share
a call, so batch effects are unmeasured. No answerer sees competing variants.

## Strict automated scores

| Configuration | Fully supported | Complete according to grader | Correct deployment refusals |
| --- | ---: | ---: | ---: |
| vector-A | 5/12 | 8/12 | 6/6 |
| vector-B | 4/12 | 6/12 | 6/6 |
| lexical-A | 5/12 | 6/12 | 6/6 |
| lexical-B | 7/12 | 7/12 | 6/6 |

The grader marked no asserted main-case claims factually incorrect, but marked
12 claim occurrences not fully supported, including two invalid exact-text
citations. This does not prove absence of factual errors. A correct fact from
model memory, an overly broad paraphrase or a quote with changed whitespace can
still fail the evidence contract. Completeness and grounding remain separate.

The source-only availability judgments for the 12 main intents are:

| Configuration | Sufficient | Partial | None | Uncertain |
| --- | ---: | ---: | ---: | ---: |
| vector-A | 8 | 2 | 1 | 1 |
| vector-B | 7 | 3 | 2 | 0 |
| lexical-A | 6 | 4 | 1 | 1 |
| lexical-B | 7 | 2 | 2 | 1 |

Three source-review witnesses changed the source text and were downgraded to
uncertain, without repair. These are evaluator failures, not retrieval misses.
Source-only judgments are also model estimates, not exhaustive human labels.
An invalid witness does not erase an independently supported answer result.

## What the examples actually show

The DNSSEC diagnostic succeeds for both vector configurations and fails full
answer support for both word-based configurations. In vector A, the answer's first
claim uses exact RFC 7858 text: DNSSEC does not protect request and response privacy.
This confirms that alternate evidence can answer the question despite missing the
previously marked RFC 4033 privacy excerpt. It remains outside the main denominator;
no original relevance labels or coverage scores were edited.

For vector A's range-request answer, the core No is supported, then an added 416
rule drops the source's preconditions. The strict all-claims criterion fails it.
This is an answer/citation issue despite useful retrieved text, not an argument
for another retrieval model.

Post-run inspection also found grader strictness worth calibrating. Vector B's
cache answer gives an explicitly RFC 2616-scoped route using expiration in the
past and must-revalidate, but the grader requires the seeded no-cache/no-store
terminology. The generic question asks for the outcome, so the alternative may
be a legitimate answer. Lexical A's range answer says No and explains SHOULD
versus MUST; the grader still requires explicit MAY-ignore wording. These are
potential reference-answer bias, not confirmed retrieval failures. The recorded
scores and artifacts remain unchanged. Selected examples are explanatory audits,
not new blind labels or a repaired leaderboard.

Neither splitter meets the predeclared three-success rule. The result retains the
recommendation to keep simple baselines and calibrate outcome-based acceptable
answers and claim support on observed development material before expansion.
The word-based fixed-window score is highest here, but this small selected pilot
with one generator and an imperfect grader cannot establish general superiority.
An agent tool loop, grep/file-search workflow, held-out comparison and latency
remain untested. There is no evidence here requiring an approximate vector index.

## Cost, verification and recovery limits

The three stages make 48 completed client calls. Service-reported client estimates:

| Stage | Calls | Estimated inference cost |
| --- | ---: | ---: |
| context | 16 | $3.90 |
| answer | 16 | $3.00 |
| judge | 16 | $4.08 |

These are the pilot's answering and grading costs, not production search prices.
Design/harness/source/prose reviews are separate preparation costs. Only additive
input/output/cache token and cost fields are reported from raw model metadata;
non-additive context-window fields in the raw helper summary are not usage.
There are no fresh latency measurements, remote store writes or target changes.

Inputs and code freeze before all context/answer inference. Two calls run at most
concurrently, with configured 360-second deadlines, process-group termination and
an additional ten-second wait before KILL. The accepted-output cap is checked after
completion, not a streaming memory bound. Final pipe draining after KILL is not
hard bounded if a descendant escaped the process group; no such event occurred.
Schema/service/deadline failures would make the entire comparison INCOMPLETE,
never a quality miss or an altered denominator. No execution failures occurred,
and no model call was rerun or answer repaired by the harness.

Independent mechanical reconstruction checks all 76 exact ranked prefixes, all
48 service/model/ID bindings and all 76 answer/control decisions against raw rows.
It verifies 146 valid citation quote occurrences. This checks arithmetic and byte
bindings, not the semantic accuracy of model judgments. Ordinary make verify also
passes on a dedicated local PostgreSQL 14 database with Python 3.14.2, including
worker tests, race checks and build; production code is unchanged.

The frozen setup.py is the original generator, not a complete regeneration recipe:
batches were regrouped by condition and plan failure wording amended before freeze.
Actual frozen artifacts govern; private notes retain this provenance. Review roles
were corrected to scope an old operator-CLI-only instruction to its actual task;
no reviewer model, permission or substantive correctness criterion was weakened.
The private harness received separate read-only code and architecture reviews
against actual file snapshots; source documentation review remains separate.

Retained records: .tools/evaluation/rfc-answers-1 contains plan.json, before-run.json,
contexts.json, the selected judgment fixture, raw stage results, outcome.json,
verified.json and postrun-audit.json. Prompts, raw model outputs and reviews stay
under .tools/reviews/rfc-answers-1. Reproduction requires provisioned private inputs
and service access; ordinary builds do not depend on them. The original question-set
SHA-256 remains d90b3ae5cb66acd4c34dddc6d5ac3dec91866451a1ddb38fc0d745807e41d994.

Frozen record identities:

- before-run.json: `fdd106f87ab73a5917fe0e41ebb16abbc0af56edcab90c22b7f9fe29af3a301f`.
- outcome.json: `f5b5887751b8bc41aca09a8635e05bdbf777411c920873900bb6e32245bad124`.
- verified.json: `5eb28ddd12c24939042e4f54150838806a5e7a0af2ce90cf02f8c338ca24c8d8`.
- postrun-audit.json: `3472399c7768e39e48cfbbe0ed7c2c982e9877baf9d4443e4b334053eb7e8cf1`.
