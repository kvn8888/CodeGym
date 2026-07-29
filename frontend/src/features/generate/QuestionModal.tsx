import { useMemo, useState } from 'react';
import { ArrowLeft, ArrowRight, LoaderCircle } from 'lucide-react';

import type { PracticeIntakeQuestion } from '../../shared/api/types';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';

export interface QuestionModalProps {
  topic: string;
  questions: PracticeIntakeQuestion[];
  initialAnswers?: Record<string, string>;
  saving?: boolean;
  onSaveAnswer: (questionId: string, optionId: string) => Promise<void> | void;
  onComplete: () => Promise<void> | void;
  onSkip: () => Promise<void> | void;
}

/**
 * A compact, resumable topic baseline. It records self-report only; the copy
 * deliberately avoids presenting answers as demonstrated skill.
 */
export function QuestionModal({
  topic,
  questions,
  initialAnswers = {},
  saving = false,
  onSaveAnswer,
  onComplete,
  onSkip,
}: QuestionModalProps) {
  const firstUnanswered = questions.findIndex((question) => !initialAnswers[question.id]);
  const [currentIndex, setCurrentIndex] = useState(
    firstUnanswered === -1 ? Math.max(0, questions.length - 1) : firstUnanswered,
  );
  const [answers, setAnswers] = useState<Record<string, string>>(initialAnswers);
  const question = questions[currentIndex];
  const selected = question ? answers[question.id] : undefined;
  const complete = useMemo(
    () => questions.length > 0 && questions.every((item) => Boolean(answers[item.id])),
    [answers, questions],
  );
  const isLast = currentIndex === questions.length - 1;

  if (!question) return null;

  const choose = async (optionId: string) => {
    const previous = answers[question.id];
    setAnswers((current) => ({ ...current, [question.id]: optionId }));
    try {
      await onSaveAnswer(question.id, optionId);
    } catch {
      setAnswers((current) => {
        const reverted = { ...current };
        if (previous) reverted[question.id] = previous;
        else delete reverted[question.id];
        return reverted;
      });
    }
  };

  return (
    <Dialog open>
      <DialogContent
        className="gap-0 overflow-hidden p-0 sm:max-w-xl"
        onEscapeKeyDown={(event) => event.preventDefault()}
        onPointerDownOutside={(event) => event.preventDefault()}
      >
        <DialogHeader className="border-b px-5 py-4 text-left sm:px-6">
          <div className="text-muted-foreground font-mono text-[11px] tracking-[0.12em] uppercase">
            Topic baseline · {currentIndex + 1}/{questions.length}
          </div>
          <DialogTitle className="mt-2 text-xl">A quick read on {topic}</DialogTitle>
          <DialogDescription>
            This is your self-reported starting point. Practice results remain the stronger signal.
          </DialogDescription>
        </DialogHeader>

        <div className="px-5 py-5 sm:px-6">
          <div className="mb-5 flex gap-1.5" aria-hidden="true">
            {questions.map((item, index) => (
              <span
                key={item.id}
                className={`h-1 flex-1 rounded-full ${
                  index <= currentIndex ? 'bg-foreground' : 'bg-muted'
                }`}
              />
            ))}
          </div>

          <p className="text-base leading-7 font-medium">{question.text}</p>
          <div className="mt-4 overflow-hidden rounded-md border">
            {question.options.map((option) => {
              const active = selected === option.id;
              return (
                <button
                  key={option.id}
                  type="button"
                  disabled={saving}
                  aria-pressed={active}
                  onClick={() => void choose(option.id)}
                  className={`flex min-h-12 w-full items-center gap-3 border-b px-4 py-3 text-left text-sm last:border-b-0 ${
                    active ? 'bg-foreground text-background' : 'hover:bg-muted/40'
                  }`}
                >
                  <span
                    className={`h-3.5 w-3.5 shrink-0 rounded-full border ${
                      active ? 'border-background bg-background shadow-[inset_0_0_0_3px_var(--foreground)]' : ''
                    }`}
                  />
                  {option.label}
                </button>
              );
            })}
          </div>
        </div>

        <div className="bg-muted/20 flex items-center justify-between gap-3 border-t px-5 py-3 sm:px-6">
          <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={() => void onSkip()}>
            Skip baseline
          </Button>
          <div className="flex items-center gap-2">
            {currentIndex > 0 && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={saving}
                onClick={() => setCurrentIndex((value) => value - 1)}
              >
                <ArrowLeft />
                Back
              </Button>
            )}
            <Button
              type="button"
              size="sm"
              disabled={!selected || saving || (isLast && !complete)}
              onClick={() =>
                isLast ? void onComplete() : setCurrentIndex((value) => value + 1)
              }
            >
              {saving ? <LoaderCircle className="animate-spin" /> : isLast ? 'Build session' : 'Next'}
              {!saving && !isLast && <ArrowRight />}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
