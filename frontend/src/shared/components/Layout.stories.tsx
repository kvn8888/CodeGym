import type { Meta, StoryObj } from '@storybook/react-vite';
import { Layout } from './Layout';

const meta: Meta<typeof Layout> = {
  title: 'Shell/Layout',
  component: Layout,
  parameters: {
    initialPath: '/',
    routePath: '/*',
  },
};

export default meta;
type Story = StoryObj<typeof Layout>;

export const OnHistory: Story = {
  parameters: { initialPath: '/' },
};

export const OnGenerate: Story = {
  parameters: { initialPath: '/generate', routePath: '/generate' },
};

export const OnDashboard: Story = {
  parameters: { initialPath: '/dashboard', routePath: '/dashboard' },
};
