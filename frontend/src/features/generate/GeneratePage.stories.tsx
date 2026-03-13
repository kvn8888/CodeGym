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
  name: 'Empty Prompt',
};

// To see the UI after clicking an example prompt, interact in Storybook directly.
// No additional stories needed — state is local to the component.
