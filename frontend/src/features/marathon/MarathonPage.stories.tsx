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

/** Exact-set selection is restored without confirming the question. */
export const MultiSelectRun: Story = {
  parameters: {
    initialPath: '/marathon?session=sess_multi_select',
  },
};

/** A written draft is restored and can be evaluated without losing its text. */
export const FreeResponseRun: Story = {
  parameters: {
    initialPath: '/marathon?session=sess_free_response',
  },
};

/** A completed persisted run reopens its aggregate results. */
export const CompletedRun: Story = {
  parameters: {
    initialPath: '/marathon?session=sess_rate_limiting',
  },
};
