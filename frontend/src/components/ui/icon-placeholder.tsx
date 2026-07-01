import * as React from 'react';
import {
  ArrowDownIcon,
  ArrowUpIcon,
  Loader2Icon,
  SquareIcon,
  StopCircleIcon,
  type LucideIcon,
} from 'lucide-react';

type IconLibraryProps = {
  lucide?: string;
  tabler?: string;
  hugeicons?: string;
  phosphor?: string;
  remixicon?: string;
};

const LUCIDE_ICONS: Record<string, LucideIcon> = {
  ArrowDownIcon,
  ArrowUpIcon,
  Loader2Icon,
  StopCircleIcon,
};

/**
 * Resolves the configured icon library (lucide per components.json) to a concrete icon.
 */
export function IconPlaceholder({
  lucide,
  className,
  ...props
}: IconLibraryProps & React.ComponentProps<'svg'>) {
  if (!lucide) {
    return null;
  }

  const Icon = LUCIDE_ICONS[lucide];

  if (!Icon) {
    return <SquareIcon className={className} {...props} />;
  }

  return <Icon className={className} {...props} />;
}