import type { Meta, StoryObj } from '@storybook/react-vite';
import { QuestionModal } from './QuestionModal';

// Sample questions that mimic what the Claude agent would generate.
const SAMPLE_QUESTIONS = [
  {
    id: 'q1',
    text: 'What programming language would you like to use?',
    options: ['Python', 'Go', 'JavaScript', 'TypeScript', 'Specify…'],
  },
  {
    id: 'q2',
    text: 'What aspect of two pointers do you want to focus on?',
    options: ['Sliding window', 'Fast/slow pointers', 'Meeting in middle', 'Specify…'],
  },
  {
    id: 'q3',
    text: 'How familiar are you with this topic?',
    options: ['Just starting out', 'Somewhat comfortable', 'Advanced — challenge me', 'Specify…'],
  },
];

const meta: Meta<typeof QuestionModal> = {
  title: 'Features/QuestionModal',
  component: QuestionModal,
  parameters: {
    // QuestionModal is a full-screen overlay, so no routing needed.
    layout: 'fullscreen',
  },
  args: {
    questions: SAMPLE_QUESTIONS,
    onComplete: (answers) => console.log('[QuestionModal] onComplete:', answers),
    onClose: () => console.log('[QuestionModal] onClose'),
  },
};

export default meta;
type Story = StoryObj<typeof QuestionModal>;

/** Default state: first question visible, no selection. */
export const Default: Story = {
  name: 'Three Questions',
};

/** A single-question series (e.g. when the agent only needs one clarification). */
export const SingleQuestion: Story = {
  args: {
    questions: [SAMPLE_QUESTIONS[0]],
  },
};

/** Two-question series. */
export const TwoQuestions: Story = {
  args: {
    questions: SAMPLE_QUESTIONS.slice(0, 2),
  },
};
