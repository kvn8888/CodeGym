# Python and Go integration fixture run

> Historical run. The current Python+Go+Express run is
> `../pqb-20261117T093221Z-fde35ce7/`.

Preliminary local development evidence only. This is not human approval,
benchmark eligibility, generation evidence, sandbox evidence, or paid-operation
evidence.

- Run: `pqb-20261117T090945Z-5cfe8ec1`
- Date: November 17, 2026
- Cases:
  - `benchmarks/problem-quality/cases/python-sum-even-numbers`
  - `benchmarks/problem-quality/cases/go-count-even-numbers`
- Execution: local development shell
- Exit code: 0
- Result: 2 cases passed, 0 failed, 0 errors, 0 not run
- Report: `report.json`
- Report labels: preliminary, development fixture, `tested_dirty: true`
- Eligibility: both cases remain `unresolved` and excluded from reviewed totals

## Reproduction

Create the project-local Python environment once:

```bash
python3.11 -m venv .venv
.venv/bin/python -m pip install -r benchmarks/problem-quality/requirements-python-tests.txt
```

Run both cases with repeatable `-case` flags from `backend/`:

```bash
PATH="$PWD/../.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/python-sum-even-numbers \
  -case ../benchmarks/problem-quality/cases/go-count-even-numbers \
  -development-fixture \
  -output ../benchmark-results/integration/pqb-20261117T090945Z-5cfe8ec1/report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 60s
```

The Python artifact keeps the declared `python -m pytest -q` test command.
Its `pytest.ini` disables output capture and the pytest cache provider. The run
also sets `PYTHONDONTWRITEBYTECODE=1`. The syntax-check build command compiles
source in memory instead of writing `py_compile` bytecode into the artifact.
No `__pycache__`, `.pytest_cache`, or `.pyc` files remain under the Python
artifact after the run.

The Go case uses `go test -v ./...`. The `-v` flag is required because
`go test ./...` suppresses stdout and stderr for passing tests, which would hide
the `CODEGYM_TEST` protocol lines. The Go build command uses
`go build -o /dev/null ./...` so validation does not leave a compiled binary in
the artifact. Both commands are declared in the case and artifact manifests
rather than implemented as harness exceptions.

## Assessment outcome

Python:

- The development reference passed all four protected tests.
- Correct alternative `alt-01` passed all four protected tests.
- `sum-odd-numbers` was rejected by `hidden-mixed-values` with `WRONG_SUM`,
  expected `6`, actual `4`.
- `ignore-negative-evens` was rejected by `hidden-negative-values` with
  `WRONG_SUM`, expected `0`, actual `2`.

Go:

- The development reference passed all four protected tests.
- Correct alternative `alt-01` passed all four protected tests.
- `count-odd-numbers` was rejected by `hidden-mixed-values` with
  `WRONG_COUNT`, expected `2`, actual `3`.
- `count-positive-evens-only` was rejected by `hidden-negative-values` with
  `WRONG_COUNT`, expected `2`, actual `1`.

The Go wrong solutions also fail `hidden-zero-value` because both omit the even
zero. Each still matches its declared intended failure. No wrong solution
survived.

## Validation executed

- `go test -v ./...` in the Go artifact: passed.
- `go build -o /dev/null ./...` and `go vet ./...` in the Go artifact: passed.
- `PATH="$PWD/../.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 go test ./cmd/problem-quality-benchmark ./internal/problemquality/... ./internal/environmentverify/... -count=1`: passed.
- `PATH="$PWD/../.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 go test ./... -count=1`: all packages passed except the two pre-existing `internal/envintegration` Daytona stub tests below.
- `go vet ./...` in `backend/`: passed.
- `gofmt -l` on changed Go files: no output.
- `git diff --check`: passed.

Skip behavior was verified separately with `python` absent from `PATH`:
`TestRunPythonSumEvenNumbersCaseExitsZero` and
`TestRunPythonAndGoCasesTogetherExitZero` both reported `--- SKIP` with an
explicit prerequisite message. The always-run structural binding tests still
execute.

Known pre-existing full-suite failures, reproducible independently of these
changes:

- `TestDaytonaVerifierRunsStagesAndAssessmentsInSandbox`:
  `verdict = "rejected" (PLATFORM_FAULT), want verified`
- `TestDaytonaVerifierRejectionDeletesSnapshot`:
  `diagnostic code = "PLATFORM_FAULT", want "BUILD_FAILED"`

## Scope

This run does not complete the Spring Boot or Express integration gaps. It does
not establish sandbox, network, or memory isolation, and it does not
demonstrate paid-budget enforcement. The earlier Python-only report remains in
`../pqb-20261117T084909Z-c36d34c7/` as historical evidence.
