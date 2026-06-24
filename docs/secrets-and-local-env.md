# Secrets and Local Environment

CodeGym uses Doppler for shared development secrets. Do not send `.env` files or
connection strings over Slack.

## First-time Doppler setup

From the repo root:

```bash
doppler login
doppler setup --project codegym --config dev --no-interactive
```

The setup command stores the `codegym/dev` binding in your local Doppler CLI
config. That file is machine-specific and should not be committed.

## Backend

The backend reads `NEON_CONNECTION_STRING` first, then falls back to
`DATABASE_URL`. When either value is present, the server uses Postgres for the
identity bootstrap and memory store. Without a database URL, it runs with
in-memory stores for local development.

```bash
cd backend
doppler run -p codegym -c dev -- go run ./cmd/server
```

To run the Neon-backed schema integration test:

```bash
cd backend
doppler run -p codegym -c dev -- go test ./internal/integration -run TestNeonIdentityAndMemoryBootstrap -count=1
```

That test bootstraps the user, tenant, membership, memory profile, and memory
event tables against Neon, verifies the membership foreign keys, and cleans up
its temporary rows. Normal `go test ./...` skips the Neon check when no database
URL is present.

Required development secrets:

| Name | Required | Purpose |
| --- | --- | --- |
| `NEON_CONNECTION_STRING` | Yes for durable memory | Neon/Postgres connection string. |
| `CODEGYM_DEV_AUTH_TOKEN` | Optional | Static bearer token for local protected routes. |
| `CODEGYM_DEV_USER_ID` | Optional | Default dev user for static-token auth. |
| `CODEGYM_DEV_TENANT_ID` | Optional | Default personal tenant for static-token auth. |
| `CODEGYM_HOST` | Optional | Backend listen host. |
| `CODEGYM_PORT` | Optional | Backend listen port. |

### Memory API smoke test

With the backend running under Doppler, validate the Postgres memory store over
HTTP:

```bash
cd backend
doppler run -p codegym -c dev -- ./scripts/memory_smoke_test.sh
```

The script uses `CODEGYM_DEV_AUTH_TOKEN` from Doppler when present. Without a
static token, it falls back to `dev:smoke-test:personal-smoke`. Override with
`CODEGYM_SMOKE_AUTH_TOKEN` or `CODEGYM_BASE_URL` when needed.

## Frontend

The frontend currently does not require private secrets to run. Keep any future
browser-visible values prefixed with the framework's public env prefix and do
not expose server-only secrets to Vite.

```bash
cd frontend
npm run dev
```

If the frontend later needs shared non-secret config from Doppler:

```bash
cd frontend
doppler run -p codegym -c dev -- npm run dev
```
