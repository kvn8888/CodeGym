# Memory Profile Maintenance

This document describes how CodeGym maintains the durable memory profile from
append-only learning events. It covers the notes-first MCQ flow, shared profile
refresh paths, deduplication, and stale-write protection.

## Scope

The profile is synthesized from deterministic memory events into the summary,
focus areas, skills, notes, and provenance used by the Memory page and future
generation. Completed MCQ rounds use notes-first maintenance: note CRUD is
computed before profile synthesis, then notes and profile are saved together.

This story does not introduce note CRUD for daily, coding, or interview refreshes.
Those paths may refresh the profile summary, skills, and focus areas, but they
preserve existing notes.

## Caller Paths

MCQ completion is the supported notes-first path. The frontend records MCQ
outcome events, records `session_completed`, flushes pending memory events, then
calls `POST /api/v1/memory/profile/maintain` with the completed round
`session_id`.

Daily refresh runs through the same profile synthesizer with trigger `daily`, but
without a focused completed MCQ `session_id`. It does not run note CRUD and skips
model work when the durable evidence digest is unchanged.

Coding and interview refreshes share the profile synthesizer so they benefit from
source tracking, bounded evidence, and stale-write protection. They do not run
notes-first CRUD in this story, and existing notes are preserved even when the
profile model returns a different `notes` array.

`POST /api/v1/memory/notes/maintain` is a compatibility alias for full profile
maintenance. It does not guarantee standalone note maintenance; note CRUD still
requires the completed MCQ gate below.

## Completed MCQ Gate

Notes-first maintenance runs only when all of these are true:

- The maintain request includes a non-empty `session_id`.
- The evidence contains an MCQ `session_completed` event whose payload
  `session_id` matches the request `session_id`.
- The evidence contains at least one matching MCQ outcome event for the same
  `session_id`.

This prevents unrelated MCQ sessions, partially flushed rounds, daily refreshes,
coding refreshes, and interview refreshes from mutating notes through the new
MCQ note CRUD path.

## Notes-First Save Flow

For a completed MCQ round, the profile synthesizer runs two model stages:

1. The note stage receives bounded event evidence and the current profile, then
   returns note actions: `create`, `update`, or `prune`.
2. The server applies those actions to candidate notes in memory only.
3. The profile stage receives the same bounded event evidence plus the profile
   context containing candidate notes.
4. The server validates the complete profile candidate.
5. The server saves summary, focus areas, skills, notes, and provenance in one
   version-checked profile write.

If note generation, profile generation, or validation fails, the existing saved
profile remains unchanged. Candidate notes are not persisted before the final
validated profile save.

## Evidence Dedupe And Source Tracking

Profile synthesis uses canonical evidence before building prompts or provenance.
Replaying the same logical MCQ question outcome or the same round completion does
not count evidence twice. Exact replays keep the original event identity for
stable source tracking.

Evidence source keys distinguish replayed evidence from genuinely new evidence.
Distinct attempts, distinct outcomes, and skip-vs-answer evidence remain separate
so retries of the same event do not erase meaningful later practice.

Memory and system audit events are excluded from learning evidence. Note-created,
note-updated, and profile-refresh audit events should not become self-reinforcing
signals for future profile conclusions.

## Retry And Stale-Write Protection

Every profile save is conditional on the profile version read at the start of the
refresh. A refresh based on an older profile cannot overwrite a newer saved
profile.

When a save fails because the profile is stale, the synthesizer retries once with
fresh inputs. If the retry also cannot save, the caller gets a retryable failure
instead of silently overwriting newer memory.

This makes uncertain responses safe to retry: duplicate evidence is deduped, and
stale saves are rejected rather than replacing newer profile or note state.

## Evidence Interpretation

Events are evidence, not conclusions. The model may revise prior summary, focus,
skill confidence, or note disposition when new evidence conflicts with earlier
memory, but conclusions should stay proportional to the available evidence.

A single incorrect answer is limited evidence. It can justify uncertainty or a
broad review need, but it should not become a precise misconception unless event
details support that conclusion.

Assisted success, including `used_help=true`, is evidence of exposure or progress
rather than independent mastery. It should not by itself raise mastery level,
promote a concept to a strength, or prune a review note.

Skips are neutral coverage signals. The UI may reveal the correct answer after a
skip, but that reveal is not learner performance. Skip-only evidence should not
create or retain review notes, and later correct evidence can resolve skip-only
uncertainty unless actual incorrect evidence remains.

## Non-Goals

This maintenance flow is not an adaptive-difficulty algorithm. It updates memory
used by later generation, but it does not choose the next difficulty or replace
generation strategy.

This story does not add a new UI. Existing Memory and generation surfaces are
reused, and MCQ loading-stage wording is owned separately by issue #113.

This story does not add note CRUD for daily, coding, or interview refreshes.
Those paths continue to preserve existing notes while using the shared profile
refresh protections.
