import React from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { DomainIcon } from "../domain-icon";

// Mock the utils module
vi.mock("@/lib/utils", () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(" "),
  getInitials: (domain: string) => {
    const parts = domain.split(".");
    if (parts.length >= 2) return parts[0].charAt(0).toUpperCase();
    return domain.charAt(0).toUpperCase();
  },
  getDomainColor: (domain: string) => {
    // Deterministic color for testing
    const COLORS = [
      "bg-blue-500",
      "bg-green-500",
      "bg-purple-500",
      "bg-orange-500",
      "bg-pink-500",
      "bg-teal-500",
      "bg-indigo-500",
      "bg-rose-500",
      "bg-cyan-500",
      "bg-amber-500",
    ];
    let hash = 0;
    for (let i = 0; i < domain.length; i++) {
      hash = domain.charCodeAt(i) + ((hash << 5) - hash);
    }
    return COLORS[Math.abs(hash) % COLORS.length];
  },
}));

describe("DomainIcon", () => {
  it("renders the correct initial for a domain", () => {
    render(<DomainIcon domain="example.com" />);
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("E");
  });

  it("renders the correct initial for a different domain", () => {
    render(<DomainIcon domain="acme.org" />);
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("A");
  });

  it("renders uppercase initial for lowercase domain", () => {
    render(<DomainIcon domain="zebra.io" />);
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("Z");
  });

  it("applies the domain as title attribute", () => {
    render(<DomainIcon domain="test.com" />);
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("title", "test.com");
  });

  it("calls onClick when clicked", () => {
    const handleClick = vi.fn();
    render(<DomainIcon domain="example.com" onClick={handleClick} />);
    fireEvent.click(screen.getByRole("button"));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it("applies a color class from getDomainColor", () => {
    render(<DomainIcon domain="example.com" />);
    const button = screen.getByRole("button");
    // getDomainColor returns a bg-* class - verify it has one
    const classes = button.className;
    expect(classes).toMatch(/bg-\w+-500/);
  });

  it("applies active ring styles when active=true", () => {
    render(<DomainIcon domain="example.com" active />);
    const button = screen.getByRole("button");
    expect(button.className).toContain("ring-2");
    expect(button.className).toContain("rounded-2xl");
  });

  it("does not apply ring styles when active=false", () => {
    render(<DomainIcon domain="example.com" active={false} />);
    const button = screen.getByRole("button");
    expect(button.className).not.toContain("ring-2");
  });

  it("shows unread dot when hasUnread=true", () => {
    const { container } = render(
      <DomainIcon domain="example.com" hasUnread />
    );
    const dot = container.querySelector(".bg-red-500");
    expect(dot).toBeInTheDocument();
  });

  it("does not show unread dot when hasUnread=false", () => {
    const { container } = render(
      <DomainIcon domain="example.com" hasUnread={false} />
    );
    const dot = container.querySelector(".bg-red-500");
    expect(dot).not.toBeInTheDocument();
  });

  it("renders small size with correct class", () => {
    render(<DomainIcon domain="example.com" size="sm" />);
    const button = screen.getByRole("button");
    expect(button.className).toContain("h-8");
    expect(button.className).toContain("w-8");
  });

  it("renders medium size by default", () => {
    render(<DomainIcon domain="example.com" />);
    const button = screen.getByRole("button");
    expect(button.className).toContain("h-12");
    expect(button.className).toContain("w-12");
  });

  it("renders large size with correct class", () => {
    render(<DomainIcon domain="example.com" size="lg" />);
    const button = screen.getByRole("button");
    expect(button.className).toContain("h-16");
    expect(button.className).toContain("w-16");
  });

  it("shows tooltip with domain name", () => {
    const { container } = render(<DomainIcon domain="mysite.dev" />);
    // Tooltip div contains the domain text
    const tooltip = container.querySelector(".opacity-0");
    expect(tooltip).toHaveTextContent("mysite.dev");
  });
});
