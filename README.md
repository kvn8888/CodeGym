# CodeGym v2

Generative AI interview practice for software engineers. CodeGym creates
LeetCode-style coding problems, MCQs, and conversational practice sessions,
then personalizes future questions from a memory profile of your strengths and
gaps.

This repository is the **v2 rewrite**: a React/Vite frontend and a Go backend
focused on auth, personal workspace data isolation, and memory. Problem
generation and secure code execution are planned; the current milestone (M1)
establishes the platform foundation.

## What's in the repo

| Path | Purpose |
| --- | --- |
| [`frontend/`](frontend/) | React 19 + Vite 7 UI — problem browser, generation flow, memory page, Storybook |
| [`backend/`](backend/) | Go API — auth → identity → personal workspace scope, memory profile/events |
| [`api/openapi.yaml`](api/openapi.yaml) | HTTP contract shared by frontend and backend |
| [`docs/`](docs/) | Architecture notes, auth flow, secrets, backend user stories |

## Quick start

Run the backend and frontend in separate terminals.

```bash
# Terminal 1 — API (in-memory store by default)
cd backend && go run ./cmd/server

# Terminal 2 — UI (proxies /api to localhost:8080)
cd frontend && npm install && npm run dev
```

Open [http://localhost:3000](http://localhost:3000). The backend listens on
`127.0.0.1:8080`.

For local API calls, send a dev bearer token:

```http
Authorization: Bearer dev:kevin:personal-dev
```

See [backend/README.md](backend/README.md) for CORS, Doppler/Neon setup, and
route details.

### Optional: Storybook

```bash
cd frontend && npm run storybook
```

Runs at [http://localhost:6006](http://localhost:6006).

## Validation

```bash
cd frontend && npm run lint && npm run build
cd backend && go test ./...
```

CI runs the same checks on pushes and PRs to `codegym-v2`.

## Architecture snapshot

```text
Browser (React/Vite)
  -> /api/v1/*  (Vite dev proxy or VITE_API_BASE_URL)
  -> Go backend
       auth -> identity -> personal workspace scope -> handlers
       memory: append events, derive profile on refresh/worker boundary
  -> Neon/Postgres (optional; in-memory store when no DB URL is set)
```

Deeper docs:

- [Auth, identity, and workspace scope](docs/auth-identity-tenant.md)
- [Backend M1 user stories](docs/backend-m1-user-stories.md)
- [Secrets and local env](docs/secrets-and-local-env.md)
- [OpenAPI contract notes](docs/openapi-contract.md)

## Branch and scope

Active development happens on **`codegym-v2`**. M1 covers middleware, personal
workspace data isolation, memory storage, and profile summarization.
Docker-based execution, deploy automation, organization/team tenancy, and
legacy v1 infrastructure stay out of scope unless explicitly revived.

## License

AGPL-3.0 — see [LICENSE](LICENSE).
