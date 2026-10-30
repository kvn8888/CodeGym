# Memory Profile Maintenance

This document describes how CodeGym maintains the durable memory profile from
append-only learning events. It covers notes-first MCQ and coding refreshes,
shared profile-refresh paths, durable deduplication, submit-token semantics, and
stale-write protection.

## Scope

The profile is synthesized from deterministic memory events into the summary,
focus areas, skills, notes, and provenance used by the Memory page and future
generation. Completed MCQ rounds and finalized graded coding submissions use
notes-first maintenance: note CRUD is computed before profile synthesis, then
notes and profile are saved together in one version-guarded write.

Daily and interview refreshes use the shared profile synthesizer, but they do not
run notes-first CRUD. They preserve existing notes while using the same bounded
evidence and stale-write protections.

## Caller Paths

- **MCQ completion.** The frontend records MCQ outcome events, records
  `session_completed`, flushes pending memory events, then calls
  `POST /api/v1/memory/profile/maintain` with the completed round `session_id`.
- **Coding completion.** A `mode:"submit"` submission that is graded pass or fail
  records attempt evidence and calls the profile synthesizer with
  `session_id`, `problem_id`, and `submission_id`. Run mode, sandbox/platform
  faults, and non-terminal submissions never trigger maintenance.
- **Coding retry.** The client retries the same completion by calling
  `POST /api/v1/memory/profile/maintain` with the exact
  `session_id` + `problem_id` + `submission_id`. Retries are idempotent and never
  re-execute learner code.
- **Daily refresh.** Runs through the profile synthesizer with trigger `daily`
  and no focused `session_id`. It skips model work when the durable evidence
  digest is unchanged.
- **Interview refresh.** Uses the shared synthesizer without note CRUD.

`POST /api/v1/memory/notes/maintain` is a deprecated compatibility alias for
full profile maintenance.

## Completion Gates

Notes-first maintenance runs only when a completed assessment is present.

MCQ gate:

- The maintain request includes a non-empty `session_id`.
- The evidence contains an MCQ `session_completed` event whose payload
  `session_id` matches the request `session_id`.
- The evidence contains at least one matching MCQ outcome event for the same
  `session_id`.

Coding gate (`submission_id` present):

- The request includes non-empty `session_id` and `problem_id`.
- Evidence contains a `workspace` attempt event for the same `submission_id`
  whose `session_id` and `problem_id` also match.
- There is exactly one graded outcome event (`attempt_solved` or
  `attempt_failed`) with `graded=true`, consistent pass/fail counts, and a valid
  `assistance_status` (`used`, `not_used`, or `unknown`).

Missing or ambiguous evidence returns `409 memory_completion_evidence_missing`
and leaves the previous profile intact.

## Notes-First Save Flow

Both notes-first paths run two model stages:

1. The note stage receives bounded event evidence and the current profile, then
   returns note actions: `create`, `update`, or `prune`.
2. The server validates the actions and applies them to candidate notes in
   memory only. Coding notes must carry a `problem_id` matching the completed
   problem; maintained note identities are updated, carried forward, or pruned
   rather than duplicated.
3. The summary/profile stage runs.
   - MCQ: the profile stage receives the bounded event evidence plus the profile
     context containing candidate notes.
   - Coding: the profile stage receives only `{version, maintained_notes}`. Raw
     event evidence is not sent to the coding summary model.
4. The server validates the complete profile candidate.
5. The server saves summary, focus areas, skills, notes, and provenance (including
   the maintained-note snapshot) in one version-checked profile write.

If note generation, profile generation, or validation fails, the existing saved
profile remains unchanged. Candidate notes are never persisted alone.
Sandbox or provider failures are not recorded as learner misconceptions.

## Provenance And Snapshots

Each successful refresh stores provenance with the refresh identifier and
attempt, trigger, input/output profile versions, provider/model, timestamps,
event references, evidence digest, stage prompt/schema revisions, completion
identity, workflow operation ID when available, and the exact maintained-note
snapshot supplied to the summary stage.

Provenance and saved notes are scoped to the authenticated workspace/user. Raw
code, hidden tests, session secrets, and raw coaching/chat text are never part of
note-stage or coding profile-stage model input, saved profiles, or provenance.

## Durable Deduplication

Coding completions are deduplicated by the durable completion identity
`(workspace_id, user_id, session_id, problem_id, submission_id)` in
`memory_profile_completions`. The completion marker is written in the same
transaction as the profile update.

- Delivering the same logical completion again does not count evidence twice and
  does not call the model again.
- A genuinely new submission has a new `submission_id`, so it remains distinct
  even with the same outcome.
- An uncertain retry of the same logical submission reuses the same
  `submission_id` and is therefore deduplicated.
- If the marker write fails, the profile save rolls back with it.

Postgres is the durable source of this dedupe; the in-memory store mirrors the
same contract for tests.

## Submit Tokens

Clients may send `client_token` on `POST /api/v1/submissions`. The backend
reserves the token under the authenticated workspace/user scope with a
fingerprint of `mode`, `problem_id`, `session_id`, and submitted files.

- Replaying the same token and fingerprint returns the original
  `submission_id`; it never starts a second execution.
- Reusing the token with different content returns `409
  submission_token_conflict`.
- If the original request still owns an unbound/in-flight execution, a duplicate
  waits briefly for the run to bind. If it still has not bound, the duplicate
  returns `409 submission_in_progress` and the client retries later with the same
  token and content.
- A joined or replayed request never reports a memory status for a refresh that
  is still running. It reports only an already-settled `synced`/`failed` status,
  or omits the field.
- On acceptance the request that owns the completion claims the session's memory
  status (`pending`) and later settles it (`synced`/`failed`) with an atomic
  conditional write.
- On an uncertain failure (timeout, `TypeError`, `submission_in_progress`, 5xx)
  the frontend keeps the token intent in `sessionStorage` and retries; on
  acceptance or definitive rejection it clears it.

Limitations:

- Token reservations are not garbage-collected automatically.
- Tokens are scoped to workspace/user, not across devices.
- A client that keeps its token intent can safely retry; a client that loses it
  before a response may start a new logical submission.

## Session Memory Status

Session state tracks `memory_update_status` (`idle`, `pending`, `synced`,
`failed`) and `last_submission_id`. Status writes are atomic:

- The request that finalized the completion claims the status
  unconditionally.
- All later status writes (final settle, retry, maintain endpoint) apply only
  while `last_submission_id` is empty or still matches that submission.
- A stale completion cannot overwrite the status owned by a newer graded
  submission.

## Retry And Stale-Write Protection

Every profile save is conditional on the profile version read at the start of the
refresh. A refresh based on an older profile cannot overwrite a newer saved
profile. When a save fails because the profile is stale, the synthesizer retries
once with fresh inputs; if the retry also fails, the caller gets a retryable
failure instead of silently overwriting newer memory.

Duplicate evidence is deduped, stale saves are rejected, and stale completion
status writes are dropped, so uncertain responses are safe to retry.

## Schema And Migration Notes

- `memory_profile_completions` (Postgres) stores durable coding completion
  markers keyed by workspace, user, session, problem, and submission, with the
  source event, refresh ID, and profile version. It is created by
  `memory.PostgresStore.EnsureSchema` and references `memory_events`.
- `submission_request_tokens` stores token reservations keyed by workspace, user,
  and token with fingerprint, run ID, nullable submission ID, and status. It is
  created by `execution.PostgresStore.EnsureSchema`.
- Session state is JSONB; `memory_update_status` and `last_submission_id` are
  additive keys with no backfill required.
- Profile provenance `schema_version` and stage revisions are recorded per
  refresh; `maintained_notes_snapshot` is additive to the existing provenance
  object.

## Evidence Interpretation

Events are evidence, not conclusions. The model may revise prior summary, focus,
skill confidence, or note disposition when new evidence conflicts with earlier
memory, but conclusions should stay proportional to the available evidence.

- A single incorrect answer is limited evidence. It can justify uncertainty or a
  broad review need, but not a precise misconception unless details support it.
- Assisted success (`used_help=true`, hints, or coaching) is evidence of exposure
  or progress, not independent mastery. It should not by itself raise mastery or
  prune review notes.
- Skips are neutral coverage signals. Skip-only evidence should not create or
  retain review notes.
- When help history is incomplete (`assistance_status=unknown`), uncertainty is
  retained rather than assuming independent work.

## Non-Goals

This maintenance flow is not an adaptive-difficulty algorithm. It updates memory
used by later generation, but it does not choose the next difficulty or replace
generation strategy. It does not add a new UI; existing Memory and generation
surfaces are reused.
