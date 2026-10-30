import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '../../test/test-utils';
import { ProblemDetailPage } from './ProblemDetailPage';

vi.mock('@monaco-editor/react', () => ({
  default: () => <div data-testid="editor" />,
}));

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function apiError(code: string, message: string, status: number) {
  return new Response(JSON.stringify({ data: null, error: { code, message } }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const problem = {
  id: 'two-sum',
  title: 'Two Sum',
  category: 'DSA',
  language: 'python',
  difficulty: 1,
  tags: ['arrays'],
  estimated_minutes: 10,
  type: 'coding',
  version: 'v1',
  description: 'Return the indices of two numbers that sum to the target.',
  subcategory: 'Arrays',
  runtime: { image: 'python:3.12', timeout_seconds: 5, memory_mb: 128, network_mode: 'off' },
  files: { skeleton: [{ path: 'solution.py' }] },
  test_config: { strategy: 'unit' },
  hints: [],
};

const session = {
  id: 'sess-1',
  kind: 'workspace',
  problem_id: 'two-sum',
  status: 'active',
  title: 'Two Sum',
  state: { schema_version: 1, hints_revealed: 0 },
  created_at: '2026-10-01T12:00:00Z',
  updated_at: '2026-10-01T12:00:00Z',
  files: [],
};

const completedSubmission = {
  status: 'completed',
  mode: 'submit',
  executed_count: 1,
  result: {
    status: 'pass',
    total: 2,
    passed: 2,
    failed: 0,
    duration_ms: 4,
    test_cases: [],
  },
  stdout: '',
  output_truncated: false,
};

interface RecordedRequest {
  path: string;
  method: string;
  body: Record<string, unknown>;
}

interface StubOptions {
  submitResponses?: Array<() => Response | Promise<Response>>;
  sessionState?: Record<string, unknown>;
}

function installFetch(options: StubOptions) {
  const requests: RecordedRequest[] = [];
  const remaining = [...(options.submitResponses ?? [])];
  const sessionFixture = options.sessionState
    ? { ...session, state: options.sessionState }
    : session;
  const fetchStub = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method ?? 'GET').toUpperCase();
    const body: Record<string, unknown> =
      typeof init?.body === 'string' ? (JSON.parse(init.body) as Record<string, unknown>) : {};
    const path = url.replace(/^.*\/api\/v1/, '');
    requests.push({ path, method, body });

    if (method === 'GET' && path === '/problems/two-sum') return apiResponse(problem);
    if (method === 'GET' && path === '/problems/two-sum/skeleton') {
      return apiResponse({ files: [{ path: 'solution.py', content: 'def two_sum(nums, target):\n    pass\n' }] });
    }
    if (method === 'GET' && path.startsWith('/sessions?')) return apiResponse([]);
    if (method === 'POST' && path === '/sessions') return apiResponse(sessionFixture, 201);
    if (method === 'GET' && path === '/sessions/sess-1') return apiResponse(sessionFixture);
    if (method === 'PUT' && path === '/sessions/sess-1/files') return apiResponse(sessionFixture);
    if (method === 'PATCH' && path === '/sessions/sess-1') return apiResponse(sessionFixture);
    if (method === 'POST' && path === '/memory/events') {
      return apiResponse({ id: `event-${requests.length}` }, 201);
    }
    if (method === 'POST' && path === '/submissions') {
      const next = remaining.shift();
      if (!next) throw new Error('unexpected submit');
      return next();
    }
    if (method === 'GET' && path === '/submissions/sub-1') return apiResponse(completedSubmission);
    if (method === 'POST' && path === '/memory/profile/maintain') {
      return apiResponse({ summary: 'updated', strengths: [], growth_edges: [], skills: [], notes: [] });
    }
    throw new Error(`unhandled request ${method} ${path}`);
  });
  vi.stubGlobal('fetch', fetchStub);
  return { requests, fetchStub };
}

async function renderPage() {
  renderWithProviders(
    <Routes>
      <Route path="/problems/:id" element={<ProblemDetailPage />} />
    </Routes>,
    { initialEntries: ['/problems/two-sum'] },
  );
  const submitButton = await screen.findByRole('button', { name: 'SUBMIT' });
  await waitFor(() => expect(submitButton).toBeEnabled());
  return submitButton;
}

function submittedTokens(requests: RecordedRequest[]) {
  return requests
    .filter((request) => request.method === 'POST' && request.path === '/submissions')
    .map((request) => request.body.client_token as string | undefined);
}

describe('ProblemDetailPage submit tokens', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
  });

  it('submits with mode "submit" and reuses the token after an uncertain failure', async () => {
    const { requests } = installFetch({
      submitResponses: [
        () => {
          throw new TypeError('fetch failed');
        },
        () => apiResponse({ submission_id: 'sub-1', memory_update_status: 'synced' }, 202),
      ],
    });
    const submitButton = await renderPage();
    const user = userEvent.setup();

    await user.click(submitButton);
    await waitFor(() => {
      expect(submittedTokens(requests)).toHaveLength(1);
    });
    await waitFor(() => expect(screen.getByRole('button', { name: 'SUBMIT' })).toBeEnabled());

    await user.click(screen.getByRole('button', { name: 'SUBMIT' }));
    await waitFor(() => {
      expect(submittedTokens(requests)).toHaveLength(2);
    });

    const tokens = submittedTokens(requests);
    expect(tokens[0]).toBeTruthy();
    expect(tokens[1]).toBe(tokens[0]);
    const submitBodies = requests.filter(
      (request) => request.method === 'POST' && request.path === '/submissions',
    );
    for (const request of submitBodies) {
      expect(request.body.mode).toBe('submit');
      expect(request.body.problem_id).toBe('two-sum');
      expect(request.body.session_id).toBe('sess-1');
    }
    // A definitive accepted response clears the stored intent.
    await waitFor(() => expect(sessionStorage.getItem('codegym:submit-token')).toBeNull());
  });

  it('uses a new token for a deliberate submit and clears it on a definitive rejection', async () => {
    const { requests } = installFetch({
      submitResponses: [
        () => apiResponse({ submission_id: 'sub-1', memory_update_status: 'synced' }, 202),
        () => apiError('invalid_submission', 'invalid submission', 400),
        () => apiResponse({ submission_id: 'sub-1', memory_update_status: 'synced' }, 202),
      ],
    });
    await renderPage();
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'SUBMIT' }));
    await waitFor(() => expect(submittedTokens(requests)).toHaveLength(1));
    await waitFor(() => expect(sessionStorage.getItem('codegym:submit-token')).toBeNull());

    // Deliberate new submit after a completed one starts from a fresh token.
    await waitFor(() => expect(screen.getByRole('button', { name: 'SUBMIT' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'SUBMIT' }));
    await waitFor(() => expect(submittedTokens(requests)).toHaveLength(2));
    await waitFor(() => expect(screen.getByRole('button', { name: 'SUBMIT' })).toBeEnabled());

    // Definitive rejection (400) also discards the stored token.
    await user.click(screen.getByRole('button', { name: 'SUBMIT' }));
    await waitFor(() => expect(submittedTokens(requests)).toHaveLength(3));
    const tokens = submittedTokens(requests);
    expect(tokens[1]).not.toBe(tokens[0]);
    expect(tokens[2]).not.toBe(tokens[1]);
    expect(sessionStorage.getItem('codegym:submit-token')).toBeNull();
  });

  it('recovers the last submission id after a reload and retries against it', async () => {
    const { requests } = installFetch({
      sessionState: {
        schema_version: 1,
        hints_revealed: 0,
        last_submission_id: 'sub-1',
        memory_update_status: 'failed',
        problem_opened_recorded: true,
      },
    });
    await renderPage();

    const retryButton = await screen.findByRole(
      'button',
      { name: 'RESULT SAVED · RETRY MEMORY' },
      { timeout: 3000 },
    );
    await userEvent.setup().click(retryButton);
    await waitFor(() => {
      const maintains = requests.filter(
        (request) => request.method === 'POST' && request.path === '/memory/profile/maintain',
      );
      expect(maintains).toHaveLength(1);
      expect(maintains[0].body.submission_id).toBe('sub-1');
    });
  });

  it('retries memory maintenance against the stored last submission id', async () => {
    const { requests } = installFetch({
      submitResponses: [
        () => apiResponse({ submission_id: 'sub-1', memory_update_status: 'failed' }, 202),
      ],
    });
    await renderPage();
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'SUBMIT' }));
    const retryButton = await screen.findByRole(
      'button',
      { name: 'RESULT SAVED · RETRY MEMORY' },
      { timeout: 3000 },
    );
    await user.click(retryButton);

    await waitFor(() => {
      const maintains = requests.filter(
        (request) => request.method === 'POST' && request.path === '/memory/profile/maintain',
      );
      expect(maintains).toHaveLength(1);
      expect(maintains[0].body).toEqual({
        session_id: 'sess-1',
        problem_id: 'two-sum',
        submission_id: 'sub-1',
      });
    });
  });
});
