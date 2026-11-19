# Four-case integration fixture run (first repeat)

Preliminary local development evidence only. This is not human approval,
benchmark eligibility, generation evidence, sandbox evidence, or paid-operation
evidence.

- Run: `pqb-20261118T091136Z-2e8beee9`
- Date: November 18, 2026
- Cases:
  - `python-sum-even-numbers`
  - `go-count-even-numbers`
  - `express-greeting-endpoint`
  - `spring-boot-greeting-endpoint`
- Excluded: `example-sum` (intentional survivor fixture)
- Execution: local development shell
- Exit code: 0
- Result: 4 cases passed, 0 failed, 0 errors, 0 not run
- Report: `report.json`
- Report labels: preliminary, development fixture, `tested_dirty: true`
- Eligibility: all four cases remain `unresolved` and excluded from reviewed totals
- Repeat comparison: `../pqb-20261118T091229Z-91f2cdb3/`

## Reproduction

Populate dependencies once (Maven and npm caches, Python virtual environment),
then run from `backend/`:

```bash
rm -rf ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint/artifact/target
PATH="$PWD/../.venv/bin:$PATH" MAVEN_ARGS='-o' \
npm_config_offline=true npm_config_audit=false npm_config_fund=false \
PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/python-sum-even-numbers \
  -case ../benchmarks/problem-quality/cases/go-count-even-numbers \
  -case ../benchmarks/problem-quality/cases/express-greeting-endpoint \
  -case ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint \
  -development-fixture \
  -output /tmp/pqb-all-four-run1.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 300s
```

The saved report beside this file was copied from that output path; a
reproduction run gets a new run ID. `MAVEN_ARGS=-o` and `npm_config_offline=true` use populated caches. Omit them
when the network is available. Spring Boot prerequisites are in
`benchmarks/problem-quality/SPRING_BOOT.md`; Express prerequisites are in
`benchmarks/problem-quality/EXPRESS.md`; Python prerequisites are in
`benchmarks/problem-quality/PYTHON.md`.

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
- Both Go wrong solutions also fail `hidden-zero-value`, but each matches its
  declared intended failure.

Express:

- Reference and `alt-01` exited 0 and passed all six protected checks.
- `missing-name-validation` rejected by `hidden-missing-name`, `HTTP_STATUS`,
  expected `400`, actual `200`.
- `wrong-response-field` rejected by `hidden-response-shape`, `JSON_FIELD`,
  expected `message`, actual `greeting`.
- The harness also passed its focused bounded-capture overflow tests.

Spring Boot:

- Reference and `alt-01` exited 0 and passed all six protected checks.
- `missing-name-validation` rejected by `hidden-missing-name`, `HTTP_STATUS`,
  expected `400`, actual `200`.
- `wrong-response-field` rejected by `hidden-response-shape`, `JSON_FIELD`,
  expected `message`, actual `greeting`.
- Spring Boot dependencies were real (Spring Boot 3.4.2, Java 21); the existing
  `target/` build output was removed before the run.

No wrong solution survived in any case.

## Scope

The report does not establish sandbox, network, or memory isolation, and it
does not demonstrate paid-budget enforcement. Historical Python-only,
Python+Go, and Python+Go+Express reports remain in the neighbouring run
directories.
