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

## Environment identity and isolation matrix

Every backend resolves one canonical environment: `dev`, `stg`, or `prd`.
Set `CODEGYM_ENVIRONMENT` explicitly on deployed services. When it is absent,
the backend uses `DOPPLER_CONFIG`, then `DOPPLER_ENVIRONMENT`; when none are
present it defaults to `dev`. Any non-empty value outside the three canonical
names prevents startup. The explicit CodeGym value always wins. Startup logs
the resolved name and `GET /ready` returns it as `data.environment`.

The following resources must be distinct in every environment:

| Resource or setting | Required separation | Why |
| --- | --- | --- |
| `CODEGYM_ENVIRONMENT` | `dev`, `stg`, and `prd` identify their matching service only. | It is the signed and labelled isolation identity. A wrong value deliberately identifies the service as another environment. |
| Neon database and `NEON_CONNECTION_STRING` | A separate Neon branch/database credential for each environment. | Database writes have no environment claim beyond the selected database. A shared connection string lets one environment read and write another environment's data. |
| Daytona account and `DAYTONA_API_KEY` | A separate Daytona credential/account for each environment. | Environment labels prevent cross-environment sweeping, but a shared account still shares quota, billing, provider visibility, and other account-level failure modes. |
| `CODEGYM_RELAY_TOKEN_SECRET` | A separate signing secret for each environment. | Signed environment claims now reject cross-environment tokens, but separate keys retain cryptographic separation and independent rotation/revocation boundaries. |
| Doppler service token | A config-scoped token for exactly one backend config. | A broadly scoped or reused token can inject the wrong database, provider, or signing credentials before the service starts. |
| Render service and frontend deployment/Auth0 application | Separate deployed service URLs, frontend origins, Auth0 clients, callbacks, and matching API audiences. | Authentication redirects, CORS, and access-token audiences must not route a preproduction browser or token into production. |

GenAI provider accounts may be intentionally shared when their data policy
allows it, but separate keys are preferred for quota, cost attribution, and
independent rotation. They are not currently an authorization boundary.

### Structural protections and their limits

- Every Daytona sandbox created by the runner has both `codegym=submission` and
  `codegym-environment=<dev|stg|prd>` labels. The orphan sweeper deletes only
  old sandboxes with its exact environment label. It logs and skips another
  environment, a missing label, or an unrecognised label.
- Every operation-scoped relay token carries its environment inside the signed
  payload. A validating service returns a distinct environment-mismatch error
  before accepting a token minted for another environment, even if both
  services share the same HMAC secret.
- Config parsing, startup logs, and the readiness payload make the resolved
  environment observable and reject unrecognised names.

These protections limit damage from credential reuse; they do not make shared
configuration correct. In particular, no application-level label can stop a
`dev` process from writing through a production Neon connection string. Keep
all resources above distinct and run the check below after configuration work.

## Repeatable isolation check

Authenticate the Doppler CLI with read access, then run from the repository:

```bash
cd backend
GOCACHE=/private/tmp/cg-env go run ./cmd/environment-isolation-check
```

The command reads `codegym/dev`, `codegym/stg`, and `codegym/prd` using
`doppler secrets download` with the encrypted fallback file
`/private/tmp/codegym-doppler-fallback`. It checks at least
`NEON_CONNECTION_STRING`, `DAYTONA_API_KEY`, and
`CODEGYM_RELAY_TOKEN_SECRET`. The table contains eight hexadecimal characters
from each SHA-256 fingerprint and overlap groups such as `dev=prd`; it never
prints a secret value or raw Doppler subprocess output.

Exit status `0` means all checked values are present and distinct. Status `1`
means an overlap or missing required value was found. Status `2` means the
check could not read or decode a Doppler config. This command is intentionally
not a CI gate yet because configuration remediation is handled separately.

## Motivating incident: 2026-08-05

A manual SHA-256 comparison on August 5, 2026 found all three isolation
failures below. The eight-character strings are historical fingerprints, not
secret values:

| Secret | `dev` | `stg` | `prd` | Historical overlap |
| --- | --- | --- | --- | --- |
| `NEON_CONNECTION_STRING` | `99fbce0f` | `c207f786` | `99fbce0f` | `dev=prd` |
| `DAYTONA_API_KEY` | `b8a8e818` | `c4c5ab35` | `c4c5ab35` | `stg=prd` |
| `CODEGYM_RELAY_TOKEN_SECRET` | `f9ea6c31` | `f9ea6c31` | `f9ea6c31` | `dev=stg=prd` |

The concrete consequences were local development writing to the production
database, the staging sweeper being able to delete production sandboxes in the
shared Daytona account, and relay tokens minted in development or staging
validating in production. The sandbox-label and signed-token protections above
now block the latter two cross-environment paths. Correct Neon separation and
all other resource separation remain mandatory.

## Local development

### Backend

```bash
cd backend
doppler run -p codegym -c dev -- go run ./cmd/server
```

What this command does, exactly:

- `doppler run`: fetches secrets from Doppler and starts a child process.
- `-p codegym`: selects the Doppler project named `codegym`.
- `-c dev`: selects the Doppler config named `dev` (`-c` means config).
- `--`: everything after this is the command Doppler should run.
- `go run ./cmd/server`: starts the Go backend using the injected env vars.

How secret injection works:

- `doppler login` authenticates your machine/user with Doppler.
- `doppler setup --project codegym --config dev` stores your local project/config binding.
- `doppler run ...` fetches that shared config's secrets and injects them into the started process environment.
- The Go app reads those values via `os.Getenv(...)` in `config.Load()`.

Important scope note:

- In memory, attached to that running process.
- Not written to your repo files.
- Not automatically set globally for all terminals forever.

Think of it as a temporary backpack handed to that one running program. When the process exits, that injected environment is gone.

To run the Neon-backed schema integration test:
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
| `CODEGYM_ENVIRONMENT` | Yes | Runtime isolation identity: exactly `dev`, `stg`, or `prd`. Explicit value wins over Doppler metadata; default is `dev`. |
| `NEON_CONNECTION_STRING` | Yes for durable memory | Neon/Postgres connection string. Each Doppler config must use its own Neon branch: `dev` → branch `dev`, `stg` → `staging`, `prd` → `production`. Never point `codegym/dev` at production. |
| `CODEGYM_AUTH_MODE` | Production | Auth mode (`auth0` or dev). |
| `CODEGYM_AUTH0_DOMAIN` | When Auth0 | Auth0 tenant domain (e.g. `dev-….us.auth0.com`). |
| `CODEGYM_AUTH0_AUDIENCE` | When Auth0 | **Auth0 API Identifier** for the CodeGym API (e.g. `https://api.codegym.app`). Must match `VITE_AUTH0_AUDIENCE`. **Not** `https://…auth0.com/api/v2/` (Management API). |
| `CODEGYM_CORS_ALLOWED_ORIGINS` | Production | Allowed browser origins (Vercel URL). |
| `CODEGYM_SEED_DEMO` | Demo deploy | Set `true` to idempotently seed the global `two-sum` problem catalog at startup. |
| `CODEGYM_DEV_AUTH_TOKEN` | Optional | Static bearer token for protected routes. |
| `CODEGYM_DEV_TENANT_ID` | Optional | Default personal tenant for static-token auth. |
| `DAYTONA_API_KEY` / `DAYTONA_API_URL` | Coding demo | Daytona sandbox access for real hidden-test execution. |
| `CODEGYM_RELAY_TOKEN_SECRET` | Agent relay | HMAC signing secret, at least 32 bytes and distinct per environment. Tokens also carry the signed environment claim. |
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
| `VITE_AUTH0_AUDIENCE` | Same **API Identifier** as `CODEGYM_AUTH0_AUDIENCE` so `getAccessTokenSilently` requests a backend-valid access token. |
| `VITE_AUTH0_CLIENT_ID` | Auth0 SPA Application Client ID (required for Auth0 UI; empty falls back to “dev auth”). |
| `VITE_API_BASE_URL` | Optional API origin; leave empty for same-origin / rewrites. |
| `VITE_USE_MOCK_API` | Set `false` for the deployed real-API demo. Mocks are dev-only regardless. |
| `VITE_APP_ORIGIN` | Canonical frontend origin (Vercel production URL). |

Keep browser-visible values prefixed with `VITE_`.

### Auth0 API Identifier (avoids `Invalid bearer token`)

Settings, memory, generate, and cost all need a Bearer **access token**. If
the SPA can sign in but `/api/v1/me` returns `Invalid bearer token`, the token
`aud` almost certainly does not match the backend audience.

1. Auth0 → **APIs** → create/select CodeGym API → copy **Identifier**.
2. Set that Identifier as both `CODEGYM_AUTH0_AUDIENCE` and `VITE_AUTH0_AUDIENCE`.
3. Authorize the SPA application for that API; set callback/logout/web origins.
4. Redeploy Render + Vercel; log out and back in.

Never set audience to `https://YOUR_TENANT.us.auth0.com/api/v2/` (Management
API). Step-by-step: [../backend/README.md](../backend/README.md) (Auth0 API setup).

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
