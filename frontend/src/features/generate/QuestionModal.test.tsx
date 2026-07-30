import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import type { PracticeIntakeQuestion } from '../../shared/api/types';
import { renderWithProviders } from '../../test/test-utils';
import { QuestionModal } from './QuestionModal';

const questions: PracticeIntakeQuestion[] = [
  {
    id: 'q1',
    dimension: 'exposure',
    text: 'How familiar are you with graph traversal?',
    options: [
      { id: 'new', label: 'This is new to me' },
      { id: 'practiced', label: 'I have practiced it' },
    ],
  },
  {
    id: 'q2',
    dimension: 'challenge',
    text: 'What should this session emphasize?',
    options: [
      { id: 'foundations', label: 'Reinforce foundations' },
      { id: 'stretch', label: 'Push me with edge cases' },
    ],
  },
];

describe('QuestionModal', () => {
  it('records answers, advances progress, and completes the baseline', async () => {
    const user = userEvent.setup();
    const onSaveAnswer = vi.fn();
    const onComplete = vi.fn();
    const { container } = renderWithProviders(
      <QuestionModal
        topic="graph traversal"
        questions={questions}
        onSaveAnswer={onSaveAnswer}
        onComplete={onComplete}
        onSkip={vi.fn()}
      />,
    );

    expect(screen.getByText('Topic baseline · 1/2')).toBeInTheDocument();
    expect(container.ownerDocument.querySelectorAll('[aria-hidden="true"] span')).toHaveLength(2);

    await user.click(screen.getByRole('button', { name: 'I have practiced it' }));
    expect(onSaveAnswer).toHaveBeenCalledWith('q1', 'practiced');
    await user.click(screen.getByRole('button', { name: 'Next' }));

    expect(screen.getByText('Topic baseline · 2/2')).toBeInTheDocument();
    const buildButton = screen.getByRole('button', { name: 'Build session' });
    expect(buildButton).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'Push me with edge cases' }));
    expect(onSaveAnswer).toHaveBeenCalledWith('q2', 'stretch');
    expect(buildButton).toBeEnabled();

    await user.click(buildButton);
    expect(onComplete).toHaveBeenCalledOnce();
  });
});
