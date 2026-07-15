import type { Meta, StoryObj } from '@storybook/react-vite';
import { GeneratePage } from './GeneratePage';

const meta: Meta<typeof GeneratePage> = {
  title: 'Pages/Generate',
  component: GeneratePage,
  parameters: {
    initialPath: '/generate',
    routePath: '/generate',
  },
};

export default meta;
type Story = StoryObj<typeof GeneratePage>;

export const Default: Story = {
  name: 'Personalized',
};

export const FirstSession: Story = {
  parameters: { mockApiScenario: 'empty' },
};

export const Prefilled: Story = {
  parameters: {
    initialPath: '/generate?prompt=Go%20concurrency%20and%20channel%20ownership',
  },
};
