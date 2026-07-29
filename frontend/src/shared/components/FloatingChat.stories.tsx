import type { Meta, StoryObj } from '@storybook/react-vite';
import { FloatingChat } from './FloatingChat';

const meta: Meta<typeof FloatingChat> = {
  title: 'Components/FloatingChat',
  component: FloatingChat,
  parameters: {
    layout: 'fullscreen',
    initialPath: '/problems/two-sum?session=sess_api_cache_draft',
  },
};

export default meta;
type Story = StoryObj<typeof FloatingChat>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <div className="min-h-screen bg-background-200">
        <Story />
      </div>
    ),
  ],
};
