import type { MemoryEvent, UserMemoryProfile } from '../shared/api/types';
import { memoryEventSources, memoryEventTypes } from '../shared/api/memoryEvents';

export const mockMemoryProfile: UserMemoryProfile = {
  summary:
    'Kevin is strongest when problems connect backend APIs, concurrency, and practical system constraints. Current practice should emphasize writing tests from edge cases, explaining tradeoffs out loud, and translating algorithm patterns into production-style code.',
  updated_at: '2026-07-10T14:30:00.000Z',
  next_review_at: '2026-07-11T14:30:00.000Z',
  strengths: [
    'Recognizes API boundary problems quickly.',
    'Connects concurrency primitives to real service behavior.',
    'Prefers reliability and verification over demo-only polish.',
  ],
  growth_edges: [
    'Slow down on hidden edge cases before coding the first pass.',
    'Practice concise explanations for why a data structure fits.',
    'Make failure and retry states explicit in UI contracts.',
  ],
  skills: [
    {
      id: 'api-design',
      label: 'API Design',
      area: 'backend',
      level: 4,
      confidence: 86,
      trend: 'up',
      last_practiced: '2026-07-09T20:52:00.000Z',
    },
    {
      id: 'concurrency',
      label: 'Concurrency',
      area: 'go',
      level: 3,
      confidence: 72,
      trend: 'flat',
      last_practiced: '2026-07-10T14:28:00.000Z',
    },
    {
      id: 'test-generation',
      label: 'Test Generation',
      area: 'verification',
      level: 3,
      confidence: 68,
      trend: 'up',
      last_practiced: '2026-07-09T18:21:00.000Z',
    },
    {
      id: 'sliding-window',
      label: 'Sliding Window',
      area: 'algorithms',
      level: 2,
      confidence: 55,
      trend: 'down',
      last_practiced: '2026-07-08T20:52:00.000Z',
    },
  ],
  notes: [
    {
      id: 'note-pagination-cache',
      problem_id: 'pagination-api-cache',
      title: 'Paginated API Cache',
      summary:
        'Good instinct to separate fetch and cache concerns. Review cursor invalidation and stale page handling before moving to retries.',
      created_at: '2026-07-09T20:54:00.000Z',
      tags: ['api', 'cache', 'edge-cases'],
      action: 'keep',
    },
    {
      id: 'note-goroutine-fanin',
      problem_id: 'goroutine-fan-in',
      title: 'Goroutine Fan-In Collector',
      summary:
        'Needs one more pass on channel close ownership. The output channel should be closed by the coordinator, not worker readers.',
      created_at: '2026-07-10T14:30:00.000Z',
      tags: ['go', 'channels', 'leaks'],
      action: 'review',
    },
    {
      id: 'note-rate-limit',
      problem_id: 'python-windowed-rate-limit',
      title: 'Windowed Rate Limiter',
      summary:
        'Sliding-window concept is improving, but timestamp eviction should happen before limit checks in every user bucket.',
      created_at: '2026-07-09T18:23:00.000Z',
      tags: ['python', 'queues', 'algorithms'],
      action: 'review',
    },
  ],
};

export const mockMemoryEvents: MemoryEvent[] = [
  {
    id: 'mem_evt_01',
    tenant_id: 'workspace_kevin',
    user_id: 'user_kevin',
    source: memoryEventSources.generate,
    type: memoryEventTypes.problemGenerated,
    summary: 'Generated an API design challenge focused on pagination and cache invalidation.',
    payload: {
      problem_id: 'pagination-api-cache',
      difficulty: 3,
      topic: 'api-design',
    },
    occurred_at: '2026-06-10T16:30:00.000Z',
    created_at: '2026-06-10T16:30:01.000Z',
  },
  {
    id: 'mem_evt_02',
    tenant_id: 'workspace_kevin',
    user_id: 'user_kevin',
    source: memoryEventSources.workspace,
    type: memoryEventTypes.attemptFailed,
    summary: 'Attempted goroutine fan-in collector and missed channel close ownership.',
    payload: {
      problem_id: 'goroutine-fan-in',
      result: 'partial',
      language: 'go',
    },
    occurred_at: '2026-06-09T20:10:00.000Z',
    created_at: '2026-06-09T20:10:03.000Z',
  },
  {
    id: 'mem_evt_03',
    tenant_id: 'workspace_kevin',
    user_id: 'user_kevin',
    source: memoryEventSources.memory,
    type: memoryEventTypes.noteCreated,
    summary: 'Captured a review note for sliding-window timestamp eviction ordering.',
    payload: {
      note_id: 'note-rate-limit',
      action: 'review',
    },
    occurred_at: '2026-06-08T13:18:00.000Z',
    created_at: '2026-06-08T13:18:00.000Z',
  },
];
