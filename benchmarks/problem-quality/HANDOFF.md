# Problem-Quality Corpus Handoff

This document describes what the harness lane can expect from the corpus.

## Corpus root

```text
benchmarks/problem-quality/
```

## Case root

```text
benchmarks/problem-quality/cases/<case-id>/
```

## Case files

Each case contains:

- `case.yaml`: case metadata and contract
- `spec.json`: reviewed exercise specification
- `expected-failure.yaml`: intended wrong-solution failure
- `review.yaml`: review metadata
- `correct/<alternative-id>/...`: correct alternative files
- `wrong/<wrong-id>/...`: wrong solution files

## Technology neutrality

The harness must not branch on technology, language, or framework. All
technology-specific details are provided by the case and its environment
manifest.

## Validation

Cases are validated using:

```text
benchmarks/problem-quality/VALIDATION.md
```

A case is not ready for harness integration until it passes that checklist.

## Report version and metric rules

Harness reports use `report_version: "2.0.0"` and must validate against
`schema/report.schema.json`.

Quality metrics include numerator, denominator, exclusions, and unavailability
reasons. Metrics include only benchmark-eligible, non-development-fixture
cases; equivalent and invalid wrong solutions remain visible but are excluded
from meaningful-wrong denominators. Measured zero is not unavailable, and zero
eligible cases are insufficient evidence.

Generation metrics are:

- `measured` only for independent-generation baselines with identity-bound
  historical observations
- `unavailable` when applicable observations are missing
- `not_applicable` for all other baselines

Observed generation values never come from `expected_outcomes`. They are never
reported as zero unless an observation actually recorded zero.

## Preliminary results

Any report based on the initial four-case sample must set:

```json
"preliminary": true
```

The report must not claim that generated problems are broadly trustworthy.
Reports carry structured findings for surviving wrong solutions, setup
failures, ambiguous specifications, and historical generation observations.
Synthetic examples must be labeled as synthetic and must not be counted as
reviewed corpus evidence. Labeled survivor and setup-failure examples are under
`benchmark-results/examples/`.

## Failure categories

Use the categories defined in `CONTRACT.md`:

- generation_failure
- ambiguous_specification
- setup_failure
- non_convergence
- reconciliation
- compile_failure
- platform_failure
- test_failure
- assertion_evidence_mismatch

## Notes for integration

- Use the case's build/test/run commands exactly as declared.
- Run correct and wrong solutions in separate clean workspaces.
- Local Python verification prerequisites and cache-safe commands are in
  `PYTHON.md`.
- Express verification prerequisites, pinned dependencies, offline setup, and
  bounded harness capture are in `EXPRESS.md`.
- Spring Boot verification prerequisites, clean-source validation, and Maven
  commands are in `SPRING_BOOT.md`.
- The Go case declares `go test -v ./...` because plain `go test ./...` hides
  passing-test output, including the `CODEGYM_TEST` protocol lines.
- Require structured assertion evidence for wrong-solution rejection.
- Record surviving wrong solutions with intended test/assertion evidence,
  comparator context, review action, and repair/review follow-ups.
- Record setup failures separately from quality failures.
- Record ambiguous specifications with reviewed ambiguity decisions.
- Record historical generation observations only when their source and
  artifact/input SHA-256 identity are supplied.
- Keep problem-quality findings separate from agent-runtime and Daytona findings.
- Real saved-exercise acceptance inputs and remaining #207 limits are listed in
  `ACCEPTANCE-207.md`.

```