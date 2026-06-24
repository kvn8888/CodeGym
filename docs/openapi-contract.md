# OpenAPI Contract Scaffold

CodeGym uses OpenAPI as the frontend/backend contract, not as a full backend
framework replacement.

## Source of Truth

- Contract: `api/openapi.yaml`
- Generated frontend types: `frontend/src/shared/api/openapi.d.ts`
- Backend implementation: `backend/internal/api`

The contract should describe the HTTP surface that the frontend can call. Keep
the Go handlers simple until the API stabilizes; backend server generation can
be introduced later with `oapi-codegen` if the contract starts drifting.

## Current Scope

The first contract covers the existing backend:

- `GET /health`
- `GET /api/v1/memory/profile`
- `POST /api/v1/memory/profile/refresh`
- `GET /api/v1/memory/events`
- `POST /api/v1/memory/events`

The spec also documents:

- `Authorization: Bearer ...`
- optional `X-CodeGym-Tenant-ID`
- the backend response envelope: `{ data, error }`

## Local Workflow

From `frontend/`:

```bash
npm run api:types
npm run api:lint
```

`api:types` regenerates TypeScript contract types. `api:lint` validates the
OpenAPI document shape and style.

## Integration Notes

- Do not hand-edit generated files.
- When changing backend JSON shape, update `api/openapi.yaml` in the same task.
- Prefer frontend code importing generated API types instead of redefining DTOs
  in `frontend/src/shared/api/types.ts`.
- Once API endpoints become broader than memory/profile, add tags by product
  area (`memory`, `generation`, `problems`, `submissions`, `chat`).
