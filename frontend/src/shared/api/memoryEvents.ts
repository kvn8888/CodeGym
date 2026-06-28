import type { components } from './openapi';

export type JsonValue = components['schemas']['JsonValue'];
export type RecordMemoryEventInput = components['schemas']['RecordMemoryEventInput'];

export const MEMORY_EVENT_SOURCES = {
  generate: 'generate',
  chat: 'chat',
  workspace: 'workspace',
  mcq: 'mcq',
  memory: 'memory',
  system: 'system',
} as const;

export type MemoryEventSource =
  (typeof MEMORY_EVENT_SOURCES)[keyof typeof MEMORY_EVENT_SOURCES];

export const MEMORY_EVENT_TYPES = {
  generate: {
    intakeStarted: 'intake_started',
    clarifyingQuestionsAnswered: 'clarifying_questions_answered',
    problemGenerated: 'problem_generated',
    mcqSetGenerated: 'mcq_set_generated',
    interviewPromptGenerated: 'interview_prompt_generated',
    generationFailed: 'generation_failed',
  },
  chat: {
    threadOpened: 'thread_opened',
    messageSent: 'message_sent',
    assistantReplied: 'assistant_replied',
    threadClosed: 'thread_closed',
  },
  workspace: {
    problemOpened: 'problem_opened',
    attemptStarted: 'attempt_started',
    testsRun: 'tests_run',
    attemptSubmitted: 'attempt_submitted',
    attemptSolved: 'attempt_solved',
    attemptFailed: 'attempt_failed',
  },
  mcq: {
    sessionStarted: 'session_started',
    questionAnswered: 'question_answered',
    sessionCompleted: 'session_completed',
    answerIncorrect: 'answer_incorrect',
  },
  memory: {
    profileViewed: 'profile_viewed',
    profileRefreshed: 'profile_refreshed',
    noteCreated: 'note_created',
    noteUpdated: 'note_updated',
    notePruned: 'note_pruned',
  },
  system: {
    workerProfileRefreshed: 'worker_profile_refreshed',
    memoryApiChecked: 'memory_api_checked',
  },
} as const satisfies Record<MemoryEventSource, Record<string, string>>;

export type MemoryEventType<Source extends MemoryEventSource> =
  Extract<
    (typeof MEMORY_EVENT_TYPES)[Source][keyof (typeof MEMORY_EVENT_TYPES)[Source]],
    string
  >;

export interface MemoryEventDraft<Source extends MemoryEventSource = MemoryEventSource> {
  source: Source;
  type: MemoryEventType<Source>;
  summary: string;
  payload?: JsonValue;
  occurredAt?: Date | string;
}

export function buildMemoryEvent<Source extends MemoryEventSource>(
  draft: MemoryEventDraft<Source>,
): RecordMemoryEventInput {
  return {
    source: draft.source,
    type: draft.type,
    summary: draft.summary,
    ...(draft.payload !== undefined ? { payload: draft.payload } : {}),
    ...(draft.occurredAt !== undefined
      ? { occurred_at: formatOccurredAt(draft.occurredAt) }
      : {}),
  };
}

export const memoryEventBuilders = {
  generate: (
    type: MemoryEventType<'generate'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.generate, type, summary, payload }),
  chat: (
    type: MemoryEventType<'chat'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.chat, type, summary, payload }),
  workspace: (
    type: MemoryEventType<'workspace'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.workspace, type, summary, payload }),
  mcq: (
    type: MemoryEventType<'mcq'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.mcq, type, summary, payload }),
  memory: (
    type: MemoryEventType<'memory'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.memory, type, summary, payload }),
  system: (
    type: MemoryEventType<'system'>,
    summary: string,
    payload?: JsonValue,
  ) => buildMemoryEvent({ source: MEMORY_EVENT_SOURCES.system, type, summary, payload }),
};

function formatOccurredAt(value: Date | string): string {
  return value instanceof Date ? value.toISOString() : value;
}
