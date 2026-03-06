import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { RecipientInput } from "../recipient-input";

// Mock the api module
vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn().mockResolvedValue([]),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock sonner toast
vi.mock("sonner", () => ({
  toast: Object.assign(vi.fn(), {
    error: vi.fn(),
    success: vi.fn(),
  }),
}));

// Mock lucide-react icons
vi.mock("lucide-react", () => ({
  X: () => <span data-testid="x-icon">X</span>,
}));

describe("RecipientInput", () => {
  let onChange: (value: string[]) => void;

  beforeEach(() => {
    onChange = vi.fn<(value: string[]) => void>();
  });

  it("renders empty input with placeholder", () => {
    render(
      <RecipientInput value={[]} onChange={onChange} placeholder="Add email" />
    );
    const input = screen.getByPlaceholderText("Add email");
    expect(input).toBeInTheDocument();
  });

  it("renders existing recipients as chips", () => {
    render(
      <RecipientInput
        value={["alice@test.com", "bob@test.com"]}
        onChange={onChange}
      />
    );
    expect(screen.getByText("alice@test.com")).toBeInTheDocument();
    expect(screen.getByText("bob@test.com")).toBeInTheDocument();
  });

  it("adds recipient on Enter", () => {
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "new@test.com" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith(["new@test.com"]);
  });

  it("adds recipient on comma", () => {
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "new@test.com," } });
    expect(onChange).toHaveBeenCalledWith(["new@test.com"]);
  });

  it("removes recipient on X click", () => {
    render(
      <RecipientInput value={["alice@test.com"]} onChange={onChange} />
    );
    const removeButtons = screen.getAllByTestId("x-icon");
    // Click the remove button (parent button element)
    fireEvent.click(removeButtons[0].closest("button")!);
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("prevents duplicate emails", () => {
    render(
      <RecipientInput value={["alice@test.com"]} onChange={onChange} />
    );
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "alice@test.com" } });
    fireEvent.keyDown(input, { key: "Enter" });
    // onChange should NOT be called since alice@test.com already exists
    expect(onChange).not.toHaveBeenCalled();
  });

  it("hides placeholder when recipients exist", () => {
    render(
      <RecipientInput
        value={["alice@test.com"]}
        onChange={onChange}
        placeholder="Add email"
      />
    );
    expect(screen.queryByPlaceholderText("Add email")).not.toBeInTheDocument();
  });

  it("clears input after adding recipient", () => {
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "new@test.com" } });
    fireEvent.keyDown(input, { key: "Enter" });
    // The component calls setInputValue("") after adding
    // We verify the input value is cleared
    expect(input.value).toBe("");
  });

  it("fetches autocomplete suggestions after 2 chars", async () => {
    const { api } = await import("@/lib/api");
    vi.mocked(api.get).mockResolvedValue([
      { email: "alice@test.com", name: "Alice", count: 5 },
    ]);

    vi.useFakeTimers();
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox");

    // Type 1 char — should NOT fetch
    fireEvent.change(input, { target: { value: "a" } });
    await act(async () => { vi.advanceTimersByTime(250); });
    expect(api.get).not.toHaveBeenCalled();

    // Type 2 chars — should fetch after debounce
    fireEvent.change(input, { target: { value: "al" } });
    await act(async () => { vi.advanceTimersByTime(250); });
    expect(api.get).toHaveBeenCalledWith(
      expect.stringContaining("/api/contacts/suggest?q=al")
    );

    vi.useRealTimers();
  });

  it("navigates suggestions with arrow keys and selects with Enter", async () => {
    const { api } = await import("@/lib/api");
    vi.mocked(api.get).mockResolvedValue([
      { email: "alice@test.com", name: "Alice", count: 5 },
      { email: "alex@test.com", name: "Alex", count: 3 },
    ]);

    vi.useFakeTimers();
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox");

    // Type to trigger suggestions
    fireEvent.change(input, { target: { value: "al" } });
    // Advance past the 200ms debounce and flush all pending promises
    await act(async () => { vi.advanceTimersByTime(250); });
    // Flush microtasks so the resolved promise updates state
    await act(async () => { vi.advanceTimersByTime(0); });

    // Suggestions should be visible now
    expect(screen.getByText("alice@test.com")).toBeInTheDocument();

    // Arrow down to first suggestion (index 0)
    fireEvent.keyDown(input, { key: "ArrowDown" });
    // Arrow down to second suggestion (index 1)
    fireEvent.keyDown(input, { key: "ArrowDown" });
    // Arrow up back to first (index 0)
    fireEvent.keyDown(input, { key: "ArrowUp" });
    // Enter to select first suggestion
    fireEvent.keyDown(input, { key: "Enter" });

    expect(onChange).toHaveBeenCalledWith(["alice@test.com"]);
    vi.useRealTimers();
  });

  it("shows toast error for invalid email address", async () => {
    const { toast } = await import("sonner");
    render(<RecipientInput value={[]} onChange={onChange} />);
    const input = screen.getByRole("combobox");

    fireEvent.change(input, { target: { value: "not-an-email" } });
    fireEvent.keyDown(input, { key: "Enter" });

    // Invalid email without @ should not trigger addRecipient (no @ in value)
    // Try with @ but still invalid
    fireEvent.change(input, { target: { value: "bad@" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("Invalid email"));
    expect(onChange).not.toHaveBeenCalled();
  });
});
