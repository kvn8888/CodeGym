import { describe, expect, it } from 'vitest';

import type { WorkflowEvent } from '../api/client';
import { mergeWorkflowEvent } from './useWorkflowProgress';

function event(sequence: number): WorkflowEvent {
  return {
    operation_id: 'workflow-1',
    sequence,
    step_id: `step-${sequence}`,
    label: `Step ${sequence}`,
    status: 'running',
    timestamp: '2026-07-30T12:00:00Z',
  };
}

describe('mergeWorkflowEvent', () => {
  it('deduplicates replayed events by sequence', () => {
    const initial = [event(1), event(2)];
    expect(mergeWorkflowEvent(initial, event(2))).toBe(initial);
  });

  it('restores strict order when reconnect delivery overlaps', () => {
    const merged = mergeWorkflowEvent([event(1), event(3)], event(2));
    expect(merged.map((item) => item.sequence)).toEqual([1, 2, 3]);
  });
});
