// In local dev, NEXT_PUBLIC_API_URL points to backend (http://localhost:8080).
// In production behind a reverse proxy, both frontend and API share the same domain,
// so we use relative paths (empty string).
const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

// Module-level flag to suppress cascading 401 API calls once session expires
let sessionExpired = false;

interface FetchOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  signal?: AbortSignal;
}

async function request<T = unknown>(
  path: string,
  opts?: FetchOptions
): Promise<T> {
  if (sessionExpired) {
    throw new ApiError(401, "Session expired");
  }

  const { body, headers, ...rest } = opts || {};
  const res = await fetch(`${API_URL}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "XMLHttpRequest",
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
    ...rest,
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));

    // Global 401 interceptor — show session expired modal instead of hard redirect
    if (res.status === 401) {
      if (!sessionExpired) {
        sessionExpired = true;
        window.dispatchEvent(new CustomEvent("session-expired"));
      }
      throw new ApiError(401, "Session expired");
    }

    // Global 402 interceptor — dispatch event for payment wall
    if (res.status === 402) {
      window.dispatchEvent(new CustomEvent("payment-required"));
    }

    throw new ApiError(res.status, err.error || res.statusText, err.code);
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export const api = {
  get: <T = unknown>(path: string, opts?: { signal?: AbortSignal }) =>
    request<T>(path, opts),
  post: <T = unknown>(path: string, body?: unknown, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "POST", body, ...opts }),
  patch: <T = unknown>(path: string, body?: unknown, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "PATCH", body, ...opts }),
  delete: <T = unknown>(path: string, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "DELETE", ...opts }),
};

export function uploadFile(path: string, formData: FormData) {
  if (sessionExpired) {
    return Promise.reject(new ApiError(401, "Session expired"));
  }

  return fetch(`${API_URL}${path}`, {
    method: "POST",
    credentials: "include",
    headers: { "X-Requested-With": "XMLHttpRequest" },
    body: formData,
  }).then(async (res) => {
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));

      if (res.status === 401) {
        if (!sessionExpired) {
          sessionExpired = true;
          window.dispatchEvent(new CustomEvent("session-expired"));
        }
        throw new ApiError(401, "Session expired");
      }
      if (res.status === 402) {
        window.dispatchEvent(new CustomEvent("payment-required"));
      }

      throw new ApiError(res.status, err.error || res.statusText);
    }
    return res.json();
  });
}
