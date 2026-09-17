import type { WorkflowEvent } from '../../shared/api/client';
import type { WorkflowConnectionState } from '../../shared/hooks/useWorkflowProgress';
import { WorkflowProgress } from '../../shared/components/WorkflowProgress';
import { cn } from '@/lib/utils';

interface GenerationProgressActivityProps {
  title?: string;
  events: WorkflowEvent[];
  connectionState?: WorkflowConnectionState;
  defaultExpanded?: boolean;
  className?: string;
}

export function GenerationProgressActivity({
  title = 'Generating coding problem',
  events,
  connectionState = 'connected',
  defaultExpanded,
  className,
}: GenerationProgressActivityProps) {
  return (
    <div className={cn('fixed top-4 right-4 z-70 w-[min(22.5rem,calc(100vw-2rem))]', className)}>
      <WorkflowProgress
        title={title}
        events={events}
        connectionState={connectionState}
        defaultExpanded={defaultExpanded}
      />
    </div>
  );
}
