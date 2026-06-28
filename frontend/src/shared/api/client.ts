import { mockApiFetch } from '../../mocks/apiProxy';
import type { components } from './openapi';

const DEFAULT_API_BASE = '/api/v1';
const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? DEFAULT_API_BASE).replace(/\/$/, '');
const USE_MOCK_API = import.meta.env.DEV && import.meta.env.VITE_USE_MOCK_API !== 'false';

export type ApiErrorBody = components['schemas']['ApiError'];
export type MemoryProfile = components['schemas']['MemoryProfile'];
export type MemoryEvent = components['schemas']['MemoryEvent'];
export type RecordMemoryEventInput = components['schemas']['RecordMemoryEventInput'];

interface APIResponse<T> {
  data: T;
  error: ApiErrorBody | null;
}

export class ApiClientError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(error: ApiErrorBody, status: number) {
    super(error.message);
    this.name = 'ApiClientError';
    this.code = error.code;
    this.status = status;
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('codegym_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };

  const url = `${API_BASE}${path}`;
  const requestOptions = {
    ...options,
    headers: { ...headers, ...options?.headers },
  };

  const res =
    (USE_MOCK_API ? await mockApiFetch(url, requestOptions) : null) ??
    (await fetch(url, requestOptions));

  const body: APIResponse<T> = await res.json();

  if (body.error) {
    throw new ApiClientError(body.error, res.status);
  }

  return body.data;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(data) }),
  put: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(data) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};

export const memoryApi = {
  getProfile: () => request<MemoryProfile>('/memory/profile'),
  refreshProfile: () => request<MemoryProfile>('/memory/profile/refresh', { method: 'POST' }),
  listEvents: () => request<MemoryEvent[]>('/memory/events'),
  recordEvent: (event: RecordMemoryEventInput) =>
    request<MemoryEvent>('/memory/events', { method: 'POST', body: JSON.stringify(event) }),
};
