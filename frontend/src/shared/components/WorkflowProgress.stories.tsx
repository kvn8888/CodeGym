import type { Meta, StoryObj } from '@storybook/react-vite';

import { Button } from '@/components/ui/button';
import { WorkflowProgress, type WorkflowStep } from './WorkflowProgress';

const activeSteps: WorkflowStep[] = [
  {
    id: 'context',
    label: 'Load learning context',
    status: 'succeeded',
    detail: 'Retrieved the current skill profile and recent practice signals.',
    tool: 'Memory service',
    duration: '180ms',
  },
  {
    id: 'plan',
    label: 'Plan question set',
    status: 'succeeded',
    detail: 'Selected topics and difficulty from the learner profile.',
    duration: '1.2s',
  },
  {
    id: 'generate',
    label: 'Generate questions',
    status: 'running',
    detail: 'Creating a balanced five-question set.',
    tool: 'AI gateway',
  },
  {
    id: 'validate',
    label: 'Validate question set',
    status: 'queued',
    detail: 'Check structure, answer keys, and explanations.',
  },
  {
    id: 'save',
    label: 'Save practice session',
    status: 'queued',
  },
];

const meta = {
  title: 'Components/Workflow Progress',
  component: WorkflowProgress,
  parameters: {
    layout: 'centered',
  },
  decorators: [
    (Story) => (
      <div className="flex min-h-96 w-[calc(100vw-2rem)] max-w-[720px] items-start justify-end bg-muted/30 p-6">
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
    steps: activeSteps.map((step) => ({ ...step, status: 'queued' })),
  },
};

export const Active: Story = {
  args: {
    title: 'Preparing practice set',
    defaultExpanded: true,
    steps: activeSteps,
  },
};

export const Collapsed: Story = {
  args: {
    title: 'Preparing practice set',
    steps: activeSteps,
  },
};

export const Completed: Story = {
  args: {
    title: 'Practice set ready',
    defaultExpanded: true,
    steps: activeSteps.map((step, index) => ({
      ...step,
      status: 'succeeded',
      duration: step.duration ?? `${index + 1}.${index}s`,
    })),
  },
};

export const Failed: Story = {
  args: {
    title: 'Could not prepare practice set',
    defaultExpanded: true,
    steps: activeSteps.map((step, index) => {
      if (index < 2) return { ...step, status: 'succeeded' };
      if (index === 2) {
        return {
          ...step,
          status: 'failed',
          detail: 'The generation provider did not return a valid question set.',
        };
      }
      return { ...step, status: 'queued' };
    }),
    footer: (
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground text-xs">Your previous work is unchanged.</span>
        <Button type="button" variant="outline" size="sm">
          Retry
        </Button>
      </div>
    ),
  },
};

export const Reconnecting: Story = {
  args: {
    title: 'Updating memory',
    connectionState: 'reconnecting',
    defaultExpanded: true,
    steps: [
      {
        id: 'persist',
        label: 'Save question results',
        status: 'succeeded',
        duration: '94ms',
      },
      {
        id: 'events',
        label: 'Aggregate learning signals',
        status: 'running',
        tool: 'Memory service',
      },
      { id: 'profile', label: 'Refresh skill profile', status: 'queued' },
      { id: 'notes', label: 'Maintain memory notes', status: 'queued' },
    ],
  },
};
