import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { WorkflowEvent } from '../api/client';
import { WorkflowProgress } from './WorkflowProgress';

function event(
  sequence: number,
  step_id: string,
  label: string,
  status: WorkflowEvent['status'],
  terminal = false,
): WorkflowEvent {
  return {
    operation_id: 'workflow-1',
    sequence,
    step_id,
    label,
    status,
    timestamp: '2026-07-30T12:00:00Z',
    metadata: terminal ? { terminal: true } : undefined,
  };
}

describe('WorkflowProgress', () => {
  it('renders the latest status for each stable backend step', () => {
    render(
      <WorkflowProgress
        title="Preparing practice set"
        defaultExpanded
        events={[
          event(1, 'validate', 'Validate question set', 'queued'),
          event(2, 'validate', 'Validate question set', 'failed'),
          event(3, 'repair', 'Repair invalid output if needed', 'running'),
          event(4, 'validate', 'Validate question set', 'succeeded'),
        ]}
      />,
    );
    const steps = within(screen.getByRole('list'));
    expect(steps.getAllByText('Validate question set')).toHaveLength(1);
    expect(steps.getByText('Repair invalid output if needed')).toBeInTheDocument();
  });

  it('announces reconnecting without inventing model activity', () => {
    render(
      <WorkflowProgress
        title="Updating memory"
        connectionState="reconnecting"
        defaultExpanded
        events={[event(1, 'evidence', 'Load learning evidence', 'running')]}
      />,
    );
    expect(screen.getByText('Reconnecting to progress stream')).toBeInTheDocument();
    expect(screen.queryByText(/thinking/i)).not.toBeInTheDocument();
  });
});
