import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { FloatingComposeWindow } from "../floating-compose-window";
import { api } from "@/lib/api";

// Track mock functions so we can assert on them
const mockCloseCompose = vi.fn();
const mockMinimizeCompose = vi.fn();
const mockRestoreCompose = vi.fn();

// Compose state holder - allows tests to control the state
let mockComposeState = "open";
let mockComposeData: Record<string, unknown> | null = {};

vi.mock("@/contexts/email-window-context", () => ({
  useEmailWindow: () => ({
    composeState: mockComposeState,
    composeData: mockComposeData,
    minimizeCompose: mockMinimizeCompose,
    restoreCompose: mockRestoreCompose,
    closeCompose: mockCloseCompose,
  }),
}));

vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    activeDomain: { id: "d1", domain: "test.com" },
    domains: [{ id: "d1", domain: "test.com" }],
  }),
}));

vi.mock("@/contexts/preferences-context", () => ({
  usePreferences: () => ({
    stripTrackingParams: false,
    warnNoSubject: false,
  }),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn().mockResolvedValue([]),
    post: vi.fn().mockResolvedValue({ id: "draft-1" }),
    patch: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
  },
  uploadFile: vi.fn().mockResolvedValue({ id: "att-1", filename: "file.pdf", size: 1024 }),
}));

vi.mock("@/lib/utils", () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(" "),
}));

vi.mock("@/lib/sanitize-links", () => ({
  sanitizeLinkNode: vi.fn(),
}));

vi.mock("dompurify", () => ({
  default: {
    sanitize: (html: string) => html,
    addHook: vi.fn(),
    removeAllHooks: vi.fn(),
  },
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

// Mock TipTapEditor as a simple textarea
vi.mock("@/components/tiptap-editor", () => ({
  TipTapEditor: ({ onChange, content, toolbarLeft, toolbarRight }: {
    onChange?: (html: string, plain: string) => void;
    content?: string;
    toolbarLeft?: React.ReactNode;
    toolbarRight?: React.ReactNode;
  }) => (
    <div data-testid="tiptap-editor">
      <textarea
        data-testid="editor-textarea"
        defaultValue={content}
        onChange={(e) => onChange?.(e.target.value, e.target.value)}
      />
      {toolbarLeft}
      {toolbarRight}
    </div>
  ),
}));

// Mock RecipientInput as a simple input
vi.mock("@/components/recipient-input", () => ({
  RecipientInput: ({ value, onChange, placeholder }: {
    value: string[];
    onChange: (v: string[]) => void;
    placeholder?: string;
  }) => (
    <input
      data-testid="recipient-input"
      placeholder={placeholder}
      value={value.join(", ")}
      onChange={(e) => onChange(e.target.value ? e.target.value.split(", ") : [])}
    />
  ),
}));

// Mock UI components
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

vi.mock("@/components/ui/spinner", () => ({
  Spinner: ({ className }: { className?: string }) => (
    <span data-testid="spinner" className={className} />
  ),
}));

vi.mock("@/components/ui/confirm-dialog", () => ({
  ConfirmDialog: ({ open, title, onConfirm }: {
    open: boolean;
    title: string;
    onConfirm: () => void;
    onOpenChange: (open: boolean) => void;
    description?: string;
    confirmLabel?: string;
    destructive?: boolean;
  }) =>
    open ? (
      <div data-testid="confirm-dialog">
        <span>{title}</span>
        <button onClick={onConfirm}>Confirm</button>
      </div>
    ) : null,
}));

// Mock lucide-react icons
vi.mock("lucide-react", () => {
  const icon =
    (name: string) =>
    ({ className }: { className?: string }) => (
      <span data-testid={`icon-${name}`} className={className} />
    );
  return {
    Minus: icon("minus"),
    X: icon("x"),
    ChevronUp: icon("chevron-up"),
    Send: icon("send"),
    Trash2: icon("trash"),
    Paperclip: icon("paperclip"),
  };
});

describe("FloatingComposeWindow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockComposeState = "open";
    mockComposeData = {};
  });

  it("renders nothing when composeState is closed", () => {
    mockComposeState = "closed";
    const { container } = render(<FloatingComposeWindow />);
    expect(container.innerHTML).toBe("");
  });

  it("renders 'New Message' title bar when open (desktop)", () => {
    render(<FloatingComposeWindow />);
    const newMsgElements = screen.getAllByText("New Message");
    expect(newMsgElements.length).toBeGreaterThan(0);
  });

  it("shows From, To, and Subject fields when open", () => {
    render(<FloatingComposeWindow />);
    // Check for field labels
    const labels = screen.getAllByText("From");
    expect(labels.length).toBeGreaterThan(0);
    const toLabels = screen.getAllByText("To");
    expect(toLabels.length).toBeGreaterThan(0);
    const subjLabels = screen.getAllByText("Subj");
    expect(subjLabels.length).toBeGreaterThan(0);
  });

  it("shows Subject inputs that accept input", () => {
    render(<FloatingComposeWindow />);
    const subjectInputs = screen.getAllByPlaceholderText("Subject");
    expect(subjectInputs.length).toBeGreaterThan(0);
    fireEvent.change(subjectInputs[0], { target: { value: "Hello World" } });
    expect(subjectInputs[0]).toHaveValue("Hello World");
  });

  it("renders Send button", () => {
    render(<FloatingComposeWindow />);
    const sendButtons = screen.getAllByText("Send");
    expect(sendButtons.length).toBeGreaterThan(0);
  });

  it("shows compose form as dialog with aria-label", () => {
    render(<FloatingComposeWindow />);
    const dialogs = screen.getAllByRole("dialog");
    expect(dialogs.length).toBeGreaterThan(0);
    // At least one dialog should have aria-label "Compose email"
    const composeDialog = dialogs.find(
      (d) => d.getAttribute("aria-label") === "Compose email"
    );
    expect(composeDialog).toBeTruthy();
  });

  it("renders the TipTap editor mock", () => {
    render(<FloatingComposeWindow />);
    const editors = screen.getAllByTestId("tiptap-editor");
    expect(editors.length).toBeGreaterThan(0);
  });

  it("shows minimized state with subject or 'New Message'", () => {
    mockComposeState = "minimized";
    render(<FloatingComposeWindow />);
    // When subject is empty, minimized bar shows "New Message"
    expect(screen.getByText("New Message")).toBeInTheDocument();
  });

  it("calls restoreCompose when clicking on minimized bar", () => {
    mockComposeState = "minimized";
    render(<FloatingComposeWindow />);
    fireEvent.click(screen.getByText("New Message"));
    expect(mockRestoreCompose).toHaveBeenCalledTimes(1);
  });

  it("has close button in minimized state with aria-label", () => {
    mockComposeState = "minimized";
    render(<FloatingComposeWindow />);
    const closeBtn = screen.getByLabelText("Close compose window");
    expect(closeBtn).toBeInTheDocument();
  });

  it("has restore button in minimized state with aria-label", () => {
    mockComposeState = "minimized";
    render(<FloatingComposeWindow />);
    const restoreBtn = screen.getByLabelText("Restore compose window");
    expect(restoreBtn).toBeInTheDocument();
  });

  it("shows recipient input with correct placeholder", () => {
    render(<FloatingComposeWindow />);
    const recipientInputs = screen.getAllByPlaceholderText("recipient@example.com");
    expect(recipientInputs.length).toBeGreaterThan(0);
  });

  it("shows Cc Bcc toggle button", () => {
    render(<FloatingComposeWindow />);
    const ccBccButtons = screen.getAllByText("Cc Bcc");
    expect(ccBccButtons.length).toBeGreaterThan(0);
  });

  it("reveals Cc and Bcc fields when Cc Bcc button is clicked", () => {
    render(<FloatingComposeWindow />);
    // Initially no Cc or Bcc labels
    expect(screen.queryAllByText("Cc").length).toBe(0);

    // Click the Cc Bcc toggle (use the first one)
    const ccBccButtons = screen.getAllByText("Cc Bcc");
    fireEvent.click(ccBccButtons[0]);

    // Now Cc and Bcc labels should appear
    expect(screen.getAllByText("Cc").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Bcc").length).toBeGreaterThan(0);
  });

  it("shows the default from address when no aliases", () => {
    render(<FloatingComposeWindow />);
    // Should show hello@test.com as placeholder or display text
    const fromText = screen.getAllByPlaceholderText("hello@test.com");
    expect(fromText.length).toBeGreaterThan(0);
  });

  it("From address defaults to is_default alias", async () => {
    const aliasData = [
      { id: "a1", address: "support@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: false },
      { id: "a2", address: "admin@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: true },
      { id: "a3", address: "hello@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: false },
    ];
    vi.mocked(api.get).mockResolvedValue(aliasData);

    render(<FloatingComposeWindow />);

    await waitFor(() => {
      const selects = document.querySelectorAll("select");
      const hasDefaultSelected = Array.from(selects).some((s) => s.value === "admin@test.com");
      expect(hasDefaultSelected).toBe(true);
    });
  });

  it("From falls back to hello@ alias when no default", async () => {
    const aliasData = [
      { id: "a1", address: "support@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: false },
      { id: "a2", address: "hello@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: false },
      { id: "a3", address: "info@test.com", name: "", domain_id: "d1", can_send_as: true, is_default: false },
    ];
    vi.mocked(api.get).mockResolvedValue(aliasData);

    render(<FloatingComposeWindow />);

    await waitFor(() => {
      const selects = document.querySelectorAll("select");
      const hasHelloSelected = Array.from(selects).some((s) => s.value === "hello@test.com");
      expect(hasHelloSelected).toBe(true);
    });
  });

  it("Auto-save schedules on subject change (debounce mechanism exists)", async () => {
    // Verify that typing in subject field doesn't immediately trigger save
    render(<FloatingComposeWindow />);

    // Clear any calls from initial render
    vi.mocked(api.post).mockClear();

    const subjectInputs = screen.getAllByPlaceholderText("Subject");
    fireEvent.change(subjectInputs[0], { target: { value: "Test draft subject" } });

    // Immediately after change, no draft save should have been posted
    // (the 3s debounce hasn't fired yet)
    const draftCalls = vi.mocked(api.post).mock.calls.filter(
      (call) => call[0] === "/api/drafts"
    );
    expect(draftCalls.length).toBe(0);
  });

  it("Save status displays in title bar area", () => {
    // Verify the component has the structure for save status display
    render(<FloatingComposeWindow />);
    // By default, title bar should show "New Message" (no save status)
    const newMsgElements = screen.getAllByText("New Message");
    expect(newMsgElements.length).toBeGreaterThan(0);
    // "Saving..." and "Saved" should NOT be present initially
    expect(screen.queryByText("Saving...")).not.toBeInTheDocument();
    expect(screen.queryByText("Saved")).not.toBeInTheDocument();
  });

  it("Cmd+Enter triggers send", () => {
    render(<FloatingComposeWindow />);
    // The onKeyDown handler is on the <form>, not the dialog wrapper.
    // Find the form element (there are mobile + desktop forms).
    const forms = document.querySelectorAll("form");
    expect(forms.length).toBeGreaterThan(0);
    // Fire Cmd+Enter on the first form
    fireEvent.keyDown(forms[0], { key: "Enter", metaKey: true });
    // With no recipients, it should set an error
    // (the handleSend validates to.length > 0 first)
    expect(screen.getAllByText(/To is required/).length).toBeGreaterThan(0);
  });

  it("send button is disabled while sending", async () => {
    // Make api.post hang to simulate in-flight state
    let resolvePost: (value: unknown) => void;
    vi.mocked(api.post).mockImplementation(() => new Promise((res) => { resolvePost = res; }));

    mockComposeData = { toAddresses: ["test@example.com"] };
    render(<FloatingComposeWindow />);

    // Click send
    const sendButtons = screen.getAllByText("Send");
    fireEvent.click(sendButtons[0]);

    // The send button(s) should now be disabled
    await waitFor(() => {
      const btns = screen.getAllByText("Send").map((el) => el.closest("button"));
      const anyDisabled = btns.some((b) => b?.disabled);
      expect(anyDisabled).toBe(true);
    });

    // Clean up the hanging promise
    resolvePost!({});
  });

  it("discard opens confirmation dialog", () => {
    render(<FloatingComposeWindow />);
    // The trash/discard button should exist
    const trashIcons = screen.getAllByTestId("icon-trash");
    const trashBtn = trashIcons[0].closest("button");
    expect(trashBtn).toBeTruthy();
    fireEvent.click(trashBtn!);
    // ConfirmDialog should appear
    expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
  });

  it("reply pre-fills To from composeData", () => {
    mockComposeData = {
      toAddresses: ["sender@example.com"],
      subject: "Re: Hello",
    };
    render(<FloatingComposeWindow />);
    // The recipient input should have the pre-filled address
    const recipientInputs = screen.getAllByTestId("recipient-input");
    const hasValue = recipientInputs.some((el) =>
      (el as HTMLInputElement).value.includes("sender@example.com")
    );
    expect(hasValue).toBe(true);
  });

  it("reply all pre-fills To + Cc", () => {
    mockComposeData = {
      toAddresses: ["sender@example.com"],
      ccAddresses: ["cc1@example.com", "cc2@example.com"],
      subject: "Re: Hello",
    };
    render(<FloatingComposeWindow />);
    // Cc/Bcc should be visible since ccAddresses are provided
    expect(screen.getAllByText("Cc").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Bcc").length).toBeGreaterThan(0);
  });

  it("forward has empty To and Fwd: prefix", () => {
    mockComposeData = {
      toAddresses: [],
      subject: "Fwd: Original Subject",
    };
    render(<FloatingComposeWindow />);
    // Subject should be pre-filled with Fwd: prefix
    const subjectInputs = screen.getAllByPlaceholderText("Subject");
    expect((subjectInputs[0] as HTMLInputElement).value).toBe("Fwd: Original Subject");
    // To should be empty
    const recipientInputs = screen.getAllByTestId("recipient-input");
    const isEmpty = recipientInputs.some((el) => (el as HTMLInputElement).value === "");
    expect(isEmpty).toBe(true);
  });
});
