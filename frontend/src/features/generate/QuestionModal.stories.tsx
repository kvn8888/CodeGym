import type { Meta, StoryObj } from '@storybook/react-vite';
import { QuestionModal } from './QuestionModal';

const questions = [
  {
    id: 'q1',
    dimension: 'exposure',
    text: 'How much prior exposure do you have to graph traversal?',
    options: [
      { id: 'new', label: 'This is new to me' },
      { id: 'recognize', label: 'I recognize the core ideas' },
      { id: 'practiced', label: 'I have solved a few related problems' },
    ],
  },
  {
    id: 'q2',
    dimension: 'application',
    text: 'Where have you used these ideas?',
    options: [
      { id: 'none', label: 'Not in practice yet' },
      { id: 'guided', label: 'In guided exercises' },
      { id: 'independent', label: 'In an independent project or interview' },
    ],
  },
  {
    id: 'q3',
    dimension: 'challenge',
    text: 'What kind of session would be most useful today?',
    options: [
      { id: 'foundations', label: 'Reinforce foundations' },
      { id: 'mixed', label: 'Mix recall with application' },
      { id: 'stretch', label: 'Push me with edge cases' },
    ],
  },
];

const meta: Meta<typeof QuestionModal> = {
  title: 'Features/Practice Intake',
  component: QuestionModal,
  parameters: { layout: 'fullscreen' },
  args: {
    topic: 'graph traversal',
    questions,
    onSaveAnswer: () => {},
    onComplete: () => {},
    onSkip: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof QuestionModal>;

export const Populated: Story = {};

export const PartialResume: Story = {
  args: { initialAnswers: { q1: 'recognize' } },
};

export const Saving: Story = {
  args: { initialAnswers: { q1: 'recognize' }, saving: true },
};
