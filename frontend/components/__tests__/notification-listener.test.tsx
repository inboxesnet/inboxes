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

// Track subscriptions by event name
const subscriptionCallbacks: Record<string, (msg: any) => void> = {};

// Mock notification context
vi.mock("@/contexts/notification-context", () => ({
  useNotifications: () => ({
    subscribe: vi.fn((event: string, handler: (msg: any) => void) => {
      subscriptionCallbacks[event] = handler;
      return () => {
        delete subscriptionCallbacks[event];
      };
    }),
  }),
}));

// Mock lucide-react
vi.mock("lucide-react", () => ({
  Bell: () => <span data-testid="bell-icon">Bell</span>,
  X: () => <span data-testid="x-icon">X</span>,
}));

// Mock sonner — in-app notifications go through the shared toast stack
const { mockToast } = vi.hoisted(() => ({
  mockToast: Object.assign(vi.fn(), {
    info: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    success: vi.fn(),
  }),
}));
vi.mock("sonner", () => ({ toast: mockToast }));

describe("NotificationListener", () => {
  let originalNotification: typeof Notification;

  beforeEach(() => {
    vi.restoreAllMocks();
    mockToast.mockClear();
    localStorage.clear();
    Object.keys(subscriptionCallbacks).forEach(k => delete subscriptionCallbacks[k]);
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
    expect(subscriptionCallbacks["email.received"]).toBeDefined();

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        payload: { from: "alice@test.com", subject: "New message" },
      });
    });

    expect(mockToast).toHaveBeenCalledWith(
      "alice@test.com",
      expect.objectContaining({ description: "New message" })
    );
  });

  it("toast auto-dismisses after 5s (duration option)", () => {
    render(<NotificationListener />);

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        payload: { from: "bob@test.com", subject: "Hello" },
      });
    });

    expect(mockToast).toHaveBeenCalledWith(
      "bob@test.com",
      expect.objectContaining({ duration: 5000 })
    );
  });

  it("toast offers an Open action when the message carries a thread", () => {
    render(<NotificationListener />);

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        domain_id: "dom1",
        thread_id: "t1",
        payload: { from: "carol@test.com", subject: "Deep link" },
      });
    });

    expect(mockToast).toHaveBeenCalledWith(
      "carol@test.com",
      expect.objectContaining({
        action: expect.objectContaining({ label: "Open" }),
      })
    );
  });

  it("skips empty payload (no toast shown)", () => {
    render(<NotificationListener />);

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        payload: {},
      });
    });

    expect(mockToast).not.toHaveBeenCalled();
  });

  it("cleans up subscription on unmount", () => {
    const { unmount } = render(<NotificationListener />);
    expect(subscriptionCallbacks["email.received"]).toBeDefined();
    unmount();
    expect(subscriptionCallbacks["email.received"]).toBeUndefined();
  });

  it("fires browser Notification when permission is granted", () => {
    const NotificationSpy = vi.fn();
    Object.defineProperty(globalThis, "Notification", {
      value: Object.assign(NotificationSpy, {
        permission: "granted",
        requestPermission: vi.fn().mockResolvedValue("granted"),
      }),
      configurable: true,
      writable: true,
    });

    render(<NotificationListener />);
    expect(subscriptionCallbacks["email.received"]).toBeDefined();

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        payload: { from: "charlie@test.com", subject: "Browser notif test" },
      });
    });

    expect(NotificationSpy).toHaveBeenCalledWith("charlie@test.com", expect.objectContaining({
      body: "Browser notif test",
    }));
  });

  it("browser Notification click opens the thread", () => {
    const instances: Array<{ onclick?: () => void; close?: () => void }> = [];
    const NotificationSpy = vi.fn(function (this: { close: () => void }) {
      this.close = vi.fn();
      instances.push(this);
    });
    Object.defineProperty(globalThis, "Notification", {
      value: Object.assign(NotificationSpy, {
        permission: "granted",
        requestPermission: vi.fn().mockResolvedValue("granted"),
      }),
      configurable: true,
      writable: true,
    });

    render(<NotificationListener />);

    act(() => {
      subscriptionCallbacks["email.received"]!({
        event: "email.received",
        domain_id: "dom1",
        thread_id: "t1",
        payload: { from: "dana@test.com", subject: "Click me" },
      });
    });

    expect(instances).toHaveLength(1);
    expect(typeof instances[0].onclick).toBe("function");
  });
});
