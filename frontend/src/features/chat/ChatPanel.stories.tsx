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
      <div className="mx-auto mt-8 h-[480px] max-w-sm overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100">
        <Story />
      </div>
    ),
  ],
};
