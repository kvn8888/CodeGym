# Python Sum Even Numbers integration fixture

> Historical run. The current Python+Go run is
> `../pqb-20261117T090945Z-5cfe8ec1/`.

Preliminary development integration evidence only. This is not human
approval, benchmark eligibility, generation evidence, or paid-operation
evidence.

- Run: `pqb-20261117T084909Z-c36d34c7`
- Date: November 17, 2026
- Case: `benchmarks/problem-quality/cases/python-sum-even-numbers`
- Execution: local development shell
- Exit code: 0
- Result: 1 case passed, 0 failed, 0 errors, 0 not run

## Assessment outcome

- The development reference passed all four protected tests.
- The correct alternative `alt-01` passed all four protected tests.
- `sum-odd-numbers` was rejected by `hidden-mixed-values` with
  `WRONG_SUM`, expected `6`, actual `4`.
- `ignore-negative-evens` was rejected by `hidden-negative-values` with
  `WRONG_SUM`, expected `0`, actual `2`.
- No wrong solution survived.
- Benchmark eligibility remains `unresolved`; the fixture is excluded from
  reviewed quality totals.

## Added integration files

- `artifact/`: runnable artifact with the inherited artifact id/version,
  learner solution, protected pytest test, and `pytest.ini`.
- `reference/solution.py`: separate reference input bound by
  `benchmark-run.json`.
- `benchmark-run.json`: explicit development-fixture binding with
  `development_fixture: true` and a non-human review label.
- `environment/manifest.json`: completed with the setup/build/protected/
  hidden-test fields required by structural validation. The inherited
  artifact identity and `test` command are preserved.
- `backend/cmd/problem-quality-benchmark/main_test.go`: end-to-end CLI
  regression test for this case.
- `backend/internal/problemquality/benchmark/benchmark_test.go`: always-run
  case, run-config, reference, and assessment-plan binding test.

## Local validation caveat

This host has no `python` alias and the Python 3.13 installation has no
pytest. The run used a temporary PATH shim mapping `python` to `python3.11`
with pytest 7.4.2. The artifact `pytest.ini` disables output capture so the
`CODEGYM_TEST` protocol lines are visible.

The report records `tested_dirty: true` because these integration files were
uncommitted when the run executed.

The run does not establish sandbox, network, or memory isolation, and it
does not demonstrate paid-budget enforcement. It does not complete the
Go, Spring Boot, or Express integration gaps.
