import { InfoIcon, LightbulbIcon } from 'lucide-react';

import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';

// ── Types ────────────────────────────────────────────────────────────────────

interface HelpFlashcardProps {
  /** The concept being explained (e.g. "Binary Search Complexity"). */
  concept: string;
  /** An explanation of the concept. Does NOT answer the question directly. */
  explanation: string;
  /** Called when the user closes the flashcard. */
  onClose: () => void;
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * HelpFlashcard — a shadcn Dialog that explains a concept without revealing the
 * answer to the current marathon question. Triggered by the "Help" button.
 */
export function HelpFlashcard({ concept, explanation, onClose }: HelpFlashcardProps) {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
        <DialogHeader className="flex-row items-center gap-2 space-y-0 bg-amber-100 px-6 pt-5 pb-3 text-left">
          <LightbulbIcon className="size-4 text-amber-900" />
          <DialogTitle className="text-base text-amber-900">{concept}</DialogTitle>
        </DialogHeader>

        <div className="px-6 pt-4 pb-6">
          <p className="text-muted-foreground text-sm leading-6">{explanation}</p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
