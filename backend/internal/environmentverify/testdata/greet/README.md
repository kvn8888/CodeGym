# Sample assessment artifact: greet-basics

Dependency-free Python exercise used as the milestone 4 assessment fixture
for issue #188. Prepared for human review; review has not occurred.

## Specification

`greet(name: str) -> str` in `solution.py`:

- Return `"Hello, {name}!"` for an ordinary name: `greet("Alice")` gives
  `"Hello, Alice!"`.
- Strip surrounding whitespace first: `greet("  Bob  ")` gives
  `"Hello, Bob!"`.
- Fall back for blank input: `greet("")` and `greet("   ")` (empty or
  whitespace-only) give `"Hello, stranger!"`.

`solution.py` is the only learner-editable file. `protected/test_greet.py`
is protected and hidden: learners never see it before assessment.

## Why the correct solution passes

`solutions/correct.py` strips the input, returns the stranger fallback when
nothing remains, and otherwise interpolates the cleaned name. All four
protected tests pass against it.

## Why the wrong solution must fail

`solutions/wrong.py` compiles and runs and passes `test_basic_greeting`,
but interpolates the raw name. The backend therefore requires it to fail
`test_strips_whitespace` with an assertion failure:
`'Hello,   Bob  !' != 'Hello, Bob!'`. (It also fails
`test_empty_name_is_stranger` and `test_empty_string_is_stranger`, which
are tolerated as additional assertion failures; only the expected
rejection is required.)

## Layout

- `manifest.json` — the builder-proposed manifest (hidden-only assessment:
  `visibleTests` is empty).
- `solution.py` — learner starter stub (raises `NotImplementedError`).
- `protected/test_greet.py` — protected unittest assessment.
- `solutions/correct.py`, `solutions/wrong.py` — backend-controlled
  assessment inputs (not part of the artifact file lists).

## Review status

REVIEW STATUS: PENDING. No human review has occurred and no reviewer is
claimed. Automated checks run the fixture end to end, but a `verified`
verdict additionally requires backend-controlled assessment inputs
carrying review evidence for the reviewed sample (`reviewedBy`), which
test harnesses must not fabricate. This establishes nothing about every
production artifact: whether production assessment inputs need
per-artifact human approval is unresolved shared policy for Track A
coordination under #184.

## Human review checklist

A reviewer approving this sample for acceptance evidence must confirm each
item below and record their identity plus a reference (date, ticket, or
commit) in the backend's `reviewedBy`/`reviewedReference` fields:

Files:

- `manifest.json` — schema `environment.manifest.v1`; artifact id
  `greet-basics-2026-10-01` version `0.1.0`; dependency-free environment
  (`dependencyFiles` and `lockfiles` empty); workspace root `"."` with
  `learnerEditable: ["solution.py"]` and `protected:
  ["protected/test_greet.py"]`; setup/build/test commands as listed;
  hidden-only assessment (`visibleTests` empty, `hiddenTests` lists the
  protected test).
- `solution.py` — starter stub only: raises `NotImplementedError` and
  leaks no solution.
- `protected/test_greet.py` — exactly four tests (`test_basic_greeting`
  `BASIC_GREETING`, `test_strips_whitespace` `STRIP_WHITESPACE`,
  `test_empty_name_is_stranger` `STRANGER_FALLBACK`,
  `test_empty_string_is_stranger` `STRANGER_FALLBACK`), each emitting one
  `SAMPLE_ASSERT` record with the stable code and exact expected/actual
  values on failure.
- `solutions/correct.py` — implements strip plus the stranger fallback.
- `solutions/wrong.py` — compiles, runs, passes the basic test, and fails
  only because it skips the strip and fallback requirements.

Behavior:

- The correct solution passes 4/4 protected tests.
- The wrong solution fails `test_strips_whitespace` with an assertion
  failure recording code `STRIP_WHITESPACE`, expected `"Hello, Bob!"`,
  actual `"Hello,   Bob  !"`.
- Protected tests are listed in `workspace.protected`, never in
  `learnerEditable`; the hidden test cannot be reached through learner
  edits, path tricks, or filesystem aliases.
- No network access or external dependency is needed at any stage.
