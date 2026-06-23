import { mockApiFetch } from '../../mocks/apiProxy';

const API_BASE = '/api/v1';
const USE_MOCK_API = import.meta.env.DEV && import.meta.env.VITE_USE_MOCK_API !== 'false';


interface APIResponse<T> {
  data: T;
  error: { code: string; message: string } | null;
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
    throw new Error(body.error.message);
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
