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