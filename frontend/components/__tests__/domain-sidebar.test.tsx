import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup, act } from "@testing-library/react";

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
  DndContext: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DragOverlay: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  closestCenter: vi.fn(),
  PointerSensor: vi.fn(),
  TouchSensor: vi.fn(),
  useSensor: vi.fn(),
  useSensors: vi.fn(() => []),
}));

// Mock @dnd-kit/sortable
vi.mock("@dnd-kit/sortable", () => ({
  SortableContext: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  verticalListSortingStrategy: {},
  horizontalListSortingStrategy: {},
  useSortable: () => ({
    attributes: {},
    listeners: {},
    setNodeRef: vi.fn(),
    transform: null,
    transition: null,
    isDragging: false,
  }),
  arrayMove: vi.fn((arr: unknown[], from: number, to: number) => {
    const result = [...arr];
    const [item] = result.splice(from, 1);
    result.splice(to, 0, item);
    return result;
  }),
}));

// Mock @dnd-kit/utilities
vi.mock("@dnd-kit/utilities", () => ({
  CSS: { Transform: { toString: () => undefined } },
}));

// Mock @tanstack/react-query
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ setQueryData: vi.fn(), invalidateQueries: vi.fn() }),
}));

// Mock query-keys
vi.mock("@/lib/query-keys", () => ({
  queryKeys: { domains: { list: () => ["domains", "list"] } },
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
let mockConnected = true;
vi.mock("@/contexts/notification-context", () => ({
  useNotifications: () => ({ connected: mockConnected }),
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
    Info: icon("info"),
  };
});

import { DomainSidebar } from "../domain-sidebar";

describe("DomainSidebar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConnected = true;
  });
  afterEach(() => {
    cleanup();
  });

  it("renders all system folders", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    for (const label of ["Inbox", "Sent", "Drafts", "Archive", "Starred", "Spam", "Trash"]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });

  it("renders compose button", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    const composeButtons = screen.getAllByText("Compose");
    expect(composeButtons.length).toBeGreaterThan(0);
  });

  it("compose button calls onCompose", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    const composeButtons = screen.getAllByText("Compose");
    fireEvent.click(composeButtons[0]);
    expect(mockOnCompose).toHaveBeenCalled();
  });

  it("active folder is highlighted (inbox is active)", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
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

  it("folder navigation calls router.push", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    const sentButtons = screen.getAllByText("Sent");
    fireEvent.click(sentButtons[0]);
    expect(mockPush).toHaveBeenCalledWith("/d/d1/sent");
  });

  it("renders settings button", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    const settingsButtons = screen.getAllByText("Settings");
    expect(settingsButtons.length).toBeGreaterThan(0);
  });

  it("renders domain icons", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    expect(screen.getAllByTestId("domain-icon-example.com").length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("domain-icon-other.com").length).toBeGreaterThan(0);
  });

  it("shows unread count badge on inbox folder", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    // The mock provides unreadCounts: { d1: 3 } and activeDomain is d1
    // The inbox folder should display the count badge with "3"
    const badges = screen.getAllByText("3");
    expect(badges.length).toBeGreaterThan(0);
  });

  it("active domain icon has data-active true", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    // d1 (example.com) is the active domain
    const activeIcons = screen.getAllByTestId("domain-icon-example.com");
    const hasActive = activeIcons.some((el) => el.getAttribute("data-active") === "true");
    expect(hasActive).toBe(true);
    // d2 (other.com) should NOT be active
    const otherIcons = screen.getAllByTestId("domain-icon-other.com");
    const otherActive = otherIcons.some((el) => el.getAttribute("data-active") === "true");
    expect(otherActive).toBe(false);
  });

  it("renders domain icons in sortable context for drag-and-drop reorder", async () => {
    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });
    // Both domain icons should render (inside SortableContext which is a passthrough)
    const d1Icons = screen.getAllByTestId("domain-icon-example.com");
    const d2Icons = screen.getAllByTestId("domain-icon-other.com");
    expect(d1Icons.length).toBeGreaterThan(0);
    expect(d2Icons.length).toBeGreaterThan(0);
    // The add-domain button should also render
    const addButtons = screen.getAllByTitle("Add domain");
    expect(addButtons.length).toBeGreaterThan(0);
  });

  it("shows offline banner after 3s of disconnection", async () => {
    mockConnected = false;
    vi.useFakeTimers();

    await act(async () => {
      render(
        <DomainSidebar
          onCompose={mockOnCompose}
          onOpenSettings={mockOnOpenSettings}
        />
      );
    });

    // Banner should NOT be visible immediately
    expect(screen.queryByText("Offline")).toBeNull();

    // Advance past the 3s delay
    await act(async () => {
      vi.advanceTimersByTime(3100);
    });

    // Now the offline banner should be visible
    expect(screen.getAllByText("Offline").length).toBeGreaterThan(0);

    vi.useRealTimers();
  });
});
