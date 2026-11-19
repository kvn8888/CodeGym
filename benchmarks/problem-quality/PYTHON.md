# Python test prerequisites

The Python corpus cases declare `python -m pytest -q` as their test command.
That command is intentionally unchanged. The local verification environment is
a project-local virtual environment at `.venv`.

## Prerequisites

- Python 3.11.x (verified with 3.11.5)
- pytest 7.4.2 (pinned in `requirements-python-tests.txt`)

## Create the environment

From the repository root:

```bash
python3.11 -m venv .venv
.venv/bin/python -m pip install -r benchmarks/problem-quality/requirements-python-tests.txt
```

## Run the benchmark

From `backend/`:

```bash
PATH="$PWD/../.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 \
  go run ./cmd/problem-quality-benchmark \
  -execution local \
  -case ../benchmarks/problem-quality/cases/python-sum-even-numbers \
  -development-fixture \
  -output /tmp/python-sum-even-numbers-report.json \
  -schema ../benchmarks/problem-quality/schema/report.schema.json \
  -budget-state '' \
  -timeout 60s
```

`PYTHONDONTWRITEBYTECODE=1` prevents `__pycache__` from being left inside
checked-in artifact inputs. The Python artifact's `pytest.ini` disables both
pytest output capture (so `CODEGYM_TEST` lines are visible) and the pytest cache
provider (so `.pytest_cache` is not created). The declared build command
compiles the source in memory instead of writing `py_compile` bytecode into the
artifact.

The CLI regression test skips explicitly when `python` or
`python -m pytest` is unavailable. Structural case/run-config/plan coverage
runs regardless of Python availability.
