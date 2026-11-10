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

## N/A metric rules

Generation metrics are:

- `measured` for independent-generation baselines
- `not_applicable` for all other baselines

They are never reported as zero.

## Preliminary results

Any report based on the initial four-case sample must set:

```json
"preliminary": true
```

The report must not claim that generated problems are broadly trustworthy.

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
- Require structured assertion evidence for wrong-solution rejection.
- Record surviving wrong solutions.
- Record ambiguous specifications.
- Keep problem-quality findings separate from agent-runtime and Daytona findings.
```