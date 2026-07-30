# Backend API Contract

CodeGym uses OpenAPI as the frontend/backend contract, not as a full backend
framework replacement.

## Source of Truth

- Contract: `api/openapi.yaml`
- Generated frontend types: `frontend/src/shared/api/openapi.d.ts`
- Backend implementation: `backend/internal/api`

The contract describes the HTTP surface that the frontend can call. Keep the Go
handlers simple until the API stabilizes; backend server generation can be
introduced later with `oapi-codegen` if the contract starts drifting.

## Current Scope

Base URL in local development is usually `http://127.0.0.1:8080`. When the
frontend uses the Vite proxy, it can call the same endpoints relative to the
browser origin.

Current routes:

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `GET` | `/health` | No | Lightweight liveness check. |
| `GET` | `/ready` | No | Readiness check; pings Postgres when a database URL is configured. |
| `GET` | `/api/v1/memory/profile` | Yes | Fetch the scoped user's derived memory profile. |
| `POST` | `/api/v1/memory/profile/refresh` | Yes | Manually synthesize and persist the scoped user's profile from bounded event evidence. |
| `POST` | `/api/v1/memory/profile/maintain` | Yes | Run set-completion profile synthesis before the next generated set. |
| `POST` | `/api/v1/practice-intakes` | Yes | Prepare, resume, or explicitly restart a normalized topic baseline. |
| `GET` | `/api/v1/practice-intakes?status=pending&limit=1` | Yes | Restore the newest unfinished baseline. |
| `PATCH` | `/api/v1/practice-intakes/{id}` | Yes | Save partial answers or mark the baseline completed/skipped. |
| `POST` | `/api/v1/generate` | Yes | Generate a validated single-select, multi-select, free-response, or mixed MCQ set. |
| `POST` | `/api/v1/mcq/evaluate` | Yes | Evaluate one free-response answer against its reference answer and rubric. |
| `GET` | `/api/v1/memory/events` | Yes | List append-only memory events newest-first with explicit UTC timestamps. |
| `POST` | `/api/v1/memory/events` | Yes | Record one append-only memory event. |
| `GET` | `/api/v1/sessions` | Yes | List resumable practice session summaries. |
| `POST` | `/api/v1/sessions` | Yes | Create a generated-problem, MCQ, or interview practice session. |
| `GET` | `/api/v1/sessions/{id}` | Yes | Fetch one full session state for resume. |
| `PATCH` | `/api/v1/sessions/{id}` | Yes | Update title, status, or state snapshot. |
| `PUT` | `/api/v1/sessions/{id}/files` | Yes | Batch upsert draft workspace files. |
| `POST` | `/api/v1/chat/threads` | Yes | Create/resume a scoped Interview or contextual-coach thread. |
| `GET` | `/api/v1/chat/threads` | Yes | Find threads by kind, status, and practice session. |
| `GET` | `/api/v1/chat/threads/{id}/messages` | Yes | Restore the append-only transcript. |
| `POST` | `/api/v1/chat/threads/{id}/turns` | Yes | Send an idempotent user turn and receive `meta`, `delta`, `complete`, or `error` SSE events. |
| `POST` | `/api/v1/workflow-operations` | Yes | Seed durable backend-owned steps for a long-running AI operation. |
| `GET` | `/api/v1/workflow-operations/{id}/events` | Yes | Replay and follow ordered `progress` SSE events from `Last-Event-ID` or `after`. |
| `POST` | `/api/v1/workflow-operations/{id}/cancel` | Yes | Terminate an unfinished scoped workflow operation. |
| `POST` | `/api/v1/chat/threads/{id}/reset` | Yes | Close a thread and create a clean successor. |
| `POST` | `/api/v1/chat/threads/{id}/finish` | Yes | Complete an interview, save its coarse assessment, and refresh memory. |
| `POST` | `/api/v1/chat/threads/{id}/exit` | Yes | Record an interview exit while retaining resume state. |

## Auth and Workspace Scope

Protected routes require:

```http
Authorization: Bearer <token>
```

For local development without Auth0, the token can use this dev format:

```http
Authorization: Bearer dev:<user-id>:<workspace-id>
```

Example:

```http
Authorization: Bearer dev:kevin:personal-dev
```

If `CODEGYM_DEV_AUTH_TOKEN` is configured, use that exact token instead. The
backend maps it to `CODEGYM_DEV_USER_ID` and `CODEGYM_DEV_WORKSPACE_ID`.

`X-CodeGym-Workspace-ID` is an optional legacy/internal workspace-scope override.
Product flows should omit it so the backend uses the authenticated user's
default personal workspace. If the header is present and the authenticated
principal is not allowed to use that scope, the backend returns `403`.

## Response Envelope

Every JSON response uses the same envelope:

```json
{
  "data": {},
  "error": null
}
```

Error responses set `data` to `null` and include a stable code plus a safe
message:

```json
{
  "data": null,
  "error": {
    "code": "unauthenticated",
    "message": "Missing bearer token."
  }
}
```

## Memory Payloads

Create event request:

```json
{
  "source": "system",
  "type": "memory_api_checked",
  "summary": "Backend smoke test verified memory event and profile APIs.",
  "payload": {
    "script": "backend-memory-smoke",
    "schema_version": 1
  },
  "occurred_at": "2026-06-24T12:30:00Z"
}
```

`source` and `type` should follow
[memory-event-naming-guide-v0.md](./memory-event-naming-guide-v0.md). Keep
`summary` short and do not place bearer tokens, API keys, connection strings,
full user source code, or raw transcripts in either `summary` or `payload`.

Create event response data:

```json
{
  "id": "mem_evt_0123456789abcdef",
  "workspace_id": "personal-dev",
  "user_id": "kevin",
  "source": "system",
  "type": "memory_api_checked",
  "summary": "Backend smoke test verified memory event and profile APIs.",
  "payload": {
    "script": "backend-memory-smoke",
    "schema_version": 1
  },
  "occurred_at": "2026-06-24T12:30:00Z",
  "created_at": "2026-06-24T12:30:01Z"
}
```

Profile response data includes `summary`, `updated_at`, `next_review_at`,
`strengths`, `growth_edges`, `skills`, `notes`, and optional synthesis
`provenance`. The same validated synthesis operation serves manual refresh,
set-completion refresh, and the scheduled worker. `/memory/notes/maintain`
remains a deprecated compatibility alias for `/memory/profile/maintain`.
Daily refresh stores a stable `provenance.evidence_digest`; when the filtered
learning evidence is unchanged, the worker skips the model call and leaves the
existing profile and timestamps untouched.

## Curl Examples

Set local shell variables:

```bash
API_BASE_URL=http://127.0.0.1:8080
TOKEN=dev:kevin:personal-dev
```

Liveness:

```bash
curl -sS "$API_BASE_URL/health"
```

Readiness:

```bash
curl -sS "$API_BASE_URL/ready"
```

Create a memory event:

```bash
curl -sS -X POST "$API_BASE_URL/api/v1/memory/events" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "source": "system",
    "type": "memory_api_checked",
    "summary": "Backend smoke test verified memory event and profile APIs.",
    "payload": {
      "script": "manual-curl",
      "schema_version": 1
    }
  }'
```

List memory events:

```bash
curl -sS "$API_BASE_URL/api/v1/memory/events" \
  -H "Authorization: Bearer $TOKEN"
```

Fetch profile:

```bash
curl -sS "$API_BASE_URL/api/v1/memory/profile" \
  -H "Authorization: Bearer $TOKEN"
```

Refresh profile from events:

```bash
curl -sS -X POST "$API_BASE_URL/api/v1/memory/profile/refresh" \
  -H "Authorization: Bearer $TOKEN"
```

Create and resume a workspace session:

```bash
curl -sS -X POST "$API_BASE_URL/api/v1/sessions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "kind": "workspace",
    "title": "Graph traversal practice",
    "problem_id": "prob_graph",
    "state": {
      "schema_version": 1,
      "language": "go",
      "active_file": "main.go"
    }
  }'
```

List active sessions:

```bash
curl -sS "$API_BASE_URL/api/v1/sessions?status=active&limit=5" \
  -H "Authorization: Bearer $TOKEN"
```

## Doppler and Neon

Run the backend with Doppler so it can read the Neon connection string and auth
configuration:

```bash
cd backend
doppler run -p codegym -c dev -- go run ./cmd/server
```

In another terminal, run the smoke check:

```bash
cd backend
./scripts/memory_smoke.sh
```

The script calls `/ready`, records one `system.memory_api_checked` event, checks
that `GET /api/v1/memory/events` returns the created ID, and fetches
`GET /api/v1/memory/profile`. It exits non-zero on the first failed step.

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
