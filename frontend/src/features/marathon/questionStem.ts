/** A single line reads as code when it carries assignment/call/bracket syntax
 *  (or Python keywords, indentation, comments) rather than sentence prose. A
 *  `?` disqualifies the line: questions are prose, Python has no `?` operator. */
function isCodeLine(line: string): boolean {
  const trimmed = line.trim();
  if (!trimmed || trimmed.includes('?')) return false;
  if (/^\s{2,}|\t/.test(line)) return true;
  if (
    /^(from\s+|import\s+|def\s+|class\s+|return\b|if\b|elif\b|else:|for\b|while\b|with\b|print\s*\(|#)/.test(
      trimmed,
    )
  )
    return true;
  return /[=()[\]{};]/.test(trimmed);
}

/** A trailing block reads as a code listing when every non-empty line is
 *  code-like and at least one line carries a strong signal (assignment,
 *  brackets, indentation, keyword, comment, or a `.name(` call). Plain
 *  parenthesized prose such as "(like a queue)" has no strong signal, so a
 *  prose paragraph after a blank line is never mistaken for code. */
function isCodeBlock(block: string): boolean {
  const lines = block.split('\n').filter((line) => line.trim() !== '');
  if (lines.length < 2) return false;
  if (!lines.every(isCodeLine)) return false;
  return lines.some((line) =>
    /=|\[|\]|{|}|;|^\s{2,}|\t|^\s*(from\s+|import\s+|def\s+|class\s+|return\b|if\b|elif\b|else:|for\b|while\b|with\b|print\s*\(|#)|\.\w+\s*\(/.test(
      line,
    ),
  );
}

export interface QuestionStem {
  /** Heading sentence(s): the first blank-line-separated block. */
  stem: string;
  /** Trailing code listing, or null when the text has no code-like block. */
  code: string | null;
}

/**
 * splitQuestionStem — separates a generated MCQ stem ("What is printed by
 * this code?\n\nx = [1, 2]\n…") into its heading and an optional trailing
 * code listing. Only a trailing block that reads as code is split out;
 * ordinary questions and prose paragraphs after a blank line stay whole so
 * existing markdown rendering is preserved.
 */
export function splitQuestionStem(text: string): QuestionStem {
  const blocks = text.split(/\n\s*\n/);
  if (blocks.length < 2) return { stem: text, code: null };
  const tail = blocks.slice(1).join('\n\n').trim();
  if (!tail || !isCodeBlock(tail)) return { stem: text, code: null };
  return { stem: blocks[0].trim(), code: tail };
}
