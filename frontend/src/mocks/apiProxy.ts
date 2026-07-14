import { mockPassingResult, mockProblems, mockProblemSummaries, mockSkeletons } from './fixtures';
import { mockMcqQuestions } from './mcqFixtures';
import { mockMemoryProfile } from './memoryFixtures';
import { mockMemoryEvents, mockMemoryProfile } from './memoryFixtures';

interface MockApiResponse<T> {
  data: T;
  error: { code: string; message: string } | null;
}

const jsonHeaders = { 'Content-Type': 'application/json' };
let memoryProfile = mockMemoryProfile;
let memoryEvents = [...mockMemoryEvents];

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

function parseBody(input?: BodyInit | null) {
  if (input == null) {
    return null;
  }

  if (typeof input === 'string') {
    return JSON.parse(input) as Record<string, unknown>;
  }

  if (input instanceof Blob) {
    throw new Error('Blob request bodies are not supported by the mock API.');
  }

  return JSON.parse(String(input)) as Record<string, unknown>;
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

  if (method === 'GET' && path === '/memory/profile') {
    return json(memoryProfile);
  }

  if (method === 'POST' && path === '/memory/profile/refresh') {
    memoryProfile = {
      ...memoryProfile,
      updated_at: new Date().toISOString(),
    };

    return json(memoryProfile);
  }

  if (method === 'GET' && path === '/memory/events') {
    return json(memoryEvents);
  }

  if (method === 'POST' && path === '/memory/events') {
    const body = parseBody(init?.body);

    if (!body || typeof body.source !== 'string' || typeof body.type !== 'string' || typeof body.summary !== 'string') {
      return error('invalid_request', 'Missing required memory event fields.', 400);
    }

    const now = new Date().toISOString();
    const event = {
      id: `mem_evt_mock_${memoryEvents.length + 1}`,
      tenant_id: 'workspace_kevin',
      user_id: 'user_kevin',
      source: body.source,
      type: body.type,
      summary: body.summary,
      payload: body.payload,
      occurred_at: typeof body.occurred_at === 'string' ? body.occurred_at : now,
      created_at: now,
    };

    memoryEvents = [...memoryEvents, event as (typeof memoryEvents)[number]];

    return json(event, { status: 201 });
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
