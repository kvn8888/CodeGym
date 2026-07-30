# Deterministic workflow progress

CodeGym reports observable backend workflow steps for long-running AI operations.
This is product telemetry, not model reasoning: the stream never contains chain
of thought, prompts, source code, retrieved records, secrets, or stack traces.

## Contract

Create a scoped operation before the long-running request:

```http
POST /api/v1/workflow-operations
Authorization: Bearer …
Content-Type: application/json

{"kind":"mcq_generation"}
```

Supported kinds are `mcq_generation`, `memory_reflection`, and
`mcq_next_round`. The response contains the operation plus backend-owned queued
steps. Pass its `operation.id` as `operation_id` to `POST /api/v1/generate` or
`POST /api/v1/memory/profile/maintain`.

Follow progress with:

```http
GET /api/v1/workflow-operations/{id}/events
Accept: text/event-stream
Authorization: Bearer …
Last-Event-ID: 7
```

Each `progress` event has this stable envelope:

```json
{
  "operation_id": "workflow_…",
  "sequence": 8,
  "step_id": "validate_questions",
  "label": "Validate question set",
  "status": "running",
  "timestamp": "2026-07-30T12:00:00Z",
  "metadata": {
    "attempt": 1,
    "max_attempts": 2
  }
}
```

`sequence` is allocated transactionally and increases within one operation.
Reconnect with `Last-Event-ID` or `?after=`; the server replays only higher
sequences in order. The final event has `metadata.terminal=true`, and then the
stream closes.

## Scope and persistence

`workflow_operations` is keyed to one workspace/user membership.
`workflow_progress_events` is append-only and cascades with the operation.
Every create, attach, list, stream, and cancel read includes both scope keys, so
knowing an operation ID does not grant access to another account or workspace.

Postgres assigns a new sequence while holding the operation row lock. The
in-memory store uses the same contract for local development and unit tests.
SSE handlers poll the durable event log rather than relying on process-local
pub/sub, so reconnect and multi-instance delivery read the same source of truth.

## Workflow ownership

The backend owns step identifiers, labels, ordering, and status transitions.
Generation and profile-synthesis code reports only steps it actually begins or
finishes:

- MCQ: load personalization, generate, validate, optional bounded repair, ready.
- Memory: load evidence, synthesize, validate, persist, ready.
- Next round: all memory steps must succeed before the backend accepts the
  generation request on the same operation.

Intermediate validation failure can be followed by the single allowed repair.
Only an event marked terminal completes the operation.

Metadata is an allowlist of bounded scalar values such as attempt counts,
question counts, safe reason codes, and retryability. Arbitrary keys and nested
records are rejected.

## Client behavior

The Marathon client creates the operation, opens the authenticated stream, sends
the operation ID with the long request, de-duplicates by sequence, reconnects
from its latest cursor, and cancels unfinished operations when abandoned.
Memory reflection remains ahead of next-round generation.

If operation creation or SSE is unavailable, the core request still runs. The
UI switches to a truthful non-streaming fallback that reports only the overall
request state and does not fabricate individual backend steps.
