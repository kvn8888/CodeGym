# opencode isolation experiment

Run date: 2026-08-03. Host: Darwin/arm64. Runtime: pinned opencode v1.18.11.
The run used a unique HOME and scratch tree under `/tmp`, a dummy API key, and
the localhost recorder only. Every `opencode run` was wrapped in GNU `timeout`.

## Executive summary

**Default session independence: PASS. Default filesystem isolation: FAIL.
Locked tool policy: PASS for the tested calls, but it is not an OS boundary.**

- Three plain `opencode run` invocations created three session IDs. Neither the
  neutral B request nor the history-seeking C request contained A's reference
  canary. opencode did not auto-resume.
- The positive control used `--continue`, reused one session ID, and put the
  prior canary-bearing user message plus assistant answer into the next request.
  The recorder can therefore detect context carryover when it exists.
- A default-ish `--auto` run could read the reference file, override the shell
  working directory and `cat` it, read the isolated HOME, and query an earlier
  conversation from opencode's SQLite database.
- A per-agent deny-first policy kept the shared spec readable, refused external
  file reads, and removed `bash` from the offered tools. That is useful
  defense-in-depth, but it only mediates opencode tool dispatch. It does not
  change Unix ownership, modes, mounts, or process credentials.

The load-bearing recommendation for §4 is **one Daytona sandbox per role**, with
a distinct HOME and working directory in each sandbox and only the spec copied
to the tests and reference sandboxes. Separate Unix users with filesystem modes
are the minimum acceptable fallback if all roles must share one sandbox.

Raw evidence is under [`isolation-evidence/`](isolation-evidence/). The compact
derived views used below are
[`session-analysis.tsv`](isolation-evidence/session-analysis.tsv),
[`event-session-ids.tsv`](isolation-evidence/event-session-ids.tsv), and
[`reach-results.tsv`](isolation-evidence/reach-results.tsv).

## Method and scope

The recorder was extended to label every request and to force an arbitrary
single tool call with caller-provided JSON arguments. It returned tool calls to
the real opencode process; opencode, not the recorder, decided whether to run or
reject each tool. No model was needed.

The locked configuration follows the pinned release's
[permission documentation](https://github.com/anomalyco/opencode/blob/v1.18.11/packages/web/src/content/docs/permissions.mdx)
and [agent documentation](https://github.com/anomalyco/opencode/blob/v1.18.11/packages/web/src/content/docs/agents.mdx):

- default deny via `"*": "deny"`
- explicit denies for `bash`, edits, glob, grep, task, webfetch, and websearch
- `read` denied by default, with only the tests file and shared spec allowed
- `external_directory` denied by default, with only the shared spec directory
  allowed
- `--dir` set to the tests role's own directory

This experiment tests accidental request-context carryover and casual
filesystem reach. It is not a malicious-code or opencode-exploit assessment.

## Milestone 1 — canary layout

The final run used `/tmp/opencode-isolation-62722`. The four one-line matches in
[`canaries.txt`](isolation-evidence/canaries.txt) were:

```text
1:CANARY_REFERENCE_9f3a2b
1:CANARY_TESTS_7c1d4e
1:CANARY_SPEC_4b8e01
1:CANARY_HOME_2d5f6a
```

The corresponding files were `tree/reference/solution.py`,
`tree/tests/test_solution.py`, `tree/spec/spec.md`, and
`home/.local/state/opencode/isolation-canary.txt`.

## Milestone 2 — session independence

The first three invocations were sequential, used the same HOME and working
directory, and omitted `--continue` and `--session`:

| Invocation | Event-stream session ID | Reference canary in any request body? |
| --- | --- | --- |
| A fresh | `ses_0369a7035ffeZsLujQVhskjTIx` | yes — supplied in A's prompt |
| B fresh | `ses_0369a67b6ffepJhL7u386LcYfp` | **no** |
| C fresh | `ses_0369a6165ffemanEZeKKH3ZsgO` | **no** |

B's agent request contained only:

```text
user: "Invocation B is neutral and contains no experiment canary."
```

C explicitly asked for history but its agent request still contained only:

```text
user: "What was discussed previously? Answer only from this conversation."
```

Both also made the normal title request. Neither B nor C contained any of the
four canary strings. The body scan also found no `ses_...` string in any of the
five session-test request groups. The system prompt did name current state:

```text
Working directory: /private/tmp/opencode-isolation-62722/session-workspace
Workspace root folder: /private/tmp/opencode-isolation-62722/session-workspace
Is directory a git repo: yes
```

That is current environment metadata, not prior-session content or a prior
session ID. No prior file contents appeared in B or C.

### Positive control

A fresh control seed was followed immediately by B with `--continue`. Both
event groups used `ses_0369a5adbfferGXNMMaNWX4jdp`, and B's request included:

```text
user: "Positive-control seed contains CANARY_REFERENCE_9f3a2b."
assistant: DONE — the work is done.
user: "Invocation B is neutral and contains no experiment canary."
```

Thus plain `opencode run` starts fresh by default, while `--continue` is a
directly observable context-carryover switch.

## Milestone 3 — filesystem reach

`Executed` means opencode ran the tool. `Refused` means its permission/tool
layer stopped the call. The output excerpts come from the raw `tool_use` events.

| Attempt | Permissive `--auto` | Locked per-agent policy |
| --- | --- | --- |
| Read shared spec (control) | **Executed:** `CANARY_SPEC_4b8e01` | **Executed:** `CANARY_SPEC_4b8e01` |
| `read` reference solution | **Executed:** `CANARY_REFERENCE_9f3a2b` | **Refused:** `rule which prevents you from using this specific tool call` |
| `bash`: `cat reference/solution.py` | **Executed**, shell exit 0: `CANARY_REFERENCE_9f3a2b` | **Refused:** `Model tried to call unavailable tool ... Available tools: read.` |
| `bash`: `grep -r CANARY_REFERENCE_9f3a2b /tmp` | **Executed**, shell exit 1: `(no output)` | **Refused:** bash unavailable |
| `read` HOME state file | **Executed:** `CANARY_HOME_2d5f6a` | **Refused:** explicit external-directory deny |
| `bash`: query session SQLite | **Executed**, shell exit 0: `"Invocation A contains CANARY_REFERENCE_9f3a2b."` | **Refused:** bash unavailable |

The broad grep deserves a precise qualification. opencode executed it; no
permission boundary stopped it. On this macOS host `/tmp` is a symlink to
`/private/tmp`, and BSD grep did not traverse that symlink when `/tmp` itself
was the recursive operand, so the command exited 1 without a match. Directly
targeting the scratch root did find both the reference file and SQLite
database, but that host-side diagnostic was not substituted for the requested
forced command.

Every opencode process in this table exited 0, including permission errors and
the shell command that exited 1. As in the protocol spike, process success is
not a semantic-success signal.

### Is this a real filesystem boundary?

**No. opencode permissions are a tool-dispatch policy, not a filesystem
boundary.** The refusals are real enforcement inside opencode: the denied read
did not execute, and a whole-tool bash deny removed bash from the advertised
tool set. But the opencode process still ran as the same Unix user that could
read every canary. No owner, mode, credential, namespace, or mount changed.

In the permissive configuration, bash remained available and walked around the
role's `--dir` simply by supplying another `workdir`; that boundary was wholly
unenforced. In the locked configuration, bash was not available in any form in
the tested request (`Available tools: read`). If any shell/custom/plugin/MCP
tool with filesystem access is later permitted, opencode configuration alone
must be treated as unenforced unless that tool is exhaustively constrained.

## Milestone 4 — session storage on disk

`opencode debug paths` identified the data directory as:

```text
/tmp/opencode-isolation-62722/home/.local/share/opencode
```

The pinned v1.18.11 build stored sessions in
`~/.local/share/opencode/opencode.db`, identified by `file` as SQLite 3.x. Its
tables included `session`, `message`, `part`, `session_input`, and
`session_message`.

Immediately after A finished, a SQLite query over `part.data` recovered:

```json
{"session_id":"ses_0369a7035ffeZsLujQVhskjTIx","text":"\"Invocation A contains CANARY_REFERENCE_9f3a2b.\""}
```

An independent binary scan also matched `opencode.db`. Later, a fresh
permissive invocation queried that same database through bash and received:

```text
"Invocation A contains CANARY_REFERENCE_9f3a2b."
```

Therefore a shared HOME is a leak channel even though the logical sessions are
fresh and their model requests do not auto-include one another. Separate
sessions solve default context carryover; they do not solve at-rest reach.

## Milestone 5 — required isolation level

Ranking is strongest/most suitable first:

| Rank | Option | Verdict for §4 |
| --- | --- | --- |
| 1 | **4. Separate Daytona sandbox per role** | **Recommended.** Separate process/filesystem/HOME by construction; copy only the spec into tests and reference. Sandbox creation was previously measured at 63–224 ms in [`spikes/daytona/README.md`](../daytona/README.md), so three creates are unlikely to dominate model latency. |
| 2 | **3. Same sandbox, separate Unix users + filesystem permissions** | Minimum acceptable enforced boundary. Use private homes and role directories, deny cross-user traversal, and keep the orchestrator outside those accounts. More operationally fragile than separate sandboxes; root/shared writable paths remain sensitive. |
| 3 | **2. Same sandbox, separate HOME + working directories** | Good default-hygiene layer. Prevents shared SQLite state and accidental relative-file discovery, but the same Unix user or any permitted shell can read sibling paths. Not sufficient as the load-bearing guarantee. |
| 4 | **1. Same sandbox, same HOME, separate sessions** | Cheapest and fresh by default, but fails enforcement: shared SQLite and ordinary filesystem reads expose other roles. Reject for §4. |

For option 4, the backend should create three sandboxes or create the tests and
reference sandboxes only after the spec exists, upload the spec separately to
each, use per-role operation tokens, extract each role's output to backend-owned
storage, and never mount one role's artifacts into another role's sandbox.
Keep the deny-first opencode policy as defense-in-depth inside each sandbox.

### Residual channels

No option above prevents the backend, relay, or model provider from mixing
contexts if it sends the wrong messages, reuses a provider-side conversation,
or exposes shared logs/traces/object storage. A malformed spec can also contain
reference- or test-derived material before fan-out. Separate sandboxes do not
fix those control-plane mistakes; operation-scoped relay tokens, stateless
requests, artifact routing checks, log redaction, and spec validation remain
required.

UNVERIFIED in this experiment: actual Unix-user isolation, actual three-way
Daytona isolation, hostile plugins/custom tools, opencode vulnerabilities, and
provider-side retention or conversation behavior.

## Reproduce

The pinned binary must already exist under `.bin/`; the script does not download
it.

```bash
cd spikes/opencode
./run-isolation.sh
./analyze-isolation.sh
```

`run-isolation.sh` creates a fresh `/tmp/opencode-isolation-<pid>` root, rebuilds
the localhost recorder, runs every opencode call under an external timeout, and
replaces only the reproducible files in `isolation-evidence/`.
