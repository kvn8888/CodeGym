# Daytona execution-layer spike

Validates [Daytona](https://www.daytona.io) sandboxes as CodeGym's code
execution layer, covering both planned tracks and the hybrid flow between
them. Standalone Go module so the backend's `go.mod` stays clean.

## Run

Secrets come from Doppler (`DAYTONA_API_KEY`, `DAYTONA_API_URL`):

```bash
cd spikes/daytona
doppler run -p codegym -c dev -- go run .            # phases A + B (~20s)
doppler run -p codegym -c dev -- go run . -promote   # + phase C (~50s)
```

Every sandbox and snapshot the spike creates is deleted before exit.

## What it proves

**Phase A — deterministic submission run.** Ephemeral sandbox with
`NetworkBlockAll`, Python solution + test file uploaded via the FS API,
tests executed, egress verified blocked from inside, sandbox deleted.
This is the LeetCode-style submission path and the AI problem-verification
path from `docs/ai-prompts/04-problem-verification.md`.

**Phase B — agent-scaffold run.** Network-enabled sandbox where the steps a
coding-agent harness would take all work: write project files, `npm install`
against the real registry, start an Express server, verify it from inside,
then reach it from outside through `GetPreviewLink` (token-authenticated
preview URL). This is the "unsupported runtime" path (Spring Boot etc.) and
the FS API here is what a Monaco file-explorer sidebar would proxy.

**Phase C — hybrid promotion (`-promote`).** The scaffolded sandbox is
promoted to a snapshot (`ExperimentalCreateSnapshot`), then a **new
network-blocked ephemeral sandbox boots from that snapshot** with
`node_modules` pre-baked and the server running with zero egress. Agent
builds an environment once; every later user gets the fast deterministic
path.

## Observed timings (2026-07-11, real API)

| Step | Time |
| --- | --- |
| Create sandbox (default snapshot) | 63–224 ms |
| Upload files / run tests | 90–320 ms |
| npm install express | ~3 s |
| Preview URL round-trip from outside | ~300 ms |
| Promote sandbox → snapshot | ~20 s |
| Boot network-blocked sandbox from promoted snapshot | ~4 s |

Notes:

- Default snapshot already ships Python 3 + Node 25 / npm 11.
- FS paths must be home-relative (absolute paths outside `$HOME` are
  rejected with 400) — relevant for the future `WorkspaceProxy` design.
- `Ephemeral: true` sandboxes still accept explicit `Delete`; the spike
  deletes eagerly rather than waiting for auto-delete-on-stop.
- Preview URLs require the `x-daytona-preview-token` header unless the
  sandbox is public.
