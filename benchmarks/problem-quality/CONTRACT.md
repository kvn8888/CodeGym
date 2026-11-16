# Problem-Quality Benchmark Contract v1

## Technology neutrality

The benchmark harness is technology-neutral. Cases provide all technology-specific
details through their manifest and build/test/run contract. Adding a technology
requires only a new case directory and generated environment artifact.

## Technology coverage and sampling

The benchmark is technology-neutral in the harness, but model behavior is not
guaranteed to be technology-neutral. A sampled benchmark establishes a baseline
and can catch systemic issues; it does not prove that every untested technology
is reliable.

Therefore:

1. Test a representative sample, not every possible technology.
2. Include cases that exercise different strategies, comparators, edge cases,
   and ambiguity types.
3. Add a new case when a technology becomes important or shows real-world
   failures.
4. Use production telemetry to decide when additional cases are needed.
5. Label results as preliminary and avoid broad claims about untested
   technologies.

## Case structure

Each case uses this directory layout:

```text
benchmarks/problem-quality/cases/<case-id>/
  case.yaml
  spec.json
  expected-failure.yaml
  review.yaml
  correct/
    <alternative-id>/
      <files>
  wrong/
    <wrong-id>/
      <files>
```

The case is identified by its directory name and `case_id`. Technology-specific
details belong in `case.yaml`, `spec.json`, and the environment manifest, not in
benchmark code.

## Build/test/run contract

Each case must declare:

- strategy: `unit`, `http`, `output`, or `custom`
- entrypoint
- setup command, if needed
- build command, if needed
- test command
- run command, if needed
- timeout in seconds
- memory limit in MB
- network mode

The harness executes these commands exactly as declared. No command may be
inferred from the technology name.

## Spec convention

Every reviewed spec must include:

- language
- strategy
- title
- description
- category
- subcategory
- tags
- difficulty
- estimated minutes
- hints
- entrypoint
- signature
- I/O contract
- comparator
- ambiguity resolutions
- function metadata when applicable

`ambiguity_resolutions` is required and must be non-empty. It must resolve edge
cases, empty inputs, tie-breaking, ordering, numeric tolerance, and concrete
field types where relevant.

## Correct alternative convention

Each case has exactly one independently written correct alternative in Split 1
planning; the story requires at least one. Each alternative must include:

- unique ID
- file map
- reviewer
- review date

A correct alternative must pass all assessment tests. It must not be the
generated reference solution unless explicitly documented as such.

## Wrong solution convention

Each case has two plausible wrong solutions. Each wrong solution must include:

- unique ID
- file map
- intended failure:
  - test ID
  - assertion code
  - expected value
  - actual value
  - reason
- learning objective rationale
- reviewer
- review date
- denominator decision

Wrong solutions must be plausible mistakes tied to the learning objective. A
failure that is unrelated to the objective does not count as a meaningful
wrong solution.

## Comparator expectations

Each case must declare comparator expectations:

- default comparator kind
- epsilon where applicable
- whether case-level overrides are allowed
- notes

Non-exact grading must be explicitly covered where relevant:

- `exact`
- `set`
- `multiset`
- `sorted`
- `float`
- `checker`

## Expected outcomes

Each case must record expected outcomes:

- generation status: `measured` or `not_applicable`
- blind agreement, if applicable
- reconciliation count, if applicable
- reconciliation direction, if applicable
- non-convergence, if applicable
- correct alternatives pass
- wrong solutions rejected

Generation metrics are `not_applicable` for baselines without independent
generation. They are never reported as zero.

## Review metadata

Each case must include review metadata:

- reviewer
- review date
- ambiguous spec flag
- notes

Disputed wrong solutions must be recorded with one of:

- `meaningful`
- `equivalent`
- `invalid`

Equivalent or invalid solutions are excluded from the meaningful
wrong-solution denominator.

## Validation checklist

A case is valid only when all of the following are true:

- [ ] Case follows the generic format.
- [ ] Technology is declared as data, not hard-coded in harness logic.
- [ ] Environment manifest is referenced.
- [ ] Build/test/run contract is complete.
- [ ] Spec has non-empty ambiguity resolutions.
- [ ] At least one correct alternative exists.
- [ ] Two wrong solutions exist.
- [ ] Each wrong solution has an objective-tied intended failure.
- [ ] Comparator expectations are present.
- [ ] Expected outcomes are recorded.
- [ ] Review metadata is complete.
- [ ] Denominator decisions are recorded.
- [ ] Demo/unreviewed assessments are excluded.

## Handoff to harness lane

When the corpus is ready, provide the harness lane with:

- corpus root path
- schema version
- example case
- validation checklist
- failure categories
- N/A metric rules
- preliminary labeling rules
- notes on pending environment artifacts

## Assessment protocol

1. The reviewed correct solution must pass all assessment tests.
2. Each plausible wrong solution must fail the intended test.
3. The wrong solution's failure must include structured assertion evidence:
   - test id
   - assertion code
   - expected value
   - actual value
4. Additional failures beyond the intended one are tolerated and recorded.
5. Wrong solutions that are equivalent to a correct solution or invalid for the
   task are recorded with `denominator_decision: equivalent` or `invalid` and
   excluded from the meaningful wrong-solution denominator.

## Generation metrics

Metrics 1-4 exist only for independent-generation baselines:

1. Blind agreement rate
2. Reconciliation frequency
3. Reconciliation direction
4. Non-convergence rate

For an independent-generation baseline with no observed generation evidence,
report these as `unavailable` with null observed fields; never copy expected
outcomes into them. For all other baselines, report them as `not_applicable`.
Never report them as zero.

## Failure categories

Use one of:

- generation_failure
- ambiguous_specification
- setup_failure
- non_convergence
- reconciliation
- compile_failure
- platform_failure
- test_failure
- assertion_evidence_mismatch

## Preliminary results

Any report based on the initial small sample must set `preliminary: true` and
must not claim that generated problems are broadly trustworthy.
```
## Local development benchmark

The local benchmark command executes real artifact setup/build/test commands and
real assessment commands on the developer machine. It is technology-neutral:
test commands come from the artifact manifest, and assertions are read from the
`CODEGYM_TEST` protocol. Python in the checked-in fixture is fixture data only.

Build and run the development fixture from `backend/`. Use the built binary
when checking exit status: `go run` reports a nonzero child exit as its own
failure, not the benchmark's exit code.

```sh
go build -o /tmp/problem-quality-benchmark ./cmd/problem-quality-benchmark
/tmp/problem-quality-benchmark \
  -execution local \
  -case ./internal/problemquality/testdata/development-order-tools \
  -development-fixture \
  -budget-state /tmp/problem-quality-budget.json \
  -output /tmp/problem-quality-report.json
```

The report is written to `/tmp/problem-quality-report.json`; the command exits
`1` because the fixture deliberately exposes a surviving wrong solution. Budget
state is written to `/tmp/problem-quality-budget.json` and reused by later runs
when the same path is supplied.

Local execution must be selected explicitly with `-execution local`. Model,
paid, and sandbox execution modes are rejected before any work runs.

### Run configuration

Each case needs a `benchmark-run.json` beside `case.yaml`. This is harness
configuration, not corpus data; the shared case schema has no reference-input
location. It binds the run to the case and artifact identity:

```json
{
  "case_id": "development-order-tools",
  "artifact_id": "development-order-tools-artifact",
  "artifact_version": "0.1.0",
  "artifact_root": "artifact",
  "manifest_path": "manifest.json",
  "development_fixture": true,
  "reference": {
    "id": "development-reference",
    "files": { "solution.py": "reference/solution.py" },
    "reviewed_by": "development-fixture",
    "reviewed_at": "2026-11-13T00:00:00Z",
    "reviewed_reference": "development fixture only; not human approval"
  }
}
```

Paths are relative to the run-config file. `artifact_root` is a local directory
containing the artifact manifest and files. The manifest remains the identity
authority: its artifact ID, version, technology, and commands must match the
case; the config cannot infer or override them. Reference files are explicit
replacements for declared learner-editable paths and are never inferred from
the first correct alternative.

A saved generated exercise may optionally supply known generation provenance
and historical usage. Omit unknown fields; the report says `unavailable`, never
`not_applicable`:

```json
"generation_provenance": {
  "model_revision": "model-revision-id",
  "prompt_revision": "prompt-revision-id",
  "config": { "temperature": 0.2 },
  "usage": {
    "input_tokens": 1200,
    "output_tokens": 340,
    "cost_micros": 4500,
    "currency": "USD",
    "cost_status": "provider_reported",
    "source": "provider usage export 2026-11-12"
  }
}
```

Omit a token or cost field only when that value is unknown; `0` means measured
zero. Omit `usage` entirely when no historical record exists. `cost_micros` is
integer micro-USD. `cost_status` is `provider_reported` or `estimated`;
estimated cost requires `pricing_basis`. `source` identifies the usage record.
Negative values, unsupported currencies, and inconsistent cost fields are
rejected.

### Input content identity

Every case report carries `content_identity`, a set of SHA-256 digests captured
before any assessment command runs. Paths are stable and relative, never
absolute machine paths:

- `case_definition`: `case.yaml`.
- `specification`: the referenced spec file, or
  `case.yaml#spec.path_or_inline` for an inline spec.
- `run_config`: the case's `benchmark-run.json`.
- `reference`: the explicit reference replacement files.
- `correct_alternatives` and `wrong_solutions`: each solution's declared source
  files.
- `artifact`: the manifest plus every input the manifest declares
  (`dependencyFiles`, `lockfiles`, `learnerEditable`, `protected`,
  `visibleTests`, `hiddenTests`).

Generated build output and caches (`__pycache__`, build directories, and similar
files) are outside the hashing scope because they are not declared inputs. A
changed case, spec, run config, solution, manifest, or declared artifact input
changes the corresponding digest; adding an undeclared cache file does not.

### Provenance status

The report keeps saved-exercise and assessment-run facts separate:

- `generation_during_assessment` is `false`: the assessment run itself made no
  model calls and generated nothing.
- `saved_exercise_provenance` is `known`, `unavailable`, or `not_applicable`
  for the saved exercise. Fixtures are `not_applicable`; a saved generated
  exercise with missing model or prompt revisions is `unavailable`.
- `saved_exercise_usage` contains imported historical usage when supplied;
  otherwise it is `unavailable` or `not_applicable` for fixtures.
- `assessment_run_usage` is `not_applicable` for the local assessment, which
  makes no model calls.

Known revisions, generation configuration, and usage supplied by the command or
run config are preserved. Top-level `model_revision` and `prompt_revision`
describe the requested baseline comparison (the command flags); each case's
`provenance` describes that saved exercise and may override them with explicit
`generation_provenance`. Tokens and costs are never fabricated. Imported usage
is not charged to the budget ledger and does not demonstrate paid enforcement;
a zero-paid local run is not evidence that the `$30` milestone was satisfied.

### Report and exit status

The report is written to `-output` (default `problem-quality-report.json`) and
validated against `schema/report.schema.json`. A report is written for completed
quality failures and for per-case configuration/execution failures. It marks
`preliminary: true`, labels fixture results, and records that no model or
sandbox operations occurred. Generation metrics are `unavailable` when the
baseline supports them but this run has no observations, and `not_applicable`
when the mechanism is unsupported; they are never zero or copied from
expectations.

- Exit `0`: every selected case completed and met benchmark expectations.
- Exit `1`: the benchmark completed but exposed quality failures, such as a
  correct alternative being rejected or a meaningful wrong solution surviving.
- Exit `2`: invalid global configuration, a per-case execution/configuration
  failure, or report write/validation failure.

The checked-in fixture deliberately exits `1`: its `case-sensitive` wrong
solution survives because the fixture's `normalization` assertion only exercises
lowercase input. Do not strengthen that test until the intentional survivor has
served its purpose.

### Budget accounting

The CLI persists a cumulative milestone budget in `-budget-state` (default
`problem-quality-budget.json`). Money is integer micro-USD and the fixed
milestone ceiling is `$30` (`30000000` micro-USD). `-prior-spend-micros`
records explicit prior spending when the state file is first created; later
runs preserve that persisted amount.

Paid adapters must reserve a bounded maximum before work, settle the actual
cost afterward, and mark unknown costs so they stay reserved until reconciled.
Reservations and settlements are keyed by operation ID, so repeating the same
request is idempotent and never double charges; a different reserved maximum
is rejected without changing state. Concurrent reservations are serialized
across processes through the persistent state file on platforms with file
locking. Windows builds explicitly reject persistent enforcement rather than
claim weaker process-local locking. The report's `budget` snapshot records the
accounting state.

This is only the accounting and enforcement primitive. No paid generation,
model, or sandbox adapter is connected to it yet, so local runs do not exercise
reservations or demonstrate paid-budget compliance. The paid-budget assessment
below records the exact missing integration; do not add speculative wiring just
to exercise the budget.

### Reviewed-case eligibility proposal

This proposal is not adopted policy. Every run must continue to report
`benchmark_eligibility: unresolved` until the owner accepts it and the
implementation can verify the required evidence.

The minimum decision set is:

1. **Reviewer authority**: name which human reviewers or review roles may mark
   corpus solutions reviewed. A free-form `reviewed_by` string, fixture label,
   agent name, or model output is not sufficient.
2. **Review scope**: decide whether a case-level approval may cover every
   reference, correct alternative, meaningful wrong solution, intended
   assertion, denominator decision, and ambiguity resolution, or require
   per-solution approval.
3. **Evidence binding**: require the review to bind artifact ID/version,
   reference ID, solution IDs, and content digests. Any changed solution or
   artifact digest invalidates the review.
4. **Review record**: require a resolvable review reference (for example a PR
   or issue URL) with reviewer identity and timestamp. Placeholder text such as
   `development-fixture` or `not human approval` can never establish eligibility.
5. **Denominator review**: an excluded/equivalent/invalid wrong solution is
   eligible for reporting only when that exclusion decision is itself reviewed.
6. **Separate eligibility from execution**: passing tests, a verified verdict,
   or a green report never promotes a case to the reviewed denominator.
7. **Revocation and re-review**: define who can withdraw approval and which
   content, artifact, or policy changes force re-review.
8. **Reporting states**: distinguish `eligible`, `ineligible`, and `unresolved`;
   a boolean alone loses why evidence was rejected.

Until those decisions land, fixtures and all other cases remain excluded from
reviewed totals and the report must not present execution success as trust.

### Paid-budget assessment

The `$30` paid milestone is not implemented. `internal/benchmarkbudget` supplies
the accounting and enforcement primitive: a persistent cumulative ceiling,
bounded reservations, idempotent settlement, unknown-cost retention, atomic
cross-process locking, and a reportable snapshot. The CLI opens and snapshots
that state, but this benchmark has no paid operation to enforce against it.

Existing infrastructure is useful but not sufficient:

- `internal/usage` records scoped token categories and estimated cost from a
  static rate card, with in-memory and Postgres stores. Its `RecordScoped` path
  can serve a server-owned caller, and `Aggregate` exposes per-user/workspace
  totals. It does not reconcile provider invoices or provide a benchmark-run
  reservation boundary.
- `internal/agentrelay` has operation-scoped token and cost budgets, deadlines,
  atomic usage accumulation, and revocation. It is designed for relayed model
  calls and depends on an active workflow operation; it does not account for
  paid sandbox compute or the benchmark's multi-case run envelope.
- `internal/problemquality/benchmark` only loads saved artifacts and invokes
  `environmentverify.VerifyAssessmentPlanWithOptions` through
  `envintegration.LocalCommandRunner` and `environmentverify.LocalProtocolAssessor`.
  It never constructs a model client, agent runtime, sandbox, or paid operation
  ID, and `runCase` never receives the budget store.

The exact missing integration is a benchmark-reachable paid operation that can
reserve before work and settle after it:

1. a generation, repair, or assessment step that actually calls a paid provider;
2. a deterministic operation ID and preflight maximum-cost estimate for each
   paid call;
3. usage/cost observation after success, failure, timeout, or cancellation,
   followed by `Settle` or `MarkUnknown`;
4. enforcement before each additional paid call and cancellation when the
   envelope is exhausted;
5. paid cost, token, reservation, and cap evidence in the final report.

No suitable existing path is connected to this benchmark. The local benchmark
intentionally evaluates fixed artifacts, while the agent-runtime benchmark is a
separate runtime-selection harness. The local fixture makes no paid calls and
does not satisfy any part of this milestone; adding speculative paid execution
here would change benchmark scope.

### Limitations and remaining work

This is local development evidence only. It does not establish sandbox network,
memory, or filesystem isolation, and it does not prove production safety or
broad trust in generated problems. Benchmark eligibility remains unresolved
until the shared per-solution review policy exists; successful execution is not
eligibility. The `$30` paid milestone remains unfinished until a
benchmark-reachable paid operation reserves and settles against the budget; a
zero-cost local run does not satisfy that budget. Metrics and recommendation work beyond this preliminary
report remain future work.
