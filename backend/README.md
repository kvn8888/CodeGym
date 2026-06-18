# CodeGym Backend

This is the first backend skeleton for the auth, tenant, and memory substrate.
It stays runnable without external services, but switches to Neon/Postgres when
`NEON_CONNECTION_STRING` or `DATABASE_URL` is present.

## Run

```bash
go run ./cmd/server
```

Default address: `127.0.0.1:8080`.

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

## Current Routes

```text
GET  /health
GET  /api/v1/memory/profile
GET  /api/v1/memory/events
POST /api/v1/memory/events
```

Memory events are append-only. The server uses the in-memory store when no
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
