import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { WSMessage, Thread, ThreadListResponse, UnreadCounts } from "@/lib/types";

// ── Mocks ──

// Capture the subscriber callback so tests can fire events directly
let subscribedHandler: ((msg: WSMessage) => void) | null = null;
const mockSetLastEventId = vi.fn();

vi.mock("@/contexts/notification-context", () => ({
  useNotifications: () => ({
    subscribe: (_event: string, handler: (msg: WSMessage) => void) => {
      subscribedHandler = handler;
      return () => {
        subscribedHandler = null;
      };
    },
    setLastEventId: mockSetLastEventId,
  }),
}));

// Track QueryClient method calls
const setQueriesDataCalls: Array<{ queryKey: unknown; updater: unknown }> = [];
const setQueryDataCalls: Array<{ queryKey: unknown; updater: unknown }> = [];
const invalidateQueriesCalls: Array<{ queryKey?: unknown; predicate?: unknown }> = [];
const queryCache: Array<{ queryKey: unknown[]; state: { data: unknown } }> = [];

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({
    setQueriesData: (filter: { queryKey: unknown }, updater: unknown) => {
      setQueriesDataCalls.push({ queryKey: filter.queryKey, updater });
      // Apply updater to matching cache entries
      for (const entry of queryCache) {
        const key = entry.queryKey;
        const filterKey = filter.queryKey as unknown[];
        if (filterKey.every((k, i) => key[i] === k)) {
          const fn = updater as (old: unknown) => unknown;
          const newData = fn(entry.state.data);
          if (newData !== undefined) entry.state.data = newData;
        }
      }
    },
    setQueryData: (queryKey: unknown, updater: unknown) => {
      setQueryDataCalls.push({ queryKey, updater });
      // Apply updater to matching cache entries
      for (const entry of queryCache) {
        const key = entry.queryKey;
        const filterKey = queryKey as unknown[];
        if (
          filterKey.length === key.length &&
          filterKey.every((k, i) => key[i] === k)
        ) {
          const fn = updater as (old: unknown) => unknown;
          const newData = fn(entry.state.data);
          if (newData !== undefined) entry.state.data = newData;
        }
      }
    },
    invalidateQueries: (opts: { queryKey?: unknown; predicate?: unknown }) => {
      invalidateQueriesCalls.push(opts);
    },
    getQueryCache: () => ({
      findAll: ({ queryKey }: { queryKey: unknown }) => {
        const filterKey = queryKey as unknown[];
        return queryCache.filter((entry) =>
          filterKey.every((k, i) => entry.queryKey[i] === k)
        );
      },
    }),
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

// ── Helpers ──

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: "t1",
    org_id: "org1",
    user_id: "u1",
    domain_id: "d1",
    subject: "Test Subject",
    participant_emails: ["alice@test.com"],
    last_message_at: "2026-01-01T00:00:00Z",
    message_count: 1,
    unread_count: 1,
    labels: ["inbox"],
    snippet: "Hello",
    original_to: "alice@test.com",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function fireEvent(msg: WSMessage) {
  if (!subscribedHandler) throw new Error("No handler subscribed");
  subscribedHandler(msg);
}

function seedCache(
  queryKey: unknown[],
  data: ThreadListResponse | UnreadCounts | Thread
) {
  queryCache.push({ queryKey, state: { data } });
}

function getCacheData<T>(queryKey: unknown[]): T | undefined {
  const entry = queryCache.find((e) =>
    (queryKey as unknown[]).every((k, i) => e.queryKey[i] === k)
  );
  return entry?.state.data as T | undefined;
}

// ── Setup ──

beforeEach(() => {
  vi.clearAllMocks();
  subscribedHandler = null;
  mockSetLastEventId.mockClear();
  setQueriesDataCalls.length = 0;
  setQueryDataCalls.length = 0;
  invalidateQueriesCalls.length = 0;
  queryCache.length = 0;
});

// Lazy import so mocks are in place
async function mountWSSync() {
  const mod = await import("@/hooks/use-ws-sync");
  // WSSync is a component that returns null
  // We can use renderHook to trigger useEffect
  // But WSSync is a function component, not a hook. We'll call it via render.
  const { render } = await import("@testing-library/react");
  const React = await import("react");
  render(React.createElement(mod.WSSync));
}

describe("WSSync event handling", () => {
  // ── email.received ──

  it("email.received — adds thread to inbox cache and adjusts unread", async () => {
    const thread = makeThread({ id: "t1", unread_count: 1, labels: ["inbox"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [], page: 1, total: 0 } as ThreadListResponse
    );
    seedCache(["domains", "unreadCounts"], { d1: 0 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "email.received",
      thread_id: "t1",
      domain_id: "d1",
      payload: { thread },
    });

    // setQueriesData should have been called for thread lists
    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads).toHaveLength(1);
    expect(listData?.threads[0].id).toBe("t1");
  });

  it("email.received — no payload invalidates thread lists", async () => {
    await mountWSSync();

    fireEvent({
      event: "email.received",
      domain_id: "d1",
    });

    // Should fall back to invalidation
    expect(invalidateQueriesCalls.length).toBeGreaterThan(0);
  });

  // ── email.sent ──

  it("email.sent — adds thread to sent cache", async () => {
    const thread = makeThread({ id: "t2", labels: ["sent"] });
    seedCache(
      ["threads", "list", "d1", "sent", 1],
      { threads: [], page: 1, total: 0 } as ThreadListResponse
    );

    await mountWSSync();

    fireEvent({
      event: "email.sent",
      thread_id: "t2",
      domain_id: "d1",
      payload: { thread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "sent", 1]);
    expect(listData?.threads).toHaveLength(1);
    expect(listData?.threads[0].id).toBe("t2");
  });

  // ── thread.starred / thread.unstarred ──

  it("thread.starred — adds starred label to cached thread", async () => {
    const thread = makeThread({ id: "t1", labels: ["inbox", "starred"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox"] })], page: 1, total: 1 }
    );

    await mountWSSync();

    fireEvent({
      event: "thread.starred",
      thread_id: "t1",
      payload: { thread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].labels).toContain("starred");
  });

  it("thread.unstarred — removes starred label from cached thread", async () => {
    const thread = makeThread({ id: "t1", labels: ["inbox"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox", "starred"] })], page: 1, total: 1 }
    );

    await mountWSSync();

    fireEvent({
      event: "thread.unstarred",
      thread_id: "t1",
      payload: { thread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].labels).not.toContain("starred");
  });

  // ── thread.muted / thread.unmuted ──

  it("thread.muted — adds muted label", async () => {
    const thread = makeThread({ id: "t1", labels: ["inbox", "muted"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox"] })], page: 1, total: 1 }
    );

    await mountWSSync();

    fireEvent({
      event: "thread.muted",
      thread_id: "t1",
      payload: { thread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].labels).toContain("muted");
  });

  it("thread.unmuted — removes muted label", async () => {
    const thread = makeThread({ id: "t1", labels: ["inbox"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox", "muted"] })], page: 1, total: 1 }
    );

    await mountWSSync();

    fireEvent({
      event: "thread.unmuted",
      thread_id: "t1",
      payload: { thread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].labels).not.toContain("muted");
  });

  // ── thread.read / thread.unread ──

  it("thread.read — sets unread_count to 0 and decrements domain unread", async () => {
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", unread_count: 3 })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 5 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.read",
      thread_id: "t1",
      domain_id: "d1",
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].unread_count).toBe(0);

    // Unread counts should have been adjusted
    const unread = getCacheData<UnreadCounts>(["domains", "unreadCounts"]);
    expect(unread?.d1).toBe(2); // 5 - 3 = 2
  });

  it("thread.unread — sets unread_count to 1 and increments domain unread", async () => {
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", unread_count: 0 })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 2 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.unread",
      thread_id: "t1",
      domain_id: "d1",
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads[0].unread_count).toBe(1);

    const unread = getCacheData<UnreadCounts>(["domains", "unreadCounts"]);
    expect(unread?.d1).toBe(3); // 2 + 1 = 3
  });

  // ── thread.archived ──

  it("thread.archived — removes inbox label, removes from inbox cache", async () => {
    const archivedThread = makeThread({ id: "t1", labels: [] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox"] })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 1 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.archived",
      thread_id: "t1",
      domain_id: "d1",
      payload: { thread: archivedThread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    // Thread should be removed from inbox since it no longer has the inbox label
    expect(listData?.threads).toHaveLength(0);
    expect(listData?.total).toBe(0);
  });

  // ── thread.trashed ──

  it("thread.trashed — adds trash label, removes from inbox cache", async () => {
    const trashedThread = makeThread({ id: "t1", labels: ["trash"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox"] })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 1 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.trashed",
      thread_id: "t1",
      domain_id: "d1",
      payload: { thread: trashedThread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads).toHaveLength(0);
  });

  // ── thread.deleted ──

  it("thread.deleted — removes thread from all caches", async () => {
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1" })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 1 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.deleted",
      thread_id: "t1",
      domain_id: "d1",
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    expect(listData?.threads).toHaveLength(0);
    expect(listData?.total).toBe(0);
  });

  // ── thread.bulk_action ──

  it("thread.bulk_action (archive) — removes inbox label from multiple threads", async () => {
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      {
        threads: [
          makeThread({ id: "t1", labels: ["inbox"] }),
          makeThread({ id: "t2", labels: ["inbox"] }),
          makeThread({ id: "t3", labels: ["inbox"] }),
        ],
        page: 1,
        total: 3,
      }
    );
    seedCache(["domains", "unreadCounts"], { d1: 3 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.bulk_action",
      payload: {
        action: "archive",
        thread_ids: ["t1", "t2"],
      },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    // t1 and t2 should be removed from inbox (archive removes inbox label)
    expect(listData?.threads).toHaveLength(1);
    expect(listData?.threads[0].id).toBe("t3");
  });

  it("thread.bulk_action (trash) — adds trash label to multiple threads", async () => {
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      {
        threads: [
          makeThread({ id: "t1", labels: ["inbox"] }),
          makeThread({ id: "t2", labels: ["inbox"] }),
        ],
        page: 1,
        total: 2,
      }
    );
    seedCache(["domains", "unreadCounts"], { d1: 2 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.bulk_action",
      payload: {
        action: "trash",
        thread_ids: ["t1", "t2"],
      },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    // Both should be removed from inbox (trash makes them not belong in inbox view)
    expect(listData?.threads).toHaveLength(0);
  });

  // ── email.status_updated ──

  it("email.status_updated — updates email delivery status in thread detail cache", async () => {
    await mountWSSync();

    fireEvent({
      event: "email.status_updated",
      thread_id: "t1",
      payload: {
        email_id: "e1",
        status: "delivered",
      },
    });

    // Should call setQueryData for the thread detail
    expect(setQueryDataCalls.length).toBeGreaterThan(0);
    const call = setQueryDataCalls.find(
      (c) => JSON.stringify(c.queryKey) === JSON.stringify(["threads", "detail", "t1"])
    );
    expect(call).toBeTruthy();
  });

  // ── sync.completed ──

  it("sync.completed — invalidates all thread and domain queries", async () => {
    await mountWSSync();

    fireEvent({ event: "sync.completed" });

    // Should invalidate thread lists and domain unread counts
    const threadListInvalidate = invalidateQueriesCalls.find(
      (c) => JSON.stringify(c.queryKey) === JSON.stringify(["threads", "list"])
    );
    const unreadInvalidate = invalidateQueriesCalls.find(
      (c) => JSON.stringify(c.queryKey) === JSON.stringify(["domains", "unreadCounts"])
    );
    expect(threadListInvalidate).toBeTruthy();
    expect(unreadInvalidate).toBeTruthy();
  });

  // ── plan.changed ──

  it("plan.changed — shows toast notification", async () => {
    const { toast } = await import("sonner");
    await mountWSSync();

    fireEvent({
      event: "plan.changed",
      payload: { plan: "cancelled" },
    });

    expect(toast.warning).toHaveBeenCalledWith(
      expect.stringContaining("cancelled")
    );
  });

  // ── domain.disconnected ──

  it("domain.disconnected — shows toast notification", async () => {
    const { toast } = await import("sonner");
    await mountWSSync();

    fireEvent({
      event: "domain.disconnected",
      payload: { domain: "test.com" },
    });

    expect(toast.error).toHaveBeenCalledWith(
      expect.stringContaining("test.com")
    );

    // Should also invalidate domains list
    const domainInvalidate = invalidateQueriesCalls.find(
      (c) => JSON.stringify(c.queryKey) === JSON.stringify(["domains", "list"])
    );
    expect(domainInvalidate).toBeTruthy();
  });

  // ── Reconnect catchup — setLastEventId tracking ──

  it("tracks event IDs via setLastEventId for reconnect catchup", async () => {
    await mountWSSync();

    fireEvent({
      id: 42,
      event: "sync.completed",
    });

    expect(mockSetLastEventId).toHaveBeenCalledWith(42);
  });

  it("does not call setLastEventId when message has no id", async () => {
    await mountWSSync();

    fireEvent({
      event: "sync.completed",
    });

    expect(mockSetLastEventId).not.toHaveBeenCalled();
  });

  // ── domain.reconnected ──

  it("domain.reconnected — shows success toast and invalidates domains list", async () => {
    const { toast } = await import("sonner");
    await mountWSSync();

    fireEvent({
      event: "domain.reconnected",
      payload: { domain: "example.com" },
    });

    expect(toast.success).toHaveBeenCalledWith(
      expect.stringContaining("example.com")
    );

    const domainInvalidate = invalidateQueriesCalls.find(
      (c) => JSON.stringify(c.queryKey) === JSON.stringify(["domains", "list"])
    );
    expect(domainInvalidate).toBeTruthy();
  });

  // ── domain.dns_degraded ──

  it("domain.dns_degraded — shows warning toast with degraded records", async () => {
    const { toast } = await import("sonner");
    await mountWSSync();

    fireEvent({
      event: "domain.dns_degraded",
      payload: { domain: "test.com", degraded: ["SPF", "DKIM"] },
    });

    expect(toast.warning).toHaveBeenCalledWith(
      expect.stringContaining("test.com")
    );
    expect(toast.warning).toHaveBeenCalledWith(
      expect.stringContaining("SPF")
    );
  });

  // ── thread.moved ──

  it("thread.moved — moves thread between views correctly", async () => {
    const movedThread = makeThread({ id: "t1", labels: ["trash"] });
    seedCache(
      ["threads", "list", "d1", "inbox", 1],
      { threads: [makeThread({ id: "t1", labels: ["inbox"] })], page: 1, total: 1 }
    );
    seedCache(["domains", "unreadCounts"], { d1: 1 } as UnreadCounts);

    await mountWSSync();

    fireEvent({
      event: "thread.moved",
      thread_id: "t1",
      domain_id: "d1",
      payload: { thread: movedThread },
    });

    expect(setQueriesDataCalls.length).toBeGreaterThan(0);
    const listData = getCacheData<ThreadListResponse>(["threads", "list", "d1", "inbox", 1]);
    // Thread moved to trash, so it should be removed from inbox
    expect(listData?.threads).toHaveLength(0);
    expect(listData?.total).toBe(0);
  });

  // ── domain.disconnected with api_key_revoked reason ──

  it("domain.disconnected with api_key_revoked — shows specific error toast", async () => {
    const { toast } = await import("sonner");
    await mountWSSync();

    fireEvent({
      event: "domain.disconnected",
      payload: { domain: "test.com", reason: "api_key_revoked" },
    });

    expect(toast.error).toHaveBeenCalledWith(
      expect.stringContaining("API key revoked")
    );
  });
});
