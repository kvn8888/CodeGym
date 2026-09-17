# Deploying the backend to Render

Production backend: [codegym.onrender.com](https://codegym.onrender.com).

Secrets come from **Doppler only**. Render holds a single `DOPPLER_TOKEN`
service token (project `codegym`, config `prd`) and injects everything else
via `doppler run` at process start. Do not paste Neon, Auth0, or GenAI secrets
into the Render dashboard.

See also [secrets-and-local-env.md](./secrets-and-local-env.md).

## Live service shape

The production web service is already configured as:

| Setting | Value |
| --- | --- |
| Root directory | `backend` |
| Branch | `codegym-v2` (auto-deploy on commit) |
| Plan | `starter` (always-on; free plan cold-starts after idle) |
| Build | Install Doppler CLI into `./bin`, then `go build -o ./bin/codegym ./cmd/server` |
| Start | `./bin/doppler run -- env CODEGYM_HOST=0.0.0.0 CODEGYM_PORT=$PORT ./bin/codegym` |
| Env on Render | **only** `DOPPLER_TOKEN` |

The backend module pins Go `1.25.4`; Render's native Go build reads that
version from `backend/go.mod`.

`CODEGYM_HOST=0.0.0.0` is required so the process is reachable from Render's
proxy. Port comes from Render's injected `PORT` (mapped to `CODEGYM_PORT`).

App secrets (`NEON_CONNECTION_STRING`, Auth0, GenAI, CORS, memory worker,
`DAYTONA_API_KEY`, `DAYTONA_API_URL`, etc.) and the demo seed flag
(`CODEGYM_SEED_DEMO=true`) live in Doppler config `prd` — edit them there, then
redeploy/restart Render.

## Blueprint (`render.yaml`)

The repo-root [render.yaml](../render.yaml) codifies the same Doppler-only
shape for new environments (or Blueprint re-creates). It is **not**
automatically bound to the existing dashboard service; Render does not adopt
pre-existing services into a Blueprint.

### Apply a new Blueprint service

1. Ensure `render.yaml` is on the branch you will deploy from.
2. Render Dashboard → **New** → **Blueprint** → connect the CodeGym repo.
3. Review the plan, then **Apply**.
4. When prompted, set `DOPPLER_TOKEN` to a **service token** for
   `codegym` / `prd` (Doppler → Access → Service Tokens).
5. Wait for the first deploy.

### Env vars on Render

| Key | Required | Notes |
| --- | --- | --- |
| `DOPPLER_TOKEN` | Yes | Service token for `codegym` / `prd` only. |

Do **not** also set `NEON_CONNECTION_STRING`, Auth0, or GenAI keys on Render
when using Doppler — that duplicates secrets and drifts from the source of
truth.

## Rotate `DOPPLER_TOKEN`

1. Create a new service token in Doppler (`prd` → Access → Service Tokens).
2. Update `DOPPLER_TOKEN` on the Render service Environment tab.
3. Redeploy or restart the service.
4. Revoke the old token in Doppler.

## Verifying the deploy

```sh
curl https://codegym.onrender.com/health   # liveness
curl https://codegym.onrender.com/ready    # liveness + DB (postgres mode when Neon is set)
```

Both should return 2xx. `/ready` fails if Doppler did not supply a working
database URL or Neon is unreachable.

For a generated-DSA rollout, verify staging before production: create a coding
practice through intake, confirm the persisted problem is visible only in the
current workspace, run its server-controlled hidden tests in Daytona, reload
the session to restore the draft and last result, and confirm compact coding
events update memory without containing source code or hidden cases. Repeat the
same authenticated smoke against production after promoting the identical
reviewed commit.

For a conversational rollout, also start an Interview in each environment,
send and stream at least one turn, reload to prove transcript resume, exit and
resume from History, then finish it. Confirm the completion event contains only
coarse strengths/growth edges, topic, mode, turn count, and duration. Open the
floating coach on both a Marathon and coding workspace and confirm it uses the
active session context without sending source files or hidden tests. Finally,
verify staging transcripts/events never appear in production and vice versa.

## Preprod refresh automation (staging)

CodeGym Staging points at a Neon preprod child branch (via Doppler config
`stg`), while production uses the parent branch. A Render Cron Job,
`codegym-preprod-refresh` (see `render.yaml`), keeps staging faithful to
production by running
[`backend/scripts/neon_preprod_refresh.sh`](../backend/scripts/neon_preprod_refresh.sh)
once daily. The script calls Neon's "Reset from parent" API for the preprod
branch, polls Neon until the branch reports `ready`, then polls staging
`/ready` until healthy.

### Required Doppler `stg` variables

Set these in Doppler (`codegym` / `stg`); never in Render, Vercel, or `prd`:

| Key | Purpose |
| --- | --- |
| `NEON_API_KEY` | Neon control-plane key with branch-reset scope. Separate from `NEON_CONNECTION_STRING`. |
| `NEON_PROJECT_ID` | Neon project id. |
| `NEON_PREPROD_BRANCH_ID` | Preprod child branch id (`br-…`). The branch that gets overwritten. |
| `NEON_PROD_BRANCH_ID` | Production parent branch id (`br-…`). Sent as `source_branch_id`. |
| `CODEGYM_STAGING_BASE_URL` | Staging origin polled for `/ready` (defaults to `https://codegym-staging.onrender.com`). |
| `NEON_API_BASE_URL` | Optional override; defaults to `https://console.neon.tech/api/v2`. |

The script refuses to run when any required variable is missing or when the
preprod and production branch ids are equal. Render holds only the
`DOPPLER_TOKEN` service token for `codegym` / `stg` on the cron job.

### Manual rerun

Trigger an out-of-schedule run from the Render Dashboard: open the
`codegym-preprod-refresh` cron job → **Runs** → **Trigger Run**. To rehearse
locally without touching Neon:

```sh
cd backend
NEON_API_KEY=dummy NEON_PROJECT_ID=dummy NEON_PREPROD_BRANCH_ID=br-preprod-1 \
  NEON_PROD_BRANCH_ID=br-prod-1 DRY_RUN=true ./scripts/neon_preprod_refresh.sh
```

`DRY_RUN=true` validates configuration and prints the planned action only.
For a real manual run, inject `stg` secrets and omit `DRY_RUN`:

```sh
cd backend
doppler run -p codegym -c stg -- ./scripts/neon_preprod_refresh.sh
```

To validate the actual Doppler `stg` configuration without calling Neon:

```sh
cd backend
DRY_RUN=true doppler run -p codegym -c stg -- ./scripts/neon_preprod_refresh.sh
```

### Verification

```sh
curl https://codegym-staging.onrender.com/ready
```

Expect 2xx with Postgres mode once the reset settles. The script already
gates on this check and exits non-zero if staging does not become healthy
within its timeout.

### Expected interruption

A reset is a complete overwrite of the preprod branch: existing staging
connections are briefly interrupted (connection details do not change), and
any staging-local data written since the last refresh is discarded. Active
staging sessions may need a reload/retry around the run window. Production
is never written to by this job.

### Where success/failure is visible

Render Cron run history is our record of the last successful refresh: a
successful run means the Neon reset completed, the branch became ready, and
staging `/ready` passed. Render captures the script's stdout/stderr per run
on the cron job's **Runs** page. Exit `0` means all three phases succeeded;
any non-zero exit marks the run failed with an `ERROR:` line naming the
phase (restore request, branch poll timeout, or `/ready` poll timeout).
Render Runs/logs plus Neon's branch "last reset" timestamp are the
operational audit trail. There is intentionally no in-database marker (a
preprod row would be wiped by the next reset), and no new table was created
for this.

### Rollback / failure recovery

Reset-from-parent leaves no backup branch, so there is nothing to roll
back to — recovery is a re-run once the underlying cause is fixed:

- **Neon API error in the logs** (e.g. preprod has its own child branches,
  or the parent was restored from a snapshot within the last ~24h, which
  blocks child resets): resolve in the Neon Console, then **Trigger Run**.
- **Staging `/ready` timeout**: check the staging web service logs and
  Doppler `stg` database wiring; the branch itself was already reset, so
  re-running the job is safe (resets are idempotent toward the same
  parent head).
- **Wrong-branch config**: the job only ever targets
  `NEON_PREPROD_BRANCH_ID`. If the ids were misconfigured, fix them in
  Doppler `stg` first. This automation never deletes branches; the old
  staging branch is left untouched.

## Existing production service

The live service at `codegym.onrender.com` was created in the dashboard and is
already on the Doppler-only build/start commands above. Use the blueprint for
new environments, or only after deliberately replacing the manual service and
repointing the Vercel `/api` rewrite destination if the hostname changes.
