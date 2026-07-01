import { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowUpIcon, RotateCwIcon, XIcon } from 'lucide-react';

import { MessageAnimated } from '@/components/message-animated';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from '@/components/ui/input-group';
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker';
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from '@/components/ui/message-scroller';
import { Spinner } from '@/components/ui/spinner';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}

const INITIAL_MESSAGES: Message[] = [
  {
    id: 'welcome',
    role: 'assistant',
    content:
      "Hey! I'm here to understand your coding background and learning goals so I can personalize your experience. What are you working towards right now?",
  },
];

const MOCK_REPLY =
  "Thanks for sharing! I'll keep that in mind as we build your learning path. I've updated your profile with this context. What else should I know about your experience?";

interface ChatPanelProps {
  onClose?: () => void;
  draggable?: boolean;
}

/**
 * Reusable chat UI - message list + input bar.
 * Used inside the floating chat window (not a full page).
 */
export function ChatPanel({ onClose, draggable = false }: ChatPanelProps) {
  const [messages, setMessages] = useState<Message[]>(INITIAL_MESSAGES);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [streamingMessageId, setStreamingMessageId] = useState<string | null>(null);
  const streamTimerRef = useRef<number | null>(null);

  const clearStreamTimer = useCallback(() => {
    if (streamTimerRef.current !== null) {
      window.clearInterval(streamTimerRef.current);
      streamTimerRef.current = null;
    }
  }, []);

  useEffect(() => () => clearStreamTimer(), [clearStreamTimer]);

  const streamAssistantReply = useCallback(
    (messageId: string) => {
      clearStreamTimer();
      setStreamingMessageId(messageId);

      const tokens = MOCK_REPLY.split(' ');
      let index = 0;

      streamTimerRef.current = window.setInterval(() => {
        index += 1;
        const nextContent = tokens.slice(0, index).join(' ');

        setMessages((prev) =>
          prev.map((message) =>
            message.id === messageId ? { ...message, content: nextContent } : message,
          ),
        );

        if (index >= tokens.length) {
          clearStreamTimer();
          setStreamingMessageId(null);
          setSending(false);
        }
      }, 70);
    },
    [clearStreamTimer],
  );

  const handleReset = () => {
    clearStreamTimer();
    setMessages(INITIAL_MESSAGES);
    setInput('');
    setSending(false);
    setStreamingMessageId(null);
  };

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const text = input.trim();
    if (!text || sending) return;

    const userMsg: Message = { id: `u-${Date.now()}`, role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setSending(true);

    // TODO: Replace with POST /api/v1/chat
    window.setTimeout(() => {
      const assistantId = `a-${Date.now()}`;
      setMessages((prev) => [
        ...prev,
        { id: assistantId, role: 'assistant', content: '' },
      ]);
      streamAssistantReply(assistantId);
    }, 500);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      e.currentTarget.form?.requestSubmit();
    }
  };

  const isStreaming = streamingMessageId !== null;
  const isBusy = sending || isStreaming;

  return (
    <Card className="flex h-full min-h-0 flex-col gap-0 rounded-none border-0 py-0 shadow-none [--card-spacing:1.5rem]">
      <CardHeader
        className={cn(
          'gap-1 border-b px-4 py-3',
          draggable && 'chat-drag-handle cursor-grab active:cursor-grabbing',
        )}
      >
        <CardTitle className="text-sm">Assistant</CardTitle>
        <CardDescription>Goals, skills, and memory</CardDescription>
        <CardAction className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="chat-drag-cancel size-7"
                aria-label="Reset conversation"
                onClick={handleReset}
                disabled={isBusy}
              >
                <RotateCwIcon className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <p>Reset</p>
            </TooltipContent>
          </Tooltip>
          {onClose ? (
            <Button
              variant="ghost"
              size="icon"
              className="chat-drag-cancel size-7"
              aria-label="Close chat"
              onClick={onClose}
            >
              <XIcon className="size-3.5" />
            </Button>
          ) : null}
        </CardAction>
      </CardHeader>

      <CardContent className="flex min-h-0 flex-1 overflow-hidden p-0">
        <MessageScrollerProvider
          autoScroll
          defaultScrollPosition="last-anchor"
          scrollPreviousItemPeek={64}
        >
          <MessageScroller className="min-h-0 flex-1">
            <MessageScrollerViewport>
              <MessageScrollerContent
                aria-busy={isBusy}
                className="p-(--card-spacing)"
              >
                {messages.map((message) => (
                  <MessageAnimated
                    key={message.id}
                    message={{
                      id: message.id,
                      role: message.role,
                      text: message.content,
                    }}
                    scrollAnchor={message.role === 'user'}
                  />
                ))}

                {sending && !isStreaming ? (
                  <MessageScrollerItem messageId="thinking-indicator" scrollAnchor={false}>
                    <Marker role="status">
                      <MarkerIcon>
                        <Spinner />
                      </MarkerIcon>
                      <MarkerContent>Thinking...</MarkerContent>
                    </Marker>
                  </MessageScrollerItem>
                ) : null}
              </MessageScrollerContent>
            </MessageScrollerViewport>
            <MessageScrollerButton />
          </MessageScroller>
        </MessageScrollerProvider>
      </CardContent>

      <CardFooter className="flex-col gap-0 border-0 px-4 pb-3 pt-2">
        <form onSubmit={handleSubmit} className="w-full">
          <InputGroup>
            <InputGroupTextarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Tell me about your goals..."
              disabled={isBusy}
              rows={2}
              className="field-sizing-content min-h-14 h-14 max-h-14 w-full overflow-y-auto px-3 py-2.5 text-sm opacity-100 disabled:opacity-60"
            />
            <InputGroupAddon align="block-end" className="pt-1">
              <InputGroupButton
                type="submit"
                variant="default"
                size="icon-sm"
                disabled={!input.trim() || isBusy}
                className="ml-auto"
              >
                <ArrowUpIcon />
                <span className="sr-only">Send</span>
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </CardFooter>
    </Card>
  );
}