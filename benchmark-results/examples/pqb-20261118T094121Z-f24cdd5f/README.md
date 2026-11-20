# Synthetic survivor example (report 2.0.0)

Preliminary local example evidence only. This is not reviewed corpus evidence,
human approval, benchmark eligibility, generation evidence, sandbox evidence,
or paid-operation evidence.

- Run: `pqb-20261118T094121Z-f24cdd5f`
- Date: November 18, 2026
- Case: `development-order-tools`
- Input: temporary copy of the checked-in development fixture
- Execution: local development shell
- Report: `report.json`
- Report format: `2.0.0`
- Labels: preliminary, development fixture, synthetic example, `tested_dirty: true`
- CLI exit: `1` (completed with a quality failure)

## Result

- Cases: 1 total, 0 passed, 1 failed, 0 errors, 0 not run
- Quality metrics: `unavailable` because the development fixture is not
  benchmark-eligible; this is insufficient evidence, not zero quality.
- Generation metrics: `not_applicable` because the baseline is not independent.
- Finding: `case-sensitive` survived its intended `normalization` / `CASE`
  rejection test. The finding records the intended assertion, comparator
  context, review action, and follow-up owners. Survival is established; the
  cause is explicitly not.

## Reproduction

From `backend/`, copy the development fixture to a temporary directory and run:

```bash
TMP=$(mktemp -d /tmp/pqb-example-survivor.XXXXXX)
cp -R internal/problemquality/testdata/development-order-tools/. "$TMP/"

PYTHONDONTWRITEBYTECODE=1 go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case "$TMP" \
  -development-fixture \
  -output /tmp/pqb-survivor-report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 60s
```

A reproduction gets a new run ID. The CLI exits `1` because the fixture
intentionally contains a surviving wrong solution; `go run` may report its own
process status for the child exit.

## Scope

The report shows one local assessment observation. It does not establish the
cause of the survivor, broad generated-problem trust, sandbox isolation, or paid
budget enforcement. Historical integration reports under
`benchmark-results/integration/` were not modified.
