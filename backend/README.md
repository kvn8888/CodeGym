# CodeGym Backend

This is the first backend skeleton for the auth, tenant, and memory substrate.
It stays runnable without external services, but switches to Neon/Postgres when
`NEON_CONNECTION_STRING` or `DATABASE_URL` is present.

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

## Dev Auth

Protected routes require a bearer token.

For local development without a configured provider, use:

```http
Authorization: Bearer dev:<user-id>:<tenant-id>
```

Example:

```http
Authorization: Bearer dev:kevin:personal-dev
```

If `CODEGYM_DEV_AUTH_TOKEN` is set, the backend accepts only that exact bearer
token and maps it to `CODEGYM_DEV_USER_ID` / `CODEGYM_DEV_TENANT_ID`.

See [../docs/auth-identity-tenant.md](../docs/auth-identity-tenant.md) for the
full auth -> identity -> tenant request flow.

## Architecture: Principal, Tenant, Workspace

**Principal** = Authenticated user + list of accessible workspaces.

When a request is authenticated (bearer token validated), the auth middleware injects
a Principal into the request context. The Principal contains:

- `UserID`: The user's unique identifier
- `DefaultTenantID`: The default workspace for this user (usually personal)
- `TenantIDs`: List of all workspaces the user is a member of

**Tenant** = A persistent, isolated data partition (workspace).

Each tenant holds:

- User's memory profile (skills, strengths, growth edges, notes)
- Event log (problem attempts, reviews, milestones)
- Shared context (if multi-user tenant, e.g., team workspace)

Most users have one tenant (personal workspace, auto-created on first auth).
The architecture supports multiple tenants to enable team/org workspaces (future):
a user can be a member of both personal-workspace AND team-data-science, switching
context via `X-CodeGym-Tenant-ID` header to operate in different workspaces with
different memory and collaborators.

**Workspace** = Application-level term for a tenant.

Alias: a persistent account/collaboration scope (not a browser session).
Contrast: a session is ephemeral (one conversation); a workspace is durable
(all interactions, memory, events accumulate there).

**Request Scope** = Every request operates in exactly one tenant.

Middleware resolves which tenant by reading `X-CodeGym-Tenant-ID` header (if present)
or falling back to the principal's default. All data operations are scoped to that tenant:

```
user=kevin, principal.TenantIDs=[personal-dev, team-science]

Request 1: GET /api/v1/memory/profile (no header)
  → operates in personal-dev → returns kevin's personal memory

Request 2: GET /api/v1/memory/profile (header: X-CodeGym-Tenant-ID=team-science)
  → operates in team-science → returns kevin's team memory
```

If the requested tenant is not in the principal's TenantIDs, the request is rejected (403).

## Current Routes

```text
GET  /health
GET  /ready

GET  /api/v1/memory/profile
GET  /api/v1/memory/events
POST /api/v1/memory/events
```

## Memory Profile Fields

`GET /api/v1/memory/profile` returns a profile object with:

- `summary`: high-level memory summary for the user in the current tenant
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

Memory events are append-only. The in-memory store is a temporary adapter behind
the `memory.Store` interface; the Neon/Postgres implementation should replace it
without changing handlers or middleware.

The server uses the in-memory store when no
database URL is configured and the Postgres store when Doppler provides the Neon
connection string.

On startup with Postgres enabled, the backend idempotently bootstraps the M1
tables with `CREATE TABLE IF NOT EXISTS`:

- `app_users`
- `tenants`
- `tenant_memberships`
- `user_memory_profiles`
- `memory_events`

This is intentionally not a migration framework yet. Since CodeGym has not used
the old Turso path in production, the first Neon schema can start as a simple
bootstrap and move to versioned migrations when the schema hardens.

