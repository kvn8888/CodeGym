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

Memory event `source` and `type` naming for emitters lives in
[../docs/memory-event-naming-guide-v0.md](../docs/memory-event-naming-guide-v0.md).

## Current Routes

```text
GET  /health
GET  /ready

GET  /api/v1/memory/profile
POST /api/v1/memory/profile/refresh
GET  /api/v1/memory/events
POST /api/v1/memory/events
```


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
- `tenants`
- `tenant_memberships`
- `user_memory_profiles`
- `memory_events`

This is intentionally not a migration framework yet. The first Neon/Postgres
schema starts as a simple bootstrap and can move to versioned migrations when
the schema hardens.
