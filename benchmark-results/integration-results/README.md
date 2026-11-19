# Harness verification: four milestone cases, report format 2.0.0

Preliminary local development evidence only. This is not human approval,
benchmark eligibility, real generation evidence, sandbox isolation evidence, or
paid-budget enforcement evidence.

- Scope: #207 harness verification. #208 was not expanded.
- Cases: `python-sum-even-numbers`, `go-count-even-numbers`,
  `express-greeting-endpoint`, `spring-boot-greeting-endpoint` (explicit
  repeatable `-case` flags).
- Excluded: `example-sum` (intentional survivor fixture).
- Mode: local development shell with `-development-fixture`; no model calls, no
  paid operations, no GitHub access, and no dependency installation.
- Report format: `report_version: "2.0.0"`, `preliminary: true`.
- Report locations:
  - `pqb-20261119T115759Z-c163eed7/report.json`
  - `pqb-20261119T115946Z-2819c704/report.json`

## Environment and prerequisites

Used the documented local prerequisites without installing anything:

- Python 3.11.5 + pytest 7.4.2 from the existing repository `.venv`.
- Go 1.27.0.
- Node 22.14.0 + npm 10.9.2 with the populated npm cache
  (`npm_config_offline=true`).
- Java 21.0.2 + Maven 3.9.6 with the populated local Maven repository
  (`MAVEN_ARGS=-o`).
- Removed only generated Spring Boot `artifact/target` output before each run.
  The artifact source inputs and checked-in labels were unchanged.

## Canonical reproduction command

Run from `backend/`. Set `REPORT` to a fresh path so saved evidence is never
overwritten.

```bash
REPORT=/tmp/pqb-four-case-$(date -u +%Y%m%dT%H%M%SZ).json
rm -rf ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint/artifact/target && PATH="$PWD/../.venv/bin:$PATH" MAVEN_ARGS='-o' npm_config_offline=true npm_config_audit=false npm_config_fund=false npm_config_update_notifier=false PYTHONDONTWRITEBYTECODE=1 go run ./cmd/problem-quality-benchmark -execution local -case ../benchmarks/problem-quality/cases/python-sum-even-numbers -case ../benchmarks/problem-quality/cases/go-count-even-numbers -case ../benchmarks/problem-quality/cases/express-greeting-endpoint -case ../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint -development-fixture -output "$REPORT" -schema ../benchmarks/problem-quality/schema/report.schema.json -budget-state '' -timeout 300s
```

The saved evidence used the same command with
`-output /tmp/pqb-integration-run1.json` and
`-output /tmp/pqb-integration-run2.json`. Each command exited `0`. The CLI
wrote the report only after validating it against
`benchmarks/problem-quality/schema/report.schema.json`. Copy `$REPORT`
unchanged to a new `benchmark-results/integration-results/<run-id>/report.json`
directory; do not overwrite saved run directories.

## Saved-input readiness and review approval

Before executing a saved real case, check runnability and provenance without
running commands or touching the budget:

```bash
cd backend
go run ./cmd/problem-quality-benchmark -readiness -case ../benchmarks/problem-quality/cases/<case-id> -baseline-kind independent
```

`provenance_complete: false` is a preserved-data gap, not a harness failure.
`missing_provenance` lists the exact fields still unavailable. Invalid
configuration exits `2`.

Review eligibility remains `unresolved` unless the owner supplies a complete
`review.yaml` and the run names an authorized role with `-review-authority`.
Approval binds to `governed_content_sha256`; changing governed content makes
the approval stale. See `benchmarks/problem-quality/ACCEPTANCE-207.md` for the
required fields and checks. The four fixture reports above predate this support
and remain correctly unresolved.

## Observed results

| Run | Cases | Passed | Failed | Error | Not run | Development fixtures | Eligible | Unresolved eligibility | Elapsed |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `pqb-20261119T115759Z-c163eed7` | 4 | 4 | 0 | 0 | 0 | 4 | 0 | 4 | 48066 ms |
| `pqb-20261119T115946Z-2819c704` | 4 | 4 | 0 | 0 | 0 | 4 | 0 | 4 | 46879 ms |

Both reports are `tested_dirty: true` at commit
`47e1d034695013b6316d60237441bf296f9fa4b2` on
`feature/problem-quality-integration` because the working tree contains the
in-progress harness changes.

## Repeat comparison

Ignoring expected differences in run IDs, timestamps, and elapsed time, both
runs agree on:

- Selected case IDs and case set (the same four cases, no `example-sum`).
- Per-case content identities: artifact, case definition, specification,
  run config, reference, correct alternatives, and wrong solutions.
- Solution outcomes: references and correct alternatives accepted; every wrong
  solution rejected.
- Intended assertion evidence and `intended_failure_matched: true` for every
  wrong solution.

| Case | Status | Reference | Correct alternative | Wrong solutions (test/code/expected/actual) |
| --- | --- | --- | --- | --- |
| `python-sum-even-numbers` | passed | accepted | `alt-01` accepted | `sum-odd-numbers` (`hidden-mixed-values`/`WRONG_SUM`/`6`/`4`), `ignore-negative-evens` (`hidden-negative-values`/`WRONG_SUM`/`0`/`2`) |
| `go-count-even-numbers` | passed | accepted | `alt-01` accepted | `count-odd-numbers` (`hidden-mixed-values`/`WRONG_COUNT`/`2`/`3`), `count-positive-evens-only` (`hidden-negative-values`/`WRONG_COUNT`/`2`/`1`) |
| `express-greeting-endpoint` | passed | accepted | `alt-01` accepted | `missing-name-validation` (`hidden-missing-name`/`HTTP_STATUS`/`400`/`200`), `wrong-response-field` (`hidden-response-shape`/`JSON_FIELD`/`message`/`greeting`) |
| `spring-boot-greeting-endpoint` | passed | accepted | `alt-01` accepted | `missing-name-validation` (`hidden-missing-name`/`HTTP_STATUS`/`400`/`200`), `wrong-response-field` (`hidden-response-shape`/`JSON_FIELD`/`message`/`greeting`) |

Per-case elapsed values differed only by timing:

| Case | Run 1 | Run 2 |
| --- | --- | --- |
| `python-sum-even-numbers` | 1460 ms | 1061 ms |
| `go-count-even-numbers` | 4932 ms | 4276 ms |
| `express-greeting-endpoint` | 21639 ms | 21300 ms |
| `spring-boot-greeting-endpoint` | 19954 ms | 20159 ms |

## Scope and remaining limits

- All four cases remain development fixtures with unresolved benchmark
  eligibility and are excluded from reviewed quality totals. Both reports show
  quality metrics `unavailable` because there are zero eligible cases; this is
  insufficient evidence, not a measured zero.
- This run contains no real generation observations. Model and prompt revisions
  are `not_applicable` for these fixtures.
- The budget ledger was disabled (`-budget-state ''`) for these runs. The
  accounting infrastructure exists but no paid adapter is connected; paid-budget
  scope and enforcement remain unresolved.
- Historical reports under `benchmark-results/integration/` and
  `benchmark-results/review/` were left unchanged.
