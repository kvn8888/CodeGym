import type { Meta, StoryObj } from '@storybook/react-vite';
import { FloatingChat } from './FloatingChat';

const meta: Meta<typeof FloatingChat> = {
  title: 'Components/FloatingChat',
  component: FloatingChat,
  parameters: {
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof FloatingChat>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <div className="min-h-screen bg-bone">
        <Story />
      </div>
    ),
  ],
};