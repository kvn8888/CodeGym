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
| `hint_revealed` | User explicitly revealed a numbered coding hint; never include hint text. |
| `attempt_submitted` | User submitted a solution attempt. |
| `attempt_solved` | Hidden/reference checks mark the attempt as solved. |
| `attempt_failed` | A submitted attempt failed checks and is useful for growth edges. |

### `mcq`

| Type | When to Emit |
| --- | --- |
| `session_started` | User started an MCQ or marathon session. |
| `question_answered` | User correctly answered one single-select or multi-select question. |
| `question_skipped` | User deliberately skipped a question; this is neutral evidence, not an incorrect answer. Include `skipped: true` and `answer_revealed: true` because the UI shows the correct answer without crediting it to the learner. |
| `free_response_evaluated` | AI evaluation completed for one written response. Store only correctness, answer length, concept, timing, and provider metadata; never store the raw answer. |
| `session_completed` | User finished the session. |
| `session_exited` | User left an unfinished session after its durable state was saved. |
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

## Emitter Ownership Policy

Events may be emitted from frontend, backend, or both surfaces, but ownership
must be explicit per event type.

Default ownership split:

- Frontend owns user-intent and UI-interaction events.
  - Examples: `intake_started`, `clarifying_questions_answered`,
    `thread_opened`, `profile_viewed`.
- Backend owns authoritative outcomes and automation events.
  - Examples: `mcq_set_generated`, `generation_failed`, `note_created`,
    `note_updated`, `note_pruned`, `worker_profile_refreshed`.

When one side can own both:

- Backend-only flow: when the action is server-triggered or server-truth
  (workers, maintenance, model results, retries, failures after async work).
- Frontend-only flow: when the action is UI-only and no backend endpoint is
  involved (open/close panels, local UX milestones).

Dual-surface emission is allowed only when the two events represent different
stages (for example, user intent vs backend outcome). Do not emit the same
semantic event twice for the same lifecycle stage.

## Naming Governance

The naming system is intentionally simple and robust:

1. This document is the policy reference for approved source and type names.
2. `api/memory-events.registry.json` is the canonical machine-readable
  registry used to generate code artifacts.
3. `frontend/src/shared/api/memoryEvents.ts` and
  `backend/internal/memory/event_names.go` are generated from that registry and
  should not be hand-edited.
4. Backend emitters and frontend emitters should use those generated names.
5. Additive evolution only: add names, avoid renaming persisted historical
   names.
6. If a semantic break is unavoidable, create a new v1 guide rather than
   rewriting v0 data.

Generation commands:

- `npm run memory-events:generate`
- `npm run memory-events:check`

### How the Generator Works

`scripts/generate-memory-event-registry.mjs` is the code generator for memory
event naming artifacts.

Its job is to read the canonical registry in
`api/memory-events.registry.json` and produce the language-specific enforcement
files used by the app:

- `frontend/src/shared/api/memoryEvents.ts`
- `backend/internal/memory/event_names.go`

What it generates:

- Frontend source constants and type constants
- Frontend source-specific event builders
- Frontend compatibility aliases when older caller names still need to map to
  approved names
- Backend source constants and type constants
- Backend source-to-type allow-list map
- Backend `ValidateEventName(...)` helper

Operational modes:

- Normal mode: writes the generated files to disk.
- `--check` mode: compares expected generated output against the checked-in
  files and fails if they are out of date.

Expected workflow:

1. Edit `api/memory-events.registry.json`.
2. Run `npm run memory-events:generate`.
3. Review the generated diffs in frontend and backend.
4. Run `npm run memory-events:check` in CI or before merge.

The `.mjs` file is not itself the runtime enforcement layer. It is the build
step that produces the enforcement-layer files for TypeScript and Go from one
shared registry.

## Justification

Why it is justified here:

1. These event names are not cosmetic. They drive memory/profile derivation and
  eventually shape LLM behavior.
2. Naming drift would silently degrade personalization rather than fail loudly.
3. You have both frontend and backend emitters, which is exactly where
  stringly-typed conventions rot if left informal.
4. A single registry plus generated artifacts is a pragmatic middle ground:
  stronger than docs-only, much lighter than a full schema platform.

Why it is not too heavy:

1. The registry is small.
2. The generator is simple.
3. The runtime behavior barely changed.
4. Review burden goes down because people edit one canonical file instead of
  remembering two systems.

Review checklist for any new event:

- Is ownership clear (frontend, backend, or staged split)?
- Is the name listed in this guide before merge?
- Is `frontend/src/shared/api/memoryEvents.ts` updated if frontend emits it?
- Is payload compact and safe per Payload Hygiene?

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

- Build frontend events through the generated helper in
  `frontend/src/shared/api/memoryEvents.ts`.
- Prefer the source-specific builders for generate, chat, workspace, MCQ,
  memory, and system flows.
- Add new names in `api/memory-events.registry.json`, regenerate artifacts, and
  then use them in UI code.

### Recent Activity Curation (`includeTypes` / `excludeTypes`)

The Memory page's compact "Recent Activity" rail is intentionally curated and
does not automatically show every recorded event type.

Scope and jurisdiction:

- Event storage scope: **all valid events** are still recorded in
  `memory_events` and available in full event views.
- UI curation scope: the compact "Recent Activity" list in
  `frontend/src/features/memory/MemoryRecentActivityWing.tsx` is filtered by
  `includeTypes` and `excludeTypes`.
- Conflict rule: if a type appears in both lists, `excludeTypes` wins.

Practical intent:

- `includeTypes` defines high-signal event outcomes that are eligible for the
  short feed.
- `excludeTypes` removes noisy or low-value timeline items from that short feed.
- This curation affects only presentation, not event validity, persistence, or
  profile derivation inputs.

Recommended maintenance workflow:

1. Add/adjust event names in `api/memory-events.registry.json`.
2. Regenerate artifacts (`npm run memory-events:generate`).
3. Decide whether the new type should appear in compact Recent Activity.
4. If yes, add it to `includeTypes`; if no, add it to `excludeTypes` or leave
   unlisted.
5. Verify behavior on Memory page:
   - compact "Recent Activity" remains concise,
   - "All memory events" still shows the full stream.

Design guideline:

- Prefer outcomes over intermediate steps in compact Recent Activity.
- Example: include `session_completed`; usually exclude `session_started`.

## Related Docs

- [auth-identity-workspace.md](./auth-identity-workspace.md) documents request scoping
  for memory routes.
- [openapi-contract.md](./openapi-contract.md) documents the HTTP contract
  workflow.
- [../api/openapi.yaml](../api/openapi.yaml) defines the memory event request
  and response schemas.
