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
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/30 backdrop-blur-sm"
      onClick={onClose}
    >
      {/* ── Modal card ────────────────────────────────────────────── */}
      <motion.div
        initial={{ opacity: 0, scale: 0.92, y: 18 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ type: 'spring', stiffness: 380, damping: 30 }}
        className="w-full max-w-lg mx-4 rounded-2xl border border-chalk bg-white overflow-hidden"
        style={{ boxShadow: 'var(--cg-card-shadow)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* ── Header ─────────────────────────────────────────────── */}
        <div className="flex items-center justify-between px-6 pt-5 pb-3">
          <h2 className="font-display text-lg font-semibold tracking-tight text-ink">
            Let's tailor your problem<span className="text-blue">.</span>
          </h2>
          {/* Close button */}
          <button
            onClick={onClose}
            className="w-6 h-6 flex items-center justify-center rounded-md text-ash hover:text-ink hover:bg-grain transition-colors"
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
                i <= currentIndex ? 'bg-blue flex-[2]' : 'bg-chalk flex-1'
              }`}
            />
          ))}
        </div>

        {/* ── Question text ──────────────────────────────────────── */}
        <div className="px-6 pb-4">
          <p className="text-sm text-graphite leading-relaxed">{question.text}</p>
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
                className={`w-full text-left text-sm px-4 py-3 rounded-xl border-2 transition-all duration-150 ${
                  selected
                    ? 'border-blue bg-blue-tint text-ink font-medium'
                    : 'border-chalk bg-shell text-graphite hover:border-ash hover:bg-bone'
                }`}
              >
                <div className="flex items-center gap-3">
                  {/* Radio-style circle indicator */}
                  <div className={`w-4 h-4 rounded-full border-2 shrink-0 flex items-center justify-center transition-colors ${
                    selected ? 'border-blue' : 'border-chalk'
                  }`}>
                    {selected && <div className="w-2 h-2 rounded-full bg-blue" />}
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
              className="w-full px-4 py-3 rounded-xl border border-chalk bg-parchment text-sm text-ink placeholder-ash focus:outline-none focus:border-ash transition-colors"
            />
          )}
        </div>

        {/* ── Footer: Back + Next/Generate ───────────────────────── */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-chalk">
          {/* Back button (hidden on first question) */}
          {currentIndex > 0 ? (
            <button
              onClick={handleBack}
              className="text-xs text-graphite hover:text-ink transition-colors"
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
            className="px-5 py-2.5 bg-ink text-bone text-xs font-bold rounded-xl hover:bg-ink-soft disabled:bg-chalk disabled:text-ash disabled:cursor-not-allowed transition-colors"
          >
            {isLast ? 'Generate' : 'Next'}
          </button>
        </div>
      </motion.div>
    </div>
  );
}
