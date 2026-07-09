import { mockApiFetch } from '../../mocks/apiProxy';

const DEFAULT_API_BASE = '/api/v1';
const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? DEFAULT_API_BASE).replace(/\/$/, '');
const USE_MOCK_API = import.meta.env.DEV && import.meta.env.VITE_USE_MOCK_API !== 'false';

type AccessTokenProvider = () => Promise<string | null>;

let accessTokenProvider: AccessTokenProvider | null = null;

interface APIResponse<T> {
  data: T;
  error: { code: string; message: string } | null;
}

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly responseText?: string;

  constructor(
    message: string,
    {
      status,
      code,
      responseText,
    }: {
      status: number;
      code: string;
      responseText?: string;
    },
  ) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.code = code;
    this.responseText = responseText;
  }
}

export function setApiAccessTokenProvider(provider: AccessTokenProvider | null) {
  accessTokenProvider = provider;
}

async function getBearerToken(): Promise<string | null> {
  if (accessTokenProvider) {
    return accessTokenProvider();
  }

  return localStorage.getItem('codegym_token');
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await getBearerToken();
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

  const body = await parseApiResponse<T>(res);

  if (!res.ok) {
    if (body?.payload?.error) {
      throw new APIError(body.payload.error.message, {
        status: res.status,
        code: body.payload.error.code,
      });
    }

    throw new APIError(formatHTTPError(res, body?.responseText), {
      status: res.status,
      code: 'http_error',
      responseText: body?.responseText,
    });
  }

  if (!body?.payload) {
    throw new APIError('The API returned an empty response.', {
      status: res.status,
      code: 'empty_response',
    });
  }

  if (body.payload.error) {
    throw new APIError(body.payload.error.message, {
      status: res.status,
      code: body.payload.error.code,
    });
  }

  return body.payload.data;
}

async function parseApiResponse<T>(
  res: Response,
): Promise<{ payload: APIResponse<T> | null; responseText?: string } | null> {
  const text = await res.text();

  if (text.trim() === '') {
    return null;
  }

  const contentType = res.headers.get('content-type') ?? '';
  if (!contentType.includes('application/json')) {
    return { payload: null, responseText: text };
  }

  try {
    return { payload: JSON.parse(text) as APIResponse<T>, responseText: text };
  } catch {
    throw new APIError(formatHTTPError(res, text), {
      status: res.status,
      code: 'invalid_json',
      responseText: text,
    });
  }
}

function formatHTTPError(res: Response, responseText?: string) {
  const details = responseText?.trim();
  const prefix = `${res.status} ${res.statusText || 'HTTP error'}`;

  if (!details) {
    return prefix;
  }

  return `${prefix}: ${details.slice(0, 160)}`;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(data) }),
  put: <T>(path: string, data: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(data) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};
