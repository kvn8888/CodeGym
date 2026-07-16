# CodeGym Backend

This is the backend skeleton for auth, personal workspace data isolation, and
memory. It stays runnable without external services, but switches to
Neon/Postgres when `NEON_CONNECTION_STRING` or `DATABASE_URL` is present.

## Run

```bash
go run ./cmd/server
```

Default address: `127.0.0.1:8080`.

## CORS (Local Frontend)

The backend now uses an explicit origin allow-list for browser requests.

Default allowed origins include common Vite localhost origins:

- `http://localhost:3000`
- `http://127.0.0.1:3000`
- `http://localhost:5173`
- `http://127.0.0.1:5173`

Override with:

```bash
CODEGYM_CORS_ALLOWED_ORIGINS=http://localhost:3000,http://127.0.0.1:3000
```

If the browser `Origin` is not on this list, CORS headers are not returned,
and browsers will block cross-origin API calls.

## Local API URL Wiring

The frontend API client defaults to relative `/api/v1`.

Recommended local setup (same-origin via Vite proxy):

- Vite dev server runs at `http://localhost:3000`
- Vite proxy forwards `/api` to `http://localhost:8080`
- No explicit frontend API env var is needed

Alternative setup (direct cross-origin calls):

```bash
# frontend .env.local
VITE_API_BASE_URL=http://localhost:8080/api/v1
```

When using cross-origin mode, ensure `CODEGYM_CORS_ALLOWED_ORIGINS` includes your
frontend origin.

For durable memory using Doppler:

```bash
doppler run -p codegym -c dev -- go run ./cmd/server
```

See [../docs/secrets-and-local-env.md](../docs/secrets-and-local-env.md) for
the shared secret contract. Production Render/Vercel use a single
`DOPPLER_TOKEN` and `doppler run` — app secrets are not stored in those
dashboards.

## GenAI multi-provider routing

`POST /api/v1/generate` goes through an in-process **provider router**. Each
enabled platform is one OpenAI-compatible adapter (`meta`, `azure`, `gemini`).
Default priority: **Meta → Azure → Gemini** (override with
`CODEGYM_GENAI_PROVIDER_ORDER`). On provider/HTTP failures the router falls
through to the next hop; MCQ validate/repair stays on the same provider so a
bad JSON shape does not burn every credit pool.

Register a hop by setting its API key (and Azure base URL). Missing keys are
skipped. Startup logs list registered providers and their base URLs (never keys).

To validate the Neon-backed memory flow against a running local backend:

```bash
./scripts/memory_smoke.sh
```

If the backend is configured with `CODEGYM_DEV_AUTH_TOKEN`, run the script
through Doppler too:

```bash
doppler run -p codegym -c dev -- ./scripts/memory_smoke.sh
```

The backend API contract, response envelopes, and curl examples are documented
in [../docs/openapi-contract.md](../docs/openapi-contract.md).

## Memory Worker

The API server starts the LLM memory-profile worker by default. Request paths
append deterministic events quickly; the worker periodically builds a bounded
evidence digest and synthesizes the summary, strengths, growth edges, skills,
and notes off the event-recording path. Invalid provider output preserves an
existing profile. A first refresh can still use the deterministic v0 profile
when generation is unavailable.

Before a daily synthesis, the worker computes a SHA-256 digest of the filtered,
bounded learning evidence. If that digest matches the profile's persisted
provenance, the tick returns without calling the model, consuming tokens, or
changing profile timestamps. New learning evidence changes the digest and
causes one normal synthesis attempt. Provider failures preserve the prior
profile and digest so the next scheduled tick can retry.

Default interval: `24h`.

Override with:

```bash
CODEGYM_MEMORY_WORKER_INTERVAL=1h
```

Choose when profile synthesis runs with:

```bash
# daily | set-completion | both (default)
CODEGYM_MEMORY_REFRESH_TRIGGER=both
```

`set-completion` is awaited before the next generated practice set reads
memory. `daily` uses the worker interval. `both` enables both paths.

Disable the worker for local debugging with:

```bash
CODEGYM_MEMORY_WORKER_DISABLED=true
```

## Auth

Protected routes require a bearer token.

For local development without a configured provider, use:

```http
Authorization: Bearer dev:<user-id>:<workspace-id>
```

Example:

```http
Authorization: Bearer dev:kevin:personal-dev
```

If `CODEGYM_DEV_AUTH_TOKEN` is set, the backend accepts only that exact bearer
token and maps it to `CODEGYM_DEV_USER_ID` / `CODEGYM_DEV_WORKSPACE_ID`.

The dev token third segment is the personal workspace id. Prefer
`CODEGYM_DEV_WORKSPACE_ID` over the legacy alias `CODEGYM_DEV_TENANT_ID`.
Product code should treat that value as the user's personal workspace ID, not as
a business SaaS organization workspace.

For Auth0, set the mode plus issuer/audience config:

```bash
CODEGYM_AUTH_MODE=auth0
CODEGYM_AUTH0_DOMAIN=your-auth0-domain.us.auth0.com
CODEGYM_AUTH0_AUDIENCE=https://api.codegym.example
```

The server also accepts `AUTH0_DOMAIN` / `AUTH0_AUDIENCE` aliases and
`CODEGYM_AUTH0_ISSUER_URL` for explicit issuer overrides. When Auth0 is
configured, backend **access tokens** are validated with Auth0 JWKS, RS256,
issuer, audience, expiry, and not-before checks before they reach
identity/workspace scope middleware. The backend does not use Auth0 client IDs
or client secrets; those belong to the frontend SPA login setup.

### Auth0 API (audience) setup — required for Settings / protected APIs

Protected routes (`GET /api/v1/me`, `/api/v1/cost`, memory, generate, …) expect
an **Auth0 access token** whose `aud` claim matches `CODEGYM_AUTH0_AUDIENCE`
exactly. Login can succeed in the SPA while API calls fail with
`Invalid bearer token` if audience is wrong.

**Do this in the Auth0 dashboard:**

1. **Applications → APIs → Create API** (or open the existing CodeGym API).
   - **Name:** e.g. `CodeGym API`
   - **Identifier:** a stable URI you invent, e.g. `https://api.codegym.app`
     (does not need to be a real HTTP endpoint — it is the JWT audience).
   - Signing algorithm: **RS256**
2. **Applications → your SPA app**
   - Authorize the SPA for that API (enable the API for the application).
   - **Allowed Callback URLs / Logout URLs / Web Origins** include the Vercel
     origin(s), e.g. `https://code-gym-rho.vercel.app` (and preview URLs if needed).
3. Put the **same Identifier** in Doppler on both sides:
   - Backend (`dev` / `prd`): `CODEGYM_AUTH0_AUDIENCE=<API Identifier>`
   - Frontend (`dev_frontend` / `prd_frontend`): `VITE_AUTH0_AUDIENCE=<API Identifier>`
   - Frontend also needs `VITE_AUTH0_DOMAIN` and `VITE_AUTH0_CLIENT_ID` (SPA
     Application → Settings → Client ID).
4. Redeploy **Render** (backend reloads audience) and **Vercel** (Vite inlines
   `VITE_*` at build time). Log out and log in again so Auth0 issues a token
   for the new audience.

**Do not use** the Management API as audience:

```text
# WRONG — causes Invalid bearer token on /api/v1/*
https://YOUR_TENANT.us.auth0.com/api/v2/
```

That is Auth0’s admin Management API, not the CodeGym backend API. SPA access
tokens for user APIs should use your **API Identifier** from step 1.

**Quick check after login:** decode the access token (jwt.io, payload only):

| Claim | Expected |
| --- | --- |
| `iss` | `https://YOUR_TENANT.us.auth0.com/` |
| `aud` | Exactly `CODEGYM_AUTH0_AUDIENCE` / `VITE_AUTH0_AUDIENCE` |
| `sub` | Auth0 user id (becomes CodeGym `user_id`) |

If `aud` is the SPA client id or `/api/v2/`, the backend will reject the token.

Frontend env names and deploy notes: [../docs/secrets-and-local-env.md](../docs/secrets-and-local-env.md),
[../docs/vercel-deploy.md](../docs/vercel-deploy.md). Full auth pipeline:
[../docs/auth-identity-workspace.md](../docs/auth-identity-workspace.md).

## Architecture: Principal and Personal Workspace Scope

**Principal** = the authenticated user plus the internal workspace IDs the
backend may use for scoping.

When a request is authenticated, the auth middleware injects a `Principal` into
the request context:

- `UserID`: the durable CodeGym user identifier. With Auth0, this is `sub`.
- `DefaultWorkspaceID`: the user's default personal workspace ID.
- `WorkspaceIDs`: allowed internal workspace scope IDs. Today this is only the
  user's personal workspace.

Package, table, and field names use `workspace` / `workspace_id`. Product
behavior is personal workspace scoping. Do not build organization/team workspace
switching or Auth0 organization claims without a new product decision.

Product flows should omit `X-CodeGym-Workspace-ID` and use the authenticated
user's default personal workspace. The header remains only as a low-level
internal override; if the requested scope is not in `Principal.WorkspaceIDs`, the
request is rejected with `403`.
Memory event `source` and `type` names for emitters are documented in
[../docs/memory-event-naming-guide-v0.md](../docs/memory-event-naming-guide-v0.md).

## Current Routes

```text
GET  /health
GET  /ready

GET  /api/v1/me
PATCH /api/v1/me
GET  /api/v1/cost

GET  /api/v1/memory/profile
POST /api/v1/memory/profile/refresh
POST /api/v1/memory/profile/maintain
GET  /api/v1/memory/events
POST /api/v1/memory/events

GET   /api/v1/sessions
POST  /api/v1/sessions
GET   /api/v1/sessions/{id}
PATCH /api/v1/sessions/{id}
PUT   /api/v1/sessions/{id}/files

POST /api/v1/generate
POST /api/v1/mcq/evaluate
POST /api/v1/memory/notes/maintain # deprecated compatibility alias
```

## GenAI usage and cost

Every successful model call (MCQ generation, memory-profile synthesis, etc.) records
`tokens_in`, `tokens_out`, provider, model, and an estimated USD cost. Totals
for the authenticated workspace user are exposed at:

```text
GET /api/v1/cost
```

Costs use a built-in rate card (USD per million tokens, as of 2026-07-15):

| Provider / model | Input / 1M | Output / 1M |
| --- | --- | --- |
| Meta Muse Spark 1.1 | $1.25 | $4.25 |
| Azure / OpenAI GPT-5.6 Terra | $2.50 | $15.00 |
| Gemini Flash-class | $0.30 | $2.50 |

Rates are estimates for product visibility, not invoices. Persist to Postgres
when `NEON_CONNECTION_STRING` is set (`genai_usage` table).

## Memory Profile Fields

`GET /api/v1/memory/profile` returns a profile object with:

- `summary`: high-level memory summary for the user in the current workspace
- `updated_at`: timestamp for last profile update
- `next_review_at`: timestamp for next scheduled review/refresh
- `strengths`: list of observed strengths
- `growth_edges`: list of growth opportunities
- `skills`: list of skill proficiency objects
- `notes`: list of problem-specific note objects

Each `skills` item includes:

- `id`
- `label`
- `area`
- `level`
- `confidence`
- `trend`
- `last_practiced`

Each `notes` item includes:

- `id`
- `problem_id`
- `title`
- `summary`
- `created_at`
- `tags`
- `action`


## Health vs Readiness

`GET /health` is a lightweight liveness check and always returns success when the
process is running.

`GET /ready` is a readiness check. In in-memory mode it returns success without
touching a database. When `NEON_CONNECTION_STRING` or `DATABASE_URL` is set, it
attempts a Postgres ping and returns a generic `503` if the database is not
reachable.

Memory events are append-only. Both the local in-memory store and the
Neon/Postgres store sit behind the `memory.Store` interface, so handlers and
middleware do not care which adapter is active.

The server uses the in-memory store when no
database URL is configured and the Postgres store when Doppler provides the Neon
connection string.

On startup with Postgres enabled, the backend idempotently bootstraps the M1
tables with `CREATE TABLE IF NOT EXISTS`:

- `app_users`
- `workspaces`
- `workspace_memberships`
- `user_memory_profiles`
- `memory_events`
- `practice_sessions`
- `session_files`

`app_users.id` is the durable CodeGym user key. With Auth0, it is the Auth0
`sub` claim. Auth0 `email` and `name` are stored as optional profile metadata
(`email` and `display_name`) when present, but neither field is used for
authorization or workspace selection.

The `workspaces` and `workspace_memberships` tables are the current schema
names for personal workspace scope. They are not a product commitment to
organization/team SaaS tenancy.

This is intentionally not a migration framework yet. The first Neon/Postgres
schema starts as a simple bootstrap and can move to versioned migrations when
the schema hardens.

## Generation Boundary

The provider-neutral generation seam lives in `backend/internal/generation`.
Provider adapters implement:

```go
type Generator interface {
	Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error)
}
```

`GenerateRequest` always carries `Kind`, `Spec`, `MemoryContext`, `Schema`, and
`ModelPolicy`. `GenerateResult` records the structured object plus the
provider/model/tokens/cost that actually served the request. Provider SDKs
belong in adapter packages, not in the interface package.

`generation.Orchestrator` composes `memory.Service` with a `Generator`, builds
memory context from the scoped profile, then calls the provider-neutral
interface. This keeps provider adapters from fetching memory directly.
