import { afterEach, describe, expect, it, vi } from 'vitest';

import { setApiAccessTokenProvider, streamWorkflowEvents } from './client';

afterEach(() => {
  vi.unstubAllGlobals();
  setApiAccessTokenProvider(null);
});

describe('streamWorkflowEvents', () => {
  it('authenticates, resumes from Last-Event-ID, and parses progress events', async () => {
    setApiAccessTokenProvider(async () => 'access-token');
    const payload = {
      operation_id: 'workflow-1',
      sequence: 8,
      step_id: 'generate_questions',
      label: 'Generate questions',
      status: 'running',
      timestamp: '2026-07-30T12:00:00Z',
    };
    const encoder = new TextEncoder();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(headers.get('Authorization')).toBe('Bearer access-token');
      expect(headers.get('Accept')).toBe('text/event-stream');
      expect(headers.get('Last-Event-ID')).toBe('7');
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(`id: 8\nevent: progress\ndata: ${JSON.stringify(payload)}\n\n`),
            );
            controller.close();
          },
        }),
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
      );
    });
    vi.stubGlobal('fetch', fetchMock);
    const events: unknown[] = [];
    await streamWorkflowEvents('workflow-1', 7, (event) => events.push(event));
    expect(events).toEqual([payload]);
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
