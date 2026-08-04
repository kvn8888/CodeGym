# Agentic Problem & Runtime Generation

**Status:** design, not yet implemented
**Supersedes:** the fixed-harness portions of [`ai-prompts/02-problem-generation.md`](ai-prompts/02-problem-generation.md),
[`ai-prompts/03-test-generation.md`](ai-prompts/03-test-generation.md), and
[`ai-prompts/04-problem-verification.md`](ai-prompts/04-problem-verification.md)

## 1. Why this document exists

The agent-scaffolded runtime track was scoped in conversation and never written
down. It existed only as two interface-only skeletons
(`execution/environment_builder.go`, `execution/workspace_proxy.go`), the
validation spike in `spikes/daytona/`, and code comments. A board audit on
2026-08-03 found **zero** items for it, so the kanban could not report what was
left to build. This document is the durable record; board items are derived from
§10.

## 2. What we are changing, and what we are keeping

The demo path that shipped in PR #119 made three shortcuts that this design
removes:

| Shortcut | Where | Replaced by |
| --- | --- | --- |
| One hand-written problem seeded into the DB | `problems/seed.go`, `CODEGYM_SEED_DEMO` | Agent-generated problems only. Delete the seed. |
| Python hardcoded, harness built by string template | `generation/problem.go:369`, `:330-365` | Agent-authored harness per language/stack (§5, §6) |
| Equality-only grading | `generation/problem.go:357` | Acceptance predicate chosen per problem (§4) |

Kept deliberately:

- **Tests are the judge.** A user's submission is graded only by the test files.
  Any strategy that passes is correct. The reference solution is a
  *generation-time* artifact and never enters a user's run — this is already
  true and must stay true.
- **Server-side trust boundary.** Hidden tests, reference solution, and
  entrypoint live behind `json:"-"` in `problems.Definition` and are assembled
  server-side at submission time.
- **Deterministic outer workflow.** The user-visible pipeline is a fixed
  sequence of named steps with streamed progress. Only the *inside* of a step is
  agentic (§3).

## 3. Deterministic workflow, agentic steps

The workflow surface already exists and ships: `internal/workflow`,
`POST /api/v1/workflow-operations`, SSE at
`GET /api/v1/workflow-operations/{id}/events`, documented in
[`workflow-progress.md`](workflow-progress.md). It is currently driven by
deterministic code; here it is driven by agent tool-calls.

Steps for `Kind = "problem_generation"` (new kind; existing kinds are
`mcq_generation`, `memory_reflection`, `mcq_next_round`):

| step_id | Label shown to the user | Agent? | Network |
| --- | --- | --- | --- |
| `spec` | Designing your problem | yes | LLM only |
| `environment` | Setting up your environment | yes | **full** (curl, npm, mvn, go mod) |
| `tests` | Writing test cases | yes | LLM only |
| `reference` | Writing a reference solution | yes | LLM only |
| `verify` | Checking the problem is solvable | no | none |
| `promote` | Saving your practice environment | no | Daytona API only |

**The backend owns the six step transitions.** It emits `workflow.Event` before
and after each invocation, so the UI's progress is correct even if a model never
calls a tool. The agent's `report_progress(step_id, label, metadata)` tool adds
*optional finer-grained detail inside* a step ("installing dependencies",
"fixing build error 2 of 3"). Making required UI state depend on model
cooperation would give up the determinism this whole section exists to provide.

Steps are declared up front so the UI can render the full list greyed out and
fill it in — progress is deterministic even though step duration and internal
actions are not.

Failure of any step fails the operation with the step_id attached. `tests` and
`reference` run **in parallel** (§4).

## 4. Independent generation: how tests and implementation align

One agent writing the spec, the tests, and the reference self-certifies its own
misreadings: it can write a test asserting the wrong thing plus a reference that
agrees, and nothing catches it. The current repair loop makes this worse — the
same model adjudicates disagreements and may rewrite a *correct* test to match a
*wrong* solution.

Instead, three isolated agent calls sharing exactly one artifact — the spec:

```
                 ┌─► tests agent      (never sees the reference solution)
spec agent ──────┤
                 └─► reference agent  (never sees the tests)
                            │
                            ▼
                    execute reference against tests
                            │
              ┌─────────────┴─────────────┐
         agree │                          │ disagree
               ▼                          ▼
            publish              spec was ambiguous → repair the SPEC,
                                 regenerate both (bounded, max 2 rounds)
```

The spec is the alignment contract, so it must be precise enough that two agents
that never communicate produce compatible artifacts. It carries:

- problem statement (user-visible markdown)
- entrypoint and exact signature, with types
- the I/O contract for the chosen strategy (§6)
- **explicit ambiguity resolutions** — the load-bearing field. e.g. "empty input
  returns `[]`, never an error"; "if several pairs qualify, any is accepted";
  "float results compared to 1e-6"
- the acceptance predicate (below)

**Disagreement is diagnostic, not noise.** When the reference fails the tests,
the default response is to fix the *spec clause* that was ambiguous, not to
patch either artifact. This is the key behavioural change from today's repair
loop.

Optional strengthening, cheap: generate 2–3 references independently. If they
agree with each other and disagree with the tests, the tests are wrong — no
adjudicating model needed.

### Isolation is an OS boundary, not a config

Independence is only real if it is enforced. Measured 2026-08-03 (§11.6): agent
contexts do not bleed between fresh runs, but a **shared `HOME` leaks anyway** —
conversations persist to `~/.local/share/opencode/opencode.db` and a later run
read a prior run's prompt out of it. Tool-permission denies did block the
reference and HOME reads, but only by removing `bash` outright, which the
`environment` step cannot tolerate.

Therefore: **one Daytona sandbox per agent role**, with only the spec copied in.
Sandbox creation measured 63–224ms in `spikes/daytona/`, so the OS boundary
costs less than the config gymnastics it replaces — and it is verified once
rather than re-verified on every opencode upgrade. Separate unix users with
private filesystem modes are the minimum same-sandbox fallback. Separate
sessions and `--dir` are defense in depth, never the guarantee.

### Acceptance predicate

Grading is a predicate over `(input, output)`, not equality. The spec names one
of:

| predicate | use |
| --- | --- |
| `exact` | unique, canonically-representable answer |
| `set` / `multiset` | order-independent collections |
| `sorted` | order-normalized sequences |
| `float(eps)` | numeric tolerance |
| `checker` | agent-authored function `check(input, output) -> bool` |

Because the tests agent never sees the reference, it *cannot* pin an arbitrary
representative for a multi-answer problem and have it hold — independent
generation pushes naturally toward `checker`. §4 and this section reinforce each
other; neither works alone.

## 5. The judge must outlive the submission

Today the harness that emits the verdict runs **inside** the submission's own
process. When the submission dies — infinite loop, OOM, segfault — the reporter
dies with it, nothing is emitted, `submission/service.go:265` fails to parse, and
the user receives `Status: "error"` with no result, no message, and no output.
An accidental infinite loop, the most common beginner failure, currently
produces a blank error.

Required structure:

- The submission runs as a **child process**; a supervisor in the sandbox is the
  parent and is what reports the verdict.
- The verdict travels **out-of-band** from the program's output — a file
  (`~/work/.codegym/verdict.json`) or a dedicated fd, not a magic line on
  stdout. `CODEGYM_RESULT`-on-stdout is replaced.
- The supervisor applies a **per-case timeout** and a memory ceiling, and turns
  death into a reportable verdict: `timeout`, `out_of_memory`, `crashed` — each
  naming the case it died on.
- The program's stdout/stderr is captured **separately** and returned to the
  user (truncated, with a byte cap), so print-debugging works.

This is the seam that gets multiplied by every language and every strategy, so it
is built once, first, before language #2.

New submission statuses: `passed | failed | timeout | out_of_memory | crashed |
error`, where `error` means a platform fault, distinct from any user-code
outcome.

## 6. Test strategies beyond `unit`

`problems.TestConfig.Strategy` exists today, is written at `seed.go:110` and
`generation/problem.go:376`, and is **read nowhere**. Backend practice — the
target use case — is not `assert f(x) == y`; it is "start the server, POST
/orders, assert 201 and this body."

v1 of CodeGym had this modelled correctly (strategies `unit | http | output |
custom` behind a `TestStrategy` interface, with api-server and library-usage
problem types). v2 collapsed to function+unit for the demo. This design restores
it:

| strategy | shape of a case | example stack |
| --- | --- | --- |
| `unit` | args → predicate over return value | DSA in any language |
| `http` | request (method/path/headers/body) → assertions on status + body | Spring Boot, Go net/http, Express |
| `output` | stdin → assertions on stdout | CLI exercises |
| `custom` | agent-authored test command; supervisor reads the verdict file | anything else |

The supervisor contract (§5) is identical across strategies: run something,
produce `verdict.json`. Only the agent-authored test code differs.

## 7. Environments: agent builds once, everyone boots fast

Generation is not the hard part — a developer can get a problem statement and
skeleton from any coding agent on their own machine in a minute. What that does
*not* give them is the fourth practice session: `mvn`/`go mod download`/`npm
install` costs minutes every single sitting.

Spike Phase C already validated the answer: the agent pays setup cost **once**,
then practice boots a snapshot with dependencies pre-baked, network blocked,
in ~4s.

```
environment step (network ON)                practice runs (network OFF)
  agent scaffolds the stack                    boot snapshot  ~4s
  builds it, runs a smoke test        ──►      upload user files
  promote to snapshot  ~20s                    run supervisor
  register stack → snapshot name               delete sandbox
```

A stack (`spring-boot-3`, `go-1.25`, `express-4`) resolves to a snapshot name via
a **stack registry** — the durable form of the `execution.Language.Snapshot`
field, which exists and is empty today. Registry entries record: stack id,
snapshot name, base image, setup commands, build/test commands, created-at,
owner, and validation state. A stack is only usable for practice after its smoke
test passed under network-blocked boot.

`EnvironmentBuilder` (`Build`/`Promote`/`Destroy`) is the interface for this and
is currently a skeleton with no implementation.

### Registered Go practice snapshot

The first concrete language registry entry is `go`:

- snapshot: `codegym-go-1-25-4-v1`
- base: Daytona's default snapshot, promoted from a network-enabled sandbox
- toolchain: the official `go1.25.4.linux-{amd64|arm64}.tar.gz`, installed at
  `/usr/local/go` with `/usr/local/bin/go`
- build gate: `go version`, compile a trivial program, and execute it before
  promotion
- practice gate: boot the promoted snapshot with `NetworkBlockAll`, upload a
  fresh trivial program, then compile and execute it with no egress

Built and promoted on 2026-08-03 from the Daytona default Linux/amd64 image.
The pre-build probe confirmed that default image had no usable `go`; the final
network-blocked gate reported `go version go1.25.4 linux/amd64` and successfully
compiled and ran the fresh smoke program.

Go problem runs use a 1,024 MB supervised address-space ceiling. The 2026-08-03
acceptance run proved that 256 MB caused `go build` to fail before the harness
started; at 1,024 MB the same network-blocked snapshot passed solution, failure,
compile-error, timeout, checker, and exact-comparator paths. Compilation remains
inside the unchanged language-agnostic supervisor's wall-clock budget.

The builder is deliberately opt-in and re-runnable:

```bash
cd backend
CODEGYM_BUILD_GO_SNAPSHOT=1 doppler run -p codegym -c dev -- \
  go test ./internal/execution -run TestBuildGoSnapshot -count=1 -v
```

If the named snapshot already exists, that command validates it without
mutation. An intentional rebuild of the same version additionally requires
`CODEGYM_REPLACE_GO_SNAPSHOT=1`; the test deletes only that exact snapshot name
before rebuilding. The ordinary non-mutating acceptance gate is:

```bash
cd backend
doppler run -p codegym -c dev -- \
  go test ./internal/execution -run TestGoSnapshot -count=1 -v
```

## 8. opencode as the agent runtime

The agent runs **inside** the Daytona sandbox, not on the backend — it needs a
real filesystem and a real shell to scaffold, build, and iterate.

- Installed into the base snapshot at image-build time (`curl -fsSL
  https://opencode.ai/install | bash`), not per-run — per-run install adds
  network dependency and latency to every generation.
- **Model access goes through a CodeGym relay, not a key in the sandbox.** The
  backend exposes an OpenAI-compatible endpoint scoped to one workflow
  operation; the sandbox authenticates with a short-lived, single-operation
  token. The relay translates to Azure using the transport already implemented
  in `generation/openaicompat/adapter.go` — `CODEGYM_GENAI_AZURE_BASE_URL`,
  `_API_KEY`, `_MODEL`, `_API_VERSION` (`config.go:213-218`), with Meta
  (`CODEGYM_GENAI_META_*`) as fallback per `DefaultGenAIProviderOrder`.

  This one decision resolves four problems at once: the long-lived Azure key
  never enters a sandbox that is running agent-authored shell commands; the
  sandbox needs egress to exactly one host (§9); token/cost accounting and hard
  budget ceilings are enforced centrally rather than trusted to the agent
  runtime; and any Azure-vs-OpenAI wire-format mismatch is handled by code we
  already own instead of by a third-party provider shim.
- **Credentials must never enter a snapshot.** Even with the relay, verify a
  promoted image carries no token material, shell history, or agent state before
  registering the stack (canary-secret test, §10).
- Pin the installed version and verify a checksum rather than piping a mutable
  `latest` script into a production image build.
- Progress: opencode tool-call → orchestrator → `workflow.Event` → SSE → UI.
- Cost: agent runs are far more expensive than a single completion. `usage`
  service must record per-operation token and sandbox-second cost.

Feasibility of this section is being researched separately before implementation
(opencode's non-interactive/headless mode, custom OpenAI-compatible provider
config, tool-call surfacing, and whether it can run under a sandbox user with no
outbound network after setup).

## 9. Capability policy per step

Each workflow step gets the least capability it needs. This is policy, not
mechanism — the mechanism (`NetworkBlockAll`, ephemeral sandboxes) is already
validated.

| step | filesystem | network | secrets |
| --- | --- | --- | --- |
| `spec`, `tests`, `reference` | scratch only | CodeGym relay only (§8) | **none** |
| `environment` | full sandbox | **unrestricted** (registries, docs) | **none** |
| `verify` | full sandbox | **none** | none |
| practice run | `~/work` only | **none** | none |

The practice run is the only one a non-trusted user's code enters, and it is the
most locked down.

**Correction (2026-08-03):** an earlier draft of this table said "LLM endpoint
only" for the generation steps. That is not implementable as written — Daytona's
`NetworkBlockAll` is all-or-nothing, and per-domain egress allowlists are
tier-gated. Combined with the relay in §8, the generation steps need egress to
exactly one host (the CodeGym backend), and no step ever holds a model key.

## 10. Work breakdown

Ordered by dependency, not by value. Each is a board item.

1. **Out-of-process judge + verdict protocol** (§5). Supervisor, `verdict.json`,
   per-case timeout, memory ceiling, separated stdout, new statuses. Replaces
   `CODEGYM_RESULT` parsing. *Do this first — it is multiplied by every language
   and strategy added later.*
2. **Mandatory limits in the execution contract** (§9). `SubmitRunInput` gains a
   required `Limits{TimeoutSeconds, MemoryMB, NetworkMode}` so the per-problem
   values in `problems.Runtime` cannot be silently dropped, as they are today.
3. **Acceptance predicates** (§4) in the spec and the supervisor.
4. **Three-agent independent generation** (§4), replacing the single-model
   generate-and-repair loop. Adds workflow kind `problem_generation` and the
   `report_progress` tool bridge (§3).
5. **Stack registry + EnvironmentBuilder** (§7). Implements `Build`/`Promote`/
   `Destroy`; populates `Language.Snapshot`.
6. **Model relay + single-operation tokens** (§8). Prerequisite for any agent
   runtime; also where aggregate token/cost/wall-clock ceilings live.
7. **Agent runtime**, decided by benchmark (§8, §12). Build the purpose-built
   tool-calling loop on `openaicompat`, and spike opencode alongside it on the
   same three tasks (Spring Boot, Go, Express). Adopt opencode only if it is
   materially better at scaffold-and-repair.
8. **Strategies `http` / `output` / `custom`** (§6) — unlocks Spring Boot and Go
   backend practice, the primary target.
9. **Public/hidden case split + Run vs Submit + stdout surfacing** (§5, and
   `03-test-generation.md`'s existing `kind`/`hidden` fields, never implemented).
10. **Deletions:** `problems/seed.go`, `CODEGYM_SEED_DEMO`, the hardcoded python
   harness template in `generation/problem.go`, and `CODEGYM_RESULT` parsing once
   (1) lands.

## 11. Agent-runtime experiment gates

From the opencode feasibility research (2026-08-03). Run in order; each gates the
next. Nothing here bakes opencode into the production pipeline.

1. ~~**Protocol test, no real key.**~~ **DONE 2026-08-03** — `spikes/opencode/`,
   pinned v1.18.11, evidence in `transcript.jsonl` / `events.jsonl`.
   - **Against a plain OpenAI-compatible endpoint (the relay shape): PASS.**
     `POST /v1/chat/completions`, `Authorization: Bearer`, full tool round trip
     — custom `report_progress` tool executed and its result returned as a
     correctly-shaped `role:"tool"` message with a matching `tool_call_id`.
     Exit 0.
   - **Against the native `azure` provider: FAIL.** Emitted
     `POST /openai/deployments/responses` (the Responses API, with a malformed
     deployment path) *despite* `useCompletionUrls: true`. Auth style was right
     — it sent `Api-Key` — but the URL and API shape were wrong. Exit 1.
   - **This makes the §8 relay mandatory, not merely preferable.** It is now the
     only verified way to drive opencode against our Azure deployment.
   - Tokens are reported per step under `part.tokens`
     (`total/input/output/reasoning/cache`); `cost` is `0` for a custom
     provider, confirming we must apply our own pricing table at the relay.
   - Egress during the isolated run: localhost only. The disable flags hold.
2. **Headless run.** One `opencode run --format json --auto` in a disposable
   network-enabled sandbox under an external timeout. Capture exit code and
   every event type.
3. **Semantic failure.** Give it a build that must fail. Determine whether the
   process still exits 0 — it likely does, since exit status reflects runner
   failure, not task failure. This is why §3 requires a backend-verified result
   manifest rather than trusting an exit code.
4. **Progress plumbing.** *Partially answered by experiment 1:* the CLI stream
   emits `tool_use` **once, at tool completion** — there is no start event. So
   the JSONL stream cannot report "npm install is running" while it runs. §3's
   backend-owned coarse transitions are therefore load-bearing, not a nicety.
   Still open: whether `opencode serve` + SSE exposes tool starts, and whether
   Daytona can stream command stdout *before* completion — the Daytona spike only
   ever read completed results, so incremental streaming remains unproven.
5. **Egress audit.** Disable autoupdate / models fetch / LSP download / default
   plugins; allow only the relay host. Record all DNS and outbound attempts at
   startup and during a run.
6. ~~**Isolation.**~~ **DONE 2026-08-03** — `spikes/opencode/ISOLATION.md`,
   evidence under `spikes/opencode/isolation-evidence/`.
   - **Context does not bleed between fresh runs.** Each plain `opencode run`
     gets a distinct `sessionID`, and invocations B and C carried no trace of
     invocation A's canary. The **positive control passed**: re-running with
     `--continue` reused A's session id *and* carried its canary, proving the
     detection method works and the clean result is not a blind negative.
   - **A shared `HOME` is a leak channel regardless of sessions.** Sessions
     persist to `~/.local/share/opencode/opencode.db`, and a later invocation
     read invocation A's prompt straight out of it, recovering
     `CANARY_REFERENCE_9f3a2b`.
   - **Default `--auto` reaches everything:** the reference solution via both
     the `read` tool and `bash cat`, the HOME state file, and the session
     database.
   - **The locked-down config did refuse** the reference and HOME reads while
     still permitting the shared spec — but **only because `bash` was removed
     from the tool set entirely** ("Available tools: read"). That is the
     limitation that matters: the `environment` step *must* have bash (npm,
     mvn, go), so it can never be locked this way. Config-level isolation is
     therefore viable for `tests`/`reference` and structurally impossible for
     `environment`.
   - **Conclusion: opencode permissions are tool-dispatch enforcement, not an
     OS boundary.** Do not treat separate sessions, `--dir`, or separate
     same-user HOME paths as the load-bearing guarantee (see §4).
7. **Runaway.** Force repeated tool use and a long shell command. Verify step
   cap, backend deadline, process-tree kill, sandbox lifetime, and the relay's
   aggregate token ceiling.
8. **Canary secret + promotion.** Promote a sandbox that handled a synthetic
   secret; scan the booted snapshot's filesystem, logs, shell history, and agent
   state for it.
9. **Benchmark.** Same Spring Boot / Go / Express tasks through opencode and the
   purpose-built loop. Compare completion rate, repair iterations, tokens, wall
   time, image overhead, policy violations. This decides item 7 of §10.

## 12. Decisions (2026-08-03)

- **Problems are per-user; environments are shared.** Generated problems stay
  workspace-scoped (`problems.Visibility = workspace`). Runtimes/snapshots are
  cached and reused across users. Keep the `global` visibility value — user
  promotion of a good problem to a shared catalog is a plausible future feature
  and the field costs nothing to retain.
  - *Consequence:* a shared snapshot means one user's agent-built image runs
    other users' code. The §7 promotion gate (smoke test must pass under
    network-blocked boot) is therefore **mandatory, not optional**, and the
    installed-package manifest/lockfile should be recorded from day one.
- **Language order: Go first**, then *try* Java/Spring Boot and TypeScript.
  Go's fast toolchain keeps harness-seam iteration cheap; Spring Boot's build
  times would make getting the seam right slow.
- **The `03-test-generation.md` safety boundary is deliberately reversed.** The
  agent authors test code; CodeGym owns the supervisor that runs it and writes
  the verdict (§5). Recorded as a conscious decision rather than drift.
- **`environment` step budget: 5 build-fix iterations or 10 minutes wall-clock,
  whichever first.** Both must be observable (per-operation metrics), and both
  are expected to *decrease* as prompts and snapshots improve — tracked as a
  board item.
- **No migration burden.** The database holds essentially nothing beyond the
  seeded Python problem, so protocol changes need no data preservation. The
  `CODEGYM_RESULT` fallback parser is retained only because
  `generation/problem.go` still emits that protocol, not to protect data.
- **New stack builds are rate-limited per user.** First user to request an
  unbuilt stack triggers the build; everyone after reuses the snapshot. Part of
  the broader rate-limiting work already on the board.

### Generation loop (revised)

Supersedes the strict three-agent scheme in §4:

1. One agent writes a **highly detailed spec**.
2. Two agents implement it **in parallel** — one writes the code, one writes the
   tests.
3. On disagreement, the **coding agent may modify either its code or the
   tests**. Test context is injected into the coding agent so the iteration
   loop benefits from prompt-cache reuse.

**Recorded caveat.** Once the coding agent can see and edit the tests,
agreement between them stops being evidence of correctness — it only shows one
conformed to the other, and editing an assertion is cheaper for a model than
rethinking an algorithm. Two mitigations, *proposed and not yet accepted*:

- **Blind first, reconcile on disagreement.** Generate both isolated; if they
  agree on the first run, the oracle is intact and the problem ships. Only on
  disagreement is test context injected for reconciliation. Costs nothing in the
  agreeing case and puts caching exactly where the iteration loop is.
- **Log which side was edited and why.** "How often did it change the tests
  rather than the code" becomes a metric; a high rate indicts the spec
  generator.

## 13. Open questions

- **Blind-first reconciliation** (§12) — accept the mitigation or run the
  coding agent with test context from the start?
- **What the user sees when `environment` exhausts its budget.** The 5-iteration
  / 10-minute ceiling is decided; the failure UX is not.
- **Verification failure policy.** If code and tests never converge, fail the
  operation outright with a retry (recommended — never surface an unverified
  problem), or offer the partial problem?
- **Cost ceiling per generation.** The relay enforces token/dollar caps; the
  actual number is unset.
