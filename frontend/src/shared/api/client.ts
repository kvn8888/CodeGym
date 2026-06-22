const apiBaseEnv = import.meta.env.VITE_API_BASE_URL as string | undefined;
const API_BASE = (apiBaseEnv?.trim() || '/api/v1').replace(/\/$/, '');

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

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: { ...headers, ...options?.headers },
  });

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
