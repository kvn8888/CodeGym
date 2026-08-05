# Agent runtime decision

Issue: #148  
Measured: 2026-08-05  
Report: [`backend/internal/agentruntime/testdata/benchmark-report.json`](../backend/internal/agentruntime/testdata/benchmark-report.json)

## Decision

Select the purpose-built runtime seam for the next implementation stage.

This is not a benchmark win. The comparison was inconclusive: both runtimes
completed 0 of 6 verified fixtures, so the predeclared rule selects
purpose-built by default. opencode remains a working adapter behind the same
contract and can be reconsidered if a later, separately budgeted comparison
shows a material advantage.

## What was built

- `AgentRuntime` accepts the goal, absolute working directory, allowed tools,
  turn ceiling, deadline, and output ceiling.
- Both adapters return the same telemetry and untrusted manifest claim. Only a
  backend `Verifier` can issue the pass/fail verdict.
- The purpose-built adapter uses the CodeGym OpenAI-compatible relay shape and
  exposes confined reads/writes, process-group-bounded shell execution, and
  `report_progress`.
- The opencode adapter pins v1.18.11, verifies the version, uses a fresh HOME
  for every run, routes through the same relay, and parses its JSONL telemetry.
- Go net/http and Express fixtures build, boot, probe an HTTP endpoint, reject
  broken variants, scan for a synthetic canary and agent state, and repeat with
  egress-dependent source rejected plus offline/proxy-blocked runtime settings.

## Predeclared comparison

Both runtimes used `azure/gpt-5.6-terra`, the same task prompt per stack, base
fixture fingerprint, verifier, relay token/budgets, zero-retry policy, turn
ceiling (8), output cap (262,144 bytes), and per-run deadline (60 seconds).
The order was interleaved for 3 repetitions over Go net/http and Express: 12
runs total.

| Runtime | Verified | Repairs (mean) | Median wall time | Tokens | Cost |
| --- | ---: | ---: | ---: | ---: | ---: |
| purpose-built | 0/6 | 0.33 | 23.890 s | 44,562 | $0.201900 |
| opencode | 0/6 | 1.17 | 24.178 s | 202,391 | $0.651760 |

Purpose-built produced six absent manifests: four turn-ceiling exhaustions and
two deadline exhaustions. opencode exited 0 and completed its runner in all six
runs, but all six manifests were malformed under the shared strict schema.
Neither runtime recorded a policy violation. Aggregate usage was 246,953 tokens
and $0.853660, below the 300,000-token and $2.00 hard ceilings.

## Milestone 1 diagnosis

The original report does not contain the evidence needed to attribute these as
two proven root causes. `BenchmarkRun` retained termination, repair count,
usage, wall time, and only the manifest status. It discarded the turn count,
tool trace, final prose, `ManifestClaim.Error`, and malformed manifest bytes,
then deleted all 12 workspaces. Therefore the six historical `malformed`
statuses are real, but their bytes are irrecoverable and it is not possible to
quote a historical malformed manifest honestly. That is a harness
observability defect, and the earlier claim that opencode had a distinct
manifest-contract bug was stronger than its retained evidence.

To avoid inferring from aggregates, one diagnostic Go fixture was reproduced
per runtime against the unchanged adapters, prompts, eight-turn ceiling, and
60-second deadline. A temporary test captured the already-returned values and
was removed immediately afterward.

### Purpose-built

The loop made concrete progress on every turn. Its actual trace was:

```text
1 report_progress {"step_id":"inspect","label":"Inspecting workspace"}
2 shell "pwd && find ..."
3 read_file {"path":"go.mod"}
4 write_file {"path":"main.go", ...}
5 write_file {"path":"main_test.go", ...}
6 report_progress {"step_id":"implement", ...}
7 shell "gofmt ... && go test ./... && go build ... && PORT=18080 ..."
8 shell "set -eu; gofmt ...; go test ./...; go build ...; PORT=18080 ..."
```

Turn 7 exited 0. Turn 8 repeated the smoke check with safer shell structure,
but the hard-ceiling rule intentionally refused to dispatch a tool call on the
last permitted model turn. The workspace already contained `main.go` and
`main_test.go`; no manifest had been written. Telemetry was exactly eight turns,
seven executed tools, zero repairs, `turn_ceiling_exhausted`, and 8,559 tokens.

This is not a loop stuck re-reading the same files. Two progress-only turns and
the repeated smoke command are avoidable overhead, but a normal inspect,
implement, unit-test, build, boot, probe, manifest, and final-response sequence
cannot reliably fit in eight model turns when the eighth turn cannot execute a
tool. The ceiling was simply too low. The two historical Express deadlines also
show that 60 seconds was not a reliable per-run allowance for its dependency
and validation path.

### opencode

The unchanged reproduction contradicted the assumption that opencode currently
finishes and emits malformed JSON. It exited 0 after exactly eight steps with
no manifest at all. Its real final prose was:

```text
Maximum steps reached.

Implemented `main.go` and `main_test.go`; `go test ./...` and `go build ./...`
passed. Remaining: endpoint smoke verification and required
`.codegym/agent-result.json` manifest were not completed.
```

The adapter nevertheless labelled the run `completed` because its ceiling
check only recognizes exhaustion when final prose is empty. The trace contained
seven tool events and one denied read of opencode's own externalized
`~/.local/share/opencode/tool-output` file. That denial consumed useful budget
and was surfaced as a policy violation even though the denied target was
runtime-owned transient tool output, not another workspace.

The manifest instructions are independently under-specified and stricter than
their role warrants. Both prompts only say to write strict JSON with
`version`, `completed`, `summary`, `artifacts`, and `checks`; they give no types
or worked example. The loader rejects every unknown field and validates
artifact paths even though the verifier ignores the manifest contents and owns
the verdict. This contract should be reduced to an unambiguous completion claim
and should return its exact parse/validation error. That is a supported harness
fix, but it is not a proven explanation for the six historical malformed files
because the harness destroyed those bytes.

The evidence therefore supports three harness fixes before another comparison:
a shared realistic budget for both adapters, truthful opencode ceiling
classification plus permission handling for its own tool-output files, and a
minimal explicit manifest claim with preserved diagnostics. It does not support
tuning only purpose-built or declaring the historical default a runtime win.

## Qualification

The comparison used 12 isolated temporary host workspaces and deleted all 12.
It created zero Daytona sandboxes. Consequently, the filesystem-escape signal
comes from confined purpose-built file operations, opencode permissions, and
post-run scanning—not a Daytona OS boundary. That prevents treating the result
as evidence that either runtime is secure on the host and reinforces the
inconclusive classification. Runtime deployment must still occur inside the
separate Daytona boundary described in `docs/agentic-problem-generation.md`.
Sandbox usage was therefore 0 seconds; the separately reported 369.330 seconds
is aggregate host wall time.

The offline Express verifier supplies a controlled local module implementing
the small Express API surface exercised by the fixture. It does not prove the
real npm package or an OS-level network block. A production benchmark should
put the pinned Express dependency in the base Daytona snapshot and run the same
black-box probe behind Daytona's network boundary.

## Spring Boot best effort

The Spring Boot fixture and black-box verifier are implemented, but the offline
gate stopped before model calls. The isolated Maven repository did not contain
the pinned `spring-boot-starter-parent:3.3.5`, so Maven correctly refused to
contact Central in offline mode. Spring Boot needs a pre-baked dependency cache
in the base snapshot before it is benchmarkable; spending model or sandbox
budget before that would not produce useful runtime evidence.
