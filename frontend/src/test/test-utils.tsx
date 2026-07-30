import type { ReactElement, ReactNode } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

import { ThemeProvider } from '@/components/theme-provider';
import { TooltipProvider } from '@/components/ui/tooltip';

interface ProviderProps {
  children: ReactNode;
  initialEntries?: string[];
}

function Providers({ children, initialEntries = ['/'] }: ProviderProps) {
  return (
    <ThemeProvider defaultTheme="light">
      <TooltipProvider delayDuration={200}>
        <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
      </TooltipProvider>
    </ThemeProvider>
  );
}

interface ProviderOptions extends Omit<RenderOptions, 'wrapper'> {
  initialEntries?: string[];
}

export function renderWithProviders(
  ui: ReactElement,
  { initialEntries, ...options }: ProviderOptions = {},
) {
  return render(ui, {
    wrapper: ({ children }) => <Providers initialEntries={initialEntries}>{children}</Providers>,
    ...options,
  });
}
