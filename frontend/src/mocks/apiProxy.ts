import { mockPassingResult, mockProblems, mockProblemSummaries, mockSkeletons } from './fixtures';
import { mockMcqQuestions } from './mcqFixtures';
import { mockMemoryProfile } from './memoryFixtures';
import { mockMemoryEvents, mockSessions } from './activityFixtures';
import type {
  MemoryEvent,
  ChatMessage,
  ChatThreadWithMessages,
  PracticeIntake,
  PracticeSession,
  Problem,
  Submission,
  SubmissionFile,
  TestResult,
  UserMemoryProfile,
  UserProfile,
} from '../shared/api/types';

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
let practiceIntakes: PracticeIntake[] = [];
let chatThreads: ChatThreadWithMessages[] = [];

const mockIntakeQuestions = [
  {
    id: 'q1',
    dimension: 'exposure',
    text: 'How much prior exposure do you have to this topic?',
    options: [
      { id: 'new', label: 'This is new to me' },
      { id: 'some', label: 'I recognize the core ideas' },
      { id: 'practiced', label: 'I have practiced it before' },
    ],
  },
  {
    id: 'q2',
    dimension: 'application',
    text: 'Where have you applied it?',
    options: [
      { id: 'none', label: 'Not in practice yet' },
      { id: 'guided', label: 'In guided exercises' },
      { id: 'independent', label: 'In an independent project or interview' },
    ],
  },
  {
    id: 'q3',
    dimension: 'challenge',
    text: 'What kind of challenge would help today?',
    options: [
      { id: 'foundations', label: 'Reinforce foundations' },
      { id: 'mixed', label: 'Mix recall with application' },
      { id: 'stretch', label: 'Push me with edge cases' },
    ],
  },
];

interface MockProblemFixture {
  problem: Problem;
  skeleton: { files: SubmissionFile[] };
  result?: TestResult;
  resumedFiles?: SubmissionFile[];
  hintsRevealed?: number;
}

let mockProblemFixture: MockProblemFixture | null = null;

export function setMockProblemFixture(fixture: MockProblemFixture | null) {
  mockProblemFixture = fixture ? structuredClone(fixture) : null;
  if (!fixture) return;

  const now = new Date().toISOString();
  const session: PracticeSession = {
    id: `storybook-${fixture.problem.id}`,
    workspace_id: mockUserProfile.default_workspace_id,
    user_id: mockUserProfile.user_id,
    kind: 'workspace',
    status: 'active',
    title: fixture.problem.title,
    problem_id: fixture.problem.id,
    state: {
      schema_version: 1,
      hints_revealed: fixture.hintsRevealed ?? 0,
    },
    files: (fixture.resumedFiles ?? []).map((file) => ({
      file_path: file.path,
      content: file.content,
      updated_at: now,
    })),
    created_at: now,
    updated_at: now,
    last_activity_at: now,
  };
  sessions = [session, ...sessions.filter((candidate) => candidate.id !== session.id)];
}

const mockEventAges = [2 * 60_000, 18 * 60_000, 26 * 60 * 60_000, 2 * 24 * 60 * 60_000];

function createMemoryEventSeed(): MemoryEvent[] {
  const now = Date.now();
  return structuredClone(mockMemoryEvents).map((event, index) => {
    const occurredAt = new Date(
      now - (mockEventAges[index] ?? index * 24 * 60 * 60_000),
    ).toISOString();
    return {
      ...event,
      occurred_at: occurredAt,
      created_at: new Date(new Date(occurredAt).getTime() + 1_000).toISOString(),
    };
  });
}

let memoryEvents: MemoryEvent[] = createMemoryEventSeed();

export type MockApiScenario =
  | 'default'
  | 'empty'
  | 'error'
  | 'loading'
  | 'nullable-memory'
  | 'intake-partial'
  | 'intake-known'
  | 'intake-skipped'
  | 'intake-error'
  | 'intake-loading';

let mockApiScenario: MockApiScenario = 'default';

export function setMockApiScenario(scenario: MockApiScenario) {
  mockApiScenario = scenario;
  sessions = structuredClone(mockSessions);
  memoryEvents = scenario === 'empty' ? [] : createMemoryEventSeed();
  practiceIntakes = [];
  chatThreads = [{
    id: 'mock-thread-completed',
    workspace_id: mockUserProfile.default_workspace_id,
    user_id: mockUserProfile.user_id,
    session_id: 'sess_interview_completed',
    kind: 'interview',
    mode: 'behavioral',
    status: 'completed',
    context: { version: 1, session_id: 'sess_interview_completed' },
    created_at: '2026-07-09T18:00:00.000Z',
    updated_at: '2026-07-09T18:24:00.000Z',
    closed_at: '2026-07-09T18:24:00.000Z',
    messages: [
      {
        id: 'mock-completed-opening',
        thread_id: 'mock-thread-completed',
        role: 'assistant',
        content: 'Tell me about a time you changed a team’s technical direction.',
        status: 'complete',
        created_at: '2026-07-09T18:00:00.000Z',
      },
      {
        id: 'mock-completed-answer',
        thread_id: 'mock-thread-completed',
        role: 'user',
        content: 'I aligned the team around a smaller migration with measurable checkpoints.',
        status: 'complete',
        created_at: '2026-07-09T18:03:00.000Z',
      },
    ],
  }];
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

  if (mockApiScenario === 'intake-loading' && method === 'POST' && path === '/practice-intakes') {
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

  if (method === 'GET' && path === '/cost') {
    return json({
      currency: 'USD',
      pricing_as_of: '2026-07-15',
      total_tokens_in: 12400,
      total_tokens_out: 6100,
      total_cost_usd: 0.0421,
      call_count: 8,
      by_provider: [
        {
          provider: 'azure',
          tokens_in: 8000,
          tokens_out: 4000,
          cost_usd: 0.08,
          call_count: 5,
        },
        {
          provider: 'meta',
          tokens_in: 4400,
          tokens_out: 2100,
          cost_usd: 0.0145,
          call_count: 3,
        },
      ],
      by_model: [],
    });
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
    if (mockApiScenario === 'nullable-memory') {
      return json({
        ...mockMemoryProfile,
        strengths: null,
        growth_edges: null,
        skills: null,
        notes: null,
      } as unknown as UserMemoryProfile);
    }
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
    if (mockApiScenario === 'nullable-memory') {
      return json(null);
    }
    return json([...memoryEvents].sort((a, b) => b.occurred_at.localeCompare(a.occurred_at)));
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

  if (method === 'GET' && path === '/practice-intakes') {
    return json(practiceIntakes.filter((record) => record.status === 'pending').slice(0, 1));
  }

  if (method === 'POST' && path === '/practice-intakes') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as {
      topic?: string;
      practice_seed?: unknown;
      restart?: boolean;
    };
    const now = new Date().toISOString();
    const terminalStatus =
      mockApiScenario === 'intake-known' || mockApiScenario === 'intake-skipped'
        ? 'skipped'
        : 'pending';
    const record: PracticeIntake = {
      id: `mock-intake-${Date.now()}`,
      workspace_id: mockUserProfile.default_workspace_id,
      user_id: mockUserProfile.user_id,
      normalized_topic: (body.topic ?? '').trim().toLowerCase(),
      original_topic: (body.topic ?? '').trim(),
      practice_seed:
        body.practice_seed && typeof body.practice_seed === 'object'
          ? (body.practice_seed as Record<string, unknown>)
          : {},
      questions: terminalStatus === 'pending' && mockApiScenario !== 'intake-error' ? mockIntakeQuestions : [],
      answers: mockApiScenario === 'intake-partial' ? { q1: 'some' } : {},
      status: terminalStatus,
      suppression_reason:
        mockApiScenario === 'intake-known'
          ? 'demonstrated_event'
          : mockApiScenario === 'intake-skipped'
            ? 'user_skipped'
            : undefined,
      generation_error:
        mockApiScenario === 'intake-error'
          ? 'The model did not return a usable baseline. Retry or skip.'
          : undefined,
      created_at: now,
      updated_at: now,
      completed_at: terminalStatus === 'skipped' ? now : undefined,
    };
    practiceIntakes = [record, ...practiceIntakes];
    return json(record);
  }

  const intakeMatch = path.match(/^\/practice-intakes\/([^/]+)$/);
  if (method === 'PATCH' && intakeMatch) {
    const index = practiceIntakes.findIndex((record) => record.id === intakeMatch[1]);
    if (index < 0) return error('intake_not_found', 'Practice intake not found.', 404);
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as {
      answers?: Record<string, string>;
      status?: PracticeIntake['status'];
    };
    const now = new Date().toISOString();
    practiceIntakes[index] = {
      ...practiceIntakes[index],
      answers: { ...practiceIntakes[index].answers, ...body.answers },
      status: body.status ?? practiceIntakes[index].status,
      updated_at: now,
      completed_at: body.status && body.status !== 'pending' ? now : practiceIntakes[index].completed_at,
    };
    return json(practiceIntakes[index]);
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
      problem_id: body.problem_id,
      generation_job_id: body.generation_job_id,
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

  const sessionFilesMatch = path.match(/^\/sessions\/([^/]+)\/files$/);
  if (method === 'PUT' && sessionFilesMatch) {
    const sessionIndex = sessions.findIndex((candidate) => candidate.id === sessionFilesMatch[1]);
    if (sessionIndex < 0) {
      return error('not_found', 'Practice session not found.', 404);
    }

    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as {
      files?: Array<{ file_path: string; content: string }>;
    };
    const now = new Date().toISOString();
    const updated: PracticeSession = {
      ...sessions[sessionIndex],
      files: (body.files ?? []).map((file) => ({ ...file, updated_at: now })),
      updated_at: now,
      last_activity_at: now,
    };
    sessions[sessionIndex] = updated;
    return json(updated);
  }

  if (method === 'GET' && path === '/chat/threads') {
    const kind = url.searchParams.get('kind');
    const status = url.searchParams.get('status');
    const sessionId = url.searchParams.get('session_id');
    return json({
      threads: chatThreads
        .filter((thread) => !kind || thread.kind === kind)
        .filter((thread) => !status || thread.status === status)
        .filter((thread) => !sessionId || thread.session_id === sessionId)
        .map((thread) => ({
          id: thread.id,
          workspace_id: thread.workspace_id,
          user_id: thread.user_id,
          session_id: thread.session_id,
          kind: thread.kind,
          mode: thread.mode,
          status: thread.status,
          context: thread.context,
          successor_id: thread.successor_id,
          created_at: thread.created_at,
          updated_at: thread.updated_at,
          closed_at: thread.closed_at,
        })),
    });
  }

  if (method === 'POST' && path === '/chat/threads') {
    const body = JSON.parse(typeof init?.body === 'string' ? init.body : '{}') as {
      kind?: 'interview' | 'coach';
      mode?: 'coding' | 'system_design' | 'behavioral' | 'open_coaching';
      session_id?: string;
    };
    const existing = chatThreads.find(
      (thread) =>
        thread.kind === (body.kind ?? 'coach') &&
        thread.session_id === body.session_id &&
        thread.status === 'active',
    );
    if (existing) return json(existing, { status: 201 });
    const now = new Date().toISOString();
    const id = `mock-thread-${Date.now()}`;
    const opening: ChatMessage[] =
      body.kind === 'interview'
        ? [{
            id: `${id}-opening`,
            thread_id: id,
            role: 'assistant',
            content: 'Walk me through how you would approach this interview topic before choosing an implementation.',
            status: 'complete',
            created_at: now,
          }]
        : [];
    const thread: ChatThreadWithMessages = {
      id,
      workspace_id: mockUserProfile.default_workspace_id,
      user_id: mockUserProfile.user_id,
      session_id: body.session_id ?? '',
      kind: body.kind ?? 'coach',
      mode: body.mode,
      status: 'active',
      context: { version: 1, session_id: body.session_id ?? '' },
      created_at: now,
      updated_at: now,
      messages: opening,
    };
    chatThreads = [thread, ...chatThreads];
    return json(thread, { status: 201 });
  }

  const chatMessagesMatch = path.match(/^\/chat\/threads\/([^/]+)\/messages$/);
  if (method === 'GET' && chatMessagesMatch) {
    const thread = chatThreads.find((candidate) => candidate.id === chatMessagesMatch[1]);
    return thread
      ? json({ messages: thread.messages })
      : error('chat_thread_not_found', 'Conversation not found.', 404);
  }

  const chatActionMatch = path.match(/^\/chat\/threads\/([^/]+)\/(reset|finish|exit)$/);
  if (method === 'POST' && chatActionMatch) {
    const thread = chatThreads.find((candidate) => candidate.id === chatActionMatch[1]);
    if (!thread) return error('chat_thread_not_found', 'Conversation not found.', 404);
    if (chatActionMatch[2] === 'reset') {
      thread.status = 'closed';
      const now = new Date().toISOString();
      const successor: ChatThreadWithMessages = {
        ...thread,
        id: `mock-thread-${Date.now()}`,
        status: 'active',
        created_at: now,
        updated_at: now,
        closed_at: undefined,
        successor_id: undefined,
        messages:
          thread.kind === 'interview'
            ? [{
                id: `mock-opening-${Date.now()}`,
                thread_id: `mock-thread-${Date.now()}`,
                role: 'assistant',
                content: 'Let’s begin again. What would you clarify first?',
                status: 'complete',
                created_at: now,
              }]
            : [],
      };
      chatThreads = [successor, ...chatThreads];
      return json(successor, { status: 201 });
    }
    if (chatActionMatch[2] === 'finish') {
      thread.status = 'completed';
      const practiceSession = sessions.find((candidate) => candidate.id === thread.session_id);
      if (practiceSession) practiceSession.status = 'completed';
      return json({
        thread,
        assessment: {
          strengths: ['Clear decomposition'],
          growth_edges: ['Tradeoff depth'],
          topic: 'Algorithms',
          mode: thread.mode ?? 'open_coaching',
          turn_count: 1,
          duration_seconds: 120,
        },
        memory_update_status: 'synced',
      });
    }
    return json({ thread });
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
    if (mockProblemFixture?.problem.id === problemId) {
      return json(mockProblemFixture.skeleton);
    }
    const files = mockSkeletons[problemId];

    if (!files) {
      return error('not_found', `No mock skeleton found for ${problemId}`);
    }

    return json({ files });
  }

  const problemMatch = path.match(/^\/problems\/([^/]+)$/);
  if (method === 'GET' && problemMatch) {
    const problemId = problemMatch[1];
    if (mockProblemFixture?.problem.id === problemId) {
      return json(mockProblemFixture.problem);
    }
    const problem = mockProblems.find((candidate) => candidate.id === problemId);

    if (!problem) {
      return error('not_found', `No mock problem found for ${problemId}`);
    }

    return json(problem);
  }

  if (method === 'POST' && path === '/submissions') {
    return json(
      { submission_id: `mock-submission-${Date.now()}`, memory_update_status: 'synced' },
      { status: 202 },
    );
  }

  const submissionMatch = path.match(/^\/submissions\/([^/]+)$/);
  if (method === 'GET' && submissionMatch) {
    const submission: Submission = {
      status: 'completed',
      result: mockProblemFixture?.result ?? mockPassingResult,
      stdout: '',
      output_truncated: false,
    };
    return json(submission);
  }

  // Memory event writes are retained for the lifetime of the mock scenario.
  if (method === 'POST' && path === '/memory/events') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as Partial<MemoryEvent>;
    const now = new Date().toISOString();
    const event: MemoryEvent = {
      id: `mock-event-${Date.now()}-${memoryEvents.length + 1}`,
      workspace_id: mockUserProfile.default_workspace_id,
      user_id: mockUserProfile.user_id,
      source: body.source ?? 'unknown',
      type: body.type ?? 'unknown',
      summary: body.summary ?? 'Recorded practice activity.',
      ...(body.payload === undefined ? {} : { payload: body.payload }),
      occurred_at: body.occurred_at ?? now,
      created_at: now,
    };
    memoryEvents.push(event);
    return json(event, { status: 201 });
  }

  // Profile refresh after a completed session; return the mock profile.
  if (method === 'POST' && path === '/memory/profile/refresh') {
    return json(mockMemoryProfile);
  }

  // Post-round full-profile synthesis; the mock returns the profile so the
  // round loop keeps moving without a backend.
  if (
    method === 'POST' &&
    (path === '/memory/profile/maintain' || path === '/memory/notes/maintain')
  ) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    return json(mockMemoryProfile);
  }

  // Generation: return a canned MCQ set shaped like the backend response so
  // the marathon flow works without a backend or GenAI key.
  if (method === 'POST' && path === '/generate') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as {
      kind?: string;
      spec?: { count?: number };
    };
    if (body.kind === 'problem') {
      const problem = mockProblems[0];
      return json({
        kind: 'problem',
        problem_id: problem.id,
        problem,
        provider: 'mock',
        model: 'mock-model',
      });
    }
    const count = Math.max(1, body.spec?.count ?? 5);
    const questions = Array.from({ length: count }, (_, index) => ({
      ...mockMcqQuestions[index % mockMcqQuestions.length],
      id: `mq${index + 1}`,
    }));
    return json({
      kind: 'mcq',
      provider: 'mock',
      model: 'mock-model',
      questions,
    });
  }

  if (method === 'POST' && path === '/mcq/evaluate') {
    const rawBody = typeof init?.body === 'string' ? init.body : '{}';
    const body = JSON.parse(rawBody) as { answer?: string };
    const answer = body.answer?.trim() ?? '';
    const correct = /level|distance|edge|queue/i.test(answer) && answer.length >= 20;
    return json({
      correct,
      feedback: correct
        ? 'Correct. You connected queue order to increasing path distance.'
        : 'Explain how queue order processes every vertex at one distance before moving to the next distance.',
      provider: 'mock',
      model: 'mock-evaluator',
    });
  }

  return error('mock_route_missing', `No mock API handler registered for ${method} ${path}`, 501);
}
