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
});
