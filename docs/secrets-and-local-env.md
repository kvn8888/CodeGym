# Secrets and Local Environment

CodeGym uses **Doppler** as the only place to edit secrets. Render and Vercel
each hold a single `DOPPLER_TOKEN` service token and fetch the rest at
build/runtime via `doppler run`. Do not paste app secrets into the Render or
Vercel dashboards.

## First-time Doppler setup (local)

From the repo root:

```bash
doppler login
doppler setup --project codegym --config dev --no-interactive
```

The setup command stores the `codegym/dev` binding in your local Doppler CLI
config. That file is machine-specific and should not be committed.

## Config map

| Doppler config | Purpose | Deploy target |
| --- | --- | --- |
| `dev` / `stg` / `prd` | Backend + shared server secrets | Render (`prd` via service token) |
| `dev_frontend` / `stg_frontend` / `prd_frontend` | Public `VITE_*` build-time config only | Vercel (`prd_frontend` via service token) |

Never sync or inject `prd` into Vercel — that would expose Neon and GenAI
credentials to the browser build.

## Local development

### Backend

```bash
cd backend
doppler run -p codegym -c dev -- go run ./cmd/server
```

Default address: `127.0.0.1:8080`. The backend reads `NEON_CONNECTION_STRING`
first, then `DATABASE_URL`. Without a database URL it uses in-memory stores.

Neon-backed integration test:

```bash
cd backend
doppler run -p codegym -c dev -- go test ./internal/integration -run TestNeonIdentityAndMemoryBootstrap -count=1
```

### Frontend

```bash
cd frontend
doppler run -p codegym -c dev_frontend -- npm run dev
```

Vite proxies `/api` to `http://localhost:8080`.

## Backend / shared secrets (`dev` · `stg` · `prd`)

| Name | Required | Purpose |
| --- | --- | --- |
| `NEON_CONNECTION_STRING` | Yes for durable memory | Neon/Postgres connection string. |
| `CODEGYM_AUTH_MODE` | Production | Auth mode (`auth0` or dev). |
| `CODEGYM_AUTH0_DOMAIN` | When Auth0 | Auth0 tenant domain. |
| `CODEGYM_AUTH0_AUDIENCE` | When Auth0 | Auth0 API audience. |
| `CODEGYM_CORS_ALLOWED_ORIGINS` | Production | Allowed browser origins (Vercel URL). |
| `CODEGYM_DEV_AUTH_TOKEN` | Optional | Static bearer token for protected routes. |
| `CODEGYM_DEV_TENANT_ID` | Optional | Default personal tenant for static-token auth. |
| `DAYTONA_API_KEY` / `DAYTONA_API_URL` | Spikes | Daytona sandbox access. |
| `VERCEL_API_GATEWAY` | Optional | Vercel AI Gateway key (not used by the multi-provider router today). |

### GenAI multi-provider registry

The backend registers every provider that has an API key and tries them in
priority order (default **meta → azure → gemini**). Failover hops on
provider/transport errors only — not on MCQ schema validate/repair.

| Name | Required for that hop | Purpose |
| --- | --- | --- |
| `CODEGYM_GENAI_PROVIDER_ORDER` | Optional | Comma list, default `meta,azure,gemini`. |
| `META_MUSE_SPARK_API` or `CODEGYM_GENAI_META_API_KEY` | Meta hop | Meta Model API key (Muse Spark). |
| `CODEGYM_GENAI_META_BASE_URL` | Optional | Default `https://api.meta.ai/v1`. |
| `CODEGYM_GENAI_META_MODEL` | Optional | Default `muse-spark-1.1`. |
| `CODEGYM_GENAI_META_MAX_TOKENS` | Optional | Default `4096` (reasoning budget). |
| `CODEGYM_GENAI_AZURE_API_KEY` | Azure hop | Azure OpenAI key (`api-key` header). |
| `CODEGYM_GENAI_AZURE_BASE_URL` | Azure hop | `https://{resource}.openai.azure.com/openai/deployments/{deployment}`. |
| `CODEGYM_GENAI_AZURE_MODEL` | Optional | Deployment name; defaults to last URL path segment. |
| `CODEGYM_GENAI_AZURE_API_VERSION` | Optional | Default `2024-10-21-preview`. |
| `CODEGYM_GEMINI_API_KEY` | Gemini hop | Last-resort Gemini key (omit from order to park). |
| `CODEGYM_GEMINI_BASE_URL` | Optional | Gemini OpenAI-compat base URL. |
| `CODEGYM_GEMINI_MODEL` | Optional | Gemini model slug (e.g. `gemini-flash-latest`). |

Providers without a key are skipped. Example: Meta + Gemini only still works
and tries Meta first.

## Frontend secrets (`*_frontend`)

| Name | Purpose |
| --- | --- |
| `VITE_AUTH0_DOMAIN` | Auth0 SPA domain (mirrors `CODEGYM_AUTH0_DOMAIN`). |
| `VITE_AUTH0_AUDIENCE` | Auth0 SPA audience (mirrors `CODEGYM_AUTH0_AUDIENCE`). |
| `VITE_AUTH0_CLIENT_ID` | Auth0 SPA client id — paste from the Auth0 dashboard. |
| `VITE_API_BASE_URL` | Optional API origin; leave empty for same-origin / rewrites. |
| `VITE_APP_ORIGIN` | Canonical frontend origin (Vercel production URL). |

Keep browser-visible values prefixed with `VITE_`.

## Production: Doppler-only deploys

Full runbooks: [render-deploy.md](./render-deploy.md) and
[vercel-deploy.md](./vercel-deploy.md). Blueprint: [render.yaml](../render.yaml).

### Render (backend)

- Service env: **only** `DOPPLER_TOKEN` (service token for `codegym` / `prd`).
- Build installs the Doppler CLI into `./bin`, then compiles the Go binary.
- Start: `./bin/doppler run -- env CODEGYM_HOST=0.0.0.0 CODEGYM_PORT=$PORT ./bin/codegym`.
- Live URL: `https://codegym.onrender.com`.

Rotate the token in Doppler (`prd` → Access → Service Tokens), then update
`DOPPLER_TOKEN` on Render.

### Vercel (frontend)

- Project env: **only** `DOPPLER_TOKEN` (service token for `codegym` / `prd_frontend`)
  on Production and Preview.
- Build command: `curl -Ls https://cli.doppler.com/install.sh | sh && doppler run -- npm run build`
  (also recorded in `frontend/vercel.json`).
- `frontend/vercel.json` rewrites `/api/*` → `https://codegym.onrender.com/api/*`
  and falls back SPA routes to `index.html`. Leave `VITE_API_BASE_URL` empty so
  the browser uses same-origin `/api/v1`.

Rotate the token in Doppler (`prd_frontend` → Access → Service Tokens), then
update `DOPPLER_TOKEN` on Vercel.

## Updating a secret

1. Change it in Doppler (dashboard or `doppler secrets set`).
2. **Backend:** redeploy or restart the Render service so `doppler run` picks up
   the new values.
3. **Frontend:** redeploy on Vercel so Vite rebuilds with the new `VITE_*` values.
4. Do not also edit the same secret in Render/Vercel.
