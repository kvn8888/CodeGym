# opencode protocol spike

This standalone spike answers whether CodeGym can run opencode against a plain
OpenAI-compatible relay without placing a real model credential in the agent
sandbox. It records the exact HTTP exchange, exercises a real custom-tool call,
captures opencode's JSONL event stream, and samples the opencode process's open
network sockets.

The result is **GO-WITH-CAVEATS** for the generic OpenAI-compatible provider.
See [FINDINGS.md](FINDINGS.md) for the captured evidence and the separate native
Azure result.

Experiment 6 from the agentic-generation design is also complete. See
[ISOLATION.md](ISOLATION.md) for the session-independence, filesystem-reach,
permission, and SQLite-storage verdicts.

## Re-run

On macOS, the spike needs Go, `curl`, `unzip`, `npm`, GNU `timeout` (Homebrew
coreutils), `lsof`, and `jq` for inspecting the output.

```bash
cd spikes/opencode
./download-opencode.sh
./run-openai-compatible.sh
```

`download-opencode.sh` detects `uname -s -m`, downloads the pinned official
release archive, verifies its SHA-256, and extracts it beneath ignored `.bin/`.
It also preloads the version-matched custom-tool runtime into an ignored HOME
template. Those setup operations intentionally contact GitHub Releases and the
npm registry. The experiment itself copies that template under
`HOME=/tmp/opencode-spike`, copies the tool into a scratch Git workspace there,
and starts opencode with an empty inherited environment. It is designed to
contact only its localhost recorder and cannot discover the real repository's
`.env`, agent instructions, or Git metadata.

The primary run overwrites these evidence files:

- `transcript.jsonl`: all recorder requests, including headers and full bodies.
- `events.jsonl`: raw `opencode run --format json` stdout.
- `tool-invocations.jsonl`: arguments observed by the local custom tool.
- `exit-code.txt`: bounded opencode process exit status.
- `egress-lsof.txt`: best-effort socket samples for the opencode process.
- `opencode.stderr.log` and `recorder.log`: diagnostic logs.

To prove that a new isolated HOME does not need runtime egress, choose a path
that does not yet exist:

```bash
OPENCODE_SPIKE_HOME=/tmp/opencode-spike-fresh ./run-openai-compatible.sh
```

The lower-priority native Azure probe is separate and does not overwrite the
primary evidence:

```bash
./run-azure.sh
```

To reproduce the isolation experiment without downloading or contacting a real
model provider:

```bash
./run-isolation.sh
./analyze-isolation.sh
```

This creates a unique scratch HOME and canary tree under `/tmp`. It labels raw
requests and events beneath `isolation-evidence/`, runs fresh and continued
session controls, and forces read/bash attempts under permissive and deny-first
per-agent policies. Every opencode invocation has an external timeout.

## Recorder by itself

The recorder is its own dependency-free Go module:

```bash
cd spikes/opencode/recorder
go build ./...
./recorder --port 8089 --transcript ../transcript.jsonl --scripted-tool-call --responses-api
```

It accepts every method/path and records it before returning either a supported
OpenAI-compatible response or a JSON 404. `--scripted-tool-call` returns
`report_progress` on the first Chat Completions turn and checks the later
`role: "tool"` message against the expected tool-call ID. `--responses-api`
also enables a minimal `POST /v1/responses` response.

For deterministic permission experiments, `--tool-call-name`,
`--tool-call-arguments`, and `--tool-call-id` force an arbitrary tool call.
`--label` adds an experiment label to every transcript entry so several
sequential recorders can append to one unambiguous JSONL file.

## What it proved

With opencode 1.18.11, `@ai-sdk/openai-compatible` used streaming
`POST /v1/chat/completions` with `Authorization: Bearer`, executed the local
tool once, sent its result back with the correct `tool_call_id`, emitted a final
text event, and exited 0. No real credential was read or used.

The CLI emits `tool_use` only after the tool has completed, so that event alone
cannot drive a live “tool started” indicator. It also makes a separate model
call to generate the session title. The native Azure probe did not honor the
tested `useCompletionUrls` configuration and called a Responses endpoint.
