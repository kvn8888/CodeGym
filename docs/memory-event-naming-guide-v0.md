# Memory Event Naming Guide v0

This guide defines the first canonical `source` and `type` names for memory
events written by the Go backend and frontend emitters. It applies to
`POST /api/v1/memory/events` and any server-side `memory.Service.RecordEvent`
callers.

## Goals

- Keep event names predictable before multiple product surfaces start writing to
  `memory_events`.
- Make summarization and profile refresh logic easy to reason about.
- Leave room for additive evolution without a schema migration.

## Field contract

Every event requires:

| Field | Rules |
| --- | --- |
| `source` | Lowercase snake_case product surface that emitted the event. Must be one of the approved values below. |
| `type` | Lowercase snake_case action name within the source. Must be one of the approved values for that source. |
| `summary` | One short human-readable sentence for profile refresh and debugging. No secrets or full user code. |
| `payload` | Optional JSON object with structured metadata. Keep small and stable. |
| `occurred_at` | Optional RFC 3339 timestamp. Omit to let the server stamp `created_at`. |

Rejected shapes return `400 invalid_memory_event` from the API.

## Approved sources

| Source | Emitted by | Purpose |
| --- | --- | --- |
| `generate` | Generate page / generation orchestration | Problem, MCQ, or interview prompt creation |
| `chat` | Floating chat / conversational practice | Free-form coaching and interview dialogue |
| `workspace` | Problem workspace / editor | Attempts, runs, and submissions against a generated problem |
| `mcq` | MCQ and marathon practice flows | Multiple-choice practice sessions |
| `memory` | Memory page and profile maintenance | Notes, profile refresh, and curation actions |
| `system` | Backend jobs and operational scripts | Smoke tests, workers, and non-user automation |

Add a new source only when a product surface is distinct enough to deserve its
own summarization rules. Prefer new `type` values inside an existing source when
the emitter is the same.

## Approved types by source

### `generate`

| Type | When to emit |
| --- | --- |
| `intake_started` | User opened the generate flow or submitted an initial prompt |
| `clarifying_questions_answered` | User answered agent clarifying questions |
| `problem_generated` | A coding problem package was produced |
| `mcq_set_generated` | An MCQ set was produced |
| `interview_prompt_generated` | A conversational interview prompt was produced |
| `generation_failed` | Generation ended in a user-visible error |

### `chat`

| Type | When to emit |
| --- | --- |
| `thread_opened` | User opened the chat panel |
| `message_sent` | User sent a message |
| `assistant_replied` | Assistant response completed |
| `thread_closed` | User dismissed or minimized chat for the session |

### `workspace`

| Type | When to emit |
| --- | --- |
| `problem_opened` | User opened a problem workspace |
| `attempt_started` | User began editing or running code |
| `tests_run` | User executed tests against their attempt |
| `attempt_submitted` | User submitted a solution attempt |
| `attempt_solved` | Hidden/reference checks mark the attempt as solved |

### `mcq`

| Type | When to emit |
| --- | --- |
| `session_started` | User started an MCQ or marathon session |
| `question_answered` | User answered one question |
| `session_completed` | User finished the session |
| `answer_incorrect` | User missed a question worth tracking for growth edges |

### `memory`

| Type | When to emit |
| --- | --- |
| `profile_viewed` | User opened the memory profile page |
| `profile_refreshed` | Profile was rebuilt from events |
| `note_created` | A problem-specific note was added |
| `note_updated` | A note summary or tags changed |
| `note_pruned` | A note was marked for pruning or removed |

### `system`

| Type | When to emit |
| --- | --- |
| `worker_profile_refreshed` | Background worker refreshed a profile |
| `memory_api_checked` | Operational smoke test or health validation |

## Example payloads

### Generate — problem created

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
    "test_case_count": 6
  }
}
```

### Chat — user message

```json
{
  "source": "chat",
  "type": "message_sent",
  "summary": "Asked for help refactoring a sliding-window solution.",
  "payload": {
    "thread_id": "chat_sess_42",
    "message_length": 128,
    "topic": "sliding_window"
  }
}
```

### Workspace — tests run

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
    "duration_ms": 840
  }
}
```

### MCQ — session completed

```json
{
  "source": "mcq",
  "type": "session_completed",
  "summary": "Finished a 10-question API patterns marathon.",
  "payload": {
    "session_id": "mcq_sess_9",
    "question_count": 10,
    "correct_count": 7,
    "topic": "api_patterns"
  }
}
```

### Memory — note created

```json
{
  "source": "memory",
  "type": "note_created",
  "summary": "Saved a note to revisit topological sort cycle detection.",
  "payload": {
    "note_id": "note_topo_01",
    "problem_id": "prob_topo_03",
    "tags": ["graphs", "cycles"],
    "action": "review"
  }
}
```

## Payload hygiene

Do **not** place these in `summary` or `payload` unless there is an explicit,
reviewed reason:

- API keys, bearer tokens, cookies, or Doppler secrets
- Full user solution source code or large test logs
- Raw chat transcripts (store IDs, lengths, and topics instead)
- Personally identifying information beyond the authenticated `user_id` already
  implied by the request scope

Prefer stable identifiers (`problem_id`, `session_id`, `thread_id`) and compact
counts over verbatim content.

## Versioning without migrations

v0 names are plain text in `memory_events.source` and `memory_events.type`. To
evolve safely:

1. Add new `type` values; do not rename existing persisted values.
2. If a payload shape changes, add a `payload.schema_version` integer and keep
   readers tolerant of missing fields.
3. Record breaking semantic changes in a future `memory-event-naming-guide-v1.md`
   rather than rewriting historical rows.
4. Summarization code should ignore unknown `type` values instead of failing the
   refresh path.

## Related docs

- [auth-identity-tenant.md](./auth-identity-tenant.md) — request scoping for
  memory routes
- [openapi-contract.md](./openapi-contract.md) — HTTP contract for memory APIs
- [../api/openapi.yaml](../api/openapi.yaml) — `RecordMemoryEventInput` schema