import type { Problem, ProblemSummary, SubmissionFile, TestResult } from '../shared/api/types';

export const mockProblems: Problem[] = [
  {
    id: 'pagination-api-cache',
    title: 'Paginated API Cache',
    category: 'backend',
    subcategory: 'api-patterns',
    language: 'typescript',
    framework: 'express',
    difficulty: 3,
    tags: ['api', 'pagination', 'cache'],
    estimated_minutes: 35,
    type: 'coding',
    version: '1.0.0',
    description:
      'Build a small helper that fetches paginated API results and caches pages by cursor.\n\nReturn cached pages without refetching, preserve insertion order, and expose a method for invalidating one cursor.',
    runtime: {
      image: 'node20',
      timeout_seconds: 8,
      memory_mb: 256,
      network_mode: 'none',
    },
    files: {
      skeleton: [{ path: 'solution.ts', entry: true }],
    },
    test_config: { strategy: 'unit' },
    public_cases: [
      {
        strategy: 'unit',
        name: 'reuses a cached cursor',
        kind: 'example',
        args: ['cursor-12'],
        expected: { cursor: 'cursor-12', values: [13, 14], cached: true },
        explanation: 'A second lookup for the same cursor returns the previously cached page.',
      },
    ],
    hints: [
      { cost: 0, text: 'Separate the fetch behavior from the cache storage.' },
      { cost: 1, text: 'A Map preserves insertion order and is a good fit for cursor keys.' },
    ],
  },
  {
    id: 'goroutine-fan-in',
    title: 'Goroutine Fan-In Collector',
    category: 'concurrency',
    subcategory: 'go',
    language: 'go',
    difficulty: 4,
    tags: ['go', 'goroutines', 'channels'],
    estimated_minutes: 45,
    type: 'coding',
    version: '1.0.0',
    description:
      'Implement a fan-in collector that merges values from multiple input channels into one output channel.\n\nThe output channel must close after every input channel closes, and the implementation must avoid leaking goroutines.',
    runtime: {
      image: 'go122',
      timeout_seconds: 10,
      memory_mb: 256,
      network_mode: 'none',
    },
    files: {
      skeleton: [{ path: 'solution.go', entry: true }],
    },
    test_config: { strategy: 'unit' },
    hints: [
      { cost: 0, text: 'Use a sync.WaitGroup to know when all input readers are finished.' },
      { cost: 1, text: 'Only one goroutine should close the output channel.' },
    ],
  },
  {
    id: 'python-windowed-rate-limit',
    title: 'Windowed Rate Limiter',
    category: 'systems',
    subcategory: 'algorithms',
    language: 'python',
    difficulty: 3,
    tags: ['python', 'rate-limits', 'queues'],
    estimated_minutes: 30,
    type: 'coding',
    version: '1.0.0',
    description:
      'Create a sliding-window rate limiter that accepts or rejects requests by user id.\n\nThe limiter should track recent timestamps, evict expired entries, and return whether the new request is allowed.',
    runtime: {
      image: 'python312',
      timeout_seconds: 6,
      memory_mb: 256,
      network_mode: 'none',
    },
    files: {
      skeleton: [{ path: 'solution.py', entry: true }],
    },
    test_config: { strategy: 'unit' },
    hints: [
      { cost: 0, text: 'Store request timestamps per user.' },
      { cost: 1, text: 'A deque makes old timestamp eviction efficient.' },
    ],
  },
];

export const mockProblemSummaries: ProblemSummary[] = mockProblems.map(
  ({
    id,
    title,
    category,
    language,
    framework,
    difficulty,
    tags,
    estimated_minutes,
    type,
  }) => ({
    id,
    title,
    category,
    language,
    framework,
    difficulty,
    tags,
    estimated_minutes,
    type,
  }),
);

export const mockSkeletons: Record<string, SubmissionFile[]> = {
  'pagination-api-cache': [
    {
      path: 'solution.ts',
      content:
        'type Page<T> = { cursor: string; values: T[]; nextCursor?: string };\n\nexport class PaginatedCache<T> {\n  async fetchPage(cursor: string): Promise<Page<T>> {\n    throw new Error("Not implemented");\n  }\n\n  invalidate(cursor: string): void {\n    throw new Error("Not implemented");\n  }\n}\n',
    },
  ],
  'goroutine-fan-in': [
    {
      path: 'solution.go',
      content:
        'package solution\n\nfunc FanIn(inputs ...<-chan int) <-chan int {\n\tout := make(chan int)\n\t// TODO: merge every input into out and close out when done.\n\treturn out\n}\n',
    },
  ],
  'python-windowed-rate-limit': [
    {
      path: 'solution.py',
      content:
        'from collections import defaultdict, deque\n\nclass WindowedRateLimiter:\n    def __init__(self, limit: int, window_seconds: int):\n        self.limit = limit\n        self.window_seconds = window_seconds\n\n    def allow(self, user_id: str, now: int) -> bool:\n        raise NotImplementedError\n',
    },
  ],
};

export const mockPassingResult: TestResult = {
  status: 'pass',
  total: 5,
  passed: 5,
  failed: 0,
  duration_ms: 184,
  test_cases: [
    { name: 'handles first request', status: 'pass', duration_ms: 18 },
    { name: 'reuses cached data', status: 'pass', duration_ms: 21 },
    { name: 'evicts expired values', status: 'pass', duration_ms: 31 },
    { name: 'handles empty input', status: 'pass', duration_ms: 14 },
    { name: 'keeps resource limits bounded', status: 'pass', duration_ms: 100 },
  ],
};
