---
name: codegym-project
description: Repository-specific workflow and architecture guidance for the CodeGym monorepo (Go backend, React frontend, filesystem-backed problem packs, sqlc/goose data layer, and Dockerized execution images). Use when implementing features, fixing bugs, adding problems, updating API routes, changing database queries/migrations, or wiring frontend pages to backend endpoints in this repository.
---

# CodeGym Project

Use this skill to execute CodeGym tasks without re-discovering project conventions.

## Workflow Decision Tree

1. Identify the task surface area.
- API behavior or server bootstrapping: inspect `backend/internal/api/` and `backend/cmd/server/main.go`.
- Problem loading or metadata filtering: inspect `backend/internal/store/problemstore/fs.go` and `backend/internal/domain/problem.go`.
- SQL schema/query/model updates: inspect `backend/internal/store/db/migrations/`, `backend/internal/store/db/queries/`, and `backend/sqlc.yaml`.
- Frontend route/page/API consumption: inspect `frontend/src/App.tsx`, `frontend/src/features/`, and `frontend/src/shared/api/`.
- Runtime/container changes: inspect `docker/`, `deploy/`, and `docker-compose.yaml`.

2. Select the shortest local validation path.
- Backend-only change: run `make test` or `cd backend && go test ./...`.
- Frontend-only change: run `cd frontend && npm run lint` and `cd frontend && npm run build` when type-level confidence is needed.
- DB query or schema change: run `make sqlc` after query/schema edits, then run backend tests.

3. Confirm compatibility with existing API shape.
- Keep backend responses wrapped in `{ data, error }` using `backend/internal/api/response/response.go` helpers.
- Keep frontend API calls routed through `frontend/src/shared/api/client.ts` so token and response handling remain centralized.

4. Prefer minimal, task-focused edits.
- Avoid broad refactors unless required by the task.
- Preserve existing naming and directory conventions for problem packs and API routes.

## Project Conventions

### Backend

- Start server with `make dev-backend` (runs `go run ./cmd/server`).
- Keep route registration in `backend/internal/api/router.go`.
- Implement per-resource handlers under `backend/internal/api/handlers/`.
- Use `problemstore.Store` for problem file access and filtering.
- Keep environment-driven settings in `backend/internal/config/config.go`.

### Frontend

- Start client with `make dev-frontend` (runs Vite).
- Start Storybook with `cd frontend && npm run storybook` (runs at port 6006).
- Define app routes in `frontend/src/App.tsx`.
- Keep pages grouped by feature under `frontend/src/features/`.
- Use `frontend/src/shared/api/client.ts` for all HTTP calls.
- Keep TypeScript API contracts in `frontend/src/shared/api/types.ts` synchronized with backend JSON fields.
- For new page components, add a `.stories.tsx` file next to the component file.

### Problem Packs

- Store problems under `problems/<language>/<category>/<problem-id>/`.
- Define metadata and runtime behavior in `problem.yaml`.
- Keep learner starter files in `skeleton/`, canonical answers in `solution/`, and tests in `tests/`.
- Keep IDs unique across all `problem.yaml` files (duplicate IDs are skipped at load time).

## Task Workflows

### Add or Update a Story (Storybook)

Storybook 10 runs at http://localhost:6006. Stories live next to their component files as `ComponentName.stories.tsx`.

Key conventions:
- The global decorator in `.storybook/preview.tsx` wraps every story in a `MemoryRouter` + `Routes` + `Route`. Stories that need `useParams` must set `parameters.initialPath` and `parameters.routePath` so the router populates params correctly.
- Mock `globalThis.fetch` inside story-level decorators (not `beforeEach` — `@storybook/test`'s lifecycle hooks depend on vitest, which is not installed).
- Tailwind styles work automatically — the `@tailwindcss/vite` plugin is inherited from `vite.config.ts` by the `react-vite` Storybook framework. No extra config needed.
- `@storybook/test` is installed but do NOT use `beforeEach` from it — use decorators instead.

Example story with fetch mock and route params:
```tsx
export const MyStory: Story = {
  parameters: {
    initialPath: '/problems/two-sum',
    routePath: '/problems/:id',
  },
  decorators: [
    (Story) => {
      globalThis.fetch = myMockFetch as typeof fetch;
      return <Story />;
    },
  ],
};
```

### Add or Update a Problem

1. Create or edit `problem.yaml` with valid `files.skeleton`, `files.solution`, `files.tests`, and `test_config.run`.
2. Add matching files in `skeleton/`, `solution/`, and `tests/`.
3. Run backend tests.
4. Start backend and verify `GET /api/v1/problems` and `GET /api/v1/problems/{id}` behavior.

### Add or Update an API Endpoint

1. Add the route in `backend/internal/api/router.go`.
2. Implement handler logic under `backend/internal/api/handlers/`.
3. Return payloads through response helpers (`response.JSON` and `response.Err`).
4. Update frontend API types and client usage if response shape changed.
5. Run backend tests and frontend lint/build for integration confidence.

### Add or Update Database Access

1. Edit SQL under `backend/internal/store/db/queries/` or migrations under `backend/internal/store/db/migrations/`.
2. Run `make sqlc` to regenerate typed query bindings.
3. Update call sites to the regenerated methods in `backend/internal/store/db/generated/`.
4. Run backend tests.

## Reference File

Read `references/api_reference.md` when detailed repository maps, key files, API endpoints, and environment variable defaults are needed.

## Generation Architecture

Problem generation uses an agent-driven flow (see `codegym-generation-flow` skill for full details):
- A Claude agent interviews the user via a multi-choice question modal (max 3 questions per series).
- A Gemini Flash model maintains a persistent user profile memory (skill level, history, preferences).
- The generation agent reads the user profile when creating personalized problems.
- Problem creation happens entirely in the backend — the frontend only handles the question modal UI and status polling.
