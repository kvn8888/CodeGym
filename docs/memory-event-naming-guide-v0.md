# Memory Event Naming Guide v0

This guide defines the canonical event vocabulary for the frontend and backend memory pipeline.

Keep these names centralized. Do not inline ad hoc `source` or `type` strings in product code.

## Approved Sources

- `generate` - problem generation and problem-selection flows
- `chat` - chat and coaching interactions
- `workspace` - code execution, submissions, and workspace activity
- `mcq` - multiple-choice question flows
- `memory` - memory-note and profile maintenance flows
- `system` - platform-level refresh, sync, and maintenance events

## Approved Types Used Today

- `problem_requested`
- `problem_generated`
- `problem_attempted`
- `attempt_passed`
- `attempt_failed`
- `answer_wrong`
- `memory_note_created`

## Frontend Contract

- Build events through `frontend/src/shared/api/memoryEvents.ts`.
- Prefer the source-specific builders for generate, chat, workspace, MCQ, memory, and system flows.
- Add new names here and in the shared helper before using them in UI code.
