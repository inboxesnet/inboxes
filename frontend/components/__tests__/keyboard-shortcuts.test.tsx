import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent } from "@testing-library/react";
import type { Thread } from "@/lib/types";

const mockThread = (id: string, labels: string[] = ["inbox"]): Thread => ({
  id,
  org_id: "org1",
  user_id: "u1",
  domain_id: "d1",
  subject: `Thread ${id}`,
  participant_emails: [],
  last_message_at: "2026-01-01T00:00:00Z",
  message_count: 1,
  unread_count: 0,
  labels,
  snippet: "test",
  original_to: "a@b.com",
  created_at: "2026-01-01T00:00:00Z",
});

const threads = [mockThread("t1"), mockThread("t2"), mockThread("t3")];

const mockToggleSelect = vi.fn();
const mockHandleBulkAction = vi.fn();
const mockHandleRefresh = vi.fn();
const mockSetFocusedIndex = vi.fn();
const mockHandleStar = vi.fn();
const mockHandleAction = vi.fn();
const mockOnCompose = vi.fn();
const mockRouterPush = vi.fn();
let mockFocusedIndex = 0;

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockRouterPush }),
  usePathname: () => "/d/d1/inbox",
}));

// Mock domain context
vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    domains: [{ id: "d1", domain: "test.com" }],
    activeDomain: { id: "d1", domain: "test.com" },
  }),
}));

// Mock thread-list context
vi.mock("@/contexts/thread-list-context", () => ({
  useThreadList: () => ({
    threads,
    selectedIds: new Set<string>(),
    toggleSelect: mockToggleSelect,
    handleBulkAction: mockHandleBulkAction,
    handleRefresh: mockHandleRefresh,
    focusedIndex: mockFocusedIndex,
    setFocusedIndex: mockSetFocusedIndex,
    handleStar: mockHandleStar,
    handleAction: mockHandleAction,
    label: "inbox" as const,
    domainId: "d1",
  }),
}));

// Mock keyboard shortcuts dialog
vi.mock("@/components/keyboard-shortcuts-dialog", () => ({
  KeyboardShortcutsDialog: ({
    open,
    onOpenChange,
  }: {
    open: boolean;
    onOpenChange: (v: boolean) => void;
  }) =>
    open ? (
      <div data-testid="shortcuts-dialog">
        <button onClick={() => onOpenChange(false)}>Close</button>
      </div>
    ) : null,
}));

import { KeyboardShortcuts } from "../keyboard-shortcuts";

describe("KeyboardShortcuts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFocusedIndex = 0;
  });
  afterEach(() => {
    cleanup();
  });

  it("j moves focus down", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "j" });
    expect(mockSetFocusedIndex).toHaveBeenCalledWith(1);
  });

  it("k moves focus up", () => {
    mockFocusedIndex = 1;
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "k" });
    expect(mockSetFocusedIndex).toHaveBeenCalledWith(0);
  });

  it("j clamps to last thread", () => {
    mockFocusedIndex = 2; // last thread index
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "j" });
    expect(mockSetFocusedIndex).toHaveBeenCalledWith(2);
  });

  it("k clamps to first thread", () => {
    mockFocusedIndex = 0;
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "k" });
    expect(mockSetFocusedIndex).toHaveBeenCalledWith(0);
  });

  it("x toggles selection", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "x" });
    expect(mockToggleSelect).toHaveBeenCalledWith("t1");
  });

  it("e archives focused", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "e" });
    expect(mockHandleAction).toHaveBeenCalledWith("t1", "archive");
  });

  it("# trashes focused", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "#" });
    expect(mockHandleAction).toHaveBeenCalledWith("t1", "trash");
  });

  it("s stars focused", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "s" });
    expect(mockHandleStar).toHaveBeenCalledWith("t1");
  });

  it("r refreshes", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "r" });
    expect(mockHandleRefresh).toHaveBeenCalled();
  });

  it("? opens shortcuts dialog", () => {
    const { queryByTestId } = render(
      <KeyboardShortcuts onCompose={mockOnCompose} />
    );
    expect(queryByTestId("shortcuts-dialog")).not.toBeInTheDocument();
    fireEvent.keyDown(document, { key: "?" });
    expect(queryByTestId("shortcuts-dialog")).toBeInTheDocument();
  });

  it("ignores keys in input elements", () => {
    const { container } = render(
      <div>
        <input data-testid="input" />
        <KeyboardShortcuts onCompose={mockOnCompose} />
      </div>
    );
    const input = container.querySelector("input")!;
    fireEvent.keyDown(input, { key: "j" });
    expect(mockSetFocusedIndex).not.toHaveBeenCalled();
  });

  it("ignores keys in textarea elements", () => {
    const { container } = render(
      <div>
        <textarea data-testid="textarea" />
        <KeyboardShortcuts onCompose={mockOnCompose} />
      </div>
    );
    const textarea = container.querySelector("textarea")!;
    fireEvent.keyDown(textarea, { key: "j" });
    expect(mockSetFocusedIndex).not.toHaveBeenCalled();
  });

  it("ignores keys in contentEditable elements", () => {
    const { container } = render(
      <div>
        <div data-testid="editable" contentEditable />
        <KeyboardShortcuts onCompose={mockOnCompose} />
      </div>
    );
    const editable = container.querySelector("[contenteditable]")!;
    // jsdom may not fully support isContentEditable, so define it
    Object.defineProperty(editable, "isContentEditable", { value: true });
    fireEvent.keyDown(editable, { key: "j" });
    expect(mockSetFocusedIndex).not.toHaveBeenCalled();
  });

  it("Cmd+N triggers compose", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    expect(mockOnCompose).toHaveBeenCalled();
  });

  it("Shift+I marks read", () => {
    render(<KeyboardShortcuts onCompose={mockOnCompose} />);
    fireEvent.keyDown(document, { key: "I", shiftKey: true });
    expect(mockHandleAction).toHaveBeenCalledWith("t1", "read");
  });
});
