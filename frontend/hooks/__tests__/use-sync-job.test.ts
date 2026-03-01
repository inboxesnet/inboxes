import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import type { SyncJob } from "@/lib/types";

const mockGet = vi.fn();
const mockPost = vi.fn();

vi.mock("@/lib/api", () => ({
  api: {
    get: (...args: unknown[]) => mockGet(...args),
    post: (...args: unknown[]) => mockPost(...args),
  },
}));

import { useSyncJob } from "../use-sync-job";

const pendingJob: SyncJob = {
  id: "job-1",
  status: "pending",
  phase: "scanning",
  imported: 0,
  total: 0,
  sent_count: 0,
  received_count: 0,
  thread_count: 0,
  address_count: 0,
  created_at: "2026-01-01T00:00:00Z",
};

const runningJob: SyncJob = {
  ...pendingJob,
  status: "running",
  phase: "importing",
  imported: 50,
  total: 100,
};

const completedJob: SyncJob = {
  ...pendingJob,
  status: "completed",
  phase: "done",
  imported: 100,
  total: 100,
  sent_count: 30,
  received_count: 70,
  thread_count: 25,
  address_count: 5,
};

const failedJob: SyncJob = {
  ...pendingJob,
  status: "failed",
  phase: "scanning",
  error_message: "API key invalid",
};

describe("useSyncJob", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("initial state: no job, no error, not running", () => {
    const { result } = renderHook(() => useSyncJob());
    expect(result.current.job).toBeNull();
    expect(result.current.error).toBe("");
    expect(result.current.isRunning).toBeFalsy();
    expect(result.current.isComplete).toBeFalsy();
    expect(result.current.isFailed).toBeFalsy();
  });

  it("startSync POSTs to /api/sync", async () => {
    mockPost.mockResolvedValue(pendingJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(mockPost).toHaveBeenCalledWith("/api/sync");
  });

  it("job is set on successful startSync", async () => {
    mockPost.mockResolvedValue(pendingJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.job).toEqual(pendingJob);
    expect(result.current.isRunning).toBe(true);
  });

  it("error is set on startSync failure", async () => {
    mockPost.mockRejectedValue(new Error("Network error"));
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.error).toBe("Failed to start sync");
  });

  it("polling starts after startSync", async () => {
    mockPost.mockResolvedValue(pendingJob);
    mockGet.mockResolvedValue(runningJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });
    expect(mockGet).toHaveBeenCalledWith(`/api/sync/${pendingJob.id}`);
  });

  it("polling stops on completed status", async () => {
    mockPost.mockResolvedValue(pendingJob);
    mockGet.mockResolvedValueOnce(completedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.isComplete).toBe(true);

    // Further polling should not happen
    mockGet.mockClear();
    await act(async () => {
      vi.advanceTimersByTime(4000);
    });
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("polling stops on failed status", async () => {
    mockPost.mockResolvedValue(pendingJob);
    mockGet.mockResolvedValueOnce(failedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.isFailed).toBe(true);
    expect(result.current.error).toBe("API key invalid");
  });

  it("isComplete is true for completed job", async () => {
    mockPost.mockResolvedValue(completedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.isComplete).toBe(true);
  });

  it("isFailed is true for failed job", async () => {
    mockPost.mockResolvedValue(failedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.isFailed).toBe(true);
  });

  it("progress message for scanning phase", async () => {
    mockPost.mockResolvedValue(pendingJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.progress?.message).toBe("Scanning emails...");
  });

  it("progress message for importing phase", async () => {
    mockPost.mockResolvedValue(runningJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.progress?.message).toBe(
      "Importing 50 of 100 emails"
    );
  });

  it("progress message for done phase", async () => {
    mockPost.mockResolvedValue(completedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.progress?.message).toContain("Imported");
    expect(result.current.progress?.message).toContain("30 sent");
    expect(result.current.progress?.message).toContain("70 received");
  });

  it("aliasesReady is true for aliases_ready phase", async () => {
    mockPost.mockResolvedValue({ ...pendingJob, phase: "aliases_ready" });
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.aliasesReady).toBe(true);
  });

  it("aliasesReady is false for scanning phase", async () => {
    mockPost.mockResolvedValue(pendingJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.aliasesReady).toBe(false);
  });

  it("result is populated for completed job", async () => {
    mockPost.mockResolvedValue(completedJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.result).toEqual({
      sent_count: 30,
      received_count: 70,
      thread_count: 25,
      address_count: 5,
    });
  });

  it("resumeJob fetches existing job", async () => {
    // resumeJob uses .then() chain — use real timers so promises resolve naturally
    vi.useRealTimers();
    mockGet.mockResolvedValue(runningJob);
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      result.current.resumeJob("job-1");
      // Allow the .then() chain to resolve
      await new Promise((r) => setTimeout(r, 10));
    });
    expect(mockGet).toHaveBeenCalledWith("/api/sync/job-1");
    // Restore fake timers for remaining tests
    vi.useFakeTimers();
  });

  it("cleanup stops polling on unmount", async () => {
    mockPost.mockResolvedValue(pendingJob);
    mockGet.mockResolvedValue(runningJob);
    const { result, unmount } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    unmount();
    // After unmount, further timer ticks should not cause errors
    await act(async () => {
      vi.advanceTimersByTime(10000);
    });
  });

  it("progress message for aliases_ready phase", async () => {
    mockPost.mockResolvedValue({ ...pendingJob, phase: "aliases_ready" });
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.progress?.message).toContain("Addresses discovered");
  });

  it("progress message for addresses phase", async () => {
    mockPost.mockResolvedValue({ ...pendingJob, phase: "addresses" });
    const { result } = renderHook(() => useSyncJob());
    await act(async () => {
      await result.current.startSync();
    });
    expect(result.current.progress?.message).toBe("Discovering addresses...");
  });
});
