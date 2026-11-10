# Problem-Quality Corpus Validation

This checklist is used to validate a corpus case before it is considered
ready for harness integration.

## Required files

- `case.yaml`
- `spec.json`
- `expected-failure.yaml`
- `review.yaml`
- at least one file under `correct/`
- exactly two directories under `wrong/`

## Case validation

- [ ] `case_id` is present and unique.
- [ ] `version` is present.
- [ ] `technology` is present.
- [ ] `objective` is present and meaningful.
- [ ] `environment_manifest` is present.
- [ ] `build_test_run.strategy` is present.
- [ ] `build_test_run.commands.test` is present.
- [ ] `build_test_run.limits` is present.
- [ ] `spec.path_or_inline` is present.
- [ ] `spec.ambiguity_resolutions_required` is true.
- [ ] `correct_alternatives` has at least one entry.
- [ ] `wrong_solutions` has exactly two entries.
- [ ] `comparator_expectations.default.kind` is present.
- [ ] `expected_outcomes` is present.
- [ ] `review.reviewed_by` is present.
- [ ] `review.reviewed_at` is present.

## Spec validation

- [ ] Spec is valid JSON.
- [ ] Language is present.
- [ ] Strategy is present.
- [ ] Title is present.
- [ ] Description is present.
- [ ] Entrypoint is present.
- [ ] Signature is present.
- [ ] I/O contract is present.
- [ ] Comparator is present.
- [ ] Ambiguity resolutions are non-empty.

## Solution validation

- [ ] Correct alternative is reviewed.
- [ ] Correct alternative is independently written.
- [ ] Each wrong solution is hand-written.
- [ ] Each wrong solution is plausible.
- [ ] Each wrong solution targets the learning objective.
- [ ] Each intended failure has test ID, code, expected, actual, and reason.
- [ ] Denominator decision is recorded for each wrong solution.

## Technology-neutrality validation

- [ ] No harness code change is required for this technology.
- [ ] No fixed technology-specific path is required.
- [ ] Environment details are provided through the manifest and contract.
- [ ] The case can be added without a backend language switch.

## Review validation

- [ ] Reviewer is recorded.
- [ ] Review date is recorded.
- [ ] Ambiguous spec flag is recorded.
- [ ] Disputed wrong solutions are documented.
- [ ] Demo/unreviewed assessments are excluded.

## Readiness for harness integration

A case is ready for the harness lane only when all sections above pass.
```