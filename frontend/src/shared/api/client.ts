import { mockApiFetch } from '../../mocks/apiProxy';
import type { components, paths } from './openapi';

const DEFAULT_API_BASE = '/api/v1';
// The relative fallback is for local Vite development, where /api is proxied to
// the local backend. Deployed builds set an explicit environment-scoped base URL.
const API_BASE = ((import.meta.env.VITE_API_BASE_URL ?? '').trim() || DEFAULT_API_BASE).replace(
  /\/$/,
  '',
);
const USE_MOCK_API = import.meta.env.DEV && import.meta.env.VITE_USE_MOCK_API !== 'false';

type ApiErrorBody = components['schemas']['ApiError'];
type MemoryProfile = paths['/api/v1/memory/profile']['get']['responses'][200]['content']['application/json']['data'];
type MemoryEvents = paths['/api/v1/memory/events']['get']['responses'][200]['content']['application/json']['data'];
type RefreshMemoryProfile = paths['/api/v1/memory/profile/refresh']['post']['responses'][200]['content']['application/json']['data'];
type RecordMemoryEventInput = paths['/api/v1/memory/events']['post']['requestBody']['content']['application/json'];
type RecordMemoryEvent = paths['/api/v1/memory/events']['post']['responses'][201]['content']['application/json']['data'];
type AccessTokenProvider = () => Promise<string | null>;

let accessTokenProvider: AccessTokenProvider | null = null;

export class ApiRequestError extends Error {
  readonly code: string;
  readonly status: number;
  readonly error: ApiErrorBody | null;

  constructor(code: string, message: string, status: number, error: ApiErrorBody | null = null) {
    super(message);
    this.name = 'ApiRequestError';
    this.code = code;
    this.status = status;
    this.error = error;
  }
}

function resolveApiBaseUrl() {
  return API_BASE;
}

function joinUrl(baseUrl: string, path: string) {
  return `${baseUrl}${path.startsWith('/') ? path : `/${path}`}`;
}

function buildHeaders(options?: RequestInit, token?: string | null) {
  const headers = new Headers(options?.headers ?? {});

  if (!headers.has('Content-Type') && options?.body != null) {
    headers.set('Content-Type', 'application/json');
  }

  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  return headers;
}

async function readJson<T>(response: Response): Promise<T> {
  const text = await response.text();

  if (!text) {
    throw new ApiRequestError('empty_response', 'The backend returned an empty response.', response.status);
  }

  try {
    return JSON.parse(text) as T;
  } catch {
    throw new ApiRequestError('invalid_json', 'The backend returned invalid JSON.', response.status);
  }
}

export function setApiAccessTokenProvider(provider: AccessTokenProvider | null) {
  accessTokenProvider = provider;
}

async function getBearerToken(): Promise<string | null> {
  if (accessTokenProvider) {
    return accessTokenProvider();
  }

  if (typeof localStorage === 'undefined') {
    return null;
  }

  try {
    return localStorage.getItem('codegym_token');
  } catch {
    return null;
  }
}

export interface ServerSentEvent<T = unknown> {
  type: 'meta' | 'delta' | 'complete' | 'error';
  data: T;
}

export async function streamChatTurn(
  threadId: string,
  input: { client_message_id: string; message: string },
  onEvent: (event: ServerSentEvent) => void,
  signal?: AbortSignal,
) {
  if (USE_MOCK_API) {
    onEvent({ type: 'meta', data: { thread_id: threadId, message_id: `mock-${Date.now()}` } });
    for (const content of ['Start with the constraints. ', 'What invariant should remain true after each step?']) {
      if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
      onEvent({ type: 'delta', data: { content } });
    }
    onEvent({
      type: 'complete',
      data: {
        id: `mock-${Date.now()}`,
        thread_id: threadId,
        role: 'assistant',
        content: 'Start with the constraints. What invariant should remain true after each step?',
        status: 'complete',
        created_at: new Date().toISOString(),
      },
    });
    return;
  }

  const token = await getBearerToken();
  const headers = buildHeaders({ body: '{}' }, token);
  headers.set('Accept', 'text/event-stream');
  const response = await fetch(
    joinUrl(resolveApiBaseUrl(), `/chat/threads/${encodeURIComponent(threadId)}/turns`),
    {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
      signal,
    },
  );
  if (!response.ok || !response.body) {
    const body = await readJson<{ error?: ApiErrorBody | null }>(response);
    throw new ApiRequestError(
      body.error?.code ?? `http_${response.status}`,
      body.error?.message ?? response.statusText ?? 'The conversation could not continue.',
      response.status,
      body.error ?? null,
    );
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const blocks = buffer.split(/\r?\n\r?\n/);
    buffer = blocks.pop() ?? '';
    for (const block of blocks) {
      let eventType = 'message';
      const dataLines: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith('event:')) eventType = line.slice(6).trim();
        if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
      }
      if (!dataLines.length) continue;
      const data = JSON.parse(dataLines.join('\n')) as unknown;
      if (eventType === 'meta' || eventType === 'delta' || eventType === 'complete' || eventType === 'error') {
        onEvent({ type: eventType, data });
      }
    }
    if (done) break;
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await getBearerToken();
  const url = joinUrl(resolveApiBaseUrl(), path);
  const requestOptions: RequestInit = {
    ...options,
    headers: buildHeaders(options, token),
  };

  const response =
    (USE_MOCK_API ? await mockApiFetch(url, requestOptions) : null) ??
    (await fetch(url, requestOptions));

  const body = await readJson<{ data: T; error: ApiErrorBody | null }>(response);

  if (body.error) {
    throw new ApiRequestError(body.error.code, body.error.message, response.status, body.error);
  }

  if (!response.ok) {
    throw new ApiRequestError(
      `http_${response.status}`,
      response.statusText || 'The backend request failed.',
      response.status,
    );
  }

  return body.data;
}

export async function getMemoryProfile() {
  return request<MemoryProfile>('/memory/profile');
}

export async function refreshMemoryProfile() {
  return request<RefreshMemoryProfile>('/memory/profile/refresh', { method: 'POST' });
}

export async function listMemoryEvents() {
  return request<MemoryEvents>('/memory/events');
}

export async function createMemoryEvent(input: RecordMemoryEventInput) {
  return request<RecordMemoryEvent>('/memory/events', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export const api = {
  request,
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(data) }),
  patch: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(data) }),
  put: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(data) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
  memory: {
    getProfile: getMemoryProfile,
    refreshProfile: refreshMemoryProfile,
    listEvents: listMemoryEvents,
    createEvent: createMemoryEvent,
  },
};

export type { RecordMemoryEventInput };
