import { useState, useRef, useEffect } from 'react';

// ── Types ────────────────────────────────────────────────────────────────────

/** A single chat message between user and assistant. */
interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}

// ── Component ────────────────────────────────────────────────────────────────

/**
 * ChatPage — conversational interface with Claude for learning goals,
 * knowledge assessment, and user memory management.
 *
 * The agent's primary mission:
 * 1. Learn the user's goals (career, interview prep, learning for fun)
 * 2. Assess existing knowledge (languages, frameworks, DSA comfort)
 * 3. Generate/adjust the persistent user memory file
 *
 * Currently uses mock responses — will wire to POST /api/v1/chat.
 */
export function ChatPage() {
  // All messages in the current conversation.
  const [messages, setMessages] = useState<Message[]>([
    {
      id: 'welcome',
      role: 'assistant',
      content:
        "Hey! I'm here to understand your coding background and learning goals so I can personalize your experience. What are you working towards right now?",
    },
  ]);

  // The text currently being typed in the input bar.
  const [input, setInput] = useState('');

  // Whether we're waiting for an assistant response.
  const [sending, setSending] = useState(false);

  // Ref to the bottom of the message list for auto-scrolling.
  const bottomRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to the latest message whenever messages change.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  /** Send the current input as a user message, then get a mock response. */
  const handleSend = async () => {
    const text = input.trim();
    if (!text || sending) return;

    // Append user message.
    const userMsg: Message = { id: `u-${Date.now()}`, role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setSending(true);

    // TODO: Replace with POST /api/v1/chat — Claude reads user memory,
    // responds, and optionally returns a memory_update payload.
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

  /** Send on Enter (without Shift), newline on Shift+Enter. */
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="flex flex-col h-[calc(100vh-1px)]">
      {/* ── Message list ─────────────────────────────────────────── */}
      <div className="flex-1 overflow-y-auto px-6 py-8">
        <div className="max-w-2xl mx-auto flex flex-col gap-4">
          {messages.map((msg) => (
            <div
              key={msg.id}
              className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
            >
              <div
                className={`max-w-[80%] px-4 py-3 text-sm leading-relaxed rounded-2xl ${
                  msg.role === 'user'
                    ? 'bg-parchment text-ink'
                    : 'bg-white border border-chalk text-graphite'
                }`}
                style={
                  msg.role === 'assistant'
                    ? { boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }
                    : undefined
                }
              >
                {msg.content}
              </div>
            </div>
          ))}

          {/* Typing indicator when waiting for response */}
          {sending && (
            <div className="flex justify-start">
              <div className="px-4 py-3 bg-white border border-chalk rounded-2xl">
                <div className="flex gap-1">
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:0ms]" />
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:150ms]" />
                  <span className="w-1.5 h-1.5 bg-ash rounded-full animate-bounce [animation-delay:300ms]" />
                </div>
              </div>
            </div>
          )}

          {/* Scroll anchor */}
          <div ref={bottomRef} />
        </div>
      </div>

      {/* ── Input bar ────────────────────────────────────────────── */}
      <div className="border-t border-chalk bg-bone px-6 py-4">
        <div className="max-w-2xl mx-auto">
          <div
            className="flex items-end rounded-2xl border border-chalk bg-white px-4 py-3"
            style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 4px 12px rgba(0,0,0,0.03)' }}
          >
            {/* Multi-line textarea that auto-sizes */}
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Tell me about your goals..."
              rows={1}
              className="flex-1 bg-transparent text-sm text-ink placeholder-ash focus:outline-none resize-none max-h-32"
            />

            {/* Send button — arrow icon matching GeneratePage style */}
            <button
              onClick={handleSend}
              disabled={sending || !input.trim()}
              className="w-9 h-9 bg-ink text-bone rounded-xl flex items-center justify-center shrink-0 ml-3 hover:bg-ink-soft disabled:bg-chalk disabled:text-ash disabled:cursor-not-allowed transition-colors"
              aria-label="Send"
            >
              <svg
                width="14"
                height="14"
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

          {/* Memory status indicator */}
          <div className="mt-2 flex items-center gap-1.5 px-1">
            <div className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
            <span className="text-[10px] text-ash">Memory synced</span>
          </div>
        </div>
      </div>
    </div>
  );
}
