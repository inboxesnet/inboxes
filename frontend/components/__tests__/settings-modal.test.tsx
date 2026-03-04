import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SettingsModal } from "../settings-modal";
import { api } from "@/lib/api";

// Mock api
vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn().mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "admin@test.com",
          name: "Admin User",
          role: "admin",
          status: "active",
          is_owner: true,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([
          {
            id: "d1",
            org_id: "org1",
            domain: "test.com",
            resend_domain_id: "rd1",
            status: "verified",
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
        ]);
      }
      return Promise.resolve({});
    }),
    post: vi.fn().mockResolvedValue({}),
    patch: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
  },
  ApiError: class ApiError extends Error {
    status: number;
    constructor(message: string, status: number) {
      super(message);
      this.status = status;
    }
  },
}));

// Mock sonner
vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

// Mock contexts
vi.mock("@/contexts/app-config-context", () => ({
  useAppConfig: () => ({ commercial: false, apiUrl: "", wsUrl: "" }),
}));

vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    activeDomain: { id: "d1", domain: "test.com" },
    domains: [{ id: "d1", domain: "test.com" }],
    refreshDomains: vi.fn().mockResolvedValue(undefined),
  }),
}));

vi.mock("@/contexts/preferences-context", () => ({
  usePreferences: () => ({
    stripTrackingParams: true,
    warnNoSubject: true,
    updatePreference: vi.fn(),
  }),
}));

// Mock useSyncJob hook
vi.mock("@/hooks/use-sync-job", () => ({
  useSyncJob: () => ({
    progress: null,
    result: null,
    error: "",
    isRunning: false,
    startSync: vi.fn(),
  }),
}));

// Mock utils
vi.mock("@/lib/utils", () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(" "),
  validatePassword: (password: string) => {
    if (password.length < 8) return "Password must be at least 8 characters";
    if (!/[A-Z]/.test(password)) return "Password must include an uppercase letter";
    if (!/[a-z]/.test(password)) return "Password must include a lowercase letter";
    if (!/[0-9]/.test(password)) return "Password must include a number";
    return null;
  },
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
    RefreshCw: icon("refresh"),
    User: icon("user"),
    Globe: icon("globe"),
    CreditCard: icon("credit-card"),
    Users: icon("users"),
    AtSign: icon("at-sign"),
    Trash2: icon("trash"),
    RotateCw: icon("rotate"),
    UserX: icon("user-x"),
    UserPlus: icon("user-plus"),
    X: icon("x"),
    Star: icon("star"),
    Pencil: icon("pencil"),
    Wrench: icon("wrench"),
    Building2: icon("building"),
    Tag: icon("tag"),
  };
});

// Mock UI components (Dialog renders children when open=true)
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; onOpenChange: (open: boolean) => void; children: React.ReactNode }) =>
    open ? <div data-testid="dialog">{children}</div> : null,
  DialogContent: ({ children, className }: { children: React.ReactNode; className?: string; onClose?: () => void }) => (
    <div data-testid="dialog-content" className={className}>{children}</div>
  ),
}));

vi.mock("@/components/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: string; size?: string }) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@/components/ui/input", () => ({
  Input: React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
    (props, ref) => <input ref={ref} {...props} />
  ),
}));

vi.mock("@/components/ui/card", () => ({
  Card: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div data-testid="card" className={className}>{children}</div>
  ),
  CardHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardTitle: ({ children }: { children: React.ReactNode }) => <h3>{children}</h3>,
  CardDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  CardContent: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
  CardFooter: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));

vi.mock("@/components/ui/spinner", () => ({
  Spinner: ({ className }: { className?: string }) => (
    <span data-testid="spinner" className={className}>Loading...</span>
  ),
}));

vi.mock("@/components/ui/badge", () => ({
  Badge: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <span className={className}>{children}</span>
  ),
}));

vi.mock("@/components/ui/confirm-dialog", () => ({
  ConfirmDialog: ({ open, title }: {
    open: boolean;
    title: string;
    onConfirm: () => void;
    onOpenChange: (open: boolean) => void;
    description?: string;
    confirmLabel?: string;
    destructive?: boolean;
  }) => open ? <div data-testid="confirm-dialog">{title}</div> : null,
}));

describe("SettingsModal", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
  };

  const defaultApiGetImpl = (url: string) => {
    if (url === "/api/users/me") {
      return Promise.resolve({
        id: "u1",
        org_id: "org1",
        email: "admin@test.com",
        name: "Admin User",
        role: "admin",
        status: "active",
        is_owner: true,
        created_at: "2026-01-01T00:00:00Z",
      });
    }
    if (url === "/api/domains/all") {
      return Promise.resolve([
        {
          id: "d1",
          org_id: "org1",
          domain: "test.com",
          resend_domain_id: "rd1",
          status: "verified",
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
      ]);
    }
    return Promise.resolve({});
  };

  beforeEach(() => {
    vi.clearAllMocks();
    // Restore the default api.get implementation that may have been overridden by previous tests
    vi.mocked(api.get).mockImplementation(defaultApiGetImpl);
  });

  it("does not render when open=false", () => {
    render(<SettingsModal open={false} onOpenChange={vi.fn()} />);
    expect(screen.queryByTestId("dialog")).not.toBeInTheDocument();
  });

  it("renders dialog when open=true", () => {
    render(<SettingsModal {...defaultProps} />);
    expect(screen.getByTestId("dialog")).toBeInTheDocument();
  });

  it("shows 'Settings' heading in sidebar", () => {
    render(<SettingsModal {...defaultProps} />);
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("shows Profile tab by default", async () => {
    render(<SettingsModal {...defaultProps} />);
    // Profile tab button should have aria-selected=true
    await waitFor(() => {
      const profileTab = screen.getByRole("tab", { name: /Profile/ });
      expect(profileTab).toHaveAttribute("aria-selected", "true");
    });
  });

  it("shows all visible tabs for admin user", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.getByRole("tab", { name: /Domains/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Team/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Aliases/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Labels/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Organization/ })).toBeInTheDocument();
  });

  it("shows System tab for non-commercial owner", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /System/ })).toBeInTheDocument();
    });
  });

  it("shows Jobs tab for admin users", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Jobs/ })).toBeInTheDocument();
    });
  });

  it("shows loading spinner initially", () => {
    render(<SettingsModal {...defaultProps} />);
    // Before data loads, spinner should be visible
    expect(screen.getAllByTestId("spinner").length).toBeGreaterThan(0);
  });

  it("switches tab when a tab button is clicked", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Domains/ })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("tab", { name: /Domains/ }));
    expect(screen.getByRole("tab", { name: /Domains/ })).toHaveAttribute(
      "aria-selected",
      "true"
    );
    expect(screen.getByRole("tab", { name: /Profile/ })).toHaveAttribute(
      "aria-selected",
      "false"
    );
  });

  it("uses tablist role for tab navigation", () => {
    render(<SettingsModal {...defaultProps} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
  });

  it("renders tabpanel with correct role", () => {
    render(<SettingsModal {...defaultProps} />);
    expect(screen.getByRole("tabpanel")).toBeInTheDocument();
  });

  it("opens with defaultTab when provided", async () => {
    render(<SettingsModal {...defaultProps} defaultTab="domains" />);
    await waitFor(() => {
      const domainsTab = screen.getByRole("tab", { name: /Domains/ });
      expect(domainsTab).toHaveAttribute("aria-selected", "true");
    });
  });

  it("shows profile form after data loads", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByDisplayValue("Admin User")).toBeInTheDocument();
    });
  });

  it("member user does NOT see Team tab", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "member@test.com",
          name: "Member User",
          role: "member",
          status: "active",
          is_owner: false,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /Team/ })).not.toBeInTheDocument();
  });

  it("member user does NOT see Organization tab", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "member@test.com",
          name: "Member User",
          role: "member",
          status: "active",
          is_owner: false,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /Organization/ })).not.toBeInTheDocument();
  });

  it("member user does NOT see Jobs tab", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "member@test.com",
          name: "Member User",
          role: "member",
          status: "active",
          is_owner: false,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /Jobs/ })).not.toBeInTheDocument();
  });

  it("non-owner does NOT see System tab", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "admin@test.com",
          name: "Admin User",
          role: "admin",
          status: "active",
          is_owner: false,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /System/ })).not.toBeInTheDocument();
  });

  it("non-commercial mode does NOT show Billing tab", async () => {
    render(<SettingsModal {...defaultProps} />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Profile/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: /Billing/ })).not.toBeInTheDocument();
  });

  it("profile name edit — changes input and submits PATCH /api/users/me", async () => {
    render(<SettingsModal {...defaultProps} />);

    // Wait for profile data to load and populate the name field
    await waitFor(() => {
      expect(screen.getByDisplayValue("Admin User")).toBeInTheDocument();
    });

    const nameInput = screen.getByDisplayValue("Admin User");
    fireEvent.change(nameInput, { target: { value: "New Name" } });
    expect(screen.getByDisplayValue("New Name")).toBeInTheDocument();

    // Find and submit the profile form — the Save button inside the Profile card
    const saveButtons = screen.getAllByRole("button", { name: /Save/ });
    // The first Save button belongs to the profile form
    fireEvent.click(saveButtons[0]);

    await waitFor(() => {
      expect(api.patch).toHaveBeenCalledWith("/api/users/me", { name: "New Name" });
    });
  });

  it("privacy tracking toggle — calls updatePreference when checkbox is toggled", async () => {
    const mockUpdatePreference = vi.fn();
    // Re-mock the preferences context to capture updatePreference calls
    const prefsMod = await import("@/contexts/preferences-context");
    vi.spyOn(prefsMod, "usePreferences").mockReturnValue({
      stripTrackingParams: true,
      warnNoSubject: true,
      updatePreference: mockUpdatePreference,
    });

    render(<SettingsModal {...defaultProps} />);

    // Wait for the profile tab to render fully
    await waitFor(() => {
      expect(screen.getByText("Privacy")).toBeInTheDocument();
    });

    // The privacy checkbox is the "Strip tracking parameters from links" checkbox
    const privacyCheckbox = screen.getByRole("checkbox", {
      name: /strip tracking parameters/i,
    });
    expect(privacyCheckbox).toBeChecked();

    // Uncheck it
    fireEvent.click(privacyCheckbox);

    expect(mockUpdatePreference).toHaveBeenCalledWith("stripTrackingParams", false);
  });

  it("domains tab — renders domain list with status", async () => {
    render(<SettingsModal {...defaultProps} />);
    // Wait for data to load, then click the Domains tab
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Domains/ })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("tab", { name: /Domains/ }));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Domains/ })).toHaveAttribute("aria-selected", "true");
    });
    // The domain "test.com" should appear somewhere in the tab content
    await waitFor(() => {
      expect(screen.getAllByText(/test\.com/).length).toBeGreaterThan(0);
    });
  });

  it("team tab — renders team member list", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1", org_id: "org1", email: "admin@test.com",
          name: "Admin User", role: "admin", status: "active", is_owner: true,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") return Promise.resolve([]);
      if (url === "/api/users") {
        return Promise.resolve([
          { id: "u1", email: "admin@test.com", name: "Admin User", role: "admin", status: "active", created_at: "2026-01-01T00:00:00Z" },
          { id: "u2", email: "member@test.com", name: "Team Member", role: "member", status: "active", created_at: "2026-01-01T00:00:00Z" },
        ]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} defaultTab="team" />);
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Team/ })).toHaveAttribute("aria-selected", "true");
    });
    await waitFor(() => {
      expect(screen.getAllByText(/Team Member/).length).toBeGreaterThan(0);
    });
  });

  it("labels tab — renders label list with create form", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1", org_id: "org1", email: "admin@test.com",
          name: "Admin User", role: "admin", status: "active", is_owner: true,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") return Promise.resolve([]);
      if (url === "/api/labels") {
        return Promise.resolve([
          { id: "l1", name: "Important" },
          { id: "l2", name: "Follow-up" },
        ]);
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    // Wait for initial data to load, then click the Labels tab
    // (defaultTab doesn't call activateTab which triggers loadLabels)
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Labels/ })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("tab", { name: /Labels/ }));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Labels/ })).toHaveAttribute("aria-selected", "true");
    });
    await waitFor(() => {
      expect(screen.getAllByText(/Important/).length).toBeGreaterThan(0);
      expect(screen.getAllByText(/Follow-up/).length).toBeGreaterThan(0);
    });
  });

  it("jobs tab — renders job list", async () => {
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1", org_id: "org1", email: "admin@test.com",
          name: "Admin User", role: "admin", status: "active", is_owner: true,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") return Promise.resolve([]);
      if (url === "/api/admin/jobs") {
        return Promise.resolve({
          jobs: [
            { id: "j1", job_type: "send", status: "completed", attempts: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
          ],
        });
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);
    // Wait for initial data load, then click Jobs tab to trigger activateTab
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Jobs/ })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("tab", { name: /Jobs/ }));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Jobs/ })).toHaveAttribute("aria-selected", "true");
    });
    // The Jobs tab should render the tabpanel
    await waitFor(() => {
      expect(screen.getByRole("tabpanel")).toBeInTheDocument();
    });
  });

  it("organization tab — edits org name and submits PATCH /api/orgs/settings", async () => {
    // Extend the mock to handle org settings endpoint
    vi.mocked(api.get).mockImplementation((url: string) => {
      if (url === "/api/users/me") {
        return Promise.resolve({
          id: "u1",
          org_id: "org1",
          email: "admin@test.com",
          name: "Admin User",
          role: "admin",
          status: "active",
          is_owner: true,
          created_at: "2026-01-01T00:00:00Z",
        });
      }
      if (url === "/api/domains/all") {
        return Promise.resolve([]);
      }
      if (url === "/api/orgs/settings") {
        return Promise.resolve({
          name: "Old Org",
          has_api_key: true,
          resend_rps: 2,
        });
      }
      return Promise.resolve({});
    });

    render(<SettingsModal {...defaultProps} />);

    // Wait for tabs to load, then switch to Organization tab
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: /Organization/ })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("tab", { name: /Organization/ }));

    // Wait for org settings to load
    await waitFor(() => {
      expect(screen.getByDisplayValue("Old Org")).toBeInTheDocument();
    });

    const orgNameInput = screen.getByDisplayValue("Old Org");
    fireEvent.change(orgNameInput, { target: { value: "New Org Name" } });
    expect(screen.getByDisplayValue("New Org Name")).toBeInTheDocument();

    // Submit the org settings form by finding it via the org name input
    const orgForm = orgNameInput.closest("form");
    expect(orgForm).toBeTruthy();
    fireEvent.submit(orgForm!);

    await waitFor(() => {
      expect(api.patch).toHaveBeenCalledWith("/api/orgs/settings", expect.objectContaining({
        name: "New Org Name",
      }));
    });
  });
});
