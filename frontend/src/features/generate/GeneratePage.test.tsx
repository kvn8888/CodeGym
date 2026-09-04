import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, vi } from 'vitest';

import { renderWithProviders } from '../../test/test-utils';
import { GeneratePage } from './GeneratePage';

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('GeneratePage', () => {
  it('sends the selected Go language when generating a coding problem', async () => {
    const requests: Array<{ url: string; method: string; body: unknown }> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? 'GET';
        const body = init?.body ? JSON.parse(String(init.body)) : null;
        requests.push({ url, method, body });

        if (url.endsWith('/memory/profile')) {
          return apiResponse({ growth_edges: [], notes: [] });
        }
        if (url.includes('/sessions?') || url.includes('/practice-intakes?')) {
          return apiResponse([]);
        }
        if (url.endsWith('/workflow-operations') && method === 'POST') {
          return apiResponse(
            {
              operation: {
                id: 'problem-workflow-1',
                workspace_id: 'workspace-1',
                user_id: 'user-1',
                kind: 'problem_generation',
                status: 'queued',
                last_sequence: 1,
                created_at: '2026-09-03T14:00:00Z',
                updated_at: '2026-09-03T14:00:00Z',
              },
              events: [
                {
                  operation_id: 'problem-workflow-1',
                  sequence: 1,
                  step_id: 'load_context',
                  label: 'Load personalization',
                  status: 'queued',
                  timestamp: '2026-09-03T14:00:00Z',
                },
              ],
            },
            201,
          );
        }
        if (url.endsWith('/workflow-operations/problem-workflow-1/events')) {
          return new Response(null, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          });
        }
        if (url.endsWith('/generate')) {
          return apiResponse({
            kind: 'problem',
            problem_id: 'generated-go-problem',
            problem: { title: 'Generated Go problem' },
            provider: 'test',
            model: 'test',
          });
        }
        if (url.endsWith('/sessions') && method === 'POST') {
          return apiResponse({ id: 'session-1' });
        }
        throw new Error(`unexpected request: ${method} ${url}`);
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<GeneratePage initialFormat="coding" />, {
      initialEntries: ['/generate'],
    });

    await user.click(screen.getByRole('radio', { name: 'Go' }));
    await user.click(screen.getByRole('button', { name: /start coding/i }));

    await waitFor(() => {
      const request = requests.find(
        (candidate) => candidate.method === 'POST' && candidate.url.endsWith('/generate'),
      );
      expect(request?.body).toMatchObject({
        kind: 'problem',
        operation_id: 'problem-workflow-1',
        spec: { language: 'go' },
      });
    });
  });

  it('streams coding progress frames into the activity as generation advances', async () => {
    const requests: Array<{ url: string; method: string; body: unknown }> = [];
    const encoder = new TextEncoder();
    let sseController: ReadableStreamDefaultController<Uint8Array> | null = null;
    let releaseGenerate: (() => void) | null = null;
    const generateGate = new Promise<void>((resolve) => {
      releaseGenerate = resolve;
    });
    let sequence = 5;
    const frame = (stepId: string, label: string, status: string, terminal = false) => {
      sequence += 1;
      return encoder.encode(
        `event: progress\ndata: ${JSON.stringify({
          operation_id: 'problem-workflow-1',
          sequence,
          step_id: stepId,
          label,
          status,
          timestamp: '2026-09-04T12:00:00Z',
          ...(terminal ? { metadata: { terminal: true } } : {}),
        })}\n\n`,
      );
    };
    const seed = (stepId: string, label: string, order: number) => ({
      operation_id: 'problem-workflow-1',
      sequence: order,
      step_id: stepId,
      label,
      status: 'queued',
      timestamp: '2026-09-04T12:00:00Z',
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? 'GET';
        const body = init?.body ? JSON.parse(String(init.body)) : null;
        requests.push({ url, method, body });

        if (url.endsWith('/memory/profile')) {
          return apiResponse({ growth_edges: [], notes: [] });
        }
        if (url.includes('/sessions?') || url.includes('/practice-intakes?')) {
          return apiResponse([]);
        }
        if (url.endsWith('/workflow-operations') && method === 'POST') {
          return apiResponse(
            {
              operation: {
                id: 'problem-workflow-1',
                workspace_id: 'workspace-1',
                user_id: 'user-1',
                kind: 'problem_generation',
                status: 'queued',
                last_sequence: 5,
                created_at: '2026-09-04T12:00:00Z',
                updated_at: '2026-09-04T12:00:00Z',
              },
              events: [
                seed('load_context', 'Load personalization', 1),
                seed('generate_problem', 'Generate coding problem', 2),
                seed('verify_solution', 'Verify reference solution', 3),
                seed('save_problem', 'Save coding problem', 4),
                seed('problem_ready', 'Coding problem ready', 5),
              ],
            },
            201,
          );
        }
        if (url.endsWith('/workflow-operations/problem-workflow-1/events')) {
          const stream = new ReadableStream<Uint8Array>({
            start(controller) {
              sseController = controller;
            },
          });
          return new Response(stream, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          });
        }
        if (url.endsWith('/generate')) {
          await generateGate;
          return apiResponse({
            kind: 'problem',
            problem_id: 'generated-problem',
            problem: { title: 'Generated problem' },
            provider: 'test',
            model: 'test',
          });
        }
        if (url.endsWith('/sessions') && method === 'POST') {
          return apiResponse({ id: 'session-1' });
        }
        throw new Error(`unexpected request: ${method} ${url}`);
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<GeneratePage initialFormat="coding" />, {
      initialEntries: ['/generate'],
    });

    await user.click(screen.getByRole('button', { name: /start coding/i }));
    await waitFor(() => expect(sseController).not.toBeNull());

    sseController!.enqueue(frame('load_context', 'Load personalization', 'succeeded'));
    sseController!.enqueue(frame('generate_problem', 'Generate coding problem', 'running'));
    await screen.findByText('Generate coding problem');

    sseController!.enqueue(frame('generate_problem', 'Generate coding problem', 'succeeded'));
    sseController!.enqueue(frame('verify_solution', 'Verify reference solution', 'running'));
    await screen.findByText('Verify reference solution');

    releaseGenerate!();
    sseController!.enqueue(frame('verify_solution', 'Verify reference solution', 'succeeded'));
    sseController!.enqueue(frame('save_problem', 'Save coding problem', 'succeeded'));
    sseController!.enqueue(frame('problem_ready', 'Coding problem ready', 'succeeded', true));
    sseController!.close();

    await waitFor(() => {
      const request = requests.find(
        (candidate) => candidate.method === 'POST' && candidate.url.endsWith('/sessions'),
      );
      expect(request?.body).toMatchObject({ problem_id: 'generated-problem' });
    });
  });
});
