import type { Meta, StoryObj } from '@storybook/react-vite';
import { MarathonPage } from './MarathonPage';

const meta: Meta<typeof MarathonPage> = {
  title: 'Pages/Marathon',
  component: MarathonPage,
  parameters: {
    initialPath: '/marathon',
    routePath: '/marathon',
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof MarathonPage>;

/** Default idle state: start screen with topic selection. */
export const Default: Story = {
  name: 'Start Screen',
};
