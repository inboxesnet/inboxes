import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { api, ApiError, uploadFile } from "../api";

const originalFetch = globalThis.fetch;

function mockFetch(response: {
  ok?: boolean;
  status?: number;
  statusText?: string;
  json?: unknown;
  body?: string;
}) {
  const {
    ok = true,
    status = 200,
    statusText = "OK",
    json,
    body,
  } = response;
  return vi.fn().mockResolvedValue({
    ok,
    status,
    statusText,
    json: json !== undefined ? vi.fn().mockResolvedValue(json) : vi.fn().mockRejectedValue(new Error("no json")),
    text: vi.fn().mockResolvedValue(body ?? ""),
  });
}

beforeEach(() => {
  vi.stubEnv("NEXT_PUBLIC_API_URL", "");
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.unstubAllEnvs();
});

describe("ApiError", () => {
  it("has status property and name ApiError", () => {
    const err = new ApiError(404, "Not Found");
    expect(err.status).toBe(404);
    expect(err.name).toBe("ApiError");
    expect(err.message).toBe("Not Found");
  });

  it("extends Error", () => {
    const err = new ApiError(500, "Server Error");
    expect(err).toBeInstanceOf(Error);
  });
});

describe("api.get", () => {
  it("sends GET with credentials: include", async () => {
    globalThis.fetch = mockFetch({ json: { data: "ok" } });
    await api.get("/test");
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/test",
      expect.objectContaining({
        credentials: "include",
      })
    );
    // GET should not have method explicitly set in our implementation
    // but should have Content-Type
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.headers["Content-Type"]).toBe("application/json");
  });
});

describe("api.post", () => {
  it("sends POST with JSON body", async () => {
    globalThis.fetch = mockFetch({ json: { id: "1" } });
    await api.post("/items", { name: "test" });
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.method).toBe("POST");
    expect(callArgs.body).toBe(JSON.stringify({ name: "test" }));
  });
});

describe("api.patch", () => {
  it("sends PATCH with JSON body", async () => {
    globalThis.fetch = mockFetch({ json: { id: "1" } });
    await api.patch("/items/1", { name: "updated" });
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.method).toBe("PATCH");
    expect(callArgs.body).toBe(JSON.stringify({ name: "updated" }));
  });
});

describe("api.delete", () => {
  it("sends DELETE", async () => {
    globalThis.fetch = mockFetch({ json: {} });
    await api.delete("/items/1");
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.method).toBe("DELETE");
  });
});

describe("api request — Content-Type header", () => {
  it("adds Content-Type: application/json", async () => {
    globalThis.fetch = mockFetch({ json: {} });
    await api.get("/test");
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.headers["Content-Type"]).toBe("application/json");
  });
});

describe("api request — error handling", () => {
  it("throws ApiError with correct status for non-ok response", async () => {
    globalThis.fetch = mockFetch({
      ok: false,
      status: 403,
      statusText: "Forbidden",
      json: { error: "access denied" },
    });
    await expect(api.get("/secret")).rejects.toThrow(ApiError);
    try {
      await api.get("/secret");
    } catch (e) {
      expect((e as ApiError).status).toBe(403);
      expect((e as ApiError).message).toBe("access denied");
    }
  });

  it("extracts error message from JSON body", async () => {
    globalThis.fetch = mockFetch({
      ok: false,
      status: 400,
      statusText: "Bad Request",
      json: { error: "invalid email" },
    });
    try {
      await api.post("/users", {});
    } catch (e) {
      expect((e as ApiError).message).toBe("invalid email");
    }
  });

  it("falls back to statusText when error body isn't JSON", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
      json: vi.fn().mockRejectedValue(new Error("not json")),
    });
    try {
      await api.get("/broken");
    } catch (e) {
      expect((e as ApiError).message).toBe("Internal Server Error");
    }
  });
});

describe("api request — 204 response", () => {
  it("returns undefined for 204", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      statusText: "No Content",
      json: vi.fn(),
    });
    const result = await api.delete("/items/1");
    expect(result).toBeUndefined();
  });
});

describe("uploadFile", () => {
  it("sends POST with FormData body (no Content-Type override)", async () => {
    const formData = new FormData();
    formData.append("file", new Blob(["test"]), "test.txt");
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ url: "/uploads/test.txt" }),
    });
    await uploadFile("/upload", formData);
    const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(callArgs.method).toBe("POST");
    expect(callArgs.body).toBe(formData);
    // Should NOT have Content-Type header (browser sets it with boundary)
    // but should have X-Requested-With for CSRF protection
    expect(callArgs.headers).toEqual({ "X-Requested-With": "XMLHttpRequest" });
  });

  it("throws ApiError for non-ok response", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 413,
      statusText: "Payload Too Large",
      json: vi.fn().mockResolvedValue({ error: "file too big" }),
    });
    await expect(uploadFile("/upload", new FormData())).rejects.toThrow(
      ApiError
    );
  });

  it("returns parsed JSON on success", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ url: "/uploads/test.txt" }),
    });
    const result = await uploadFile("/upload", new FormData());
    expect(result).toEqual({ url: "/uploads/test.txt" });
  });
});
