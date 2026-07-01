import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

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
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="px-6 pt-5 pb-3 text-left">
          <DialogTitle className="text-xl tracking-[-0.4px]">Tailor Your Problem</DialogTitle>
        </DialogHeader>

        {/* Progress dots */}
        <div className="flex gap-1.5 px-6 pb-4">
          {questions.map((_, i) => (
            <div
              key={i}
              className={`h-1.5 rounded-full transition-all duration-300 ${
                i <= currentIndex ? 'bg-primary flex-[2]' : 'bg-muted flex-1'
              }`}
            />
          ))}
        </div>

        {/* Question text */}
        <div className="px-6 pb-4">
          <p className="text-muted-foreground text-sm leading-6">{question.text}</p>
        </div>

        {/* Options */}
        <div className="flex flex-col gap-2 px-6 pb-4">
          {question.options.map((option) => {
            const selected = currentAnswer.selectedOption === option;
            return (
              <button
                key={option}
                onClick={() => handleSelect(option)}
                className={`w-full rounded-lg border px-4 py-3 text-left text-sm transition-all duration-150 ${
                  selected
                    ? 'border-primary bg-accent text-foreground font-medium'
                    : 'bg-background text-muted-foreground hover:bg-accent/50'
                }`}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-full border transition-colors ${
                      selected ? 'border-primary' : 'border-input'
                    }`}
                  >
                    {selected && <div className="bg-primary h-2 w-2 rounded-full" />}
                  </div>
                  {option}
                </div>
              </button>
            );
          })}

          {/* Free-text input (shown when "Specify…" is selected) */}
          {isSpecify && (
            <Input
              type="text"
              value={currentAnswer.freeText ?? ''}
              onChange={(e) => handleFreeText(e.target.value)}
              placeholder="Type your answer…"
              autoFocus
            />
          )}
        </div>

        {/* Footer: Back + Next/Generate */}
        <div className="flex items-center justify-between border-t px-6 py-4">
          {currentIndex > 0 ? (
            <Button variant="ghost" size="sm" onClick={handleBack}>
              ← Back
            </Button>
          ) : (
            <div />
          )}

          <Button onClick={handleNext} disabled={!canAdvance}>
            {isLast ? 'Generate' : 'Next'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
