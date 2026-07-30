import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ThemeProvider, useTheme } from './theme-provider';
import { renderWithProviders } from '@/test/test-utils';

function ThemeControl() {
  const { theme, setTheme } = useTheme();
  return (
    <>
      <span>{theme}</span>
      <button type="button" onClick={() => setTheme('system')}>
        Use system
      </button>
    </>
  );
}

describe('ThemeProvider', () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.classList.remove('light', 'dark');
  });

  it('restores the stored theme and applies its document class', async () => {
    window.localStorage.setItem('codegym-theme', 'dark');

    renderWithProviders(<ThemeControl />);

    expect(screen.getByText('dark')).toBeInTheDocument();
    await waitFor(() => expect(document.documentElement).toHaveClass('dark'));
    expect(document.documentElement).not.toHaveClass('light');
  });

  it('persists a change and resolves the system theme', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ThemeProvider defaultTheme="light">
        <ThemeControl />
      </ThemeProvider>,
    );

    await user.click(screen.getByRole('button', { name: 'Use system' }));

    expect(window.localStorage.getItem('codegym-theme')).toBe('system');
    await waitFor(() => expect(document.documentElement).toHaveClass('light'));
  });
});
