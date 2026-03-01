import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { PaymentWall } from "../payment-wall";

// Mock lucide-react
vi.mock("lucide-react", () => ({
  CreditCard: () => <span data-testid="credit-icon">CreditCard</span>,
}));

// Mock sonner
vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}));

// Mock api
vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn().mockResolvedValue({ role: "admin" }),
    post: vi.fn().mockResolvedValue({ url: "https://checkout.stripe.com/session" }),
  },
  ApiError: class extends Error {
    status: number;
    constructor(msg: string, status: number) {
      super(msg);
      this.status = status;
    }
  },
}));

// Mock Dialog to render inline
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({
    children,
    open,
  }: {
    children: React.ReactNode;
    open: boolean;
    onOpenChange: (v: boolean) => void;
  }) => (open ? <div data-testid="dialog">{children}</div> : null),
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dialog-content">{children}</div>
  ),
}));

// Mock Button
vi.mock("@/components/ui/button", () => ({
  Button: ({
    children,
    onClick,
    variant,
    disabled,
    ...props
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    variant?: string;
    disabled?: boolean;
    size?: string;
    className?: string;
  }) => (
    <button onClick={onClick} disabled={disabled} data-variant={variant} {...props}>
      {children}
    </button>
  ),
}));

// Mock Spinner
vi.mock("@/components/ui/spinner", () => ({
  Spinner: ({ className }: { className?: string }) => (
    <span data-testid="spinner" className={className} />
  ),
}));

describe("PaymentWall", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    // Clear URL params
    Object.defineProperty(window, "location", {
      value: { ...window.location, search: "", href: "http://localhost:3000" },
      writable: true,
      configurable: true,
    });
  });
  afterEach(() => {
    cleanup();
  });

  it("is not rendered initially", () => {
    render(<PaymentWall />);
    expect(screen.queryByTestId("dialog")).not.toBeInTheDocument();
  });

  it("renders on payment-required event", async () => {
    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(screen.getByTestId("dialog")).toBeInTheDocument();
    });
  });

  it("shows Upgrade Required heading", async () => {
    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(screen.getByText("Upgrade Required")).toBeInTheDocument();
    });
  });

  it("shows admin upgrade messaging when role is admin", async () => {
    const { api } = await import("@/lib/api");
    (api.get as ReturnType<typeof vi.fn>).mockResolvedValue({ role: "admin" });

    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(
        screen.getByText("Upgrade to Pro")
      ).toBeInTheDocument();
    });
  });

  it("shows member messaging when role is member", async () => {
    const { api } = await import("@/lib/api");
    (api.get as ReturnType<typeof vi.fn>).mockResolvedValue({ role: "member" });

    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(
        screen.getByText(/ask your admin/i)
      ).toBeInTheDocument();
    });
  });

  it("dismiss closes the dialog", async () => {
    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(screen.getByText("Dismiss")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Dismiss"));
    expect(screen.queryByTestId("dialog")).not.toBeInTheDocument();
  });

  it("shows loading state during checkout", async () => {
    const { api } = await import("@/lib/api");
    (api.get as ReturnType<typeof vi.fn>).mockResolvedValue({ role: "admin" });
    // Make post hang
    (api.post as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(screen.getByText("Upgrade to Pro")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Upgrade to Pro"));
    await waitFor(() => {
      expect(screen.getByText("Redirecting...")).toBeInTheDocument();
    });
  });

  it("prevents double checkout", async () => {
    const { api } = await import("@/lib/api");
    (api.get as ReturnType<typeof vi.fn>).mockResolvedValue({ role: "admin" });
    (api.post as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    render(<PaymentWall />);
    fireEvent(window, new Event("payment-required"));
    await waitFor(() => {
      expect(screen.getByText("Upgrade to Pro")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Upgrade to Pro"));
    await waitFor(() => {
      expect(screen.getByText("Redirecting...")).toBeInTheDocument();
    });
    // Button should be disabled to prevent double checkout
    const btn = screen.getByText("Redirecting...").closest("button");
    expect(btn).toBeDisabled();
  });
});
