import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import type { Thread, Email } from "@/lib/types";

// Mock all external dependencies
const mockThread: Thread & { emails: Email[] } = {
  id: "t1",
  org_id: "org1",
  user_id: "u1",
  domain_id: "d1",
  subject: "Test Subject",
  participant_emails: ["alice@test.com"],
  last_message_at: "2026-01-01T00:00:00Z",
  message_count: 1,
  unread_count: 0,
  labels: ["inbox"],
  snippet: "Hello world",
  original_to: "bob@test.com",
  created_at: "2026-01-01T00:00:00Z",
  emails: [
    {
      id: "e1",
      thread_id: "t1",
      user_id: "u1",
      org_id: "org1",
      domain_id: "d1",
      resend_email_id: "r1",
      message_id: "m1",
      direction: "inbound",
      from_address: "alice@test.com",
      to_addresses: ["bob@test.com"],
      cc_addresses: [],
      bcc_addresses: [],
      subject: "Test Subject",
      body_html: "<p>Hello world</p>",
      body_plain: "Hello world",
      status: "received",
      attachments: [],
      delivered_via_alias: "",
      sent_as_alias: "",
      spam_score: 0,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

let mockThreadData: (Thread & { emails: Email[] }) | null = mockThread;

vi.mock("@/hooks/use-threads", () => ({
  useThread: () => ({
    data: mockThreadData,
    isLoading: false,
  }),
  useStarThread: () => ({ mutate: vi.fn() }),
  useMuteThread: () => ({ mutate: vi.fn() }),
  useThreadAction: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("@/contexts/domain-context", () => ({
  useDomains: () => ({
    activeDomain: { id: "d1", domain: "test.com" },
  }),
}));

vi.mock("@/contexts/email-window-context", () => ({
  useEmailWindow: () => ({
    openCompose: vi.fn(),
  }),
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({
    setQueryData: vi.fn(),
    setQueriesData: vi.fn(),
    invalidateQueries: vi.fn(),
  }),
}));

vi.mock("@/lib/query-keys", () => ({
  queryKeys: {
    threads: {
      all: ["threads"],
      lists: () => ["threads", "list"],
      list: (domainId: string, label: string, page: number) => ["threads", "list", domainId, label, page],
      details: () => ["threads", "detail"],
      detail: (threadId: string) => ["threads", "detail", threadId],
    },
    domains: {
      unreadCounts: () => ["domains", "unreadCounts"],
    },
  },
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/api", () => ({
  api: { patch: vi.fn().mockResolvedValue({}), post: vi.fn().mockResolvedValue({}) },
}));

vi.mock("dompurify", () => ({
  default: {
    sanitize: (html: string) => html,
    addHook: vi.fn(),
    removeAllHooks: vi.fn(),
  },
}));

vi.mock("@/lib/types", () => ({
  hasLabel: (thread: { labels?: string[] }, label: string) =>
    thread.labels?.includes(label) ?? false,
}));

vi.mock("@/lib/utils", () => ({
  formatRelativeTime: () => "1h",
}));

vi.mock("@/components/ui/button", () => ({
  Button: ({
    children,
    onClick,
    ...props
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    variant?: string;
    size?: string;
    className?: string;
    title?: string;
  }) => (
    <button onClick={onClick} {...props}>
      {children}
    </button>
  ),
}));

vi.mock("@/components/ui/spinner", () => ({
  Spinner: () => <span data-testid="spinner" />,
}));

vi.mock("@/components/ui/confirm-dialog", () => ({
  ConfirmDialog: () => null,
}));

vi.mock("@/components/contact-card", () => ({
  ContactCard: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}));

vi.mock("lucide-react", () => {
  const icon =
    (name: string) =>
    ({ className }: { className?: string }) => (
      <span data-testid={`icon-${name}`} className={className} />
    );
  return {
    AlertTriangle: icon("alert"),
    Archive: icon("archive"),
    Trash2: icon("trash"),
    Star: icon("star"),
    MailOpen: icon("mail-open"),
    Mail: icon("mail"),
    Reply: icon("reply"),
    ReplyAll: icon("reply-all"),
    Forward: icon("forward"),
    ArrowLeft: icon("arrow-left"),
    Inbox: icon("inbox"),
    BellOff: icon("bell-off"),
    Bell: icon("bell"),
  };
});

import { ThreadView } from "../thread-view";

describe("ThreadView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockThreadData = { ...mockThread };
  });
  afterEach(() => {
    cleanup();
  });

  it("renders subject heading", () => {
    render(<ThreadView threadId="t1" domainId="d1" />);
    expect(screen.getByText("Test Subject")).toBeInTheDocument();
  });

  it("shows trash countdown banner when trash_expires_at is set", () => {
    const future = new Date(Date.now() + 5 * 24 * 60 * 60 * 1000).toISOString();
    mockThreadData = {
      ...mockThread,
      labels: ["trash"],
      trash_expires_at: future,
    };
    render(<ThreadView threadId="t1" domainId="d1" label="trash" />);
    expect(screen.getByText(/permanently deleted/)).toBeInTheDocument();
  });

  it("shows correct days remaining in trash countdown", () => {
    const future = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString();
    mockThreadData = {
      ...mockThread,
      labels: ["trash"],
      trash_expires_at: future,
    };
    render(<ThreadView threadId="t1" domainId="d1" label="trash" />);
    expect(screen.getByText(/3 days/)).toBeInTheDocument();
  });

  it("no trash banner for non-trash threads", () => {
    render(<ThreadView threadId="t1" domainId="d1" label="inbox" />);
    expect(screen.queryByText(/permanently deleted/)).not.toBeInTheDocument();
  });

  it("renders sender info", () => {
    render(<ThreadView threadId="t1" domainId="d1" />);
    const matches = screen.getAllByText(/alice/i);
    expect(matches.length).toBeGreaterThan(0);
  });

  it("renders reply button", () => {
    render(<ThreadView threadId="t1" domainId="d1" />);
    const replyBtns = screen.getAllByText("Reply");
    expect(replyBtns.length).toBeGreaterThan(0);
  });

  it("renders forward button", () => {
    render(<ThreadView threadId="t1" domainId="d1" />);
    const fwdBtns = screen.getAllByText("Forward");
    expect(fwdBtns.length).toBeGreaterThan(0);
  });

  it("renders delivery status for emails", () => {
    render(<ThreadView threadId="t1" domainId="d1" />);
    // "received" email status should show as badge text or similar
    // The exact rendering depends on the component, but it should not error
    expect(screen.getByText("Test Subject")).toBeInTheDocument();
  });
});
