import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ThreadToolbar } from "../thread-toolbar";
import type { Thread } from "@/lib/types";

// Mock lucide-react icons
vi.mock("lucide-react", () => {
  const icon =
    (name: string) =>
    ({ className }: { className?: string }) => (
      <span data-testid={`icon-${name}`} className={className} />
    );
  return {
    Archive: icon("archive"),
    Trash2: icon("trash"),
    Mail: icon("mail"),
    MailOpen: icon("mail-open"),
    AlertTriangle: icon("alert"),
    Inbox: icon("inbox"),
    RefreshCw: icon("refresh"),
    ChevronLeft: icon("chevron-left"),
    ChevronRight: icon("chevron-right"),
    ChevronDown: icon("chevron-down"),
    Minus: icon("minus"),
    Tag: icon("tag"),
    BellOff: icon("bell-off"),
  };
});

// Mock sonner
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

// Mock api
vi.mock("@/lib/api", () => ({
  api: { get: vi.fn().mockResolvedValue([]) },
}));

// Mock domain context
vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    activeDomain: { id: "d1", domain: "test.com" },
    domains: [{ id: "d1", domain: "test.com" }],
  }),
}));

const mkThread = (
  id: string,
  unread = 0,
  labels: string[] = ["inbox"]
): Thread => ({
  id,
  org_id: "org1",
  user_id: "u1",
  domain_id: "d1",
  subject: `Thread ${id}`,
  participant_emails: [],
  last_message_at: "2026-01-01T00:00:00Z",
  message_count: 1,
  unread_count: unread,
  labels,
  snippet: "",
  last_sender: "",
  original_to: "a@b.com",
  created_at: "2026-01-01T00:00:00Z",
});

const baseProps = {
  label: "inbox" as const,
  threads: [mkThread("t1"), mkThread("t2", 1)],
  selectedIds: new Set<string>(),
  allSelected: false,
  someSelected: false,
  hasSelection: false,
  selectAllPages: false,
  onToggleSelectAll: vi.fn(),
  onSelectIds: vi.fn(),
  onToggleSelectAllPages: vi.fn(),
  onBulkAction: vi.fn(),
  onRefresh: vi.fn(),
  page: 1,
  total: 2,
  limit: 50,
  onPageChange: vi.fn(),
};

describe("ThreadToolbar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders checkbox", () => {
    render(<ThreadToolbar {...baseProps} />);
    const checkbox = screen.getByRole("checkbox");
    expect(checkbox).toBeInTheDocument();
  });

  it("checkbox triggers onToggleSelectAll", () => {
    render(<ThreadToolbar {...baseProps} />);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(baseProps.onToggleSelectAll).toHaveBeenCalled();
  });

  it("shows refresh button when no selection", () => {
    render(<ThreadToolbar {...baseProps} />);
    expect(screen.getByTitle("Refresh")).toBeInTheDocument();
  });

  it("refresh button calls onRefresh", () => {
    render(<ThreadToolbar {...baseProps} />);
    fireEvent.click(screen.getByTitle("Refresh"));
    expect(baseProps.onRefresh).toHaveBeenCalled();
  });

  it("shows archive button for inbox view with selection", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Archive")).toBeInTheDocument();
  });

  it("shows move to inbox for archive view with selection", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        label="archive"
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Move to Inbox")).toBeInTheDocument();
  });

  it("shows trash button for inbox view with selection", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Trash")).toBeInTheDocument();
  });

  it("shows delete permanently for trash view with selection", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        label="trash"
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Delete permanently")).toBeInTheDocument();
  });

  it("shows pagination range", () => {
    render(<ThreadToolbar {...baseProps} />);
    expect(screen.getByText(/1–2 of 2/)).toBeInTheDocument();
  });

  it("prev button disabled on page 1", () => {
    render(<ThreadToolbar {...baseProps} />);
    const buttons = screen.getAllByRole("button");
    const prevBtn = buttons.find(
      (b) => b.querySelector('[data-testid="icon-chevron-left"]') !== null
    );
    expect(prevBtn).toHaveAttribute("disabled");
  });

  it("next button calls onPageChange", () => {
    render(
      <ThreadToolbar {...baseProps} page={1} total={100} limit={50} />
    );
    const buttons = screen.getAllByRole("button");
    const nextBtn = buttons.find(
      (b) => b.querySelector('[data-testid="icon-chevron-right"]') !== null
    );
    if (nextBtn) {
      fireEvent.click(nextBtn);
      expect(baseProps.onPageChange).toHaveBeenCalledWith(2);
    }
  });

  it("shows select-all banner when allSelected and more pages", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        allSelected={true}
        total={100}
        limit={50}
      />
    );
    expect(
      screen.getByText(/All 2 conversations on this page are selected/)
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Select all 100 conversations/)
    ).toBeInTheDocument();
  });

  it("shows selected-all-pages banner", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        allSelected={true}
        selectAllPages={true}
        total={100}
        limit={50}
      />
    );
    expect(
      screen.getByText(/All 100 conversations are selected/)
    ).toBeInTheDocument();
  });

  it("bulk actions disabled when isPending", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
        isPending={true}
      />
    );
    expect(screen.getByTitle("Archive")).toBeDisabled();
    expect(screen.getByTitle("Trash")).toBeDisabled();
  });

  it("dropdown opens on chevron click", () => {
    render(<ThreadToolbar {...baseProps} />);
    const chevronBtn = screen
      .getAllByRole("button")
      .find(
        (b) => b.querySelector('[data-testid="icon-chevron-down"]') !== null
      );
    if (chevronBtn) {
      fireEvent.click(chevronBtn);
      expect(screen.getByText("All")).toBeInTheDocument();
      expect(screen.getByText("None")).toBeInTheDocument();
      expect(screen.getByText("Read")).toBeInTheDocument();
      expect(screen.getByText("Unread")).toBeInTheDocument();
      expect(screen.getByText("Starred")).toBeInTheDocument();
      expect(screen.getByText("Unstarred")).toBeInTheDocument();
    }
  });

  it("mark as read visible when selection has unread", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        threads={[mkThread("t1", 1)]}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Mark as read")).toBeInTheDocument();
  });

  it("mark as unread visible when selection has read", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        threads={[mkThread("t1", 0)]}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Mark as unread")).toBeInTheDocument();
  });

  it("mute toggle is visible with selection", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    // Mute button should exist (either "Mute" or "Unmute")
    const muteBtn = screen.getByTitle(/Mute|Unmute/);
    expect(muteBtn).toBeInTheDocument();
  });

  it("hides refresh button when selection active", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.queryByTitle("Refresh")).not.toBeInTheDocument();
  });

  it("spam button visible in inbox view", () => {
    render(
      <ThreadToolbar
        {...baseProps}
        selectedIds={new Set(["t1"])}
        hasSelection={true}
      />
    );
    expect(screen.getByTitle("Report spam")).toBeInTheDocument();
  });
});
