import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";

const mockPush = vi.fn();
const mockOnCompose = vi.fn();
const mockOnOpenSettings = vi.fn();
const mockOnCloseSidebar = vi.fn();

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => "/d/d1/inbox",
}));

// Mock next-themes
vi.mock("next-themes", () => ({
  useTheme: () => ({ theme: "light", setTheme: vi.fn() }),
}));

// Mock @dnd-kit/core
vi.mock("@dnd-kit/core", () => ({
  useDroppable: () => ({ isOver: false, setNodeRef: vi.fn() }),
}));

// Mock domain context
vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    domains: [
      { id: "d1", domain: "example.com" },
      { id: "d2", domain: "other.com" },
    ],
    activeDomain: { id: "d1", domain: "example.com" },
    setActiveDomainId: vi.fn(),
    unreadCounts: { d1: 3 },
  }),
}));

// Mock notification context
vi.mock("@/contexts/notification-context", () => ({
  useNotifications: () => ({ connected: true }),
}));

// Mock DomainIcon
vi.mock("@/components/domain-icon", () => ({
  DomainIcon: ({
    domain,
    active,
    onClick,
  }: {
    domain: string;
    active: boolean;
    hasUnread: boolean;
    onClick: () => void;
  }) => (
    <button data-testid={`domain-icon-${domain}`} data-active={active} onClick={onClick}>
      {domain}
    </button>
  ),
}));

// Mock api
vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn().mockResolvedValue([]),
    post: vi.fn().mockResolvedValue({}),
  },
}));

// Mock sonner
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

// Mock lucide-react icons
vi.mock("lucide-react", () => {
  const icon = (name: string) => ({ className }: { className?: string }) => (
    <span data-testid={`icon-${name}`} className={className} />
  );
  return {
    Inbox: icon("inbox"),
    Send: icon("send"),
    FileText: icon("file-text"),
    Archive: icon("archive"),
    Star: icon("star"),
    Trash2: icon("trash"),
    AlertTriangle: icon("alert"),
    PenSquare: icon("pen"),
    Settings: icon("settings"),
    Plus: icon("plus"),
    X: icon("x"),
    Sun: icon("sun"),
    Moon: icon("moon"),
    LogOut: icon("logout"),
    Tag: icon("tag"),
    WifiOff: icon("wifi-off"),
    Keyboard: icon("keyboard"),
  };
});

import { DomainSidebar } from "../domain-sidebar";

describe("DomainSidebar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders all system folders", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    for (const label of ["Inbox", "Sent", "Drafts", "Archive", "Starred", "Spam", "Trash"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });

  it("renders compose button", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    const composeButtons = screen.getAllByText("Compose");
    expect(composeButtons.length).toBeGreaterThan(0);
  });

  it("compose button calls onCompose", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    const composeButtons = screen.getAllByText("Compose");
    fireEvent.click(composeButtons[0]);
    expect(mockOnCompose).toHaveBeenCalled();
  });

  it("active folder is highlighted (inbox is active)", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    // All Inbox buttons should exist (mobile + desktop)
    const inboxButtons = screen.getAllByText("Inbox");
    // At least one should have the active styling class
    const hasActive = inboxButtons.some(
      (btn) =>
        btn.closest("button")?.className.includes("bg-accent") ||
        btn.closest("button")?.className.includes("font-medium")
    );
    expect(hasActive).toBe(true);
  });

  it("folder navigation calls router.push", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    const sentButtons = screen.getAllByText("Sent");
    fireEvent.click(sentButtons[0]);
    expect(mockPush).toHaveBeenCalledWith("/d/d1/sent");
  });

  it("renders settings button", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    const settingsButtons = screen.getAllByText("Settings");
    expect(settingsButtons.length).toBeGreaterThan(0);
  });

  it("renders domain icons", () => {
    render(
      <DomainSidebar
        onCompose={mockOnCompose}
        onOpenSettings={mockOnOpenSettings}
      />
    );
    expect(screen.getAllByTestId("domain-icon-example.com").length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("domain-icon-other.com").length).toBeGreaterThan(0);
  });
});
