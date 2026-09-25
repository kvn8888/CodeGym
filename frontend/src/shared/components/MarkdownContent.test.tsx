import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { MarkdownContent } from './MarkdownContent';

describe('MarkdownContent (issue #110)', () => {
  it('renders inline backticks as code without literal backticks', () => {
    const { container } = render(
      <MarkdownContent variant="inline">{'Use `O(1)` for lookup'}</MarkdownContent>,
    );

    const code = screen.getByText('O(1)');
    expect(code.tagName).toBe('CODE');
    expect(container.textContent).not.toContain('`');
  });

  it('renders title backticks as code without literal backticks', () => {
    const { container } = render(
      <MarkdownContent variant="title">{'What does `arr.push(x)` return?'}</MarkdownContent>,
    );

    const code = screen.getByText('arr.push(x)');
    expect(code.tagName).toBe('CODE');
    expect(container.textContent).not.toContain('`');
  });

  it('renders the title variant inside h2 without inserting a div', () => {
    const { container } = render(
      <h2>
        <MarkdownContent variant="title">{'What does `arr.push(x)` do?'}</MarkdownContent>
      </h2>,
    );

    const heading = container.querySelector('h2');
    expect(heading).not.toBeNull();
    expect(heading?.querySelector('div')).toBeNull();
    expect(heading?.querySelector('span')).not.toBeNull();
  });

  it('leaves ordinary text unchanged without code elements', () => {
    const { container } = render(
      <MarkdownContent variant="title">{'Append to a dynamic array'}</MarkdownContent>,
    );

    expect(screen.getByText('Append to a dynamic array')).toBeInTheDocument();
    expect(container.querySelector('code')).toBeNull();
    expect(container.querySelector('p')).toBeNull();
  });

  it('does not execute untrusted HTML from model copy', () => {
    const { container } = render(
      <MarkdownContent>{'<img src="x" onerror="alert(1)"> hello'}</MarkdownContent>,
    );

    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('hello');
  });
});
