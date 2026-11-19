# Synthetic setup-failure example (report 2.0.0)

Preliminary local example evidence only. This is not reviewed corpus evidence,
human approval, benchmark eligibility, generation evidence, sandbox evidence,
or paid-operation evidence.

- Run: `pqb-20261118T094129Z-074310c1`
- Date: November 18, 2026
- Case: `development-order-tools`
- Input: temporary copy of the checked-in development fixture with a broken
  setup command
- Execution: local development shell
- Report: `report.json`
- Report format: `2.0.0`
- Labels: preliminary, development fixture, synthetic example, `tested_dirty: true`
- CLI exit: `2` (per-case execution/configuration failure)

## Result

- Cases: 1 total, 0 passed, 0 failed, 1 error, 0 not run
- Quality metrics: `unavailable` because the case is a blocked development
  fixture. The setup failure is not counted as a passed or failed quality sample.
- Generation metrics: `not_applicable` because the baseline is not independent.
- Finding: `setup_failure` with the failing setup detail, a repair action, and
  follow-ups for the harness and reporting owners.

## Temporary input mutation

The copied manifest's setup command was replaced with:

```text
python3 -c 'import pqb_missing_dependency'
```

The checked-in fixture was not modified.

## Reproduction

From `backend/`:

```bash
TMP=$(mktemp -d /tmp/pqb-example-setup-failure.XXXXXX)
cp -R internal/problemquality/testdata/development-order-tools/. "$TMP/"
python3 - "$TMP/artifact/manifest.json" <<'PY'
import json, sys
path = sys.argv[1]
manifest = json.load(open(path))
manifest["commands"]["setup"] = "python3 -c 'import pqb_missing_dependency'"
open(path, "w").write(json.dumps(manifest, indent=2) + "\n")
PY

PYTHONDONTWRITEBYTECODE=1 go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case "$TMP" \
  -development-fixture \
  -output /tmp/pqb-setup-failure-report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 60s
```

A reproduction gets a new run ID. The CLI exits `2` for the per-case setup
failure; `go run` may report its own process status for the child exit.

## Scope

The report shows one local setup failure. It does not establish the cause of any
broader environment problem, broad generated-problem trust, sandbox isolation,
or paid budget enforcement. Historical integration reports under
`benchmark-results/integration/` were not modified.
