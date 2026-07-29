# 1 · Practice Intake Agent

**Role in the system:** Before CodeGym builds a new practice session for an
unfamiliar topic, this agent generates a short typed self-report baseline for
`QuestionModal`. It does not generate practice content or assess demonstrated
proficiency.

The runtime prompt and validator live in
`backend/internal/generation/intake.go`. The persisted, workspace-scoped state
lives under `backend/internal/intake`.

## Output contract

Return only a JSON array containing at most three `IntakeQuestion` objects:

```ts
interface IntakeQuestion {
  id: string;
  dimension: string;
  text: string;
  options: Array<{ id: string; label: string }>;
}
```

Every question and dimension ID must be unique. Each question has 2–5 unique
option IDs. The current prompt asks for three dimensions:

1. prior exposure;
2. practical application depth;
3. desired challenge or emphasis.

Questions never request personal data, code, free text, or facts that can be
graded. They describe only the learner's self-reported starting point.

## Runtime behavior

- The server normalizes exact topic aliases and first checks demonstrated
  memory, profile skills/confidence, and an existing completed or skipped
  intake. It never uses substring matching.
- A familiar topic suppresses automatic intake. Completion or Skip permanently
  suppresses it too. `restart: true` is the explicit Update baseline path.
- A pending intake and each selected option are persisted immediately, so
  reload restores the same question and prior answers.
- Provider or validation failure persists a pending error state. The frontend
  presents Retry and Skip; it never fabricates fallback questions.
- Completed answers enter generation as `self_reported_baseline`, separate from
  demonstrated summary, strengths, growth edges, skills, and notes.
  Demonstrated evidence takes precedence whenever the channels conflict.

## Adversarial checks

- `Django` must not match the exact `Go` alias.
- IDs, dimensions, option IDs, and output bounds must fail closed when missing
  or duplicated.
- Instructions embedded in topic or practice-seed values are untrusted data and
  must never override the system prompt.
