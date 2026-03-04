import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useUnreadBadge } from "../use-unread-badge";

// Mock the useDomains context
const mockUseDomains = vi.fn();
vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => mockUseDomains(),
}));

describe("useUnreadBadge", () => {
  let originalTitle: string;

  beforeEach(() => {
    originalTitle = document.title;
    // Remove any existing favicon link for a clean slate
    document.querySelectorAll('link[rel="icon"]').forEach((el) => el.remove());
  });

  afterEach(() => {
    document.title = originalTitle;
    vi.restoreAllMocks();
  });

  function setDomainsMock(
    unreadCounts: Record<string, number>,
    domains: Array<{ id: string; domain: string }> = []
  ) {
    mockUseDomains.mockReturnValue({
      unreadCounts,
      domains,
    });
  }

  it("sets document title with page title and base title when no domain", () => {
    setDomainsMock({});
    renderHook(() => useUnreadBadge("Inbox"));
    expect(document.title).toBe("Inbox - Inboxes.net");
  });

  it("includes unread count in title when domain has unreads", () => {
    setDomainsMock(
      { "d1": 5 },
      [{ id: "d1", domain: "example.com" }]
    );
    renderHook(() => useUnreadBadge("Inbox", "d1"));
    expect(document.title).toBe("Inbox (5) - example.com - Inboxes.net");
  });

  it("omits unread count when domain has zero unreads", () => {
    setDomainsMock(
      { "d1": 0 },
      [{ id: "d1", domain: "example.com" }]
    );
    renderHook(() => useUnreadBadge("Inbox", "d1"));
    expect(document.title).toBe("Inbox - example.com - Inboxes.net");
  });

  it("includes domain name in title when domainId is provided", () => {
    setDomainsMock(
      {},
      [{ id: "d1", domain: "cx.agency" }]
    );
    renderHook(() => useUnreadBadge("Sent", "d1"));
    expect(document.title).toBe("Sent - cx.agency - Inboxes.net");
  });

  it("omits domain name from title when domainId is undefined", () => {
    setDomainsMock({ "d1": 3 }, [{ id: "d1", domain: "example.com" }]);
    renderHook(() => useUnreadBadge("Inbox"));
    expect(document.title).toBe("Inbox - Inboxes.net");
  });

  it("handles unknown domainId gracefully (no domain name)", () => {
    setDomainsMock({ "d1": 3 }, [{ id: "d1", domain: "example.com" }]);
    renderHook(() => useUnreadBadge("Inbox", "nonexistent"));
    // No domain found, no unread for this ID, no domain name in title
    expect(document.title).toBe("Inbox - Inboxes.net");
  });

  it("updates title when unread count changes", () => {
    setDomainsMock(
      { "d1": 2 },
      [{ id: "d1", domain: "example.com" }]
    );
    const { rerender } = renderHook(() => useUnreadBadge("Inbox", "d1"));
    expect(document.title).toBe("Inbox (2) - example.com - Inboxes.net");

    // Simulate count change
    setDomainsMock(
      { "d1": 10 },
      [{ id: "d1", domain: "example.com" }]
    );
    rerender();
    expect(document.title).toBe("Inbox (10) - example.com - Inboxes.net");
  });

  it("uses custom page title", () => {
    setDomainsMock({});
    renderHook(() => useUnreadBadge("Drafts"));
    expect(document.title).toBe("Drafts - Inboxes.net");
  });
});
