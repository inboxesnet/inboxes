import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ContactCard } from "../contact-card";

// Mock lucide-react icons
vi.mock("lucide-react", () => ({
  Copy: () => <span data-testid="copy-icon">Copy</span>,
  Check: () => <span data-testid="check-icon">Check</span>,
}));

describe("ContactCard", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders trigger children", () => {
    render(
      <ContactCard email="test@example.com">
        <span>Click me</span>
      </ContactCard>
    );
    expect(screen.getByText("Click me")).toBeInTheDocument();
  });

  it("shows popup on click with email", () => {
    render(
      <ContactCard email="jane.doe@example.com">
        <span>Jane</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("Jane"));
    expect(screen.getByText("jane.doe@example.com")).toBeInTheDocument();
  });

  it("formats name from email (first.last)", () => {
    render(
      <ContactCard email="john.smith@company.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    // getDisplayName converts "john.smith" to "John Smith"
    expect(screen.getByText("John Smith")).toBeInTheDocument();
  });

  it("shows copy button text", () => {
    render(
      <ContactCard email="test@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("Copy email address")).toBeInTheDocument();
  });

  it("closes on Escape key", () => {
    render(
      <ContactCard email="test@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("test@example.com")).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    // The popup should be gone
    expect(screen.queryByText("Copy email address")).not.toBeInTheDocument();
  });
});
