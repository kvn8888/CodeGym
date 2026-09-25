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

const titleComponents: Components = {
  p: ({ children }) => <>{children}</>,
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
        {children}
      </ReactMarkdown>
    </Wrapper>
  );
}
