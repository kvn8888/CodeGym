import { useState, useRef, useEffect } from 'react';

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}

/**
 * Reusable chat UI - message list + input bar.
 * Used inside the floating chat window (not a full page).
 */
export function ChatPanel() {
  const [messages, setMessages] = useState<Message[]>([
    {
      id: 'welcome',
      role: 'assistant',
      content:
        "Hey! I'm here to understand your coding background and learning goals so I can personalize your experience. What are you working towards right now?",
    },
  ]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, sending]);

  const handleSend = async () => {
    const text = input.trim();
    if (!text || sending) return;

    const userMsg: Message = { id: `u-${Date.now()}`, role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setSending(true);

    // TODO: Replace with POST /api/v1/chat
    setTimeout(() => {
      const botMsg: Message = {
        id: `a-${Date.now()}`,
        role: 'assistant',
        content:
          "Thanks for sharing! I'll keep that in mind as we build your learning path. I've updated your profile with this context. What else should I know about your experience?",
      };
      setMessages((prev) => [...prev, botMsg]);
      setSending(false);
    }, 1200);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex-1 min-h-0 overflow-y-auto px-4 py-4">
        <div className="flex flex-col gap-3">
          {messages.map((msg) => (
            <div
              key={msg.id}
              className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
            >
              <div
                className={`max-w-[85%] rounded-xl px-3.5 py-2.5 text-sm leading-relaxed ${
                  msg.role === 'user'
                    ? 'bg-gray-100 text-gray-1000'
                    : 'border border-gray-alpha-200 bg-background-100 text-gray-900'
                }`}
                style={
                  msg.role === 'assistant'
                    ? { boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }
                    : undefined
                }
              >
                {msg.content}
              </div>
            </div>
          ))}

          {sending && (
            <div className="flex justify-start">
              <div
                className="rounded-xl border border-gray-alpha-200 bg-background-100 px-3.5 py-2.5"
                style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
              >
                <div className="flex gap-1">
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-gray-700 [animation-delay:0ms]" />
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-gray-700 [animation-delay:150ms]" />
                  <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-gray-700 [animation-delay:300ms]" />
                </div>
              </div>
            </div>
          )}

          <div ref={bottomRef} />
        </div>
      </div>

      <div className="shrink-0 border-t border-gray-alpha-200 bg-background-100 px-4 py-3">
        <div
          className="flex items-end rounded-xl border border-gray-alpha-200 bg-background-100 px-3 py-2.5"
          style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
        >
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Tell me about your goals..."
            rows={1}
            className="max-h-24 flex-1 resize-none bg-transparent text-sm text-gray-1000 placeholder-gray-700 focus-visible:outline-none"
          />
          <button
            onClick={handleSend}
            disabled={sending || !input.trim()}
            className="cg-focus ml-2 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-1000 text-background-100 transition-colors hover:bg-gray-900 disabled:cursor-not-allowed disabled:bg-gray-100 disabled:text-gray-700"
            aria-label="Send"
          >
            <svg
              width="13"
              height="13"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M3 8h10M9 4l4 4-4 4" />
            </svg>
          </button>
        </div>
        <div className="mt-2 flex items-center gap-1.5 px-0.5">
          <div className="h-1.5 w-1.5 rounded-full bg-green-700" />
          <span className="text-xs text-gray-700">Memory synced</span>
        </div>
      </div>
    </div>
  );
}
