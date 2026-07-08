# Memory Event Naming Guide v0

This guide defines the first approved `source` and `type` names for memory
events written by frontend emitters, backend handlers, workers, and local smoke
scripts. It applies to `POST /api/v1/memory/events` and any server-side
`memory.Service.RecordEvent` callers.

The goal is consistency, not strict permanence. v0 names are plain text in
`memory_events.source` and `memory_events.type`, so the safest path is additive:
add new names when needed and keep readers tolerant of unknown older values.

## Event Shape

Every memory event has these fields:

| Field | Rule |
| --- | --- |
| `source` | Lowercase snake_case product surface or backend emitter. Use one approved value below. |
| `type` | Lowercase snake_case action within the source. Use one approved value for that source. |
| `summary` | One short human-readable sentence for debugging and profile refresh. No secrets or full user code. |
| `payload` | Optional compact JSON object with stable metadata. Prefer IDs, counts, tags, and booleans over raw content. |
| `occurred_at` | Optional RFC 3339 timestamp for when the user action happened. Omit when server time is good enough. |

## Approved Sources

| Source | Emitter | Purpose |
| --- | --- | --- |
| `generate` | Generate page or generation orchestration | Problem, MCQ, or interview prompt generation |
| `chat` | Floating chat or conversational interview practice | Coaching, interview dialogue, and user questions |
| `workspace` | Problem workspace, editor, test runner, submission flow | Attempts, test runs, and solved/failed signals |
| `mcq` | MCQ and marathon practice flows | Multiple-choice practice sessions |
| `memory` | Memory page, note UI, profile maintenance | Notes, profile refresh, and curation actions |
| `system` | Backend jobs, workers, smoke scripts | Operational checks and non-user automation |

Add a new source only when the emitting surface needs distinct summarization
rules. If the emitter is the same, prefer a new `type` under an existing source.

## Approved Types

### `generate`

| Type | When to Emit |
| --- | --- |
| `intake_started` | User opened the generate flow or submitted an initial prompt. |
| `clarifying_questions_answered` | User answered agent clarifying questions. |
| `problem_generated` | A coding problem package was produced. |
| `mcq_set_generated` | An MCQ set was produced. |
| `interview_prompt_generated` | A conversational interview prompt was produced. |
| `generation_failed` | Generation ended in a user-visible error. |

### `chat`

| Type | When to Emit |
| --- | --- |
| `thread_opened` | User opened the chat panel. |
| `message_sent` | User sent a message. |
| `assistant_replied` | Assistant response completed. |
| `thread_closed` | User dismissed or minimized chat for the session. |

### `workspace`

| Type | When to Emit |
| --- | --- |
| `problem_opened` | User opened a problem workspace. |
| `attempt_started` | User began editing or running code. |
| `tests_run` | User executed tests against an attempt. |
| `attempt_submitted` | User submitted a solution attempt. |
| `attempt_solved` | Hidden/reference checks mark the attempt as solved. |
| `attempt_failed` | A submitted attempt failed checks and is useful for growth edges. |

### `mcq`

| Type | When to Emit |
| --- | --- |
| `session_started` | User started an MCQ or marathon session. |
| `question_answered` | User answered one question. |
| `session_completed` | User finished the session. |
| `answer_incorrect` | User missed a question worth tracking for growth edges. |

### `memory`

| Type | When to Emit |
| --- | --- |
| `profile_viewed` | User opened the memory profile page. |
| `profile_refreshed` | Profile was rebuilt from events. |
| `note_created` | A problem-specific note was added. |
| `note_updated` | A note summary or tags changed. |
| `note_pruned` | A note was marked for pruning or removed. |

### `system`

| Type | When to Emit |
| --- | --- |
| `worker_profile_refreshed` | Background worker refreshed a profile. |
| `memory_api_checked` | Operational smoke test or API validation touched memory. |

## Example Payloads

### Generate: Problem Created

```json
{
  "source": "generate",
  "type": "problem_generated",
  "summary": "Generated a medium Go LRU cache problem.",
  "payload": {
    "format": "problem",
    "difficulty": "medium",
    "language": "go",
    "topic": "lru_cache",
    "problem_id": "prob_lru_01",
    "test_case_count": 6,
    "schema_version": 1
  }
}
```

### Chat: User Message

```json
{
  "source": "chat",
  "type": "message_sent",
  "summary": "Asked for help refactoring a sliding-window solution.",
  "payload": {
    "thread_id": "chat_sess_42",
    "message_length": 128,
    "topic": "sliding_window",
    "schema_version": 1
  }
}
```

### Workspace: Tests Run

```json
{
  "source": "workspace",
  "type": "tests_run",
  "summary": "Ran tests for two-sum attempt; 2 of 5 passed.",
  "payload": {
    "problem_id": "prob_two_sum_01",
    "language": "typescript",
    "passed": 2,
    "total": 5,
    "duration_ms": 840,
    "schema_version": 1
  }
}
```

### MCQ: Session Completed

```json
{
  "source": "mcq",
  "type": "session_completed",
  "summary": "Finished a 10-question API patterns marathon.",
  "payload": {
    "session_id": "mcq_sess_9",
    "question_count": 10,
    "correct_count": 7,
    "topic": "api_patterns",
    "schema_version": 1
  }
}
```

### Memory: Note Created

```json
{
  "source": "memory",
  "type": "note_created",
  "summary": "Saved a note to revisit topological sort cycle detection.",
  "payload": {
    "note_id": "note_topo_01",
    "problem_id": "prob_topo_03",
    "tags": ["graphs", "cycles"],
    "action": "review",
    "schema_version": 1
  }
}
```

### System: Smoke Test Checked Memory

```json
{
  "source": "system",
  "type": "memory_api_checked",
  "summary": "Backend smoke test verified memory event and profile APIs.",
  "payload": {
    "script": "backend-memory-smoke",
    "profile_fetched": true,
    "event_listed": true,
    "schema_version": 1
  }
}
```

## Payload Hygiene

Do not place these in `summary` or `payload` unless there is an explicit,
reviewed product reason:

- API keys, bearer tokens, cookies, Doppler secrets, Neon connection strings, or
  provider credentials
- Full user solution source code
- Large test logs or stack traces
- Raw chat transcripts
- Sensitive personally identifying information beyond the authenticated
  request scope

Prefer stable identifiers (`problem_id`, `session_id`, `thread_id`, `note_id`),
compact counts, booleans, tags, and coarse topics. When content length matters,
store length or token counts instead of the content itself.

## Versioning Rules

v0 is designed to evolve without a schema migration:

1. Add new `type` values instead of renaming persisted values.
2. Add new `source` values only for clearly distinct product surfaces.
3. Put `payload.schema_version` on new structured payloads when the shape may
   need to evolve.
4. Keep summarization tolerant of missing payload fields.
5. Unknown names should not break profile refresh. Runtime validation can reject
   new writes in strict mode, but historical readers should remain lenient.
6. Breaking semantic changes should get a future
   `memory-event-naming-guide-v1.md` rather than rewriting history.

## Frontend Contract

- Build frontend events through `frontend/src/shared/api/memoryEvents.ts`.
- Prefer the source-specific builders for generate, chat, workspace, MCQ,
  memory, and system flows.
- Add new names here and in the shared helper before using them in UI code.

## Related Docs

- [auth-identity-tenant.md](./auth-identity-tenant.md) documents request scoping
  for memory routes.
- [openapi-contract.md](./openapi-contract.md) documents the HTTP contract
  workflow.
- [../api/openapi.yaml](../api/openapi.yaml) defines the memory event request
  and response schemas.
