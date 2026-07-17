import type { ReactNode } from 'react';

import { cn } from '@/lib/utils';

export function WorkspacePage({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('mx-auto w-full max-w-[1180px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8', className)}>
      {children}
    </div>
  );
}

export function WorkspacePageHeader({
  title,
  description,
  actions,
  className,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        'mb-6 flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-start sm:justify-between',
        className,
      )}
    >
      <div className="min-w-0">
        <h1 className="text-2xl leading-8 font-semibold">{title}</h1>
        {description && (
          <div className="text-muted-foreground mt-1 max-w-3xl text-sm leading-5">{description}</div>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

export function WorkspaceSectionHeader({
  title,
  description,
  actions,
  className,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('mb-3 flex min-h-8 items-end justify-between gap-4', className)}>
      <div className="min-w-0">
        <h2 className="text-sm leading-5 font-semibold">{title}</h2>
        {description && <div className="text-muted-foreground mt-0.5 text-xs leading-4">{description}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

export function WorkspaceEmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode;
  title: string;
  description: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'bg-muted/20 flex min-h-40 flex-col items-start justify-center rounded-lg border border-dashed px-5 py-6',
        className,
      )}
    >
      {icon && <div className="text-muted-foreground mb-3">{icon}</div>}
      <h3 className="text-sm font-semibold">{title}</h3>
      <div className="text-muted-foreground mt-1 max-w-lg text-sm leading-5">{description}</div>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
