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

`CODEGYM_HOST=0.0.0.0` is required so the process is reachable from Render's
proxy. Port comes from Render's injected `PORT` (mapped to `CODEGYM_PORT`).

App secrets (`NEON_CONNECTION_STRING`, Auth0, GenAI, CORS, memory worker, etc.)
live in Doppler config `prd` — edit them there, then redeploy/restart Render.

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

## Existing production service

The live service at `codegym.onrender.com` was created in the dashboard and is
already on the Doppler-only build/start commands above. Use the blueprint for
new environments, or only after deliberately replacing the manual service and
repointing the Vercel `/api` rewrite destination if the hostname changes.
