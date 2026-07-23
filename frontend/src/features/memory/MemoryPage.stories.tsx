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
  name: 'Populated',
};

export const Empty: Story = {
  parameters: { mockApiScenario: 'empty' },
};

export const LegacyNullableCollections: Story = {
  name: 'Legacy nullable collections',
  parameters: { mockApiScenario: 'nullable-memory' },
};

export const Loading: Story = {
  parameters: { mockApiScenario: 'loading' },
};

export const ErrorState: Story = {
  name: 'Error',
  parameters: { mockApiScenario: 'error' },
};
