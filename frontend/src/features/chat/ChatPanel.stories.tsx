import type { Meta, StoryObj } from '@storybook/react-vite';
import { ChatPanel } from './ChatPanel';

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
  decorators: [
    (Story) => (
      <div className="h-[480px] max-w-sm mx-auto mt-8 border border-chalk rounded-2xl overflow-hidden bg-bone">
        <Story />
      </div>
    ),
  ],
};