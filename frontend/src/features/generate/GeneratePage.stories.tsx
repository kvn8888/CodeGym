import type { Meta, StoryObj } from '@storybook/react-vite';
import { GeneratePage } from './GeneratePage';
import type { PracticeIntake } from '../../shared/api/types';

const now = new Date().toISOString();
const intakeQuestions = [
  {
    id: 'q1',
    dimension: 'exposure',
    text: 'How much prior exposure do you have to graph traversal?',
    options: [
      { id: 'new', label: 'This is new to me' },
      { id: 'some', label: 'I recognize the core ideas' },
      { id: 'practiced', label: 'I have practiced it before' },
    ],
  },
  {
    id: 'q2',
    dimension: 'application',
    text: 'Where have you applied these ideas?',
    options: [
      { id: 'none', label: 'Not in practice yet' },
      { id: 'guided', label: 'In guided exercises' },
      { id: 'independent', label: 'In a project or interview' },
    ],
  },
  {
    id: 'q3',
    dimension: 'challenge',
    text: 'What kind of challenge would help today?',
    options: [
      { id: 'foundations', label: 'Reinforce foundations' },
      { id: 'mixed', label: 'Mix recall with application' },
      { id: 'stretch', label: 'Push me with edge cases' },
    ],
  },
];

function intake(overrides: Partial<PracticeIntake> = {}): PracticeIntake {
  return {
    id: 'intake-story',
    workspace_id: 'workspace-story',
    user_id: 'user-story',
    normalized_topic: 'graph traversal',
    original_topic: 'Graph traversal',
    practice_seed: {},
    questions: intakeQuestions,
    answers: {},
    status: 'pending',
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

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

export const IntakePopulated: Story = {
  args: { initialIntake: intake() },
};

export const IntakePartialResumed: Story = {
  args: { initialIntake: intake({ answers: { q1: 'some' } }) },
};

export const IntakeLoading: Story = {
  args: { initialStarting: true },
  parameters: { initialPath: '/generate?prompt=Graph%20traversal' },
};

export const GeneratedCodingProgress: Story = {
  name: 'Generated coding · progress',
  args: { initialFormat: 'coding', initialStarting: true },
  parameters: { initialPath: '/generate?prompt=Graph%20shortest%20paths' },
};

export const GeneratedCodingFailure: Story = {
  name: 'Generated coding · retryable failure',
  args: {
    initialFormat: 'coding',
    initialStartError: 'The model did not return a usable coding problem. Try again.',
  },
  parameters: { initialPath: '/generate?prompt=Graph%20shortest%20paths' },
};

export const IntakeError: Story = {
  args: {
    initialIntake: intake({
      questions: [],
      generation_error: 'The model did not return a usable baseline. Retry or skip.',
    }),
  },
};

export const KnownTopicSuppressed: Story = {
  name: 'Known topic · suppressed',
  args: {
    initialIntake: intake({
      normalized_topic: 'go',
      original_topic: 'Go',
      status: 'skipped',
      suppression_reason: 'demonstrated_event',
      questions: [],
    }),
  },
};

export const SkippedTopicSuppressed: Story = {
  name: 'Skipped topic · suppressed',
  args: {
    initialIntake: intake({
      normalized_topic: 'rust',
      original_topic: 'Rust',
      status: 'skipped',
      suppression_reason: 'user_skipped',
      questions: [],
    }),
  },
};
