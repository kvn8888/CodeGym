# Python, Go, and Express integration fixture run

Preliminary local development evidence only. This is not human approval,
benchmark eligibility, generation evidence, sandbox evidence, or paid-operation
evidence.

- Run: `pqb-20261117T093221Z-fde35ce7`
- Date: November 17, 2026
- Cases:
  - `benchmarks/problem-quality/cases/python-sum-even-numbers`
  - `benchmarks/problem-quality/cases/go-count-even-numbers`
  - `benchmarks/problem-quality/cases/express-greeting-endpoint`
- Execution: local development shell
- Exit code: 0
- Result: 3 cases passed, 0 failed, 0 errors, 0 not run
- Report: `report.json`
- Report labels: preliminary, development fixture, `tested_dirty: true`
- Eligibility: all three cases remain `unresolved` and excluded from reviewed totals

## Reproduction

Create the project-local Python environment once:

```bash
python3.11 -m venv .venv
.venv/bin/python -m pip install -r benchmarks/problem-quality/requirements-python-tests.txt
```

Run all three cases with repeatable `-case` flags from `backend/`:

```bash
PATH="$PWD/../.venv/bin:$PATH" \
npm_config_offline=true npm_config_audit=false npm_config_fund=false \
PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/python-sum-even-numbers \
  -case ../benchmarks/problem-quality/cases/go-count-even-numbers \
  -case ../benchmarks/problem-quality/cases/express-greeting-endpoint \
  -development-fixture \
  -output ../benchmark-results/integration/pqb-20261117T093221Z-fde35ce7/report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 120s
```

Omit `npm_config_offline=true` when npm can reach the registry. The local run
used the npm cache: `npm ci --offline` installed 68 packages, including
Express 4.22.2 from the pinned `package-lock.json`. If neither network nor cache
can satisfy the lockfile, Express execution is blocked; structural checks and
the Python/Go cases are unaffected.

Run Express individually by passing only its `-case` path. Its prerequisites and
offline setup are documented in `benchmarks/problem-quality/EXPRESS.md`.

## Case setup

Express uses the real dependency, not a stub:

- `package.json` pins `express` to `4.22.2`.
- `package-lock.json` is lockfileVersion 3.
- Setup is `npm ci`, separate from assessment.
- Build is `node --check src/app.js && node --check test/greeting.test.js`.
- Test is `npm test`, which runs `artifact/test/greeting.test.js`.

The test harness chooses an available loopback port, starts `src/app.js` as a
child process, polls readiness with a 10-second bound, applies a 3-second bound
to each request, and terminates the child with `SIGTERM` plus a `SIGKILL` fallback.
No server process remained after any run.

## Assessment outcome

Python:

- Reference and `alt-01` passed all four protected tests.
- `sum-odd-numbers` rejected by `hidden-mixed-values`, `WRONG_SUM`, expected
  `6`, actual `4`.
- `ignore-negative-evens` rejected by `hidden-negative-values`, `WRONG_SUM`,
  expected `0`, actual `2`.

Go:

- Reference and `alt-01` passed all four protected tests.
- `count-odd-numbers` rejected by `hidden-mixed-values`, `WRONG_COUNT`,
  expected `2`, actual `3`.
- `count-positive-evens-only` rejected by `hidden-negative-values`,
  `WRONG_COUNT`, expected `2`, actual `1`.
- Go wrong solutions also fail `hidden-zero-value`, but each matches its
  declared intended failure.

Express:

- Reference and `alt-01` exited 0 and passed all six protected checks.
- `missing-name-validation` exited 1 and was rejected by `hidden-missing-name`
  with `HTTP_STATUS`, expected `400`, actual `200`. It also fails
  `hidden-blank-name`.
- `wrong-response-field` exited 1 and was rejected by `hidden-response-shape`
  with `JSON_FIELD`, expected `message`, actual `greeting`. It also fails the
  message-value checks because the `message` field is absent.
- No wrong solution survived.

## Validation executed

- `npm ci --offline`: succeeded (Express 4.22.2, 68 packages).
- Express artifact `npm test`: passed all six protected checks.
- `TestRunExpressGreetingEndpointCaseExitsZero`: passed, exit 0.
- `TestRunPythonGoAndExpressCasesTogetherExitZero`: passed, exit 0.
- `PATH="$PWD/../.venv/bin:$PATH" npm_config_offline=true npm_config_audit=false npm_config_fund=false PYTHONDONTWRITEBYTECODE=1 go test ./... -count=1`: all packages passed except the two pre-existing `internal/envintegration` Daytona stub tests below.
- `go vet ./...`: passed.
- `gofmt -l` on changed Go files: no output.
- `git diff --check`: passed.
- No Python caches, Go artifact binary, or Express server process remained.

Known pre-existing full-suite failures, reproducible independently of these
changes:

- `TestDaytonaVerifierRunsStagesAndAssessmentsInSandbox`:
  `verdict = "rejected" (PLATFORM_FAULT), want verified`
- `TestDaytonaVerifierRejectionDeletesSnapshot`:
  `diagnostic code = "PLATFORM_FAULT", want "BUILD_FAILED"`

## Scope

This run does not complete the Spring Boot integration gap. It does not
establish sandbox, network, or memory isolation, and it does not demonstrate
paid-budget enforcement. Prior Python-only and Python+Go reports remain as
historical evidence in the neighbouring run directories.
