import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { afterEach, vi } from 'vitest';

import type {
  PracticeSession,
  Problem,
  Submission,
  SubmissionMode,
} from '../../shared/api/types';
import { renderWithProviders } from '../../test/test-utils';
import { ProblemDetailPage } from './ProblemDetailPage';

vi.mock('@monaco-editor/react', () => ({
  default: () => <div data-testid="code-editor" />,
}));

const problem: Problem = {
  id: 'unit-problem',
  title: 'Unit problem',
  category: 'algorithms',
  language: 'python',
  difficulty: 2,
  tags: ['arrays'],
  estimated_minutes: 20,
  type: 'coding',
  version: '1',
  description: 'Return the transformed value.',
  runtime: {
    image: 'python:3.13',
    timeout_seconds: 20,
    memory_mb: 256,
    network_mode: 'block-all',
  },
  files: { skeleton: [{ path: 'solution.py', entry: true }] },
  test_config: { strategy: 'unit' },
};

const session: PracticeSession = {
  id: 'session-1',
  workspace_id: 'workspace-1',
  user_id: 'user-1',
  kind: 'workspace',
  status: 'active',
  title: problem.title,
  problem_id: problem.id,
  state: { schema_version: 1, problem_opened_recorded: true },
  files: [],
  created_at: '2026-08-04T12:00:00.000Z',
  updated_at: '2026-08-04T12:00:00.000Z',
  last_activity_at: '2026-08-04T12:00:00.000Z',
};

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function setupProblemApi(problemFixture: Problem = problem) {
  const submissionBodies: Array<{ mode?: SubmissionMode }> = [];
  let submittedMode: SubmissionMode = 'submit';

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost');
      const path = url.pathname.replace('/api/v1', '') || '/';
      const method = init?.method ?? 'GET';

      if (method === 'GET' && path === `/problems/${problem.id}`) {
        return apiResponse(problemFixture);
      }
      if (method === 'GET' && path === `/problems/${problem.id}/skeleton`) {
        return apiResponse({ files: [{ path: 'solution.py', content: 'def solve():\n    pass\n' }] });
      }
      if (method === 'GET' && path === '/sessions') {
        return apiResponse([session]);
      }
      if (method === 'GET' && path === `/sessions/${session.id}`) {
        return apiResponse(session);
      }
      if (method === 'PUT' && path === `/sessions/${session.id}/files`) {
        return apiResponse(session);
      }
      if (method === 'PATCH' && path === `/sessions/${session.id}`) {
        return apiResponse(session);
      }
      if (method === 'POST' && path === '/submissions') {
        const body = JSON.parse(String(init?.body)) as { mode?: SubmissionMode };
        submissionBodies.push(body);
        submittedMode = body.mode ?? 'submit';
        return apiResponse({ submission_id: 'submission-1' }, 202);
      }
      if (method === 'GET' && path === '/submissions/submission-1') {
        const total = submittedMode === 'run' ? 3 : 12;
        const submission: Submission = {
          status: 'completed',
          mode: submittedMode,
          executed_count: total,
          result: {
            status: 'pass',
            total,
            passed: total,
            failed: 0,
            duration_ms: 12,
            test_cases: [],
          },
          stdout: '',
          output_truncated: false,
        };
        return apiResponse(submission);
      }

      throw new Error(`unexpected request: ${method} ${path}`);
    }),
  );

  return submissionBodies;
}

function renderProblemPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/problems/:id" element={<ProblemDetailPage />} />
    </Routes>,
    { initialEntries: [`/problems/${problem.id}?session=${session.id}`] },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ProblemDetailPage submissions', () => {
  it('sends run mode and labels a passing result as a sample run', async () => {
    const submissionBodies = setupProblemApi();
    const user = userEvent.setup();
    renderProblemPage();

    await user.click(await screen.findByRole('button', { name: /^run$/i }));

    await waitFor(() => expect(submissionBodies).toContainEqual(expect.objectContaining({ mode: 'run' })));
    expect(await screen.findByText(/sample run · 3 sample cases passed/i)).toBeInTheDocument();
    expect(screen.queryByText(/graded submit · 3 of 3 passed/i)).not.toBeInTheDocument();
  });

  it('sends submit mode and labels a passing result as graded', async () => {
    const submissionBodies = setupProblemApi();
    const user = userEvent.setup();
    renderProblemPage();

    await user.click(await screen.findByRole('button', { name: /^submit$/i }));

    await waitFor(() =>
      expect(submissionBodies).toContainEqual(expect.objectContaining({ mode: 'submit' })),
    );
    expect(await screen.findByText(/graded submit · 12 of 12 passed/i)).toBeInTheDocument();
    expect(screen.queryByText(/sample run/i)).not.toBeInTheDocument();
  });
});

describe('ProblemDetailPage worked examples', () => {
  it('renders unit arguments, expected output, and explanation', async () => {
    setupProblemApi({
      ...problem,
      public_cases: [
        {
          strategy: 'unit',
          name: 'finds a pair near the start',
          kind: 'example',
          args: [[2, 7, 11, 15], 9],
          expected: [0, 1],
          explanation: 'The values at indices 0 and 1 add up to 9.',
        },
      ],
    });
    renderProblemPage();

    const examples = await screen.findByRole('region', { name: 'Worked examples' });
    expect(within(examples).getByText('Arguments')).toBeInTheDocument();
    expect(within(examples).getByText('Expected output')).toBeInTheDocument();
    expect(within(examples).getByText(/2,\s+7,\s+11,\s+15/)).toBeInTheDocument();
    expect(within(examples).getByText(/values at indices 0 and 1 add up to 9/i)).toBeInTheDocument();
  });

  it('renders an HTTP request and expected response in their native shape', async () => {
    setupProblemApi({
      ...problem,
      test_config: { strategy: 'http' },
      public_cases: [
        {
          strategy: 'http',
          name: 'creates a user',
          kind: 'example',
          request: {
            method: 'post',
            path: '/users',
            headers: { 'Content-Type': 'application/json' },
            body: { name: 'Ada' },
          },
          expected: {
            status: 201,
            json: { created: true },
          },
          explanation: 'A valid payload creates the user.',
        },
      ],
    });
    renderProblemPage();

    const examples = await screen.findByRole('region', { name: 'Worked examples' });
    expect(within(examples).getByText('Request')).toBeInTheDocument();
    expect(within(examples).getByText('POST')).toBeInTheDocument();
    expect(within(examples).getByText(/\/users/)).toBeInTheDocument();
    expect(within(examples).getByText('Request body')).toBeInTheDocument();
    expect(within(examples).getByText(/"name": "Ada"/)).toBeInTheDocument();
    expect(within(examples).getByText('Expected response')).toBeInTheDocument();
    expect(within(examples).getByText('HTTP 201')).toBeInTheDocument();
    expect(within(examples).getByText(/"created": true/)).toBeInTheDocument();
  });

  it('renders no examples section when public cases are absent', async () => {
    setupProblemApi();
    renderProblemPage();

    expect(await screen.findByRole('heading', { name: problem.title })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Worked examples' })).not.toBeInTheDocument();
  });
});
