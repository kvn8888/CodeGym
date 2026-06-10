import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryPage } from './MemoryPage';

const meta: Meta<typeof MemoryPage> = {
  title: 'Pages/Memory',
  component: MemoryPage,
  parameters: {
    initialPath: '/memory',
    routePath: '/memory',
  },
};

export default meta;
type Story = StoryObj<typeof MemoryPage>;

export const Default: Story = {
  name: 'Mock Profile',
};
