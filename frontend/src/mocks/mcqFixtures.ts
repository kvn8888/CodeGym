import type { MCQQuestionType } from '../shared/api/types';

export interface MockMcqQuestion {
  id: string;
  type?: MCQQuestionType;
  text: string;
  options?: string[];
  correctIndex?: number;
  correctIndices?: number[];
  expectedAnswer?: string;
  rubric?: string;
  concept: string;
  helpContent: string;
}

/** Canned MCQ set for the mock API proxy, shaped like the backend
 *  GenerateMCQResult questions (see POST /api/v1/generate). */
export const mockMcqQuestions: MockMcqQuestion[] = [
  {
    id: 'mq1',
    type: 'single_select',
    text: 'What is the amortized time complexity of appending to a **dynamic array**?',
    options: ['`O(n)`', '`O(1)`', '`O(log n)`', '`O(n log n)`'],
    correctIndex: 1,
    concept: 'Amortized Analysis',
    helpContent:
      'Appends are `O(1)` most of the time; occasional resizes cost `O(n)` but happen so rarely that the average per-append cost stays constant.\n\n```ts\n// amortized O(1) append\narr.push(x)\n```',
  },
  {
    id: 'mq2',
    type: 'single_select',
    text: 'Which HTTP status code should a rate limiter return when a client exceeds its quota?',
    options: ['400', '403', '429', '503'],
    correctIndex: 2,
    concept: 'Rate Limiting',
    helpContent:
      '429 Too Many Requests signals quota exhaustion and usually carries a Retry-After header so clients can back off.',
  },
  {
    id: 'mq3',
    type: 'single_select',
    text: 'In SQL, which JOIN returns rows from the left table even when there is no match on the right?',
    options: ['INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'CROSS JOIN'],
    correctIndex: 1,
    concept: 'SQL Joins',
    helpContent:
      'A LEFT JOIN keeps every row from the left table, filling unmatched right-side columns with NULL.',
  },
  {
    id: 'mq4',
    type: 'multi_select',
    text: 'Which operations are commonly O(1) in a well-designed LRU cache?',
    options: [
      'Read an existing key',
      'Insert or update a key',
      'Sort every cached key',
      'Scan every entry for eviction',
    ],
    correctIndices: [0, 1],
    concept: 'LRU Caching',
    helpContent:
      'A hash map locates entries while a doubly linked list updates recency and evicts from one end without scanning the cache.',
  },
  {
    id: 'mq5',
    type: 'free_response',
    text: 'Why does breadth-first search find a shortest path in an unweighted graph?',
    expectedAnswer:
      'Breadth-first search explores vertices in nondecreasing distance from the source, one level at a time.',
    rubric:
      'The answer must explain that queue order processes every distance-k vertex before any distance-(k+1) vertex.',
    concept: 'Breadth-First Search',
    helpContent:
      'Focus on the relationship between queue order and the number of edges from the source.',
  },
];
