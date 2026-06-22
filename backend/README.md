# CodeGym Backend

This is the first backend skeleton for the auth, tenant, and memory substrate.
It is intentionally small and dependency-free while the product contracts settle.

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
