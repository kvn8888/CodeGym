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
the shared secret contract.

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

The API server starts the memory profile refresh worker by default. The request
path appends raw memory events quickly; the worker periodically calls
`memory.Service.RefreshAllProfiles` off the request path so derived profiles can
be refreshed without blocking event recording.

Default interval: `24h`.

Override with:

```bash
CODEGYM_MEMORY_WORKER_INTERVAL=1h
```

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

See [../docs/auth-identity-workspace.md](../docs/auth-identity-workspace.md) for the
full auth -> identity -> personal workspace scope request flow.

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

GET  /api/v1/memory/profile
POST /api/v1/memory/profile/refresh
GET  /api/v1/memory/events
POST /api/v1/memory/events

GET   /api/v1/sessions
POST  /api/v1/sessions
GET   /api/v1/sessions/{id}
PATCH /api/v1/sessions/{id}
PUT   /api/v1/sessions/{id}/files
```

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
