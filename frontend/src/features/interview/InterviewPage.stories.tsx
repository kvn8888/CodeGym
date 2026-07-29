import type { Meta, StoryObj } from '@storybook/react-vite';
import { InterviewPage } from './InterviewPage';

const meta: Meta<typeof InterviewPage> = {
  title: 'Pages/Interview',
  component: InterviewPage,
  parameters: {
    initialPath: '/interviews/sess_interview_active',
    routePath: '/interviews/:sessionId',
  },
};

export default meta;
type Story = StoryObj<typeof InterviewPage>;

export const Resumable: Story = {};

export const CompletedTranscript: Story = {
  parameters: {
    initialPath: '/interviews/sess_interview_completed',
  },
};

export const MobileDark: Story = {
  parameters: {
    viewport: { defaultViewport: 'mobile1' },
  },
  decorators: [
    (Story) => (
      <div className="dark min-h-screen bg-background">
        <Story />
      </div>
    ),
  ],
};
