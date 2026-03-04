import React from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { DragPreview } from "../drag-preview";
import type { Thread } from "@/lib/types";

// Mock thread-helpers
vi.mock("@/lib/thread-helpers", () => ({
  extractSender: (emails: string[]) => {
    if (!emails || emails.length === 0) return "Unknown";
    const first = emails[0];
    const atIndex = first.indexOf("@");
    return atIndex > 0 ? first.substring(0, atIndex) : first;
  },
}));

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: "t1",
    org_id: "org1",
    user_id: "u1",
    domain_id: "d1",
    subject: "Test Subject",
    participant_emails: ["alice@example.com"],
    last_message_at: "2026-01-01T00:00:00Z",
    message_count: 1,
    unread_count: 0,
    labels: ["inbox"],
    snippet: "Hello world",
    original_to: "hello@test.com",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("DragPreview", () => {
  it("renders the thread subject", () => {
    render(<DragPreview thread={makeThread({ subject: "Important Email" })} />);
    expect(screen.getByText("Important Email")).toBeInTheDocument();
  });

  it("renders the sender extracted from participant emails", () => {
    render(
      <DragPreview
        thread={makeThread({
          participant_emails: ["bob@company.com"],
        })}
      />
    );
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  it("does not show count text when count is 1 (default)", () => {
    render(<DragPreview thread={makeThread()} count={1} />);
    expect(screen.queryByText(/more/)).not.toBeInTheDocument();
  });

  it("does not show count text when count prop is omitted", () => {
    render(<DragPreview thread={makeThread()} />);
    expect(screen.queryByText(/more/)).not.toBeInTheDocument();
  });

  it("shows '+1 more' when count is 2", () => {
    render(<DragPreview thread={makeThread()} count={2} />);
    expect(screen.getByText("+1 more")).toBeInTheDocument();
  });

  it("shows '+4 more' when count is 5", () => {
    render(<DragPreview thread={makeThread()} count={5} />);
    expect(screen.getByText("+4 more")).toBeInTheDocument();
  });

  it("shows '+99 more' when count is 100", () => {
    render(<DragPreview thread={makeThread()} count={100} />);
    expect(screen.getByText("+99 more")).toBeInTheDocument();
  });

  it("shows 'Unknown' sender when participant_emails is empty", () => {
    render(
      <DragPreview thread={makeThread({ participant_emails: [] })} />
    );
    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });

  it("renders with truncation styling", () => {
    const { container } = render(
      <DragPreview
        thread={makeThread({ subject: "A very long subject line" })}
      />
    );
    const subjectEl = container.querySelector(".truncate");
    expect(subjectEl).toBeInTheDocument();
  });
});
