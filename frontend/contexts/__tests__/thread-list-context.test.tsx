import React from "react";
import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { ThreadListProvider, useThreadList } from "../thread-list-context";
import type { Thread } from "@/lib/types";

const mockThread: Thread = {
  id: "t1",
  org_id: "org1",
  user_id: "u1",
  domain_id: "d1",
  subject: "Test Thread",
  participant_emails: ["a@b.com"],
  last_message_at: "2026-01-01T00:00:00Z",
  message_count: 1,
  unread_count: 0,
  labels: ["inbox"],
  snippet: "Hello",
  original_to: "a@b.com",
  created_at: "2026-01-01T00:00:00Z",
};

const mockValue = {
  threads: [mockThread],
  selectedIds: new Set(["t1"]),
  toggleSelect: vi.fn(),
  handleBulkAction: vi.fn(),
  handleRefresh: vi.fn(),
  focusedIndex: 0,
  setFocusedIndex: vi.fn(),
  handleStar: vi.fn(),
  handleAction: vi.fn(),
  label: "inbox" as const,
  domainId: "d1",
};

describe("useThreadList", () => {
  it("returns null outside provider", () => {
    const { result } = renderHook(() => useThreadList());
    expect(result.current).toBeNull();
  });

  it("returns value when inside provider", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <ThreadListProvider value={mockValue}>{children}</ThreadListProvider>
    );
    const { result } = renderHook(() => useThreadList(), { wrapper });
    expect(result.current).not.toBeNull();
  });

  it("threads are accessible", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <ThreadListProvider value={mockValue}>{children}</ThreadListProvider>
    );
    const { result } = renderHook(() => useThreadList(), { wrapper });
    expect(result.current?.threads).toHaveLength(1);
    expect(result.current?.threads[0].subject).toBe("Test Thread");
  });

  it("selectedIds are accessible", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <ThreadListProvider value={mockValue}>{children}</ThreadListProvider>
    );
    const { result } = renderHook(() => useThreadList(), { wrapper });
    expect(result.current?.selectedIds.has("t1")).toBe(true);
  });
});
