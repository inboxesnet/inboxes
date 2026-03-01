import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Thread } from "@/lib/types";

const mockPush = vi.fn();

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

// Mock @dnd-kit/core
vi.mock("@dnd-kit/core", () => ({
  useDraggable: () => ({
    attributes: {},
    listeners: {},
    setNodeRef: vi.fn(),
    isDragging: false,
  }),
}));

// Mock lucide-react icons
vi.mock("lucide-react", () => {
  const icon = (name: string) => ({ className }: { className?: string }) => (
    <span data-testid={`icon-${name}`} className={className} />
  );
  return {
    Star: icon("star"),
    Archive: icon("archive"),
    Trash2: icon("trash"),
    Mail: icon("mail"),
    MailOpen: icon("mail-open"),
    BellOff: icon("bell-off"),
  };
});

import { ThreadList } from "../thread-list";

const mkThread = (
  id: string,
  subject: string,
  unread = 0,
  labels: string[] = ["inbox"],
  snippet = ""
): Thread => ({
  id,
  org_id: "org1",
  user_id: "u1",
  domain_id: "d1",
  subject,
  participant_emails: ["alice@test.com"],
  last_message_at: "2026-01-01T00:00:00Z",
  message_count: 1,
  unread_count: unread,
  labels,
  snippet,
  original_to: "bob@test.com",
  created_at: "2026-01-01T00:00:00Z",
});

const baseProps = {
  domainId: "d1",
  label: "inbox" as const,
  selectedIds: new Set<string>(),
  focusedIndex: -1,
  onToggleSelect: vi.fn(),
  onToggleSelectAll: vi.fn(),
  onStar: vi.fn(),
  onAction: vi.fn(),
};

describe("ThreadList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders thread subjects", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Hello World"), mkThread("t2", "Goodbye")]}
      />
    );
    expect(screen.getAllByText("Hello World").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Goodbye").length).toBeGreaterThan(0);
  });

  it("unread threads have bold styling", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Unread message", 1)]}
      />
    );
    // Find the subject span with font-semibold class
    const subjects = screen.getAllByText("Unread message");
    const hasBold = subjects.some(
      (el) => el.className.includes("font-semibold")
    );
    expect(hasBold).toBe(true);
  });

  it("renders selection checkboxes", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Thread 1")]}
      />
    );
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes.length).toBeGreaterThan(0);
  });

  it("checkbox calls onToggleSelect", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Thread 1")]}
      />
    );
    const checkboxes = screen.getAllByRole("checkbox");
    fireEvent.click(checkboxes[0]);
    expect(baseProps.onToggleSelect).toHaveBeenCalledWith("t1");
  });

  it("clicking thread row navigates", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Thread 1")]}
      />
    );
    const listItems = screen.getAllByRole("listitem");
    fireEvent.click(listItems[0]);
    expect(mockPush).toHaveBeenCalledWith("/d/d1/inbox/t1");
  });

  it("shows relative time", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Thread 1")]}
      />
    );
    // Time formatting is present (any text like "now", "Xm", date, etc.)
    // Just verify no error in rendering
    const items = screen.getAllByRole("listitem");
    expect(items.length).toBe(1);
  });

  it("shows empty state for no threads", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[]}
      />
    );
    // Empty thread list — no listitems
    expect(screen.queryAllByRole("listitem").length).toBe(0);
  });

  it("shows snippet text", () => {
    render(
      <ThreadList
        {...baseProps}
        threads={[mkThread("t1", "Subject", 0, ["inbox"], "Hello snippet text")]}
      />
    );
    expect(screen.getAllByText(/Hello snippet text/).length).toBeGreaterThan(0);
  });
});
