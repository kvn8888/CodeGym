import type { Meta, StoryObj } from '@storybook/react-vite';
import { MarathonPage } from './MarathonPage';

const meta: Meta<typeof MarathonPage> = {
  title: 'Pages/Marathon',
  component: MarathonPage,
  parameters: {
    initialPath: '/marathon?session=sess_go_concurrency',
    routePath: '/marathon',
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof MarathonPage>;

/** An active persisted run restores its exact question, timer, and prior score. */
export const ActiveRun: Story = {};

/** A completed persisted run reopens its aggregate results. */
export const CompletedRun: Story = {
  parameters: {
    initialPath: '/marathon?session=sess_rate_limiting',
  },
};
