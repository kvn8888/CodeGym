import type { Meta, StoryObj } from '@storybook/react-vite';
import { ProblemDetailPage } from './ProblemDetailPage';
import type { Problem, SubmissionFile, TestResult } from '../../shared/api/types';

const meta: Meta<typeof ProblemDetailPage> = {
  title: 'Pages/ProblemDetail',
  component: ProblemDetailPage,
  parameters: {
    initialPath: '/problems/two-sum',
    routePath: '/problems/:id',
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof ProblemDetailPage>;

const twoSum: Problem = {
  id: 'two-sum',
  title: 'Two Sum',
  version: '1',
  category: 'dsa',
  language: 'go',
  difficulty: 1,
  tags: ['arrays', 'hash-map'],
  estimated_minutes: 15,
  type: 'function',
  description: `Given an array of integers \`nums\` and an integer \`target\`, return indices of the two numbers such that they add up to target.

You may assume that each input would have **exactly one solution**, and you may not use the same element twice.

## Example

\`\`\`
Input: nums = [2,7,11,15], target = 9
Output: [0,1]
Explanation: Because nums[0] + nums[1] == 9, we return [0, 1].
\`\`\`

## Constraints

- \`2 <= nums.length <= 10^4\`
- \`-10^9 <= nums[i] <= 10^9\`
- Only one valid answer exists.`,
  hints: [
    { cost: 0, text: 'Think about how you can use a hash map to avoid nested loops.' },
    { cost: 1, text: 'For each number x, check if target - x exists in the hash map.' },
  ],
  runtime: {
    image: 'codegym/go122',
    timeout_seconds: 30,
    memory_mb: 256,
    network_mode: 'none',
  },
  files: { skeleton: [{ path: 'solution.go', entry: true }] },
  test_config: { strategy: 'unit' },
};

const expressePagination: Problem = {
  id: 'express-pagination',
  title: 'Implement Cursor-Based Pagination in Express',
  version: '1',
  category: 'api-patterns',
  language: 'javascript',
  framework: 'express',
  difficulty: 3,
  tags: ['rest', 'pagination'],
  estimated_minutes: 25,
  type: 'api-server',
  description: `Build a \`GET /api/items\` endpoint that supports cursor-based pagination.

## Requirements

- Accept \`?cursor=\` and \`?limit=\` query params
- Return \`{ items, nextCursor, hasMore }\`
- Encode the cursor as a base64 string of the last item's ID
- Default limit is 20, max is 100

## Example

\`\`\`
GET /api/items?limit=2
{ "items": [...], "nextCursor": "MTI=", "hasMore": true }

GET /api/items?cursor=MTI=&limit=2
{ "items": [...], "nextCursor": "MjQ=", "hasMore": false }
\`\`\``,
  hints: [{ cost: 0, text: "Encode the last item's ID using Buffer.from(id).toString('base64')" }],
  runtime: {
    image: 'codegym/node20-express',
    timeout_seconds: 30,
    memory_mb: 256,
    network_mode: 'bridge',
  },
  files: { skeleton: [{ path: 'index.js', entry: true }, { path: 'package.json', readonly: true }] },
  test_config: { strategy: 'http' },
};

const goSkeleton: { files: SubmissionFile[] } = {
  files: [
    {
      path: 'solution.go',
      content: `package solution

func TwoSum(nums []int, target int) []int {
\t// TODO: implement
\treturn nil
}
`,
    },
  ],
};

const expressSkeletonFiles: { files: SubmissionFile[] } = {
  files: [
    {
      path: 'index.js',
      content: `const express = require('express');
const app = express();
app.use(express.json());

// TODO: Implement GET /api/items with cursor-based pagination
app.get('/api/items', (req, res) => {
  const { cursor, limit = 20 } = req.query;
  // Your code here
  res.json({ items: [], nextCursor: null, hasMore: false });
});

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => console.log(\`Server running on port \${PORT}\`));
`,
    },
    {
      path: 'package.json',
      content: `{
  "name": "solution",
  "version": "1.0.0",
  "dependencies": { "express": "^4.18.0" }
}`,
    },
  ],
};

const passedResult: TestResult = {
  status: 'pass',
  total: 5,
  passed: 5,
  failed: 0,
  duration_ms: 12,
  test_cases: [
    { name: 'TestBasicCase', status: 'pass', duration_ms: 2 },
    { name: 'TestMiddleElements', status: 'pass', duration_ms: 1 },
    { name: 'TestSameValues', status: 'pass', duration_ms: 1 },
    { name: 'TestNegativeNumbers', status: 'pass', duration_ms: 4 },
    { name: 'TestLargerArray', status: 'pass', duration_ms: 4 },
  ],
};

const failedResult: TestResult = {
  status: 'fail',
  total: 5,
  passed: 2,
  failed: 3,
  duration_ms: 8,
  test_cases: [
    { name: 'TestBasicCase', status: 'pass', duration_ms: 2 },
    { name: 'TestMiddleElements', status: 'pass', duration_ms: 1 },
    { name: 'TestSameValues', status: 'fail', duration_ms: 1, error: 'Expected [1 3] but got []' },
    { name: 'TestNegativeNumbers', status: 'fail', duration_ms: 2, error: 'Expected [0 2] but got []' },
    { name: 'TestLargerArray', status: 'fail', duration_ms: 2, error: 'Expected [3 7] but got []' },
  ],
};

const compileErrorResult: TestResult = {
  status: 'fail',
  total: 0,
  passed: 0,
  failed: 0,
  duration_ms: 0,
  test_cases: [],
  compile_error: `./solution.go:5:2: syntax error: unexpected }, expected expression
./solution.go:6:1: syntax error: non-declaration statement outside function body`,
};

function makeFetch(problem: Problem, skeleton: { files: SubmissionFile[] }, result?: TestResult) {
  return (url: string, options?: RequestInit) => {
    const u = String(url);
    if (u.includes('/skeleton')) {
      return Promise.resolve(
        new Response(JSON.stringify({ data: skeleton, error: null }), {
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    }
    if (u.includes('/submissions') && options?.method === 'POST') {
      return Promise.resolve(
        new Response(JSON.stringify({ data: { submission_id: 'sub-123' }, error: null }), {
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    }
    if (u.includes('/submissions')) {
      return Promise.resolve(
        new Response(
          JSON.stringify({ data: { status: result ? 'pass' : 'pending', result }, error: null }),
          { headers: { 'Content-Type': 'application/json' } },
        ),
      );
    }
    if (u.includes('/problems')) {
      return Promise.resolve(
        new Response(JSON.stringify({ data: problem, error: null }), {
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ data: null, error: { code: 'NOT_FOUND', message: 'Not found' } }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    );
  };
}

export const GoTwoSum: Story = {
  name: 'Go – Two Sum (editor)',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetch(twoSum, goSkeleton) as typeof fetch;
      return <Story />;
    },
  ],
};

export const GoTwoSumPassed: Story = {
  name: 'Go – All Tests Passed',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetch(twoSum, goSkeleton, passedResult) as typeof fetch;
      return <Story />;
    },
  ],
};

export const GoTwoSumFailed: Story = {
  name: 'Go – Tests Failed',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetch(twoSum, goSkeleton, failedResult) as typeof fetch;
      return <Story />;
    },
  ],
};

export const GoTwoSumCompileError: Story = {
  name: 'Go – Compile Error',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetch(twoSum, goSkeleton, compileErrorResult) as typeof fetch;
      return <Story />;
    },
  ],
};

export const ExpressPagination: Story = {
  name: 'JS/Express – Pagination (multi-file)',
  parameters: {
    initialPath: '/problems/express-pagination',
    routePath: '/problems/:id',
  },
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetch(expressePagination, expressSkeletonFiles) as typeof fetch;
      return <Story />;
    },
  ],
};
