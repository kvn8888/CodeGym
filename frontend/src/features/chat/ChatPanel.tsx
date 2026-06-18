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
                className={`max-w-[85%] px-3.5 py-2.5 text-xs leading-relaxed rounded-2xl ${
                  msg.role === 'user'
                    ? 'bg-ink text-bone'
                    : 'bg-shell text-graphite'
                }`}
                style={
                  msg.role === 'assistant'
                    ? { boxShadow: 'var(--cg-card-shadow)' }
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
                className="px-3.5 py-2.5 bg-white border border-chalk rounded-2xl"
                style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
              >
                <div className="flex gap-1">
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:0ms]" />
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:150ms]" />
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:300ms]" />
                </div>
              </div>
            </div>
          )}

          <div ref={bottomRef} />
        </div>
      </div>

      <div className="shrink-0 border-t border-chalk bg-bone px-4 py-3">
        <div
          className="flex items-end rounded-2xl border border-chalk bg-white px-3 py-2.5"
          style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
        >
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Tell me about your goals..."
            rows={1}
            className="flex-1 bg-transparent text-xs text-ink placeholder-ash focus-visible:outline-none resize-none max-h-24"
          />
          <button
            onClick={handleSend}
            disabled={sending || !input.trim()}
            className="w-8 h-8 bg-ink text-bone rounded-xl flex items-center justify-center shrink-0 ml-2 hover:bg-ink-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 focus-visible:ring-offset-white disabled:bg-chalk disabled:text-ash disabled:cursor-not-allowed transition-colors"
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
          <div className="w-1.5 h-1.5 rounded-full bg-moss" />
          <span className="text-[10px] text-ash">Memory synced</span>
        </div>
      </div>
    </div>
  );
}
