import type { Meta, StoryObj } from '@storybook/react-vite';
import { ChatPanel } from './ChatPanel';
import type { ChatThreadWithMessages } from '../../shared/api/types';

const coachThread: ChatThreadWithMessages = {
  id: 'story-coach',
  workspace_id: 'story-workspace',
  user_id: 'story-user',
  session_id: 'story-session',
  kind: 'coach',
  status: 'active',
  context: { version: 1, session_id: 'story-session', problem_id: 'two-sum' },
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  messages: [
    {
      id: 'story-assistant',
      thread_id: 'story-coach',
      role: 'assistant',
      content: 'What invariant should remain true as you scan the array?',
      status: 'complete',
      created_at: new Date().toISOString(),
    },
  ],
};

const meta: Meta<typeof ChatPanel> = {
  title: 'Components/ChatPanel',
  component: ChatPanel,
  parameters: {
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof ChatPanel>;

export const Default: Story = {
  args: { initialThread: coachThread },
  decorators: [
    (Story) => (
      <div className="mx-auto mt-8 h-[480px] max-w-sm overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100">
        <Story />
      </div>
    ),
  ],
};

export const InterviewResume: Story = {
  args: {
    kind: 'interview',
    mode: 'coding',
    initialThread: {
      ...coachThread,
      id: 'story-interview',
      kind: 'interview',
      mode: 'coding',
      messages: [
        {
          ...coachThread.messages[0],
          id: 'opening',
          thread_id: 'story-interview',
          content: 'Design an approach for finding the first repeated value in a stream.',
        },
        {
          id: 'answer',
          thread_id: 'story-interview',
          role: 'user',
          content: 'I would start with a set and clarify whether memory is bounded.',
          status: 'complete',
          created_at: new Date().toISOString(),
        },
      ],
    },
  },
  decorators: [
    (Story) => (
      <div className="mx-auto mt-8 h-[620px] max-w-2xl overflow-hidden rounded-xl border bg-background">
        <Story />
      </div>
    ),
  ],
};

export const CoachMobileDark: Story = {
  args: { initialThread: coachThread },
  decorators: [
    (Story) => (
      <div className="dark min-h-screen bg-background p-2">
        <div className="h-[560px] w-[340px] max-w-full overflow-hidden rounded-xl border bg-background">
          <Story />
        </div>
      </div>
    ),
  ],
};
