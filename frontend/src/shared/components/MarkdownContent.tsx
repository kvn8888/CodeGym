import { Children, type ReactNode } from 'react';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';

import { cn } from '@/lib/utils';

type MarkdownVariant = 'prose' | 'title' | 'inline';

interface MarkdownContentProps {
  children: string;
  className?: string;
  /**
   * - prose: full markdown body (coach messages, help, feedback)
   * - title: question heading; paragraphs inherit surrounding title styles
   * - inline: option labels and tight UI chrome without block margins
   */
  variant?: MarkdownVariant;
}

/**
 * Title paragraphs stay heading-safe: ordinary single newlines become <br>
 * (valid phrasing content inside <h2>) instead of collapsing to spaces, so
 * generated stems with plain-\n code lines keep each line separate.
 */
function TitleLines({ children }: { children?: ReactNode }) {
  const lines: ReactNode[] = [];
  Children.forEach(children, (child, index) => {
    if (typeof child !== 'string') {
      lines.push(child);
      return;
    }
    child.split('\n').forEach((line, lineIndex) => {
      if (lineIndex > 0) lines.push(<br key={`title-br-${index}-${lineIndex}`} />);
      lines.push(line);
    });
  });
  return <>{lines}</>;
}

const titleComponents: Components = {
  p: ({ children }) => <TitleLines>{children}</TitleLines>,
};

const inlineComponents: Components = {
  p: ({ children }) => <span className="inline">{children}</span>,
  pre: ({ children }) => (
    <pre className="my-1 max-w-full overflow-x-auto rounded-md px-2 py-1.5 text-[0.85em]">
      {children}
    </pre>
  ),
};

/**
 * Renders markdown (including fenced code blocks) with the shared prose-geist
 * styles used by problem descriptions. Safe for coach chat, MCQ stems, and options.
 */
export function MarkdownContent({
  children,
  className,
  variant = 'prose',
}: MarkdownContentProps) {
  if (!children.trim()) return null;

  const components =
    variant === 'title' ? titleComponents : variant === 'inline' ? inlineComponents : undefined;

  // Title/inline slots live inside <h2> and <button> chrome, where a <div>
  // wrapper would be invalid heading/button content. Prose keeps the <div>.
  const Wrapper = variant === 'prose' ? 'div' : 'span';

  // Generated stems separate code lines with plain newlines (including blank
  // lines), which markdown would otherwise collapse inside the <h2> heading.
  // Fold blank lines so every break renders as one <br> via TitleLines.
  const source = variant === 'title' ? children.replace(/\n{2,}/g, '\n') : children;

  return (
    <Wrapper
      className={cn(
        'prose-geist min-w-0 max-w-none break-words',
        variant === 'title' && 'text-inherit [&_code]:text-[0.9em]',
        variant === 'inline' &&
          'text-inherit leading-inherit [&_p]:m-0 [&_code]:text-[0.9em] [&_pre]:text-[0.85em]',
        className,
      )}
    >
      <ReactMarkdown rehypePlugins={[rehypeHighlight]} components={components}>
        {source}
      </ReactMarkdown>
    </Wrapper>
  );
}
