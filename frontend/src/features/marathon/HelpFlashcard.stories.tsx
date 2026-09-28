import type { Meta, StoryObj } from '@storybook/react-vite';

import { HelpFlashcard } from './HelpFlashcard';

const meta: Meta<typeof HelpFlashcard> = {
  title: 'Pages/Marathon/HelpFlashcard',
  component: HelpFlashcard,
  parameters: {
    layout: 'centered',
  },
};

export default meta;
type Story = StoryObj<typeof HelpFlashcard>;

export const MarkdownExplanation: Story = {
  args: {
    concept: 'Inline Markdown Rendering',
    explanation:
      'Use `Array.prototype.push` when you want to mutate the existing array. Raw HTML like <button>not a real button</button> should stay inert, while **light emphasis** remains readable.',
    onClose: () => undefined,
  },
};
