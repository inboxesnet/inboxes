import React from "react";
import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
  afterEach,
} from "vitest";
import { render, screen, fireEvent, cleanup, act, waitFor } from "@testing-library/react";
import { NotificationListener } from "../notification-listener";

const PROMPT_DISMISSED_KEY = "notification_prompt_dismissed";

// Track subscriptions
let subscriptionCallback: ((msg: any) => void) | null = null;

// Mock notification context
vi.mock("@/contexts/notification-context", () => ({
  useNotifications: () => ({
    subscribe: vi.fn((event: string, handler: (msg: any) => void) => {
      subscriptionCallback = handler;
      return () => {
        subscriptionCallback = null;
      };
    }),
  }),
}));

// Mock lucide-react
vi.mock("lucide-react", () => ({
  Bell: () => <span data-testid="bell-icon">Bell</span>,
  X: () => <span data-testid="x-icon">X</span>,
}));

describe("NotificationListener", () => {
  let originalNotification: typeof Notification;

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    subscriptionCallback = null;
    originalNotification = globalThis.Notification;

    // Mock Notification API
    Object.defineProperty(globalThis, "Notification", {
      value: {
        permission: "default",
        requestPermission: vi.fn().mockResolvedValue("granted"),
      },
      configurable: true,
      writable: true,
    });
  });

  afterEach(() => {
    Object.defineProperty(globalThis, "Notification", {
      value: originalNotification,
      configurable: true,
      writable: true,
    });
    cleanup();
  });

  it("mounts without error", () => {
    render(<NotificationListener />);
  });

  it("shows prompt when permission is default", () => {
    render(<NotificationListener />);
    expect(screen.getByText("Enable notifications?")).toBeInTheDocument();
  });

  it("hides prompt when permission is granted", () => {
    Object.defineProperty(globalThis.Notification, "permission", {
      value: "granted",
      configurable: true,
    });
    render(<NotificationListener />);
    expect(screen.queryByText("Enable notifications?")).not.toBeInTheDocument();
  });

  it("hides prompt when dismissed in localStorage", () => {
    localStorage.setItem(PROMPT_DISMISSED_KEY, "1");
    render(<NotificationListener />);
    expect(screen.queryByText("Enable notifications?")).not.toBeInTheDocument();
  });

  it("Enable button calls requestPermission", async () => {
    render(<NotificationListener />);
    fireEvent.click(screen.getByText("Enable"));
    await waitFor(() => {
      expect(Notification.requestPermission).toHaveBeenCalled();
    });
  });

  it("Not now sets localStorage and hides prompt", () => {
    render(<NotificationListener />);
    fireEvent.click(screen.getByText("Not now"));
    expect(localStorage.getItem(PROMPT_DISMISSED_KEY)).toBe("1");
    expect(screen.queryByText("Enable notifications?")).not.toBeInTheDocument();
  });

  it("shows toast on email.received with from and subject", () => {
    render(<NotificationListener />);
    expect(subscriptionCallback).not.toBeNull();

    act(() => {
      subscriptionCallback!({
        event: "email.received",
        payload: { from: "alice@test.com", subject: "New message" },
      });
    });

    expect(screen.getByText("alice@test.com")).toBeInTheDocument();
    expect(screen.getByText("New message")).toBeInTheDocument();
  });

  it("auto-dismisses toast after 5s", async () => {
    vi.useFakeTimers();
    render(<NotificationListener />);

    act(() => {
      subscriptionCallback!({
        event: "email.received",
        payload: { from: "bob@test.com", subject: "Hello" },
      });
    });

    expect(screen.getByText("bob@test.com")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(5100);
    });

    expect(screen.queryByText("bob@test.com")).not.toBeInTheDocument();
    vi.useRealTimers();
  });

  it("skips empty payload (no toast shown)", () => {
    render(<NotificationListener />);

    act(() => {
      subscriptionCallback!({
        event: "email.received",
        payload: {},
      });
    });

    // No toast should appear for empty payload
    expect(screen.queryByText("alice@test.com")).not.toBeInTheDocument();
  });

  it("cleans up subscription on unmount", () => {
    const { unmount } = render(<NotificationListener />);
    expect(subscriptionCallback).not.toBeNull();
    unmount();
    expect(subscriptionCallback).toBeNull();
  });
});
