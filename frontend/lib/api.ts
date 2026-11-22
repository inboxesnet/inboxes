// In local dev, NEXT_PUBLIC_API_URL points to backend (http://localhost:8080).
// In production behind a reverse proxy, both frontend and API share the same domain,
// so we use relative paths (empty string).
const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

// Module-level flag to suppress cascading 401 API calls once session expires
let sessionExpired = false;

// Key of the persisted query cache. Keep it equal to CACHE_KEY in
// lib/query-client.ts.
const QUERY_CACHE_KEY = "inboxes-qc";

// The persisted query cache holds mail data. Remove it when the session
// ends, so that the next user of this browser cannot read it.
function clearPersistedCache() {
  try {
    localStorage.removeItem(QUERY_CACHE_KEY);
  } catch {
    // Storage is not available. There is nothing to remove.
  }
}

let sessionProbe: Promise<boolean> | null = null;

// One 401 is not proof of a dead session. A request that was in flight
// during a token renewal can get a 401 while the cookie is already new.
// This probe asks the server. All parallel callers share one probe.
// It resolves to true only when the server answers 401.
function sessionIsDead(): Promise<boolean> {
  if (!sessionProbe) {
    sessionProbe = fetch(`${API_URL}/api/users/me`, {
      credentials: "include",
      cache: "no-store",
      headers: { "X-Requested-With": "XMLHttpRequest" },
    })
      .then(
        (res) => res.status === 401,
        () => false
      )
      .finally(() => {
        sessionProbe = null;
      });
  }
  return sessionProbe;
}

function latchSessionExpired() {
  if (!sessionExpired) {
    sessionExpired = true;
    clearPersistedCache();
    window.dispatchEvent(new CustomEvent("session-expired"));
  }
}

// Handles a 401 from a session-protected call. It returns true when the
// session is alive and the caller can send the request again.
async function canRetryAfter401(alreadyRetried: boolean): Promise<boolean> {
  if (await sessionIsDead()) {
    latchSessionExpired();
    return false;
  }
  return !alreadyRetried;
}

interface FetchOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  signal?: AbortSignal;
  // Session probes (e.g. "am I logged in?" checks on auth pages) must not
  // trip the global 401 latch — an expected 401 there would block all
  // later calls, including the login POST itself.
  noLatch?: boolean;
}

async function request<T = unknown>(
  path: string,
  opts?: FetchOptions,
  retried = false
): Promise<T> {
  const { body, headers, noLatch, ...rest } = opts || {};
  // Auth endpoints are exempt from the latch in both directions: a failed
  // login is a 401 that must reach the user as "invalid credentials", and
  // a successful login must always be able to reach the network.
  const latchExempt = noLatch || path.startsWith("/api/auth/");

  if (sessionExpired && !latchExempt) {
    throw new ApiError(401, "Session expired");
  }
  if (path === "/api/auth/logout") {
    clearPersistedCache();
  }
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
      if (latchExempt) {
        throw new ApiError(401, err.error || "unauthorized", err.code);
      }
      if (await canRetryAfter401(retried)) {
        return request<T>(path, opts, true);
      }
      throw new ApiError(401, sessionExpired ? "Session expired" : "unauthorized");
    }

    // Global 402 interceptor — dispatch event for payment wall
    if (res.status === 402) {
      window.dispatchEvent(new CustomEvent("payment-required"));
    }

    throw new ApiError(res.status, err.error || res.statusText, err.code);
  }

  // A successful auth call (login, verify, claim) starts a fresh session.
  if (path.startsWith("/api/auth/")) {
    sessionExpired = false;
    clearPersistedCache();
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
  get: <T = unknown>(path: string, opts?: { signal?: AbortSignal; noLatch?: boolean }) =>
    request<T>(path, opts),
  post: <T = unknown>(path: string, body?: unknown, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "POST", body, ...opts }),
  patch: <T = unknown>(path: string, body?: unknown, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "PATCH", body, ...opts }),
  delete: <T = unknown>(path: string, opts?: { signal?: AbortSignal }) =>
    request<T>(path, { method: "DELETE", ...opts }),
};

export function uploadFile(
  path: string,
  formData: FormData,
  retried = false
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
): Promise<any> {
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
        if (await canRetryAfter401(retried)) {
          return uploadFile(path, formData, true);
        }
        throw new ApiError(401, sessionExpired ? "Session expired" : "unauthorized");
      }
      if (res.status === 402) {
        window.dispatchEvent(new CustomEvent("payment-required"));
      }

      throw new ApiError(res.status, err.error || res.statusText);
    }
    return res.json();
  });
}

// uploadFileWithProgress uploads via XMLHttpRequest so byte progress can be
// reported. onProgress receives 0-100.
export function uploadFileWithProgress(
  path: string,
  formData: FormData,
  onProgress?: (percent: number) => void,
  retried = false
): Promise<{ id: string; filename: string; content_type: string; size: number }> {
  if (sessionExpired) {
    return Promise.reject(new ApiError(401, "Session expired"));
  }

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_URL}${path}`);
    xhr.withCredentials = true;
    xhr.setRequestHeader("X-Requested-With", "XMLHttpRequest");

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100));
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch {
          reject(new ApiError(xhr.status, "Invalid server response"));
        }
        return;
      }
      let message = xhr.statusText;
      try {
        const parsed = JSON.parse(xhr.responseText);
        if (parsed.error) message = parsed.error;
      } catch {
        // keep statusText
      }
      if (xhr.status === 401) {
        canRetryAfter401(retried).then((retry) => {
          if (retry) {
            uploadFileWithProgress(path, formData, onProgress, true).then(resolve, reject);
          } else {
            reject(new ApiError(401, sessionExpired ? "Session expired" : "unauthorized"));
          }
        });
        return;
      }
      if (xhr.status === 402) {
        window.dispatchEvent(new CustomEvent("payment-required"));
      }
      reject(new ApiError(xhr.status, message));
    };
    xhr.onerror = () => reject(new ApiError(0, "Network error"));
    xhr.send(formData);
  });
}
