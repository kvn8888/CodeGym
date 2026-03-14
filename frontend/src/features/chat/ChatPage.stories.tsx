import type { Meta, StoryObj } from '@storybook/react-vite';
import { ChatPage } from './ChatPage';

const meta: Meta<typeof ChatPage> = {
  title: 'Pages/Chat',
  component: ChatPage,
  parameters: {
    initialPath: '/chat',
    routePath: '/chat',
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof ChatPage>;

/** Default state with welcome message from Claude. */
export const Default: Story = {
  name: 'Welcome Message',
};
