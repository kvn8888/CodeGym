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

For all other baselines, report these as `not_applicable`. Never report them as
zero.

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