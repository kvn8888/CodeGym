import type { Meta, StoryObj } from '@storybook/react-vite';
import { SettingsPage } from './SettingsPage';

const meta: Meta<typeof SettingsPage> = {
  title: 'Pages/Settings',
  component: SettingsPage,
  parameters: {
    initialPath: '/settings',
    routePath: '/settings',
  },
};

export default meta;
type Story = StoryObj<typeof SettingsPage>;

export const Default: Story = {};
