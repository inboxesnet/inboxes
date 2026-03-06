import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import OnboardingPage from "../page";

// --- Mocks ---
// Use vi.hoisted to ensure variables are available when vi.mock factories run

const { mockPush, mockReplace, mockApi, mockRefreshDomains, mockRefreshUnreadCounts, mockStartSync, mockResumeJob } = vi.hoisted(() => ({
  mockPush: vi.fn(),
  mockReplace: vi.fn(),
  mockApi: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
  mockRefreshDomains: vi.fn().mockResolvedValue(undefined),
  mockRefreshUnreadCounts: vi.fn().mockResolvedValue(undefined),
  mockStartSync: vi.fn(),
  mockResumeJob: vi.fn(),
}));

let mockSyncJobReturn = {
  progress: null as null | { phase: string; imported: number; total: number; message: string },
  result: null as null | { sent_count: number; received_count: number; thread_count: number; address_count: number },
  error: "",
  isRunning: false,
  isComplete: false,
  isFailed: false,
  aliasesReady: false,
  startSync: mockStartSync,
  resumeJob: mockResumeJob,
};

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => ({
    get: () => null,
  }),
}));

vi.mock("@/lib/api", () => ({
  api: mockApi,
  ApiError: class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  },
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    activeDomain: null,
    domains: [],
    refreshDomains: mockRefreshDomains,
    refreshUnreadCounts: mockRefreshUnreadCounts,
  }),
}));

vi.mock("@/contexts/app-config-context", () => ({
  useAppConfig: () => ({ commercial: false, apiUrl: "", wsUrl: "" }),
}));

vi.mock("@/hooks/use-sync-job", () => ({
  useSyncJob: () => mockSyncJobReturn,
}));

// Mock lucide-react icons
vi.mock("lucide-react", () => {
  const icon =
    (name: string) =>
    ({ className }: { className?: string }) => (
      <span data-testid={`icon-${name}`} className={className} />
    );
  return {
    Check: icon("check"),
    Minus: icon("minus"),
    Key: icon("key"),
    Globe: icon("globe"),
    Download: icon("download"),
    Users: icon("users"),
    ArrowRight: icon("arrow-right"),
    Star: icon("star"),
    Archive: icon("archive"),
    Search: icon("search"),
    Mail: icon("mail"),
    AlertTriangle: icon("alert-triangle"),
    X: icon("x"),
  };
});

// Mock UI components
vi.mock("@/components/ui/button", () => ({
  Button: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: string;
    size?: string;
  }) => <button {...props}>{children}</button>,
}));

vi.mock("@/components/ui/input", () => ({
  Input: React.forwardRef<
    HTMLInputElement,
    React.InputHTMLAttributes<HTMLInputElement>
  >((props, ref) => <input ref={ref} {...props} />),
}));

vi.mock("@/components/ui/card", () => ({
  Card: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => (
    <div data-testid="card" className={className}>
      {children}
    </div>
  ),
  CardHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CardTitle: ({ children }: { children: React.ReactNode }) => (
    <h3>{children}</h3>
  ),
  CardDescription: ({ children }: { children: React.ReactNode }) => (
    <p>{children}</p>
  ),
  CardContent: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => <div className={className}>{children}</div>,
  CardFooter: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => <div className={className}>{children}</div>,
}));

vi.mock("@/components/ui/spinner", () => ({
  Spinner: ({ className }: { className?: string }) => (
    <span data-testid="spinner" className={className}>
      Loading...
    </span>
  ),
}));

vi.mock("@/components/ui/badge", () => ({
  Badge: ({
    children,
    variant,
  }: {
    children: React.ReactNode;
    variant?: string;
  }) => <span data-testid="badge" data-variant={variant}>{children}</span>,
}));

vi.mock("@/lib/utils", () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(" "),
}));

// --- Helpers ---

const FAKE_DOMAINS = [
  {
    id: "d1",
    org_id: "org1",
    domain: "example.com",
    resend_domain_id: "rd1",
    status: "active",
    mx_verified: true,
    spf_verified: true,
    dkim_verified: true,
    catch_all_enabled: false,
    display_order: 0,
    dns_records: null,
    hidden: false,
    verified_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "d2",
    org_id: "org1",
    domain: "other.com",
    resend_domain_id: "rd2",
    status: "verified",
    mx_verified: true,
    spf_verified: true,
    dkim_verified: true,
    catch_all_enabled: false,
    display_order: 1,
    dns_records: null,
    hidden: false,
    verified_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
  },
];

const FAKE_ADDRESSES = [
  {
    id: "a1",
    domain_id: "d1",
    address: "hello@example.com",
    local_part: "hello",
    type: "unclaimed" as const,
    email_count: 12,
  },
  {
    id: "a2",
    domain_id: "d1",
    address: "support@example.com",
    local_part: "support",
    type: "unclaimed" as const,
    email_count: 5,
  },
];

/**
 * Sets up api.get to return the connect step (initial onboarding).
 */
function setupConnectStep() {
  mockApi.get.mockImplementation((url: string) => {
    if (url === "/api/onboarding/status") {
      return Promise.resolve({ step: "connect" });
    }
    return Promise.resolve({});
  });
}

/**
 * Sets up api.get to return the domains step, with domains loaded.
 */
function setupDomainsStep() {
  mockApi.get.mockImplementation((url: string) => {
    if (url === "/api/onboarding/status") {
      return Promise.resolve({ step: "domains" });
    }
    if (url === "/api/domains/all") {
      return Promise.resolve(FAKE_DOMAINS);
    }
    return Promise.resolve({});
  });
}

/**
 * Sets up api.get to return the sync step.
 */
function setupSyncStep() {
  mockApi.get.mockImplementation((url: string) => {
    if (url === "/api/onboarding/status") {
      return Promise.resolve({ step: "sync" });
    }
    if (url === "/api/domains/all") {
      return Promise.resolve(FAKE_DOMAINS);
    }
    return Promise.resolve({});
  });
}

/**
 * Sets up api.get to return the addresses step with discovered addresses.
 */
function setupAddressesStep() {
  mockApi.get.mockImplementation((url: string) => {
    if (url === "/api/onboarding/status") {
      return Promise.resolve({ step: "addresses" });
    }
    if (url === "/api/domains/all") {
      return Promise.resolve(FAKE_DOMAINS);
    }
    if (url === "/api/onboarding/addresses") {
      return Promise.resolve(FAKE_ADDRESSES);
    }
    return Promise.resolve({});
  });
}

// --- Tests ---

describe("OnboardingPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSyncJobReturn = {
      progress: null,
      result: null,
      error: "",
      isRunning: false,
      isComplete: false,
      isFailed: false,
      aliasesReady: false,
      startSync: mockStartSync,
      resumeJob: mockResumeJob,
    };
  });

  it("renders connect step with API key input", async () => {
    setupConnectStep();
    render(<OnboardingPage />);

    await waitFor(() => {
      expect(
        screen.getByText("Connect your Resend account")
      ).toBeInTheDocument();
    });

    expect(screen.getByLabelText("Resend API Key")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("re_...")
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Connect/ })
    ).toBeInTheDocument();
  });

  it("shows error when API key is invalid (connect rejects)", async () => {
    setupConnectStep();

    // ApiError uses (status, message) constructor
    const { ApiError } = await import("@/lib/api");
    mockApi.post.mockRejectedValueOnce(new ApiError(422, "Invalid API key"));

    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByLabelText("Resend API Key")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Resend API Key"), {
      target: { value: "re_bad_key" },
    });
    fireEvent.submit(screen.getByLabelText("Resend API Key").closest("form")!);

    await waitFor(() => {
      expect(screen.getByText("Invalid API key")).toBeInTheDocument();
    });
  });

  it("transitions to domains step after valid connect", async () => {
    setupConnectStep();

    mockApi.post.mockResolvedValueOnce({ domains: FAKE_DOMAINS });

    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByLabelText("Resend API Key")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Resend API Key"), {
      target: { value: "re_valid_key_123" },
    });
    fireEvent.submit(screen.getByLabelText("Resend API Key").closest("form")!);

    await waitFor(() => {
      expect(screen.getByText("Your domains")).toBeInTheDocument();
    });

    expect(screen.getByText("example.com")).toBeInTheDocument();
    expect(screen.getByText("other.com")).toBeInTheDocument();
  });

  it("renders domain checkboxes and toggles select all/none", async () => {
    setupDomainsStep();
    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByText("Your domains")).toBeInTheDocument();
    });

    // Both domains should be rendered with their names
    expect(screen.getByText("example.com")).toBeInTheDocument();
    expect(screen.getByText("other.com")).toBeInTheDocument();

    // The Continue button should show count of selected domains (both selected initially)
    expect(
      screen.getByRole("button", { name: /Continue with 2 domains/ })
    ).toBeInTheDocument();

    // Click "Deselect all" to deselect everything
    const deselectButton = screen.getByText("Deselect all");
    fireEvent.click(deselectButton);

    // After deselecting all, button text changes and continue is disabled
    await waitFor(() => {
      expect(screen.getByText("Select all")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: /Continue with 0 domains/ })
    ).toBeDisabled();

    // Click "Select all" to re-select everything
    fireEvent.click(screen.getByText("Select all"));
    await waitFor(() => {
      expect(screen.getByText("Deselect all")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: /Continue with 2 domains/ })
    ).not.toBeDisabled();
  });

  it("sync step shows progress bar when importing", async () => {
    mockSyncJobReturn = {
      ...mockSyncJobReturn,
      progress: {
        phase: "importing",
        imported: 42,
        total: 100,
        message: "Importing 42 of 100 emails",
      },
      result: null,
      isRunning: true,
    };

    setupSyncStep();
    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByText("Import email history")).toBeInTheDocument();
    });

    // Progress counter text
    expect(screen.getByText("Importing emails...")).toBeInTheDocument();
    expect(screen.getByText("42 / 100")).toBeInTheDocument();
  });

  it("sync step shows rotating tips", async () => {
    mockSyncJobReturn = {
      ...mockSyncJobReturn,
      progress: {
        phase: "scanning",
        imported: 0,
        total: 0,
        message: "Scanning emails...",
      },
      result: null,
      isRunning: true,
    };

    setupSyncStep();
    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByText("Import email history")).toBeInTheDocument();
    });

    // The first tip should be displayed (index 0 by default)
    expect(
      screen.getByText("Star important threads")
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Click the star icon on any thread to keep it at the top of your mind."
      )
    ).toBeInTheDocument();

    // Tip dot buttons should be rendered (5 tips = 5 buttons)
    // Click the third dot to switch to that tip
    const tipDots = screen.getAllByRole("button").filter((btn) => {
      // The tip dots are small buttons without text content
      return btn.className.includes("rounded-full") && btn.textContent === "";
    });
    expect(tipDots.length).toBe(5);

    // Click the third dot (index 2) to switch to "Search across everything"
    fireEvent.click(tipDots[2]);
    expect(
      screen.getByText("Search across everything")
    ).toBeInTheDocument();
  });

  it("addresses step renders address type options", async () => {
    setupAddressesStep();
    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByText("Set up addresses")).toBeInTheDocument();
    });

    expect(
      screen.getByText(
        "Categorize each discovered address as individual, group, or skip."
      )
    ).toBeInTheDocument();

    // Both addresses should be listed
    expect(screen.getByText("hello@example.com")).toBeInTheDocument();
    expect(screen.getByText("support@example.com")).toBeInTheDocument();

    // Each address should show its email count
    expect(screen.getByText("12 emails")).toBeInTheDocument();
    expect(screen.getByText("5 emails")).toBeInTheDocument();

    // Type buttons: Individual, Group, Skip for each address (2 addresses x 3 = 6 total)
    const individualButtons = screen.getAllByText("Individual");
    const groupButtons = screen.getAllByText("Group");
    const skipButtons = screen.getAllByText("Skip");
    expect(individualButtons).toHaveLength(2);
    expect(groupButtons).toHaveLength(2);
    expect(skipButtons).toHaveLength(2);

    // Complete setup button
    expect(
      screen.getByRole("button", { name: /Complete setup/ })
    ).toBeInTheDocument();
  });

  it("complete button navigates to inbox on success", async () => {
    setupAddressesStep();

    mockApi.post.mockImplementation((url: string) => {
      if (url === "/api/onboarding/addresses") {
        return Promise.resolve({});
      }
      if (url === "/api/onboarding/complete") {
        return Promise.resolve({ first_domain_id: "d1" });
      }
      return Promise.resolve({});
    });

    render(<OnboardingPage />);

    await waitFor(() => {
      expect(screen.getByText("Set up addresses")).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByRole("button", { name: /Complete setup/ })
    );

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/d/d1/inbox");
    });

    expect(mockRefreshDomains).toHaveBeenCalled();
    expect(mockRefreshUnreadCounts).toHaveBeenCalled();
  });
});
