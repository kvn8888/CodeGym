# Session History and Resume Data Model v0

Spike output for [#69](https://github.com/kvn8888/CodeGym/issues/69). Plans how
session history and resume work across generated problems, MCQ sessions, and
conversational interview practice. Interview persistence details stay gated on
the [#68](https://github.com/kvn8888/CodeGym/issues/68) scope spike.

## Session vs Memory Event vs Saved Problem Attempt

| Concept | What it is | Where it lives | Mutability |
| --- | --- | --- | --- |
| Practice session | One bounded practice activity (a generated-problem workspace, an MCQ run, an interview conversation). The unit of history and resume. | `practice_sessions` (Postgres) | Mutable: status transitions, state snapshot updates |
| Memory event | Compact learning signal for profile summarization only. Never read for resume. | `memory_events` (existing) | Append-only |
| Saved problem attempt | The content of the user's work: draft code files and, later, submission results. | `session_files` (drafts) plus future submission tables | Draft files upsert; submissions append |

Rule of thumb: if losing the data means the user cannot pick up where they left
off, it is product state in Postgres. If losing it only degrades
personalization quality, it is a memory event. This follows directly from the
payload hygiene rules in
[memory-event-naming-guide-v0.md](./memory-event-naming-guide-v0.md): memory
events must not carry full solution code or raw chat transcripts, so resume
data can never live there.

## Durable Data Model (Postgres)

One polymorphic sessions table plus a files table for workspace drafts. Kinds
share history-list and resume semantics; they differ only in the shape of the
`state` snapshot, so per-kind tables would be three near-identical stores for
the Week 7 slice. Escape hatch: promote a kind to its own table when its state
stops fitting a snapshot (interview transcripts already do not — see below).

```sql
CREATE TABLE practice_sessions (
    id text PRIMARY KEY,                     -- sess_<ulid>
    workspace_id text NOT NULL,                 -- personal workspace scope, see auth-identity-workspace.md
    user_id text NOT NULL,
    kind text NOT NULL,                      -- 'workspace' | 'mcq' | 'interview'
    status text NOT NULL DEFAULT 'active',   -- 'active' | 'completed' | 'abandoned'
    title text NOT NULL DEFAULT '',          -- human label for history lists
    problem_id text,                         -- generated problem, when kind = 'workspace'
    generation_job_id text,                  -- provenance link to the generation run
    state jsonb NOT NULL DEFAULT '{}',       -- kind-specific resume snapshot
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_activity_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    FOREIGN KEY (workspace_id, user_id)
        REFERENCES workspace_memberships (workspace_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_practice_sessions_scope_activity
    ON practice_sessions (workspace_id, user_id, last_activity_at DESC);
CREATE INDEX idx_practice_sessions_scope_active
    ON practice_sessions (workspace_id, user_id) WHERE status = 'active';
```

```sql
-- Draft code for workspace sessions; the main resume payload.
CREATE TABLE session_files (
    session_id text NOT NULL REFERENCES practice_sessions (id) ON DELETE CASCADE,
    file_path text NOT NULL,
    content text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, file_path)
);
```

Schema management follows the existing memory-store pattern: idempotent
`EnsureSchema` statements with scope FKs to `workspace_memberships`, matching
`backend/internal/memory/postgres_store.go`.

### State Snapshot Shapes

Every snapshot carries `schema_version` (same versioning stance as the memory
naming guide: additive, readers tolerant of missing fields).

- `workspace`: editor context, not the code itself.

  ```json
  { "schema_version": 1, "language": "go", "active_file": "main.go",
    "last_run": { "passed": 2, "total": 5 } }
  ```

- `mcq`: full progress inline — answer and skip records are small.

  ```json
  { "schema_version": 1, "question_index": 4, "selected_index": null,
    "confirmed": false,
    "results": [ { "questionId": "q1", "selectedIndex": 2, "correct": true } ],
    "skipped_questions": [ { "questionId": "q2", "round": 1 } ] }
  ```

  The active question index, timer, draft selection or written response,
  completed AI evaluation, generated questions, answer
  results, and skips are persisted so the run can reopen exactly where it left
  off. A skip is continuity data in the session snapshot (separate from answer
  results for UI), and memory records it as a neutral `question_skipped` event.

- `interview`: pointer state only; the transcript needs an append-only
  `session_messages` table (`id`, `session_id`, `role`, `content`,
  `created_at`). Sketched here, **not built until #68 selects a slice**.

### What Stays Out of This Model

- Submission results and submitted code get their own tables when the
  execution gateway lands (port of the v1 `submissions` / `submission_files`
  shapes); a submission will reference its `session_id`.
- Generated problem content itself belongs to the generation epic, referenced
  by `problem_id` / `generation_job_id`, never copied into session state.

## Memory Profile vs Product State

Profile summarization reads only `memory_events`; the sessions API never reads
`memory_events`. Sessions and events are linked one way: events carry
`payload.session_id` (already the guide's recommendation to prefer stable
identifiers).

| Data | Home | Why |
| --- | --- | --- |
| Draft code, chat transcript, MCQ progress | Session tables | Needed byte-for-byte to resume; banned from event payloads |
| Solved/failed outcomes, counts, topics, durations | Memory events | Compact signals the summarizer wants |
| History list the user browses | Session tables | Product state; must not depend on profile refresh |

No new event sources or types are needed for v0. Emission mapping uses
existing approved names from the naming guide:

| Session moment | Event |
| --- | --- |
| Workspace session created/opened | `workspace.problem_opened`, then `workspace.attempt_started` |
| Workspace resumed later | Same events with `"resumed": true` in the payload (payload flag instead of new type) |
| MCQ session created / question / done | `mcq.session_started` / `mcq.question_answered` / `mcq.session_completed` |
| Interview opened / messages | `chat.thread_opened` etc., pending #68 |

## Minimal API Routes

All under the protected `/api/v1/` middleware chain
([auth-identity-workspace.md](./auth-identity-workspace.md)), documented in
[../api/openapi.yaml](../api/openapi.yaml) per the
[openapi-contract.md](./openapi-contract.md) workflow.

```text
GET    /api/v1/sessions                     ?kind=&status=&limit=&cursor=
                                            history list; summaries only, no state/files
POST   /api/v1/sessions                     { kind, problem_id?, title?, state? }
GET    /api/v1/sessions/{id}                full state for resume (workspace responses include files)
PATCH  /api/v1/sessions/{id}                { state?, status?, title? } — autosave + transitions
PUT    /api/v1/sessions/{id}/files          batch upsert of draft files (workspace autosave)
```

Frontend consumers:

- Dashboard "continue where you left off" card: `GET /sessions?status=active&limit=5`
- History page: paginated `GET /sessions`
- Workspace / MCQ resume: `GET /sessions/{id}` on entry
- Autosave: client-debounced (a few seconds idle) `PATCH` + `PUT files`; the
  server just upserts and bumps `last_activity_at`

## Open Product Decisions

- **Abandonment**: v0 proposal is no TTL sweep — a session stays `active` until
  completed or explicitly replaced. Revisit if the active list gets noisy.
- **One active workspace session per problem?** Proposal: on opening a problem,
  reuse the existing active session for that `(user, problem_id)` instead of
  creating a duplicate.
- **Interview persistence** is gated on #68; only the table sketch above exists
  until that spike lands.

## Selected Week 6/7 Slice

Week 6 already has the frontend shells (#61–#67, mock data). The Week 7 slice
implements the backend and wires history/resume:

1. Practice sessions schema and store (backend)
2. Sessions API routes and OpenAPI contract
3. Frontend session history and resume wiring (typed client, dashboard card,
   resume into the #63 workspace and #66 MCQ shells)

Implementation issues are linked from #69.
