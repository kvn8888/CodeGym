import type { Meta, StoryObj } from '@storybook/react-vite';

import type { WorkflowEvent, WorkflowStatus } from '../api/client';
import { WorkflowProgress } from './WorkflowProgress';

const labels = [
  ['load_context', 'Load personalization'],
  ['generate_questions', 'Generate questions'],
  ['validate_questions', 'Validate question set'],
  ['repair_questions', 'Repair invalid output if needed'],
  ['questions_ready', 'Question set ready'],
] as const;

function events(statuses: WorkflowStatus[], terminalIndex = -1): WorkflowEvent[] {
  return labels.map(([step_id, label], index) => ({
    operation_id: 'workflow_story',
    sequence: index + 1,
    step_id,
    label,
    status: statuses[index] ?? 'queued',
    timestamp: new Date(Date.UTC(2026, 6, 30, 12, 0, index)).toISOString(),
    metadata: index === terminalIndex ? { terminal: true, retryable: statuses[index] === 'failed' } : undefined,
  }));
}

const activeEvents = events(['succeeded', 'running', 'queued', 'queued', 'queued']);

const meta = {
  title: 'Components/Workflow Progress',
  component: WorkflowProgress,
  parameters: { layout: 'centered' },
  decorators: [
    (Story) => (
      <div className="bg-muted/30 flex min-h-96 w-[calc(100vw-2rem)] max-w-[720px] items-start justify-end p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WorkflowProgress>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connecting: Story = {
  args: {
    title: 'Preparing practice set',
    connectionState: 'connecting',
    defaultExpanded: true,
    events: events(['queued', 'queued', 'queued', 'queued', 'queued']),
  },
};

export const Active: Story = {
  args: {
    title: 'Preparing practice set',
    defaultExpanded: true,
    events: activeEvents,
  },
};

export const Completed: Story = {
  args: {
    title: 'Preparing practice set',
    defaultExpanded: true,
    events: events(
      ['succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded'],
      4,
    ),
  },
};

export const Failed: Story = {
  args: {
    title: 'Preparing practice set',
    defaultExpanded: true,
    events: events(['succeeded', 'succeeded', 'failed', 'queued', 'queued'], 2),
  },
};

export const Reconnecting: Story = {
  args: {
    title: 'Preparing practice set',
    connectionState: 'reconnecting',
    defaultExpanded: true,
    events: activeEvents,
  },
};

export const NonStreamingFallback: Story = {
  args: {
    title: 'Preparing practice set',
    connectionState: 'fallback',
    defaultExpanded: true,
    events: [
      {
        operation_id: 'fallback',
        sequence: 1,
        step_id: 'request',
        label: 'Prepare question set',
        status: 'running',
        timestamp: '2026-07-30T12:00:00Z',
      },
    ],
  },
};
