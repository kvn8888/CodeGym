import { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowUpIcon, RotateCwIcon, SquareIcon, XIcon } from 'lucide-react';

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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

import { api, streamChatTurn } from '../../shared/api/client';
import type {
  ChatKind,
  ChatMessage,
  ChatThreadWithMessages,
  InterviewMode,
} from '../../shared/api/types';

interface ChatPanelProps {
  sessionId?: string;
  kind?: ChatKind;
  mode?: InterviewMode;
  questionId?: string;
  onClose?: () => void;
  draggable?: boolean;
  initialThread?: ChatThreadWithMessages;
  onThreadChange?: (thread: ChatThreadWithMessages) => void;
  readOnly?: boolean;
}

const COACH_WELCOME: ChatMessage = {
  id: 'coach-welcome',
  thread_id: '',
  role: 'assistant',
  content:
    'I can help you reason through this session.\n\nShare what feels stuck, a concept to review, or a snippet:\n\n```ts\nfunction twoSum(nums: number[], target: number) {\n  // ...\n}\n```\n\nWhat are you working on?',
  status: 'complete',
  created_at: '',
};

export function ChatPanel({
  sessionId,
  kind = 'coach',
  mode,
  questionId,
  onClose,
  draggable = false,
  initialThread,
  onThreadChange,
  readOnly = false,
}: ChatPanelProps) {
  const [thread, setThread] = useState<ChatThreadWithMessages | null>(initialThread ?? null);
  const [messages, setMessages] = useState<ChatMessage[]>(
    initialThread?.messages ?? (kind === 'coach' ? [COACH_WELCOME] : []),
  );
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(!initialThread);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastTurn, setLastTurn] = useState<{ id: string; message: string } | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const loadThread = useCallback(async () => {
    if (initialThread) return;
    if (!sessionId) {
      setLoading(false);
      setError('Open this assistant from an active practice session.');
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const next = await api.post<ChatThreadWithMessages>('/chat/threads', {
        kind,
        mode,
        session_id: sessionId,
        question_id: questionId,
      });
      setThread(next);
      onThreadChange?.(next);
      setMessages(next.messages.length > 0 ? next.messages : kind === 'coach' ? [COACH_WELCOME] : []);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Could not restore this conversation.');
    } finally {
      setLoading(false);
    }
  }, [initialThread, kind, mode, onThreadChange, questionId, sessionId]);

  useEffect(() => {
    void loadThread();
    return () => abortRef.current?.abort();
  }, [loadThread]);

  const sendTurn = useCallback(
    async (turn: { id: string; message: string }) => {
      if (!thread || sending) return;
      setSending(true);
      setError(null);
      setLastTurn(turn);
      setMessages((current) => {
        const withoutWelcome = current.filter((message) => message.id !== 'coach-welcome');
        if (withoutWelcome.some((message) => message.client_message_id === turn.id)) {
          return withoutWelcome.filter((message) => message.status !== 'interrupted');
        }
        return [
          ...withoutWelcome,
          {
            id: `user-${turn.id}`,
            thread_id: thread.id,
            role: 'user',
            content: turn.message,
            status: 'complete',
            client_message_id: turn.id,
            created_at: new Date().toISOString(),
          },
        ];
      });

      const controller = new AbortController();
      abortRef.current = controller;
      let assistantId = `assistant-${turn.id}`;
      let assistantContent = '';
      try {
        await streamChatTurn(
          thread.id,
          { client_message_id: turn.id, message: turn.message },
          (event) => {
            if (event.type === 'meta') {
              const data = event.data as { message_id?: string };
              assistantId = data.message_id ?? assistantId;
              setMessages((current) => [
                ...current.filter((message) => message.id !== assistantId),
                {
                  id: assistantId,
                  thread_id: thread.id,
                  role: 'assistant',
                  content: '',
                  status: 'complete',
                  created_at: new Date().toISOString(),
                },
              ]);
            }
            if (event.type === 'delta') {
              const data = event.data as { content?: string };
              assistantContent += data.content ?? '';
              setMessages((current) =>
                current.map((message) =>
                  message.id === assistantId ? { ...message, content: assistantContent } : message,
                ),
              );
            }
            if (event.type === 'complete') {
              const complete = event.data as ChatMessage;
              setMessages((current) =>
                current.map((message) => (message.id === assistantId ? complete : message)),
              );
            }
            if (event.type === 'error') {
              const data = event.data as { message?: string };
              throw new Error(data.message ?? 'The response was interrupted.');
            }
          },
          controller.signal,
        );
        setLastTurn(null);
      } catch (turnError) {
        controller.abort();
        setMessages((current) =>
          current.map((message) =>
            message.id === assistantId ? { ...message, status: 'interrupted' } : message,
          ),
        );
        setError(
          turnError instanceof DOMException && turnError.name === 'AbortError'
            ? 'Response stopped. You can retry the turn.'
            : turnError instanceof Error
              ? turnError.message
              : 'The response was interrupted.',
        );
      } finally {
        abortRef.current = null;
        setSending(false);
      }
    },
    [sending, thread],
  );

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const text = input.trim();
    if (!text || sending || !thread) return;
    setInput('');
    void sendTurn({ id: crypto.randomUUID(), message: text });
  };

  const handleReset = async () => {
    if (!thread || sending) return;
    setLoading(true);
    setError(null);
    try {
      const next = await api.post<ChatThreadWithMessages>(`/chat/threads/${thread.id}/reset`, {});
      setThread(next);
      onThreadChange?.(next);
      setMessages(next.messages.length > 0 ? next.messages : kind === 'coach' ? [COACH_WELCOME] : []);
      setInput('');
      setLastTurn(null);
    } catch (resetError) {
      setError(resetError instanceof Error ? resetError.message : 'Could not reset the conversation.');
    } finally {
      setLoading(false);
    }
  };

  const isBusy = loading || sending;
  const canSend = !readOnly && thread?.status === 'active';
  const visibleMessages = messages.filter(
    (message) => message.status !== 'interrupted' || message.content.trim(),
  );

  return (
    <Card className="flex h-full min-h-0 flex-col gap-0 rounded-none border-0 py-0 shadow-none [--card-spacing:1.5rem]">
      <CardHeader
        className={cn(
          'gap-1 border-b px-4 py-3',
          draggable && 'chat-drag-handle cursor-grab active:cursor-grabbing',
        )}
      >
        <CardTitle className="text-sm">{kind === 'interview' ? 'Interview' : 'Coach'}</CardTitle>
        <CardDescription>
          {kind === 'interview' ? 'Your transcript is saved with this session' : 'Context from this session only'}
        </CardDescription>
        <CardAction className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="chat-drag-cancel size-7"
                aria-label="Reset conversation"
                onClick={() => void handleReset()}
                disabled={isBusy || !canSend}
              >
                <RotateCwIcon className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent><p>Reset</p></TooltipContent>
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
        <MessageScrollerProvider autoScroll defaultScrollPosition="last-anchor" scrollPreviousItemPeek={64}>
          <MessageScroller className="min-h-0 flex-1">
            <MessageScrollerViewport>
              <MessageScrollerContent aria-busy={isBusy} className="p-(--card-spacing)">
                {visibleMessages.map((message) => (
                  <MessageAnimated
                    key={message.id}
                    message={{ id: message.id, role: message.role, text: message.content }}
                    scrollAnchor={message.role === 'user'}
                  />
                ))}
                {isBusy && !visibleMessages.some((message) => message.role === 'assistant' && !message.content) ? (
                  <MessageScrollerItem messageId="thinking-indicator" scrollAnchor={false}>
                    <Marker role="status">
                      <MarkerIcon><Spinner /></MarkerIcon>
                      <MarkerContent>{loading ? 'Restoring conversation...' : 'Thinking...'}</MarkerContent>
                    </Marker>
                  </MessageScrollerItem>
                ) : null}
                {error ? (
                  <MessageScrollerItem messageId="chat-error" scrollAnchor>
                    <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100">
                      <p>{error}</p>
                      {lastTurn && !sending ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="mt-2 h-7"
                          onClick={() => void sendTurn(lastTurn)}
                        >
                          Retry
                        </Button>
                      ) : null}
                    </div>
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
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              placeholder={kind === 'interview' ? 'Answer the interviewer...' : 'Ask for a nudge...'}
              disabled={isBusy || !canSend}
              rows={2}
              className="field-sizing-content min-h-14 h-14 max-h-14 w-full overflow-y-auto px-3 py-2.5 text-sm opacity-100 disabled:opacity-60"
            />
            <InputGroupAddon align="block-end" className="pt-1">
              {sending ? (
                <InputGroupButton
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  className="ml-auto"
                  onClick={() => abortRef.current?.abort()}
                >
                  <SquareIcon />
                  <span className="sr-only">Stop response</span>
                </InputGroupButton>
              ) : (
                <InputGroupButton
                  type="submit"
                  variant="default"
                  size="icon-sm"
                  disabled={!input.trim() || isBusy || !canSend}
                  className="ml-auto"
                >
                  <ArrowUpIcon />
                  <span className="sr-only">Send</span>
                </InputGroupButton>
              )}
            </InputGroupAddon>
          </InputGroup>
        </form>
      </CardFooter>
    </Card>
  );
}
