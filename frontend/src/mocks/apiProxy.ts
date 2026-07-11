import { mockPassingResult, mockProblems, mockProblemSummaries, mockSkeletons } from './fixtures';
import { mockMcqQuestions } from './mcqFixtures';
import { mockMemoryProfile } from './memoryFixtures';
import { mockMemoryEvents, mockSessions } from './activityFixtures';
import type { PracticeSession, UserProfile } from '../shared/api/types';

interface MockApiResponse<T> {
  data: T;
  error: { code: string; message: string } | null;
}

const jsonHeaders = { 'Content-Type': 'application/json' };

let mockUserProfile: UserProfile = {
  user_id: 'auth0|mock-user',
  email: 'kevin@example.com',
  display_name: 'Kevin Chen',
  display_name_source: 'oauth',
  default_workspace_id: 'personal-auth0-mock-user',
};

let sessions: PracticeSession[] = structuredClone(mockSessions);

export type MockApiScenario = 'default' | 'empty' | 'error' | 'loading';

let mockApiScenario: MockApiScenario = 'default';

export function setMockApiScenario(scenario: MockApiScenario) {
  mockApiScenario = scenario;
  sessions = structuredClone(mockSessions);
}

function json<T>(data: T, init?: ResponseInit): Response {
  const body: MockApiResponse<T> = { data, error: null };
  return new Response(JSON.stringify(body), {
    status: init?.status ?? 200,
    headers: jsonHeaders,
  });
}

function error(code: string, message: string, status = 404): Response {
  const body: MockApiResponse<null> = { data: null, error: { code, message } };
  return new Response(JSON.stringify(body), {
    status,
    headers: jsonHeaders,
  });
}

function toUrl(input: RequestInfo | URL): URL {
  if (input instanceof Request) {
    return new URL(input.url, window.location.origin);
  }

  return new URL(input.toString(), window.location.origin);
}

export async function mockApiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response | null> {
  const url = toUrl(input);
  const method = (init?.method ?? 'GET').toUpperCase();

  if (!url.pathname.startsWith('/api/v1')) {
    return null;
  }

  const path = url.pathname.replace('/api/v1', '') || '/';

  if (mockApiScenario === 'loading' && method === 'GET') {
    await new Promise<never>(() => {});
  }

  if (
    mockApiScenario === 'error' &&
    method === 'GET' &&
    (path === '/memory/profile' || path === '/memory/events' || path === '/sessions')
  ) {
    return error('mock_unavailable', 'The learning workspace could not be loaded.', 503);
  }

  if (method === 'GET' && path === '/me') {
    return json(mockUserProfile);
  }

  if (method === 'PATCH' && path === '/me') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as { display_name?: string };
    const displayName = body.display_name?.trim();
    if (!displayName) {
      return error('invalid_profile', 'display name is required', 400);
    }

    mockUserProfile = {
      ...mockUserProfile,
      display_name: displayName,
      display_name_source: 'user',
    };
    return json(mockUserProfile);
  }

  if (method === 'GET' && path === '/memory/profile') {
    return json(
      mockApiScenario === 'empty'
        ? {
            ...mockMemoryProfile,
            summary: 'CodeGym is ready to learn from your first practice session.',
            strengths: [],
            growth_edges: [],
            skills: [],
            notes: [],
          }
        : mockMemoryProfile,
    );
  }

  if (method === 'GET' && path === '/memory/events') {
    return json(mockApiScenario === 'empty' ? [] : mockMemoryEvents);
  }

  if (method === 'GET' && path === '/sessions') {
    const status = url.searchParams.get('status');
    const kind = url.searchParams.get('kind');
    const limit = Number(url.searchParams.get('limit') ?? 20);
    const filtered = (mockApiScenario === 'empty' ? [] : sessions)
      .filter((session) => !status || session.status === status)
      .filter((session) => !kind || session.kind === kind)
      .sort((a, b) => b.last_activity_at.localeCompare(a.last_activity_at))
      .slice(0, limit)
      .map((session) => ({
        id: session.id,
        workspace_id: session.workspace_id,
        user_id: session.user_id,
        kind: session.kind,
        status: session.status,
        title: session.title,
        problem_id: session.problem_id,
        generation_job_id: session.generation_job_id,
        created_at: session.created_at,
        updated_at: session.updated_at,
        last_activity_at: session.last_activity_at,
        completed_at: session.completed_at,
      }));
    return json(filtered);
  }

  if (method === 'POST' && path === '/sessions') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as Partial<PracticeSession>;
    const now = new Date().toISOString();
    const session: PracticeSession = {
      id: `sess_${Date.now().toString(36)}`,
      workspace_id: mockUserProfile.default_workspace_id,
      user_id: mockUserProfile.user_id,
      kind: body.kind ?? 'mcq',
      status: 'active',
      title: body.title?.trim() || 'Untitled practice',
      created_at: now,
      updated_at: now,
      last_activity_at: now,
      state: body.state ?? {},
      files: [],
    };
    sessions = [session, ...sessions];
    return json(session, { status: 201 });
  }

  const sessionMatch = path.match(/^\/sessions\/([^/]+)$/);
  if (sessionMatch) {
    const sessionIndex = sessions.findIndex((candidate) => candidate.id === sessionMatch[1]);
    if (sessionIndex < 0) {
      return error('not_found', 'Practice session not found.', 404);
    }

    if (method === 'GET') {
      return json(sessions[sessionIndex]);
    }

    if (method === 'PATCH') {
      const rawBody = typeof init?.body === 'string' ? init.body : '{}';
      const body = JSON.parse(rawBody) as Partial<PracticeSession>;
      const now = new Date().toISOString();
      const updated: PracticeSession = {
        ...sessions[sessionIndex],
        ...body,
        updated_at: now,
        last_activity_at: now,
        completed_at:
          body.status === 'completed' ? now : sessions[sessionIndex].completed_at,
      };
      sessions[sessionIndex] = updated;
      return json(updated);
    }
  }

  if (method === 'GET' && path === '/problems') {
    const language = url.searchParams.get('language');
    const problems = language
      ? mockProblemSummaries.filter((problem) => problem.language === language)
      : mockProblemSummaries;

    return json({ problems, total: problems.length });
  }

  const problemSkeletonMatch = path.match(/^\/problems\/([^/]+)\/skeleton$/);
  if (method === 'GET' && problemSkeletonMatch) {
    const problemId = problemSkeletonMatch[1];
    const files = mockSkeletons[problemId];

    if (!files) {
      return error('not_found', `No mock skeleton found for ${problemId}`);
    }

    return json({ files });
  }

  const problemMatch = path.match(/^\/problems\/([^/]+)$/);
  if (method === 'GET' && problemMatch) {
    const problemId = problemMatch[1];
    const problem = mockProblems.find((candidate) => candidate.id === problemId);

    if (!problem) {
      return error('not_found', `No mock problem found for ${problemId}`);
    }

    return json(problem);
  }

  if (method === 'POST' && path === '/submissions') {
    return json({ submission_id: `mock-submission-${Date.now()}` }, { status: 202 });
  }

  const submissionMatch = path.match(/^\/submissions\/([^/]+)$/);
  if (method === 'GET' && submissionMatch) {
    return json({
      status: 'completed',
      result: mockPassingResult,
    });
  }

  // Memory event writes are fire-and-forget from product flows; accept and echo.
  if (method === 'POST' && path === '/memory/events') {
    return json({ id: `mock-event-${Date.now()}`, created_at: new Date().toISOString() }, { status: 201 });
  }

  // Profile refresh after a completed session; return the mock profile.
  if (method === 'POST' && path === '/memory/profile/refresh') {
    return json(mockMemoryProfile);
  }

  // Post-round reflection (deterministic refresh + LLM note CRUD); the mock
  // just returns the profile so the round loop keeps moving without a backend.
  if (method === 'POST' && path === '/memory/notes/maintain') {
    return json(mockMemoryProfile);
  }

  // Generation: return a canned MCQ set shaped like the backend response so
  // the marathon flow works without a backend or GenAI key.
  if (method === 'POST' && path === '/generate') {
    return json({
      kind: 'mcq',
      provider: 'mock',
      model: 'mock-model',
      questions: mockMcqQuestions,
    });
  }

  return error('mock_route_missing', `No mock API handler registered for ${method} ${path}`, 501);
}
