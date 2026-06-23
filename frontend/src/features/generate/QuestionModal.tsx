import { useState } from 'react';
import { motion } from 'motion/react';

// ── Types ────────────────────────────────────────────────────────────────────

/** A single question the agent wants the user to answer. */
export interface Question {
  /** Unique identifier for this question (e.g. "q1"). */
  id: string;
  /** The question text shown to the user. */
  text: string;
  /** Multiple-choice options. The last one is always "Specify…" which
   *  reveals a free-text input so the user can type a custom answer. */
  options: string[];
}

/** The user's answer to one question. */
export interface Answer {
  questionId: string;
  selectedOption: string;
  /** Populated when the user chose "Specify…" and typed a custom answer. */
  freeText?: string;
}

/** Props for the QuestionModal component. */
export interface QuestionModalProps {
  /** The series of questions to display (max 3 per series). */
  questions: Question[];
  /** Called when the user finishes answering all questions in the series. */
  onComplete: (answers: Answer[]) => void;
  /** Called when the user dismisses the modal without finishing. */
  onClose: () => void;
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * QuestionModal — a step-through modal that presents the user with a series
 * of multiple-choice questions before generating a problem. The agent uses
 * the answers to tailor difficulty, topic focus, and language.
 *
 * Features:
 * - Progress dots showing current question (1 / 2 / 3)
 * - Radio-style option buttons (only one selectable at a time)
 * - "Specify…" as the final option → reveals a text input
 * - [Next] advances to the next question; [Generate] on the last one
 * - Backdrop overlay that closes on click
 */
export function QuestionModal({ questions, onComplete, onClose }: QuestionModalProps) {
  // Index of the question currently being displayed (0-based).
  const [currentIndex, setCurrentIndex] = useState(0);

  // Accumulates answers as the user progresses through questions.
  // Each entry maps to the question at the same index.
  const [answers, setAnswers] = useState<Answer[]>(
    questions.map((q) => ({ questionId: q.id, selectedOption: '' }))
  );

  // The current question object being displayed.
  const question = questions[currentIndex];

  // The answer object for the current question.
  const currentAnswer = answers[currentIndex];

  // Whether the user chose "Specify…" and needs to type a free-text answer.
  const isSpecify = currentAnswer.selectedOption === 'Specify…';

  // True when the user has selected an option (and typed text if "Specify…").
  const canAdvance =
    currentAnswer.selectedOption !== '' &&
    (!isSpecify || (currentAnswer.freeText?.trim() ?? '') !== '');

  // Whether the user is on the last question in the series.
  const isLast = currentIndex === questions.length - 1;

  /** Updates the selected option for the current question. */
  const handleSelect = (option: string) => {
    setAnswers((prev) =>
      prev.map((a, i) =>
        i === currentIndex
          ? { ...a, selectedOption: option, freeText: option === 'Specify…' ? a.freeText : undefined }
          : a
      )
    );
  };

  /** Updates the free-text field when "Specify…" is chosen. */
  const handleFreeText = (text: string) => {
    setAnswers((prev) =>
      prev.map((a, i) =>
        i === currentIndex ? { ...a, freeText: text } : a
      )
    );
  };

  /** Advances to the next question or completes the series. */
  const handleNext = () => {
    if (!canAdvance) return;
    if (isLast) {
      onComplete(answers);
    } else {
      setCurrentIndex((i) => i + 1);
    }
  };

  /** Goes back to the previous question. */
  const handleBack = () => {
    if (currentIndex > 0) {
      setCurrentIndex((i) => i - 1);
    }
  };

  return (
    /* ── Backdrop overlay ───────────────────────────────────────────── */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-gray-alpha-700 backdrop-blur-sm"
      onClick={onClose}
    >
      {/* ── Modal card ────────────────────────────────────────────── */}
      <motion.div
        initial={{ opacity: 0, scale: 0.92, y: 18 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ type: 'spring', stiffness: 380, damping: 32 }}
        className="mx-4 w-full max-w-lg overflow-hidden rounded-xl border border-gray-alpha-200 bg-background-100"
        style={{ boxShadow: 'var(--cg-modal-shadow)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* ── Header ─────────────────────────────────────────────── */}
        <div className="flex items-center justify-between px-6 pt-5 pb-3">
          <h2 className="text-xl font-semibold leading-7 tracking-[-0.4px] text-gray-1000">
            Tailor Your Problem
          </h2>
          {/* Close button */}
          <button
            onClick={onClose}
            className="cg-focus flex h-7 w-7 items-center justify-center rounded-md text-gray-700 transition-colors hover:bg-gray-alpha-100 hover:text-gray-1000"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>

        {/* ── Progress dots ──────────────────────────────────────── */}
        <div className="flex gap-1.5 px-6 pb-4">
          {questions.map((_, i) => (
            <div
              key={i}
              className={`h-1.5 rounded-full transition-all duration-300 ${
                i <= currentIndex ? 'flex-[2] bg-blue-700' : 'flex-1 bg-gray-200'
              }`}
            />
          ))}
        </div>

        {/* ── Question text ──────────────────────────────────────── */}
        <div className="px-6 pb-4">
          <p className="text-sm leading-6 text-gray-900">{question.text}</p>
        </div>

        {/* ── Options ────────────────────────────────────────────── */}
        <div className="px-6 pb-4 flex flex-col gap-2">
          {question.options.map((option) => {
            // Whether this specific option is currently selected.
            const selected = currentAnswer.selectedOption === option;
            return (
              <button
                key={option}
                onClick={() => handleSelect(option)}
                className={`w-full rounded-xl border px-4 py-3 text-left text-sm transition-all duration-150 ${
                  selected
                    ? 'border-blue-400 bg-blue-100 font-medium text-gray-1000'
                    : 'border-gray-alpha-200 bg-background-100 text-gray-900 hover:border-gray-alpha-400 hover:bg-gray-100'
                }`}
              >
                <div className="flex items-center gap-3">
                  {/* Radio-style circle indicator */}
                  <div className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-full border transition-colors ${
                    selected ? 'border-blue-700' : 'border-gray-alpha-400'
                  }`}>
                    {selected && <div className="h-2 w-2 rounded-full bg-blue-700" />}
                  </div>
                  {option}
                </div>
              </button>
            );
          })}

          {/* ── Free-text input (shown when "Specify…" is selected) ── */}
          {isSpecify && (
            <input
              type="text"
              value={currentAnswer.freeText ?? ''}
              onChange={(e) => handleFreeText(e.target.value)}
              placeholder="Type your answer…"
              autoFocus
              className="cg-focus w-full rounded-md border border-gray-alpha-200 bg-background-100 px-4 py-3 text-sm text-gray-1000 placeholder-gray-700 transition-colors"
            />
          )}
        </div>

        {/* ── Footer: Back + Next/Generate ───────────────────────── */}
        <div className="flex items-center justify-between border-t border-gray-alpha-200 px-6 py-4">
          {/* Back button (hidden on first question) */}
          {currentIndex > 0 ? (
            <button
              onClick={handleBack}
              className="text-sm text-gray-900 transition-colors hover:text-gray-1000"
            >
              ← Back
            </button>
          ) : (
            <div />
          )}

          {/* Next / Generate button */}
          <button
            onClick={handleNext}
            disabled={!canAdvance}
            className="cg-focus h-10 rounded-md bg-gray-1000 px-5 text-sm font-medium text-background-100 transition-colors hover:bg-gray-900 disabled:cursor-not-allowed disabled:bg-gray-100 disabled:text-gray-700"
          >
            {isLast ? 'Generate' : 'Next'}
          </button>
        </div>
      </motion.div>
    </div>
  );
}
