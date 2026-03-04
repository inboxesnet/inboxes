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

  it("shows initial letter in avatar circle", () => {
    render(
      <ContactCard email="mary@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    // getInitials returns first char of local part uppercase: "M"
    expect(screen.getByText("M")).toBeInTheDocument();
  });

  it("formats name with underscores (john_doe -> John Doe)", () => {
    render(
      <ContactCard email="john_doe@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("John Doe")).toBeInTheDocument();
  });

  it("formats name with hyphens (jane-smith -> Jane Smith)", () => {
    render(
      <ContactCard email="jane-smith@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("Jane Smith")).toBeInTheDocument();
  });

  it("toggles popup open and closed on repeated clicks", () => {
    render(
      <ContactCard email="toggle@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    // Open
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("toggle@example.com")).toBeInTheDocument();

    // Close
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.queryByText("Copy email address")).not.toBeInTheDocument();
  });

  it("trigger has role=button and tabIndex=0", () => {
    render(
      <ContactCard email="test@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    const trigger = screen.getByRole("button", { name: "trigger" });
    expect(trigger).toHaveAttribute("tabindex", "0");
  });

  it("opens popup on Enter key", () => {
    render(
      <ContactCard email="keyboard@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    const trigger = screen.getByRole("button", { name: "trigger" });
    fireEvent.keyDown(trigger, { key: "Enter" });
    expect(screen.getByText("keyboard@example.com")).toBeInTheDocument();
  });

  it("opens popup on Space key", () => {
    render(
      <ContactCard email="space@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    const trigger = screen.getByRole("button", { name: "trigger" });
    fireEvent.keyDown(trigger, { key: " " });
    expect(screen.getByText("space@example.com")).toBeInTheDocument();
  });

  it("closes popup on outside click", () => {
    render(
      <div>
        <ContactCard email="test@example.com">
          <span>trigger</span>
        </ContactCard>
        <div data-testid="outside">Outside</div>
      </div>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("test@example.com")).toBeInTheDocument();

    // Click outside
    fireEvent.mouseDown(screen.getByTestId("outside"));
    expect(screen.queryByText("Copy email address")).not.toBeInTheDocument();
  });

  it("handles single-word email local part (admin@example.com -> Admin)", () => {
    render(
      <ContactCard email="admin@example.com">
        <span>trigger</span>
      </ContactCard>
    );
    fireEvent.click(screen.getByText("trigger"));
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });
});
