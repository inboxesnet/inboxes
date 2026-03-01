import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
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
});
