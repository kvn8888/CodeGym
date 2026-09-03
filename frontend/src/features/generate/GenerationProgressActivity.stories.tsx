import type { Meta, StoryObj } from '@storybook/react-vite';

import type { WorkflowEvent, WorkflowStatus } from '../../shared/api/client';
import type { PracticeFormat } from '../../shared/api/types';
import { GeneratePage } from './GeneratePage';
import { GenerationProgressActivity } from './GenerationProgressActivity';

const codingSteps = [
  ['load_context', 'Read your memory profile'],
  ['generate_problem', 'Draft the coding problem'],
  ['verify_solution', 'Verify the reference solution'],
  ['save_problem', 'Save the coding problem'],
  ['problem_ready', 'Coding problem ready'],
] as const;

const mcqSteps = [
  ['memory_context', 'Read your memory profile'],
  ['select_concepts', 'Select target concepts'],
  ['generate_questions', 'Generate question set'],
  ['validate_questions', 'Validate answer choices'],
  ['save_session', 'Save practice session'],
] as const;

type ProgressStep = readonly [string, string];

function workflowEvents(
  operationId: string,
  steps: readonly ProgressStep[],
  statuses: WorkflowStatus[],
  terminalIndex = -1,
): WorkflowEvent[] {
  return steps.map(([step_id, label], index) => ({
    operation_id: operationId,
    sequence: index + 1,
    step_id,
    label,
    status: statuses[index] ?? 'queued',
    timestamp: new Date(Date.UTC(2026, 8, 3, 14, 0, index)).toISOString(),
    metadata: index === terminalIndex ? { terminal: true, retryable: statuses[index] === 'failed' } : undefined,
  }));
}

function codingEvents(statuses: WorkflowStatus[], terminalIndex = -1): WorkflowEvent[] {
  return workflowEvents('coding-generation-story', codingSteps, statuses, terminalIndex);
}

function mcqEvents(statuses: WorkflowStatus[], terminalIndex = -1): WorkflowEvent[] {
  return workflowEvents('mcq-generation-story', mcqSteps, statuses, terminalIndex);
}

const meta = {
  title: 'Pages/Generate/Generation Progress',
  component: GenerationProgressActivity,
  parameters: {
    initialPath: '/generate?prompt=Graph%20shortest%20paths',
    routePath: '/generate',
  },
  render: (args, context) => (
    <>
      <GeneratePage
        initialFormat={(context.parameters.generationFormat as PracticeFormat | undefined) ?? 'coding'}
        initialStarting
      />
      <GenerationProgressActivity {...args} />
    </>
  ),
} satisfies Meta<typeof GenerationProgressActivity>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Starting: Story = {
  args: {
    defaultExpanded: true,
    connectionState: 'connecting',
    events: codingEvents(['queued', 'queued', 'queued', 'queued', 'queued']),
  },
};

export const MCQGeneration: Story = {
  name: 'MCQ generation',
  parameters: { generationFormat: 'mcq' },
  args: {
    title: 'Generating question set',
    defaultExpanded: true,
    events: mcqEvents(['succeeded', 'succeeded', 'running', 'queued', 'queued']),
  },
};

export const DraftingProblem: Story = {
  args: {
    defaultExpanded: true,
    events: codingEvents(['succeeded', 'running', 'queued', 'queued', 'queued']),
  },
};

export const VerifyingTests: Story = {
  args: {
    defaultExpanded: true,
    events: codingEvents(['succeeded', 'succeeded', 'running', 'queued', 'queued']),
  },
};

export const Collapsed: Story = {
  args: {
    defaultExpanded: false,
    events: codingEvents(['succeeded', 'running', 'queued', 'queued', 'queued']),
  },
};

export const Complete: Story = {
  args: {
    defaultExpanded: true,
    events: codingEvents(
      ['succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded'],
      4,
    ),
  },
};

export const RetryableFailure: Story = {
  args: {
    defaultExpanded: true,
    events: codingEvents(['succeeded', 'succeeded', 'failed', 'queued', 'queued'], 2),
  },
};

export const Reconnecting: Story = {
  args: {
    defaultExpanded: true,
    connectionState: 'reconnecting',
    events: codingEvents(['succeeded', 'running', 'queued', 'queued', 'queued']),
  },
};

export const StreamUnavailable: Story = {
  args: {
    defaultExpanded: true,
    connectionState: 'fallback',
    events: [
      {
        operation_id: 'coding-generation-fallback',
        sequence: 1,
        step_id: 'request',
        label: 'Generate coding problem',
        status: 'running',
        timestamp: '2026-09-03T14:00:00Z',
      },
    ],
  },
};
