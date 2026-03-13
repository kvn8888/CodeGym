# CodeGym Repository Reference

## Repository Layout

- `backend/`: Go API server, domain model, data access, execution orchestration.
- `frontend/`: React + TypeScript + Vite UI.
- `problems/`: Filesystem-backed coding problems grouped by language/category.
- `docker/`: Runtime image definitions and build scripts for execution environments.
- `deploy/`: Production-oriented Dockerfiles and nginx config.
- `data/`: SQLite file location used by local runtime.

## Common Commands

### Root Makefile

- `make dev`: start backend and frontend together.
- `make dev-backend`: run backend server (`go run ./cmd/server`).
- `make dev-frontend`: run Vite dev server.
- `make test`: run backend tests (`go test ./...`).
- `make lint`: run backend lint and frontend eslint.
- `make sqlc`: regenerate database code from SQL files.
- `make migrate`: apply goose migrations to local SQLite database.

### Frontend Direct

- `cd frontend && npm run dev`
- `cd frontend && npm run lint`
- `cd frontend && npm run build`

## Backend Architecture

- Server entrypoint: `backend/cmd/server/main.go`.
- Router setup: `backend/internal/api/router.go`.
- Response envelope helpers: `backend/internal/api/response/response.go`.
- Problem listing/detail handlers: `backend/internal/api/handlers/problems.go`.
- Problem ingestion and filtering: `backend/internal/store/problemstore/fs.go`.
- Shared domain model: `backend/internal/domain/problem.go`.
- Config/env loading: `backend/internal/config/config.go`.

### Current HTTP Surface

- `GET /health`
- `GET /api/v1/problems`
- `GET /api/v1/problems/{id}`
- `GET /api/v1/problems/{id}/skeleton`

The submissions/generation/user/auth routes exist as placeholders returning `not_implemented`.

## API Contract

- Backend JSON payloads are wrapped as:
	- success: `{ "data": <payload>, "error": null }`
	- failure: `{ "data": null, "error": { "code": "...", "message": "..." } }`
- Frontend client and error behavior live in `frontend/src/shared/api/client.ts`.
- Frontend type contracts live in `frontend/src/shared/api/types.ts`.

## Problem Pack Contract

Each problem usually includes:

- `problem.yaml`: metadata and runtime/test settings.
- `skeleton/`: learner-facing starting files.
- `solution/`: hidden canonical answer files.
- `tests/`: hidden grading tests.

`problem.yaml` fields map directly to `backend/internal/domain/problem.go`.

## Database and SQLC

- Migrations: `backend/internal/store/db/migrations/`.
- SQL queries: `backend/internal/store/db/queries/`.
- Generated sqlc package: `backend/internal/store/db/generated/`.
- sqlc config: `backend/sqlc.yaml`.

For any migration or query update, regenerate sqlc outputs and re-run backend tests.

## Environment Variables

Key runtime variables used by backend config:

- `CODEGYM_HOST` (default `0.0.0.0`)
- `CODEGYM_PORT` (default `8080`)
- `CODEGYM_DB_PATH` (default `./data/codegym.db`)
- `CODEGYM_PROBLEMS_DIR` (default `./problems`)
- `CODEGYM_JWT_SECRET` (required)
- `ANTHROPIC_API_KEY` (optional for AI functionality)
- `CODEGYM_MAX_CONCURRENT_EXECUTIONS` (default `4`)
- `CODEGYM_DEFAULT_TIMEOUT_SECONDS` (default `30`)
- `CODEGYM_DEFAULT_MEMORY_MB` (default `256`)
