import type { UserMemoryProfile } from '../shared/api/types';

export const mockMemoryProfile: UserMemoryProfile = {
  summary:
    'Kevin is strongest when problems connect backend APIs, concurrency, and practical system constraints. Current practice should emphasize writing tests from edge cases, explaining tradeoffs out loud, and translating algorithm patterns into production-style code.',
  updated_at: '2026-06-10T17:30:00.000Z',
  next_review_at: '2026-06-11T09:00:00.000Z',
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
      last_practiced: '2026-06-10',
    },
    {
      id: 'concurrency',
      label: 'Concurrency',
      area: 'go',
      level: 3,
      confidence: 72,
      trend: 'flat',
      last_practiced: '2026-06-09',
    },
    {
      id: 'test-generation',
      label: 'Test Generation',
      area: 'verification',
      level: 3,
      confidence: 68,
      trend: 'up',
      last_practiced: '2026-06-10',
    },
    {
      id: 'sliding-window',
      label: 'Sliding Window',
      area: 'algorithms',
      level: 2,
      confidence: 55,
      trend: 'down',
      last_practiced: '2026-06-08',
    },
  ],
  notes: [
    {
      id: 'note-pagination-cache',
      problem_id: 'pagination-api-cache',
      title: 'Paginated API Cache',
      summary:
        'Good instinct to separate fetch and cache concerns. Review cursor invalidation and stale page handling before moving to retries.',
      created_at: '2026-06-10T16:45:00.000Z',
      tags: ['api', 'cache', 'edge-cases'],
      action: 'keep',
    },
    {
      id: 'note-goroutine-fanin',
      problem_id: 'goroutine-fan-in',
      title: 'Goroutine Fan-In Collector',
      summary:
        'Needs one more pass on channel close ownership. The output channel should be closed by the coordinator, not worker readers.',
      created_at: '2026-06-09T20:20:00.000Z',
      tags: ['go', 'channels', 'leaks'],
      action: 'review',
    },
    {
      id: 'note-rate-limit',
      problem_id: 'python-windowed-rate-limit',
      title: 'Windowed Rate Limiter',
      summary:
        'Sliding-window concept is improving, but timestamp eviction should happen before limit checks in every user bucket.',
      created_at: '2026-06-08T13:15:00.000Z',
      tags: ['python', 'queues', 'algorithms'],
      action: 'review',
    },
  ],
};
