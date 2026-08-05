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
