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

That test bootstraps the user, personal workspace membership, memory profile,
and memory event tables against Neon, verifies the membership foreign keys, and
cleans up its temporary rows. Normal `go test ./...` skips the Neon check when
no database URL is present.

Required development secrets:

| Name | Required | Purpose |
| --- | --- | --- |
| `NEON_CONNECTION_STRING` | Yes for durable memory | Neon/Postgres connection string. |
| `CODEGYM_AUTH_MODE` | Optional | Set to `auth0` to require Auth0 configuration. Defaults to dev auth unless Auth0 issuer/audience are present. |
| `AUTH0_DOMAIN` / `CODEGYM_AUTH0_DOMAIN` | Yes for Auth0 | Auth0 domain, e.g. `your-auth0-domain.us.auth0.com`. |
| `AUTH0_AUDIENCE` / `CODEGYM_AUTH0_AUDIENCE` | Yes for Auth0 | API audience expected in Auth0 access tokens. |
| `CODEGYM_AUTH0_ISSUER_URL` | Optional | Explicit issuer URL override when domain is not enough. |
| `CODEGYM_AUTH0_CLOCK_SKEW` | Optional | Go duration for token time skew, e.g. `30s`. Defaults to no skew. |
| `CODEGYM_DEV_AUTH_TOKEN` | Optional | Static bearer token for local protected routes. |
| `CODEGYM_DEV_USER_ID` | Optional | Default dev user for static-token auth. |
| `CODEGYM_DEV_TENANT_ID` | Optional | Default personal workspace ID for static-token auth. Legacy env name. |
| `CODEGYM_HOST` | Optional | Backend listen host. |
| `CODEGYM_PORT` | Optional | Backend listen port. |

## Memory smoke test

Start the backend in one terminal:

```bash
cd backend
doppler run -p codegym -c dev -- go run ./cmd/server
```

Then run the smoke test in another terminal:

```bash
cd backend
./scripts/memory_smoke.sh
```

If `CODEGYM_DEV_AUTH_TOKEN` lives in Doppler, run the script through Doppler so
it can send the same static token that the backend expects:

```bash
cd backend
doppler run -p codegym -c dev -- ./scripts/memory_smoke.sh
```

The smoke script uses `CODEGYM_API_BASE_URL` when set, otherwise
`http://127.0.0.1:8080`. For auth, it uses `CODEGYM_DEV_AUTH_TOKEN` when set;
otherwise it builds a local dev token from `CODEGYM_DEV_USER_ID` and
`CODEGYM_DEV_TENANT_ID`:

```text
Authorization: Bearer dev:<user-id>:<workspace-id>
```

The script calls `/ready`, writes a `system.memory_api_checked` event, verifies
that the event appears in `GET /api/v1/memory/events`, and fetches
`GET /api/v1/memory/profile`. It exits non-zero if any API call or response
shape check fails.

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
