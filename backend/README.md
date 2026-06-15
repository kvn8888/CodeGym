# CodeGym Backend

This is the first backend skeleton for the auth, tenant, and memory substrate.
It is intentionally small and dependency-free while the product contracts settle.

## Run

```bash
go run ./cmd/server
```

Default address: `127.0.0.1:8080`.

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

Memory events are append-only. The in-memory store is a temporary adapter behind
the `memory.Store` interface; the Neon/Postgres implementation should replace it
without changing handlers or middleware.
