/** Canned MCQ set for the mock API proxy, shaped like the backend
 *  GenerateMCQResult questions (see POST /api/v1/generate). */
export const mockMcqQuestions = [
  {
    id: 'mq1',
    text: 'What is the amortized time complexity of appending to a dynamic array?',
    options: ['O(n)', 'O(1)', 'O(log n)', 'O(n log n)'],
    correctIndex: 1,
    concept: 'Amortized Analysis',
    helpContent:
      'Appends are O(1) most of the time; occasional resizes cost O(n) but happen so rarely that the average per-append cost stays constant.',
  },
  {
    id: 'mq2',
    text: 'Which HTTP status code should a rate limiter return when a client exceeds its quota?',
    options: ['400', '403', '429', '503'],
    correctIndex: 2,
    concept: 'Rate Limiting',
    helpContent:
      '429 Too Many Requests signals quota exhaustion and usually carries a Retry-After header so clients can back off.',
  },
  {
    id: 'mq3',
    text: 'In SQL, which JOIN returns rows from the left table even when there is no match on the right?',
    options: ['INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'CROSS JOIN'],
    correctIndex: 1,
    concept: 'SQL Joins',
    helpContent:
      'A LEFT JOIN keeps every row from the left table, filling unmatched right-side columns with NULL.',
  },
  {
    id: 'mq4',
    text: 'What does an LRU cache evict when it reaches capacity?',
    options: [
      'The largest entry',
      'The least-recently-used entry',
      'The oldest inserted entry',
      'A random entry',
    ],
    correctIndex: 1,
    concept: 'LRU Caching',
    helpContent:
      'LRU tracks access recency: every get/put marks an entry as recently used, and the entry untouched the longest is evicted first.',
  },
  {
    id: 'mq5',
    text: 'Which Go primitive is the idiomatic way to wait for a group of goroutines to finish?',
    options: ['time.Sleep', 'sync.WaitGroup', 'channel close', 'runtime.Gosched'],
    correctIndex: 1,
    concept: 'Go Concurrency',
    helpContent:
      'sync.WaitGroup counts in-flight goroutines: Add before starting, Done when each finishes, Wait blocks until the count reaches zero.',
  },
];
