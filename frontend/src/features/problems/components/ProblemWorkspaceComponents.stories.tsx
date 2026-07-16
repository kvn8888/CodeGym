import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react-vite';

import type { Problem, SubmissionFile, TestResult } from '@/shared/api/types';
import {
  ProblemFileToolbar,
  ProblemHeader,
  ProblemStatement,
  ProblemTestResults,
} from './ProblemWorkspaceComponents';

const problem: Problem = {
  id: 'merge-intervals',
  version: '1',
  title: 'Merge Intervals',
  category: 'dsa',
  language: 'typescript',
  difficulty: 3,
  tags: ['arrays', 'sorting', 'intervals'],
  estimated_minutes: 25,
  type: 'function',
  description: `Given an array of intervals where \`intervals[i] = [start, end]\`, merge all overlapping intervals and return the non-overlapping intervals that cover every input interval.

## Example

\`\`\`text
Input: [[1,3],[2,6],[8,10],[15,18]]
Output: [[1,6],[8,10],[15,18]]
\`\`\`

## Constraints

- \`1 <= intervals.length <= 10^4\`
- \`0 <= start <= end <= 10^4\``,
  runtime: {
    image: 'codegym/node25',
    timeout_seconds: 30,
    memory_mb: 256,
    network_mode: 'none',
  },
  files: { skeleton: [{ path: 'solution.ts', entry: true }] },
  test_config: { strategy: 'unit' },
  hints: [
    { cost: 0, text: 'Sort the intervals by their start value before scanning them.' },
    { cost: 1, text: 'Compare each interval with the end of the last merged interval.' },
  ],
};

const files: SubmissionFile[] = [
  { path: 'solution.ts', content: 'export function merge(intervals: number[][]) {\n  return [];\n}' },
  { path: 'solution.test.ts', content: "import { merge } from './solution';" },
];

const passedResult: TestResult = {
  status: 'pass',
  total: 4,
  passed: 4,
  failed: 0,
  duration_ms: 38,
  test_cases: [
    { name: 'merges overlapping intervals', status: 'pass', duration_ms: 8 },
    { name: 'keeps disjoint intervals', status: 'pass', duration_ms: 6 },
    { name: 'handles a single interval', status: 'pass', duration_ms: 3 },
    { name: 'merges touching boundaries', status: 'pass', duration_ms: 7 },
  ],
};

const failedResult: TestResult = {
  status: 'fail',
  total: 4,
  passed: 2,
  failed: 2,
  duration_ms: 31,
  test_cases: [
    { name: 'merges overlapping intervals', status: 'pass', duration_ms: 8 },
    { name: 'keeps disjoint intervals', status: 'pass', duration_ms: 6 },
    {
      name: 'merges touching boundaries',
      status: 'fail',
      duration_ms: 7,
      error: 'Expected [[1,5]], received [[1,3],[3,5]]',
    },
    {
      name: 'handles nested intervals',
      status: 'fail',
      duration_ms: 10,
      error: 'Expected [[1,10]], received [[1,10],[2,4]]',
    },
  ],
};

const meta: Meta = {
  title: 'Problems/Workspace Components',
  parameters: { layout: 'centered' },
};

export default meta;
type Story = StoryObj;

export const Statement: Story = {
  render: () => (
    <div className="w-[calc(100vw-2rem)] max-w-[680px] bg-background p-6">
      <ProblemHeader problem={problem} />
      <ProblemStatement description={problem.description} hints={problem.hints} />
    </div>
  ),
};

function ToolbarPreview() {
  const [activeFile, setActiveFile] = useState(0);
  const [running, setRunning] = useState(false);

  return (
    <div className="w-[calc(100vw-2rem)] max-w-[760px] overflow-hidden rounded-md border border-[#333]">
      <ProblemFileToolbar
        files={files}
        activeFile={activeFile}
        onActiveFileChange={setActiveFile}
        onRun={() => {
          setRunning(true);
          window.setTimeout(() => setRunning(false), 1200);
        }}
        running={running}
      />
      <pre className="h-48 overflow-auto bg-[#1e1e1e] p-4 font-mono text-xs leading-5 text-[#d4d4d4]">
        {files[activeFile]?.content}
      </pre>
    </div>
  );
}

export const FileToolbar: Story = {
  render: () => <ToolbarPreview />,
};

export const TestsRunning: Story = {
  render: () => (
    <div className="h-56 w-[calc(100vw-2rem)] max-w-[640px] overflow-hidden rounded-md border border-[#333]">
      <ProblemTestResults state="running" />
    </div>
  ),
};

export const TestsPassed: Story = {
  render: () => (
    <div className="h-64 w-[calc(100vw-2rem)] max-w-[640px] overflow-hidden rounded-md border border-[#333]">
      <ProblemTestResults state="complete" result={passedResult} />
    </div>
  ),
};

export const TestsFailed: Story = {
  render: () => (
    <div className="h-72 w-[calc(100vw-2rem)] max-w-[640px] overflow-hidden rounded-md border border-[#333]">
      <ProblemTestResults state="complete" result={failedResult} />
    </div>
  ),
};

export const ExecutionUnavailable: Story = {
  render: () => (
    <div className="h-56 w-[calc(100vw-2rem)] max-w-[640px] overflow-hidden rounded-md border border-[#333]">
      <ProblemTestResults
        state="error"
        error="The execution service is temporarily unavailable. Your solution is still saved."
        onRetry={() => undefined}
      />
    </div>
  ),
};
