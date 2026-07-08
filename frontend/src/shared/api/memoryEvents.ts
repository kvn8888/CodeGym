import type { components } from './openapi';

export const memoryEventSources = {
  generate: 'generate',
  chat: 'chat',
  workspace: 'workspace',
  mcq: 'mcq',
  memory: 'memory',
  system: 'system',
} as const;

export type MemoryEventSource = (typeof memoryEventSources)[keyof typeof memoryEventSources];

export const memoryEventTypes = {
  problemRequested: 'problem_requested',
  problemGenerated: 'problem_generated',
  problemAttempted: 'problem_attempted',
  attemptPassed: 'attempt_passed',
  attemptFailed: 'attempt_failed',
  answerWrong: 'answer_wrong',
  memoryNoteCreated: 'memory_note_created',
} as const;

export type MemoryEventType = (typeof memoryEventTypes)[keyof typeof memoryEventTypes];
export type MemoryEventInput = components['schemas']['RecordMemoryEventInput'];
export type MemoryEventPayload = components['schemas']['JsonValue'];

interface BuildMemoryEventInput {
  type: MemoryEventType;
  summary: string;
  payload?: MemoryEventPayload;
  occurredAt?: string | Date;
}

function toOccurredAt(value?: string | Date) {
  if (!value) {
    return undefined;
  }

  return (value instanceof Date ? value : new Date(value)).toISOString();
}

function buildMemoryEvent(source: MemoryEventSource, input: BuildMemoryEventInput): MemoryEventInput {
  return {
    source,
    type: input.type,
    summary: input.summary,
    ...(input.payload === undefined ? {} : { payload: input.payload }),
    ...(toOccurredAt(input.occurredAt) ? { occurred_at: toOccurredAt(input.occurredAt) } : {}),
  };
}

export function buildGenerateEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.generate, input);
}

export function buildChatEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.chat, input);
}

export function buildWorkspaceEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.workspace, input);
}

export function buildMcqEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.mcq, input);
}

export function buildMemorySourceEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.memory, input);
}

export function buildSystemEvent(input: BuildMemoryEventInput) {
  return buildMemoryEvent(memoryEventSources.system, input);
}
