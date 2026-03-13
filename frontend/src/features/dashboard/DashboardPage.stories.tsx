import type { Meta, StoryObj } from '@storybook/react-vite';
import { DashboardPage } from './DashboardPage';

const meta: Meta<typeof DashboardPage> = {
  title: 'Pages/Dashboard',
  component: DashboardPage,
  parameters: {
    initialPath: '/dashboard',
    routePath: '/dashboard',
  },
};

export default meta;
type Story = StoryObj<typeof DashboardPage>;

export const Default: Story = {
  name: 'Empty State (no auth yet)',
};
