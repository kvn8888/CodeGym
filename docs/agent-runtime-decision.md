# Agent runtime decision

Issue: #148  
Measured: 2026-08-05  
Reports:

- local spending gate: [`backend/internal/agentruntime/testdata/benchmark-report.json`](../backend/internal/agentruntime/testdata/benchmark-report.json)
- corrected Daytona result: [`backend/internal/agentruntime/testdata/benchmark-daytona-report.json`](../backend/internal/agentruntime/testdata/benchmark-daytona-report.json)
- first Daytona harness failure: [`backend/internal/agentruntime/testdata/benchmark-daytona-attempt1-report.json`](../backend/internal/agentruntime/testdata/benchmark-daytona-attempt1-report.json)
- corrected-run segments: [`backend/internal/agentruntime/testdata/benchmark-daytona-attempt2-partial-report.json`](../backend/internal/agentruntime/testdata/benchmark-daytona-attempt2-partial-report.json) and [`backend/internal/agentruntime/testdata/benchmark-daytona-continuation-report.json`](../backend/internal/agentruntime/testdata/benchmark-daytona-continuation-report.json)

## Decision

Select the opencode adapter for the next implementation stage. In the corrected
real-dependency result, opencode completed 4 of 6 tasks and purpose-built
completed 3 of 6. Neither had a policy violation. Completion rate is the
predeclared primary criterion, so the rule selects opencode; this is not a
default-by-inconclusiveness decision.

This is an operational decision, not a broad statistical claim. Six attempts
per runtime are a small sample, the adapters themselves still ran in isolated
host workspaces, and the Daytona launcher imposed an unstated root-entrypoint
layout on two otherwise plausible candidates. Those limitations make the
exact one-task margin non-conclusive as a general runtime-quality claim. They
do not change what the predeclared rule produces for the recorded verdicts.

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
- The result manifest is now a minimal completion claim. Both prompts include
  the worked example `{"version":1,"completed":true}`; optional descriptive
  fields and unknown fields do not control the verdict.
- The default comparison budget is shared by both adapters: 16 turns and a
  two-minute per-run deadline.
- Benchmark reports retain turns, tool traces, final prose, raw manifest bytes,
  exact manifest diagnostics, progress, usage, and resource ledgers.

## Original inconclusive comparison

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

## Milestone 2 fixes and no-cost proof

The shared budget was raised from 8 to 16 turns and from 60 seconds to two
minutes for both runtimes. The turn traces justify that number: the smallest
successful purpose-built reproduction needed eight model turns before it could
write a manifest, while a real Express path also needs dependency validation,
boot, and an HTTP probe. The ceiling remains bounded and identical.

The prompts now state that the manifest is an untrusted completion claim, show
the exact minimal JSON, and explain that the backend verifier owns pass/fail.
The loader requires only `version: 1` and `completed: true`; it accepts optional
summary, artifacts, checks, and unknown descriptive fields. Missing or malformed
claims produce an exact diagnostic such as `result manifest field \"completed\"
must be true` or the JSON decoder error. Reports preserve the raw claim.

The opencode adapter now checks the manifest before classifying exit 0 as
completion, correctly reports a reached step ceiling even when final prose is
present, and does not call a denied read of its own isolated tool-output spill
path a candidate policy violation. Other permission failures remain
disqualifying. The fixture egress scanner now permits literal loopback-only
test probes while continuing to reject external or variable egress targets.

Fake-relay tests, with no credentials and no model or sandbox cost, prove that
both adapters produce a contract-valid claim on a simulated success and surface
the concrete manifest diagnostic on malformed output. Tests also cover
opencode ceiling-with-prose classification, runtime-owned tool-output denials,
loopback-versus-external egress scanning, report evidence retention, and the
minimal/extended manifest contract.

## Milestone 3 local spending gate

The existing 12-run interleaved benchmark was rerun locally with the shared
16-turn/two-minute budget. It passed the spending gate because both runtimes
reached the verifier and each completed 5 of 6 tasks:

| Runtime | Verified | Repairs (mean) | Median wall time | Tokens | Cost |
| --- | ---: | ---: | ---: | ---: | ---: |
| purpose-built | 5/6 | 0.17 | 26.418 s | 89,477 | $0.368100 |
| opencode | 5/6 | 0.67 | 23.971 s | 181,497 | $0.569811 |

The raw report's `both-disqualified` decision is not a runtime result. Its two
policy signals were classifier false positives found from the retained
evidence: purpose-built used `fetch("http://127.0.0.1:...")` in a loopback test,
and opencode was denied a read of its own isolated tool-output spill file.
Those classifiers were corrected before Daytona without rerunning paid local
attempts. The one genuine opencode failure is now directly inspectable:

```text
{\"version\":1,\"completed\":true,\"summary\":\"Go net/http health service implemented and verified.\",...}
```

It wrote JSON with literal backslashes, and the retained diagnostic is
`decode result manifest: invalid character '\\' looking for beginning of object
key string`. This proves that the explicit contract fixed five of six opencode
claims while still rejecting genuinely malformed bytes with an actionable
error. The local gate used 270,974 tokens and $0.937911. It created and deleted
12 host workspaces and created no Daytona sandboxes.

## Milestone 4 real dependency conditions

The Daytona snapshot preflight verified `codegym-go-1-25-4-v3` with Go 1.25.4,
Node 25.9.0, and npm 11.12.1. For every contract-valid candidate, an ephemeral
Daytona sandbox received only candidate source/module files, ran real `go mod
download`/`go mod verify` or `npm install`, then ran the black-box loopback HTTP
probe. Express resolved and verified the actual pinned `express@5.1.0` package.

The first 12-run attempt is deliberately preserved as a harness failure. Ten
contract-valid candidates resolved dependencies, but all ten were stopped
before the HTTP probe because the verifier tried to apply `networkBlockAll`
after installation. Daytona's organization tier already enforces its own
network restrictions and rejects per-sandbox overrides. That attempt therefore
recorded 0/6 for both runtimes, 10/10 sandboxes deleted, 116.026 sandbox-seconds,
278,864 tokens, and $0.980680. It is not used for the runtime decision.

The verifier was changed to continue only for that exact documented tier-policy
response and to fail closed on every other firewall error. Source egress checks
still run before sandbox creation, and the service/probe stay on loopback. The
corrected suite's cost guard stopped after order 11, so a continuation executed
only the unstarted scheduled order 12 with the same conditions. No completed
order was replayed. The canonical report validates and merges orders 1-11 and
12 and sums both relay-segment ceilings and resource ledgers.

| Runtime | Verified | Repairs (mean) | Median wall time | Tokens | Cost |
| --- | ---: | ---: | ---: | ---: | ---: |
| purpose-built | 3/6 | 0.83 | 30.716 s | 76,251 | $0.315360 |
| opencode | 4/6 | 1.17 | 42.188 s | 236,696 | $0.757787 |

Purpose-built completed all three Go tasks. Its Express runs had two deadline
exhaustions with absent manifests and one verifier rejection because the agent
used `index.js` while the Daytona launcher assumed root `server.js`. opencode
completed two Go and two Express tasks; one Express run hit the deadline, and
one Go candidate used `cmd/service/main.go` while the launcher assumed a root
main package. The latter two layout requirements were not stated in the task
prompts. Their workspaces were deleted as required, so their self-reported
successful checks cannot be promoted to verified passes after the fact.

The corrected result created/deleted 12/12 host workspaces and 9/9 Daytona
sandboxes, recording 109.585 sandbox-seconds. It used 312,947 tokens and
$1.073147. The predeclared completion-rate rule selects opencode 4/6 over
purpose-built 3/6. The decision is conclusive under that operational rule, but
the entrypoint-layout limitation means the one-run margin is not conclusive
evidence of a general capability difference.

## Qualification and resource/spend ledger

The coding-agent processes still ran in isolated host workspaces because the
operation-scoped local relay was not reachable from Daytona. Real dependency
resolution and black-box verification ran in Daytona. This is stronger than
the local gate but is not evidence that either adapter itself is safe to deploy
on the host; production execution still belongs inside the separate Daytona
boundary described in `docs/agentic-problem-generation.md`.

Across this work, 21 Daytona sandboxes were created and all 21 were deleted:
2 snapshot preflights, 10 in the preserved failed attempt, and 9 in the
corrected result. Exact benchmark reports record 225.611 sandbox-seconds; the
two short preflights are outside those report meters. A final labelled-sandbox
readback found zero remaining benchmark sandboxes.

Paid reports record 862,785 model tokens and $2.991738: $0.937911 for the local
gate, $0.980680 for the preserved failed Daytona attempt, and $1.073147 for the
corrected Daytona result. The two Milestone 1 diagnostic reproductions used an
additional 43,840 tokens, but that temporary diagnostic did not meter price, so
the exact all-in dollar total is unknown and is higher than the reported
$2.991738. Daytona dollar billing was not exposed; only sandbox counts and
seconds are reported. The full suite was rerun after the first attempt exposed
the account-policy verifier bug; that invalid attempt remains recorded but is
not part of the decision. Within the corrected suite, no completed order was
replayed: its continuation ran only the one schedule position the cost gate had
not started.

## Spring Boot best effort

The Spring Boot fixture and black-box verifier are implemented, but the offline
gate stopped before model calls. The isolated Maven repository did not contain
the pinned `spring-boot-starter-parent:3.3.5`, so Maven correctly refused to
contact Central in offline mode. Spring Boot needs a pre-baked dependency cache
in the base snapshot before it is benchmarkable; spending model or sandbox
budget before that would not produce useful runtime evidence. That cache is a
direct prerequisite/input for the stack registry work tracked by #149; this
benchmark does not implement the registry or `EnvironmentBuilder`.
