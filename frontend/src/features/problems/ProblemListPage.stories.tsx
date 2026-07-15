import type { Meta, StoryObj } from '@storybook/react-vite';
import { ProblemListPage } from './ProblemListPage';
import type { PracticeSessionSummary, ProblemSummary } from '../../shared/api/types';

const meta: Meta<typeof ProblemListPage> = {
  title: 'Pages/ProblemList',
  component: ProblemListPage,
  parameters: {
    initialPath: '/problems',
    routePath: '/problems',
  },
};

export default meta;
type Story = StoryObj<typeof ProblemListPage>;

const sampleProblems: ProblemSummary[] = [
  {
    id: 'two-sum',
    title: 'Two Sum',
    category: 'dsa',
    language: 'go',
    difficulty: 1,
    tags: ['arrays', 'hash-map'],
    estimated_minutes: 15,
    type: 'function',
  },
  {
    id: 'express-pagination',
    title: 'Implement Cursor-Based Pagination in Express',
    category: 'api-patterns',
    language: 'javascript',
    framework: 'express',
    difficulty: 3,
    tags: ['rest', 'pagination', 'cursor'],
    estimated_minutes: 25,
    type: 'api-server',
  },
  {
    id: 'go-fan-out',
    title: 'Fan-Out / Fan-In with Goroutines',
    category: 'concurrency',
    language: 'go',
    difficulty: 4,
    tags: ['goroutines', 'channels', 'concurrency'],
    estimated_minutes: 35,
    type: 'system',
  },
  {
    id: 'pytorch-mnist',
    title: 'Train a Simple MNIST Classifier',
    category: 'ml',
    language: 'python',
    framework: 'pytorch',
    difficulty: 3,
    tags: ['pytorch', 'neural-network', 'mnist'],
    estimated_minutes: 40,
    type: 'library-usage',
  },
  {
    id: 'linked-list-cpp',
    title: 'Implement a Singly Linked List in C++',
    category: 'dsa',
    language: 'cpp',
    difficulty: 2,
    tags: ['linked-list', 'pointers', 'data-structures'],
    estimated_minutes: 20,
    type: 'function',
  },
  {
    id: 'spring-rest',
    title: 'Build a REST API with Spring Boot',
    category: 'api-patterns',
    language: 'java',
    framework: 'spring-boot',
    difficulty: 3,
    tags: ['rest', 'spring', 'jpa'],
    estimated_minutes: 45,
    type: 'api-server',
  },
];

const workspaceId = 'personal-storybook-user';
const userId = 'auth0|storybook-user';

const sampleSessions: PracticeSessionSummary[] = [
  {
    id: 'sess_go_concurrency',
    workspace_id: workspaceId,
    user_id: userId,
    kind: 'mcq',
    status: 'active',
    title: 'Go concurrency and channel ownership',
    created_at: '2026-07-15T12:18:00.000Z',
    updated_at: '2026-07-15T13:42:00.000Z',
    last_activity_at: '2026-07-15T13:42:00.000Z',
  },
  {
    id: 'sess_distributed_systems',
    workspace_id: workspaceId,
    user_id: userId,
    kind: 'mcq',
    status: 'active',
    title: 'Distributed systems failure modes',
    created_at: '2026-07-13T18:05:00.000Z',
    updated_at: '2026-07-13T18:19:00.000Z',
    last_activity_at: '2026-07-13T18:19:00.000Z',
  },
  {
    id: 'sess_rate_limiting',
    workspace_id: workspaceId,
    user_id: userId,
    kind: 'mcq',
    status: 'completed',
    title: 'Sliding-window rate limiting',
    created_at: '2026-07-14T20:03:00.000Z',
    updated_at: '2026-07-14T20:21:00.000Z',
    last_activity_at: '2026-07-14T20:21:00.000Z',
    completed_at: '2026-07-14T20:21:00.000Z',
  },
  {
    id: 'sess_sql_planning',
    workspace_id: workspaceId,
    user_id: userId,
    kind: 'mcq',
    status: 'abandoned',
    title: 'SQL joins and query planning',
    created_at: '2026-07-12T16:42:00.000Z',
    updated_at: '2026-07-12T16:51:00.000Z',
    last_activity_at: '2026-07-12T16:51:00.000Z',
  },
];

interface FetchScenario {
  problems?: ProblemSummary[];
  sessions?: PracticeSessionSummary[];
  loading?: boolean;
  sessionsError?: boolean;
}

function apiResponse(data: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify({ data, error: null }), {
    status: init?.status ?? 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

function makeFetchMock({
  problems = sampleProblems,
  sessions = sampleSessions,
  loading = false,
  sessionsError = false,
}: FetchScenario = {}) {
  return (input: RequestInfo | URL) => {
    if (loading) return new Promise<Response>(() => {});

    const url = new URL(input.toString(), window.location.origin);
    if (url.pathname.endsWith('/sessions') && sessionsError) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: null,
            error: { code: 'mock_unavailable', message: 'Practice data could not be loaded.' },
          }),
          { status: 503, headers: { 'Content-Type': 'application/json' } },
        ),
      );
    }

    if (url.pathname.endsWith('/sessions')) return Promise.resolve(apiResponse(sessions));
    if (url.pathname.endsWith('/problems')) {
      return Promise.resolve(apiResponse({ problems, total: problems.length }));
    }
    return Promise.resolve(apiResponse(null));
  };
}

function withFetchMock(scenario?: FetchScenario) {
  return (Story: () => React.JSX.Element) => {
    globalThis.fetch = makeFetchMock(scenario) as typeof fetch;
    return <Story />;
  };
}

export const Populated: Story = {
  decorators: [withFetchMock()],
};

export const Empty: Story = {
  parameters: { mockApiScenario: 'empty' },
  decorators: [withFetchMock({ sessions: [] })],
};

export const Loading: Story = {
  parameters: { mockApiScenario: 'loading' },
  decorators: [withFetchMock({ loading: true })],
};

export const ErrorState: Story = {
  name: 'Error',
  parameters: { mockApiScenario: 'error' },
  decorators: [withFetchMock({ sessionsError: true })],
};
