import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { SessionExpiredModal } from "../session-expired-modal";

// Mock lucide-react
vi.mock("lucide-react", () => ({
  LogIn: () => <span data-testid="login-icon">LogIn</span>,
}));

// Mock Button
vi.mock("@/components/ui/button", () => ({
  Button: ({
    children,
    onClick,
    ...props
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    className?: string;
  }) => (
    <button onClick={onClick} {...props}>
      {children}
    </button>
  ),
}));

describe("SessionExpiredModal", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
  });

  it("is not rendered initially", () => {
    render(<SessionExpiredModal />);
    expect(screen.queryByText("Session Expired")).not.toBeInTheDocument();
  });

  it("renders on session-expired event", () => {
    render(<SessionExpiredModal />);
    fireEvent(window, new Event("session-expired"));
    expect(screen.getByText("Session Expired")).toBeInTheDocument();
  });

  it("shows Session Expired heading", () => {
    render(<SessionExpiredModal />);
    fireEvent(window, new Event("session-expired"));
    expect(screen.getByText("Session Expired")).toBeInTheDocument();
  });

  it("shows Log in button", () => {
    render(<SessionExpiredModal />);
    fireEvent(window, new Event("session-expired"));
    expect(screen.getByText("Log in")).toBeInTheDocument();
  });

  it("redirects to /login on button click", () => {
    const locationSpy = vi.spyOn(window, "location", "get").mockReturnValue({
      ...window.location,
      href: "",
    });
    // Use defineProperty for setter
    const setHref = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, href: "" },
      writable: true,
      configurable: true,
    });
    Object.defineProperty(window.location, "href", {
      set: setHref,
      configurable: true,
    });

    render(<SessionExpiredModal />);
    fireEvent(window, new Event("session-expired"));
    fireEvent.click(screen.getByText("Log in"));
    expect(setHref).toHaveBeenCalledWith("/login");

    locationSpy.mockRestore();
  });

  it("is non-dismissible (stays open after clicking backdrop)", () => {
    render(<SessionExpiredModal />);
    fireEvent(window, new Event("session-expired"));
    // Modal should still be visible — there's no dismiss mechanism
    expect(screen.getByText("Session Expired")).toBeInTheDocument();
  });

  it("cleans up event listener on unmount", () => {
    const removeEventSpy = vi.spyOn(window, "removeEventListener");
    const { unmount } = render(<SessionExpiredModal />);
    unmount();
    expect(removeEventSpy).toHaveBeenCalledWith(
      "session-expired",
      expect.any(Function)
    );
    removeEventSpy.mockRestore();
  });
});
