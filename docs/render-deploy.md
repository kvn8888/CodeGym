# Deploying the backend to Render

The repo root includes a Render Blueprint (`render.yaml`) that codifies the
backend API service already running manually at
[codegym.onrender.com](https://codegym.onrender.com), so it can be
reproduced or re-created without clicking through the dashboard by hand.

## What the blueprint defines

- One `web` service, `codegym-backend`, using Render's native Go runtime
  (`runtime: go`), built from `backend/` (`rootDir: backend`).
- Build: `go build -o app ./cmd/server`; start: `./app`.
- `plan: free` (spins down after 15 min idle; cold start on next request).
  Upgrade to `starter` or higher for an always-on instance.
- Auto-deploy tracks the `codegym-v2` branch.
- `healthCheckPath: /health`, used by Render to gate deploy promotion.
- `CODEGYM_HOST=0.0.0.0` (required — the app's default, `127.0.0.1`, is not
  reachable from Render's edge/proxy layer).
- Port: not set explicitly. The app reads `CODEGYM_PORT` and falls back to
  Render's injected `PORT`, so Render's own port assignment is honored.
- `CODEGYM_AUTH_MODE=auth0` and non-secret GenAI values
  (`CODEGYM_GENAI_BASE_URL`, `CODEGYM_GENAI_MODEL`) are set with defaults.
- All secrets are declared with `sync: false`, meaning Render will prompt
  for a value on first apply and will not overwrite it on later syncs.

## How to apply it

1. Push this branch (or merge it) so `render.yaml` is on the branch Render
   will build from.
2. In the Render Dashboard: **New** -> **Blueprint** -> connect the
   `codegym` GitHub repo -> Render detects `render.yaml` at the repo root.
3. Render will propose creating a *new* service, `codegym-backend`. Review
   the plan, then click **Apply**.
4. Fill in the `sync: false` env vars when prompted (or after creation, in
   the service's **Environment** tab — see list below).
5. Trigger a deploy (or wait for the first automatic one).

## Env vars you must set by hand (`sync: false`)

| Key | Where it comes from |
|---|---|
| `NEON_CONNECTION_STRING` | Neon Postgres connection string (already set on the existing manual service — copy it over). |
| `CODEGYM_AUTH0_DOMAIN` | Auth0 tenant domain, e.g. `your-tenant.us.auth0.com`. |
| `CODEGYM_AUTH0_AUDIENCE` | Auth0 API identifier/audience. |
| `CODEGYM_GENAI_API_KEY` | Gemini API key from Google AI Studio. |

Also review/replace the placeholder value for `CODEGYM_CORS_ALLOWED_ORIGINS`
(defaults to a placeholder Vercel URL) with the real frontend origin(s),
comma-separated.

The default GenAI pairing uses Gemini's native Interactions API:
`CODEGYM_GENAI_BASE_URL=https://generativelanguage.googleapis.com/v1beta` and
`CODEGYM_GENAI_MODEL=gemini-3.5-flash`. To use Vercel AI Gateway instead, set
`CODEGYM_GENAI_BASE_URL=https://ai-gateway.vercel.sh/v1` and use a
vendor-prefixed model slug such as `google/gemini-2.5-flash`.

Optional, not set in the blueprint (leave unset unless needed):
`CODEGYM_MEMORY_WORKER_DISABLED` (`true` to disable the background memory
worker) and `CODEGYM_MEMORY_WORKER_INTERVAL` (Go duration, e.g. `1h`).

## Verifying the deploy

```sh
curl https://codegym-backend.onrender.com/health   # liveness only
curl https://codegym-backend.onrender.com/ready    # liveness + DB check
```

Both should return a 2xx response. `/ready` will fail if
`NEON_CONNECTION_STRING` is missing or the database is unreachable.

## Note on the existing manual service

The service already running at `codegym.onrender.com` was created by hand
in the dashboard and is **not** retroactively managed by this blueprint —
Render does not adopt pre-existing services into a Blueprint automatically.
You have two options:

- **Keep it as-is**: leave the manual service running unmanaged, and use
  this blueprint only for new environments (e.g. a staging service).
- **Adopt the blueprint**: delete/rename the manual service and let the
  blueprint create `codegym-backend` fresh, then repoint DNS/frontend
  config at the new service URL if it differs.
