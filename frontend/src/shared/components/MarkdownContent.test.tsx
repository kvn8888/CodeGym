import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { splitQuestionStem } from '../../features/marathon/questionStem';
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

  it('keeps the generated Python stem on separate lines inside h2', () => {
    const { container } = render(
      <h2>
        <MarkdownContent variant="title">
          {'What is printed by this code?\n\nx = [1, 2]\ny = x\ny.append(3)\nprint(x)'}
        </MarkdownContent>
      </h2>,
    );

    const heading = container.querySelector('h2');
    expect(heading?.querySelector('div')).toBeNull();
    expect(heading?.querySelector('pre')).toBeNull();
    // blank line + three single newlines each become one <br>
    expect(heading?.querySelectorAll('br')).toHaveLength(4);
    expect(heading?.textContent).toContain('What is printed by this code?');
    expect(heading?.textContent).toContain('y.append(3)');
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

describe('splitQuestionStem (MCQ code-block layout)', () => {
  const EXACT_TEXT = 'What is printed by this code?\n\nx = [1, 2]\ny = x\ny.append(3)\nprint(x)';
  const EXACT_LINES = ['x = [1, 2]', 'y = x', 'y.append(3)', 'print(x)'];

  it('separates the heading from the Python listing in the exact regression case', () => {
    const { stem, code } = splitQuestionStem(EXACT_TEXT);

    expect(stem).toBe('What is printed by this code?');
    expect(code?.split('\n')).toEqual(EXACT_LINES);
  });

  it('renders the heading as heading text with the code in a separate block', () => {
    const { stem, code } = splitQuestionStem(EXACT_TEXT);
    const { container } = render(
      <>
        <h2>
          <MarkdownContent variant="title">{stem}</MarkdownContent>
        </h2>
        {code !== null && (
          <pre>
            <code>{code}</code>
          </pre>
        )}
      </>,
    );

    const heading = container.querySelector('h2');
    expect(heading?.textContent).toBe('What is printed by this code?');
    expect(heading?.querySelector('div')).toBeNull();
    expect(heading?.querySelector('pre')).toBeNull();
    expect(container.querySelector('pre code')?.textContent?.split('\n')).toEqual(EXACT_LINES);
  });

  it('leaves ordinary single-paragraph questions whole', () => {
    const text = 'What is the time complexity of **binary search**?';

    expect(splitQuestionStem(text)).toEqual({ stem: text, code: null });
  });

  it('does not mistake a prose paragraph after a blank line for code', () => {
    const text = 'Which data structure uses **FIFO** ordering?\n\nThink of a line (like a queue).';
    const { stem, code } = splitQuestionStem(text);

    expect(code).toBeNull();
    expect(stem).toBe(text);
  });

  it('does not mistake parenthesized prose lines for a code listing', () => {
    const text = 'What holds true?\n\nThink of a line (like a queue).\nServed in order (FIFO).';

    expect(splitQuestionStem(text).code).toBeNull();
  });
});
