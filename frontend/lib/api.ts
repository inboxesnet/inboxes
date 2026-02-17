// In local dev, NEXT_PUBLIC_API_URL points to backend (http://localhost:8080).
// In production behind Caddy, both frontend and API share the same domain,
// so we use relative paths (empty string).
const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

interface FetchOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
}

async function request<T = unknown>(
  path: string,
  opts?: FetchOptions
): Promise<T> {
  const { body, headers, ...rest } = opts || {};
  const res = await fetch(`${API_URL}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
    ...rest,
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, err.error || res.statusText);
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export const api = {
  get: <T = unknown>(path: string) => request<T>(path),
  post: <T = unknown>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body }),
  patch: <T = unknown>(path: string, body?: unknown) =>
    request<T>(path, { method: "PATCH", body }),
  delete: <T = unknown>(path: string) =>
    request<T>(path, { method: "DELETE" }),
};

export function uploadFile(path: string, formData: FormData) {
  return fetch(`${API_URL}${path}`, {
    method: "POST",
    credentials: "include",
    body: formData,
  }).then(async (res) => {
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new ApiError(res.status, err.error || res.statusText);
    }
    return res.json();
  });
}
